package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"golang.org/x/sync/errgroup"
)

type OpenAIUsageThread struct {
	ThreadID            string   `json:"thread_id"`
	CreatedAt           *string  `json:"created_at"`
	DescendantThreadIDs []string `json:"descendant_thread_ids,omitempty"`
}

type OpenAIUsageDetailsRequest struct {
	Kind          string              `json:"kind"`
	StartDate     string              `json:"start_date,omitempty"`
	EndDate       string              `json:"end_date,omitempty"`
	WindowMinutes int                 `json:"window_minutes,omitempty"`
	Cursor        string              `json:"cursor,omitempty"`
	ThreadIDs     []string            `json:"thread_ids,omitempty"`
	Threads       []OpenAIUsageThread `json:"threads,omitempty"`
}

type OpenAIUsageDetails struct {
	Kind                string          `json:"kind"`
	Data                json.RawMessage `json:"data"`
	Unavailable         bool            `json:"unavailable"`
	UpstreamStatus      int             `json:"upstream_status,omitempty"`
	FetchedAt           int64           `json:"fetched_at"`
	RefreshAfterSeconds int             `json:"refresh_after_seconds"`
	Truncated           bool            `json:"truncated,omitempty"`
}

type openAIUsageDetailsCacheEntry struct {
	value   *OpenAIUsageDetails
	expires time.Time
}

func usageInvalid(message string) error {
	return infraerrors.New(http.StatusBadRequest, "OPENAI_USAGE_INVALID_QUERY", message)
}

func (r OpenAIUsageDetailsRequest) validate() error {
	switch r.Kind {
	case "daily", "workspace_daily":
		start, err := time.Parse("2006-01-02", r.StartDate)
		end, endErr := time.Parse("2006-01-02", r.EndDate)
		if err != nil || endErr != nil || end.Before(start) || end.Sub(start) > 90*24*time.Hour {
			return usageInvalid("choose a valid date range of at most 90 days")
		}
	case "balance_history":
		if r.WindowMinutes != 300 && r.WindowMinutes != 10080 {
			return usageInvalid("window_minutes must be 300 or 10080")
		}
		if len(r.Cursor) > 2048 {
			return usageInvalid("cursor is too long")
		}
	case "threads", "top_threads":
		ids := r.ThreadIDs
		if r.Kind == "top_threads" {
			if len(r.Threads) == 0 || len(r.Threads) > 100 {
				return usageInvalid("provide between 1 and 100 root threads")
			}
			ids = nil
			for _, thread := range r.Threads {
				if thread.CreatedAt != nil {
					if _, err := time.Parse(time.RFC3339, *thread.CreatedAt); err != nil {
						return usageInvalid("invalid thread creation time")
					}
				}
				ids = append(ids, thread.ThreadID)
				ids = append(ids, thread.DescendantThreadIDs...)
			}
		}
		if len(ids) == 0 || len(ids) > 1000 {
			return usageInvalid("provide between 1 and 1000 thread IDs")
		}
		seen := make(map[string]bool, len(ids))
		for _, id := range ids {
			if strings.TrimSpace(id) == "" || len(id) > 200 || seen[id] {
				return usageInvalid("thread IDs must be nonempty, unique and at most 200 characters")
			}
			seen[id] = true
		}
	case "remaining_balance", "monthly", "plan_history":
	default:
		return usageInvalid("unsupported usage query")
	}
	return nil
}

// QueryUsageDetails keeps each resource's own cache and freshness. In
// particular, plan percentages, credit balances and token totals never share
// a counter. Credentials are resolved before cache lookup to isolate reauth.
func (s *OpenAIQuotaService) QueryUsageDetails(ctx context.Context, accountID int64, query OpenAIUsageDetailsRequest) (*OpenAIUsageDetails, error) {
	if err := query.validate(); err != nil {
		return nil, err
	}
	account, err := s.quotaCredentialAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !account.IsOpenAIOAuth() {
		return nil, usageInvalid("usage details require an OpenAI OAuth account")
	}
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	encoded, err := json.Marshal(struct {
		Query       OpenAIUsageDetailsRequest
		Credentials map[string]any
		ProxyID     *int64
		ExtraRoute  string
	}{query, account.Credentials, account.ProxyID, fmt.Sprint(accountEgressRoute(account, proxyURL))})
	if err != nil {
		return nil, usageInvalid("invalid account credentials")
	}
	key := fmt.Sprintf("%d:%x", accountID, sha256.Sum256(encoded))
	s.usageDetailsMu.Lock()
	entry, ok := s.usageDetailsCache[key]
	s.usageDetailsMu.Unlock()
	if ok && time.Now().Before(entry.expires) && query.Kind != "monthly" && query.Kind != "remaining_balance" {
		return entry.value, nil
	}
	result := s.usageDetailsFlight.DoChan(key, func() (any, error) {
		callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		value, err := s.fetchUsageDetails(callCtx, accountID, query)
		if err != nil {
			return nil, err
		}
		s.usageDetailsMu.Lock()
		defer s.usageDetailsMu.Unlock()
		if s.usageDetailsCache == nil {
			s.usageDetailsCache = make(map[string]openAIUsageDetailsCacheEntry)
		}
		for k, cached := range s.usageDetailsCache {
			if time.Now().After(cached.expires) || len(s.usageDetailsCache) >= 512 {
				delete(s.usageDetailsCache, k)
			}
		}
		s.usageDetailsCache[key] = openAIUsageDetailsCacheEntry{value, time.Now().Add(time.Duration(value.RefreshAfterSeconds) * time.Second)}
		return value, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-result:
		if result.Err != nil {
			return nil, result.Err
		}
		return result.Val.(*OpenAIUsageDetails), nil
	}
}

func (s *OpenAIQuotaService) fetchUsageDetails(ctx context.Context, accountID int64, query OpenAIUsageDetailsRequest) (*OpenAIUsageDetails, error) {
	result := &OpenAIUsageDetails{Kind: query.Kind, FetchedAt: time.Now().Unix(), RefreshAfterSeconds: 60, Data: json.RawMessage("null")}
	path, method, params := "", http.MethodGet, make(url.Values)
	var body any
	switch query.Kind {
	case "daily", "workspace_daily":
		path = "/wham/usage/daily-token-usage-breakdown"
		if query.Kind == "workspace_daily" {
			path = "/wham/usage/daily-workspace-user-token-usage-breakdown"
			params["breakdown_by[]"] = []string{"model"}
			params["modes[]"] = []string{"codex", "work"}
		}
		params.Set("start_date", query.StartDate)
		params.Set("end_date", query.EndDate)
		params.Set("group_by", "day")
	case "remaining_balance", "monthly":
		account, err := s.quotaCredentialAccount(ctx, accountID)
		if err != nil {
			return nil, err
		}
		principal := strings.TrimSpace(account.GetCredential("chatgpt_account_id"))
		if principal == "" {
			principal = strings.TrimSpace(account.GetCredential("organization_id"))
		}
		if principal == "" {
			return nil, usageInvalid("account identity is unavailable")
		}
		path = "/accounts/" + url.PathEscape(principal) + "/remaining_balance"
		if query.Kind == "monthly" {
			path = "/accounts/" + url.PathEscape(principal) + "/spend-controls/current-user/monthly-usage"
			params.Set("supports_usage_limit_modes", "true")
		}
	case "plan_history":
		path = "/wham/usage/plan_limit_history"
		params.Set("days", "7")
		result.RefreshAfterSeconds = 300
	case "balance_history":
		path = "/wham/usage/plan_limit_balance_history"
		params.Set("window_minutes", fmt.Sprint(query.WindowMinutes))
		result.RefreshAfterSeconds = 300
		if query.Cursor != "" {
			params.Set("cursor", query.Cursor)
		}
	case "threads":
		return s.queryUsageThreads(ctx, accountID, query.ThreadIDs)
	case "top_threads":
		path, method = "/wham/usage/thread_usage/query_v2", http.MethodPost
		body = map[string]any{"threads": query.Threads}
		result.RefreshAfterSeconds = 300
	}
	status, _, data, err := s.usageHTTP(ctx, accountID, method, path, params, body)
	if err != nil {
		return nil, err
	}
	if status == 401 || status == 403 || status == 404 || status == 503 {
		result.Unavailable, result.UpstreamStatus = true, status
		return result, nil
	}
	if status < 200 || status >= 300 {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_USAGE_UPSTREAM_ERROR", "usage query returned %d", status)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("invalid usage response")
	}
	if query.Kind == "balance_history" {
		data, result.Truncated, err = s.collectUsageBalancePages(ctx, accountID, path, params, data)
		if err != nil {
			return nil, err
		}
	}
	result.Data = data
	return result, nil
}

func (s *OpenAIQuotaService) collectUsageBalancePages(ctx context.Context, accountID int64, path string, params url.Values, first []byte) ([]byte, bool, error) {
	var result map[string]json.RawMessage
	if err := json.Unmarshal(first, &result); err != nil {
		return nil, false, err
	}
	var points []json.RawMessage
	if err := json.Unmarshal(result["points"], &points); err != nil {
		return nil, false, err
	}
	seen := make(map[string]bool)
	for page := 1; ; page++ {
		var cursor string
		_ = json.Unmarshal(result["next_cursor"], &cursor)
		if cursor == "" {
			data, err := json.Marshal(result)
			return data, false, err
		}
		if seen[cursor] {
			return nil, false, fmt.Errorf("balance history cursor repeated")
		}
		if page >= 20 || len(points) >= 5000 {
			data, err := json.Marshal(result)
			return data, true, err
		}
		seen[cursor] = true
		params.Set("cursor", cursor)
		status, _, data, err := s.usageHTTP(ctx, accountID, http.MethodGet, path, params, nil)
		if err != nil {
			return nil, false, err
		}
		if status < 200 || status >= 300 {
			return nil, false, fmt.Errorf("balance history page returned %d", status)
		}
		var next struct {
			Points []json.RawMessage `json:"points"`
			Next   json.RawMessage   `json:"next_cursor"`
		}
		if err := json.Unmarshal(data, &next); err != nil {
			return nil, false, err
		}
		points = append(points, next.Points...)
		result["points"], _ = json.Marshal(points)
		result["next_cursor"] = next.Next
		encoded, err := json.Marshal(result)
		if err != nil || int64(len(encoded)) > openAICodexAuxiliaryBodyLimit {
			return nil, false, fmt.Errorf("balance history exceeds the size limit")
		}
	}
}

func (s *OpenAIQuotaService) queryUsageThreads(ctx context.Context, accountID int64, ids []string) (*OpenAIUsageDetails, error) {
	result := &OpenAIUsageDetails{Kind: "threads", FetchedAt: time.Now().Unix(), RefreshAfterSeconds: 60, Data: json.RawMessage("null")}
	type threadReply struct {
		Threads []json.RawMessage `json:"threads"`
	}
	var fetch func([]string) (*threadReply, int, error)
	fetch = func(batch []string) (*threadReply, int, error) {
		status, _, data, err := s.usageHTTP(ctx, accountID, http.MethodPost, "/wham/usage/thread_usage/query", nil, map[string]any{"thread_ids": batch})
		if err != nil {
			return nil, status, err
		}
		if status == 503 && strings.Contains(string(data), "Thread usage is incomplete.") && len(batch) > 1 {
			mid := (len(batch) + 1) / 2
			a, status, err := fetch(batch[:mid])
			if err != nil || a == nil {
				return nil, status, err
			}
			b, status, err := fetch(batch[mid:])
			if err != nil || b == nil {
				return nil, status, err
			}
			return &threadReply{append(a.Threads, b.Threads...)}, 200, nil
		}
		if status == 503 && strings.Contains(string(data), "Thread usage is incomplete.") && len(batch) == 1 {
			return &threadReply{Threads: []json.RawMessage{}}, 200, nil
		}
		if status == 401 || status == 403 || status == 404 || status == 503 {
			return nil, status, nil
		}
		if status < 200 || status >= 300 {
			return nil, status, fmt.Errorf("thread usage returned %d", status)
		}
		var reply threadReply
		err = json.Unmarshal(data, &reply)
		return &reply, status, err
	}
	count := (len(ids) + 99) / 100
	replies := make([]*threadReply, count)
	statuses := make([]int, count)
	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(2)
	for i := 0; i < count; i++ {
		g.Go(func() error {
			if err := groupCtx.Err(); err != nil {
				return err
			}
			end := min((i+1)*100, len(ids))
			var err error
			replies[i], statuses[i], err = fetch(ids[i*100 : end])
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	var all threadReply
	for i, reply := range replies {
		if reply == nil {
			result.Unavailable, result.UpstreamStatus = true, statuses[i]
			return result, nil
		}
		all.Threads = append(all.Threads, reply.Threads...)
	}
	result.Data, _ = json.Marshal(all)
	return result, nil
}
