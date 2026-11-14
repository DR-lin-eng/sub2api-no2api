package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type usageTestRepository struct {
	stubQuotaAccountRepo
	mu      sync.Mutex
	updates map[int64]map[string]any
}

func (r *usageTestRepository) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.updates == nil {
		r.updates = make(map[int64]map[string]any)
	}
	r.updates[id] = updates
	return nil
}

func newUsageTestService(t *testing.T, handler http.HandlerFunc) (*OpenAIQuotaService, *usageTestRepository) {
	t.Helper()
	account := &Account{ID: 41, Status: StatusActive, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "fixture-token", "chatgpt_account_id": "acct-41", "chatgpt_user_id": "user-41"}}
	repo := &usageTestRepository{stubQuotaAccountRepo: stubQuotaAccountRepo{accounts: map[int64]*Account{41: account}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	tokens := &stubQuotaTokenCache{tokens: map[string]string{OpenAITokenCacheKey(account): "fixture-token"}}
	return NewOpenAIQuotaService(repo, nil, NewOpenAITokenProvider(repo, tokens, nil), newQuotaRedirectingFactory(server)), repo
}

func usageSnapshot(sequence int, generated int64, user string) []byte {
	return []byte(fmt.Sprintf(`{"type":"usage.snapshot","payload":{"version":1,"stream_id":"stream-41","sequence":%d,"generated_at_ms":%d,"change_published_at_ms":%d,"usage_by_client":{"electron":{"account_id":"acct-41","user_id":%q,"plan_type":"plus","rate_limit":{"allowed":true,"primary_window":{"used_percent":25,"limit_window_seconds":18000,"reset_at":1900000000}},"credits":{"balance":12}}}}}`, sequence, generated, generated, user))
}

func TestOpenAIUsageSnapshotValidation(t *testing.T) {
	expected := &OpenAIQuotaUsage{AccountID: "acct-41", UserID: "user-41", PlanType: "plus"}
	state := &openAIUsageSnapshotState{}
	usage, err := state.parse(usageSnapshot(1, 1000, "user-41"), expected)
	require.NoError(t, err)
	require.Equal(t, 25.0, usage.RateLimit.PrimaryWindow.UsedPercent)
	require.JSONEq(t, `{"balance":12}`, string(usage.Credits))
	usage, err = state.parse(usageSnapshot(1, 1000, "user-41"), expected)
	require.NoError(t, err)
	require.Nil(t, usage)
	_, err = state.parse(usageSnapshot(2, 2000, "another-user"), expected)
	require.ErrorContains(t, err, "identity")
	_, err = state.parse(usageSnapshot(3, 3000, "user-41"), expected)
	require.ErrorContains(t, err, "gap")
	_, err = state.parse([]byte(strings.Replace(string(usageSnapshot(2, 2000, "user-41")), `"version":1`, `"version":2`, 1)), expected)
	require.Error(t, err)
	_, err = state.parse([]byte(`{"type":"usage.snapshot","payload":{}}`), expected)
	require.Error(t, err)
	_, err = state.parse([]byte(strings.Replace(string(usageSnapshot(2, 2000, "user-41")), `"plan_type":"plus"`, `"plan_type":"team"`, 1)), expected)
	require.ErrorContains(t, err, "identity")
}

func TestOpenAIUsageResourceContractsAndCache(t *testing.T) {
	var calls atomic.Int32
	svc, repo := newUsageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "Bearer fixture-token", r.Header.Get("Authorization"))
		require.Equal(t, "acct-41", r.Header.Get("ChatGPT-Account-ID"))
		require.Equal(t, "codex", r.Header.Get("OAI-App-Brand"))
		require.Equal(t, "1", r.Header.Get("x-openai-codex-usage-categories"))
		switch r.URL.Path {
		case "/backend-api/wham/usage/daily-token-usage-breakdown":
			require.Equal(t, "day", r.URL.Query().Get("group_by"))
			require.Equal(t, "2026-11-01", r.URL.Query().Get("start_date"))
		case "/backend-api/wham/usage/daily-workspace-user-token-usage-breakdown":
			require.Equal(t, []string{"model"}, r.URL.Query()["breakdown_by[]"])
			require.Equal(t, []string{"codex", "work"}, r.URL.Query()["modes[]"])
		case "/backend-api/accounts/acct-41/spend-controls/current-user/monthly-usage":
			require.Equal(t, "true", r.URL.Query().Get("supports_usage_limit_modes"))
			require.Equal(t, "no-store", r.Header.Get("Cache-Control"))
			require.Equal(t, "no-cache", r.Header.Get("Pragma"))
		case "/backend-api/accounts/acct-41/remaining_balance":
		default:
			t.Errorf("unexpected resource %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[],"data_freshness_ts":"2026-11-07T00:00:00Z"}`))
	})
	daily := OpenAIUsageDetailsRequest{Kind: "daily", StartDate: "2026-11-01", EndDate: "2026-11-07"}
	for range 2 {
		result, err := svc.QueryUsageDetails(context.Background(), 41, daily)
		require.NoError(t, err)
		require.False(t, result.Unavailable)
	}
	require.EqualValues(t, 1, calls.Load(), "daily queries share their minute cache")
	repo.accounts[41].Credentials["chatgpt_user_id"] = "reauthorized-user"
	_, err := svc.QueryUsageDetails(context.Background(), 41, daily)
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load())
	daily.Kind = "workspace_daily"
	_, err = svc.QueryUsageDetails(context.Background(), 41, daily)
	require.NoError(t, err)
	for _, kind := range []string{"monthly", "remaining_balance"} {
		for range 2 {
			_, err = svc.QueryUsageDetails(context.Background(), 41, OpenAIUsageDetailsRequest{Kind: kind})
			require.NoError(t, err)
		}
	}
	require.EqualValues(t, 7, calls.Load(), "mount/focus balance and no-store monthly queries stay fresh")
}

func TestOpenAIUsageBalancePaginationAndUnavailable(t *testing.T) {
	var calls atomic.Int32
	svc, _ := newUsageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if strings.Contains(r.URL.Path, "plan_limit_history") {
			w.WriteHeader(403)
			return
		}
		require.Equal(t, "300", r.URL.Query().Get("window_minutes"))
		if r.URL.Query().Get("cursor") == "next" {
			_, _ = w.Write([]byte(`{"points":[{"remaining_basis_points":5000,"observed_at":"2026-11-07T01:00:00Z"}],"next_cursor":null}`))
			return
		}
		_, _ = w.Write([]byte(`{"points":[{"remaining_basis_points":8000,"observed_at":"2026-11-07T00:00:00Z"}],"next_cursor":"next","data_as_of":"2026-11-07T01:00:00Z"}`))
	})
	result, err := svc.QueryUsageDetails(context.Background(), 41, OpenAIUsageDetailsRequest{Kind: "balance_history", WindowMinutes: 300})
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load())
	require.False(t, result.Truncated)
	var body struct {
		Points []any  `json:"points"`
		AsOf   string `json:"data_as_of"`
	}
	require.NoError(t, json.Unmarshal(result.Data, &body))
	require.Len(t, body.Points, 2)
	require.Equal(t, "2026-11-07T01:00:00Z", body.AsOf)
	result, err = svc.QueryUsageDetails(context.Background(), 41, OpenAIUsageDetailsRequest{Kind: "plan_history"})
	require.NoError(t, err)
	require.True(t, result.Unavailable)
	require.Equal(t, 403, result.UpstreamStatus)
	require.JSONEq(t, "null", string(result.Data))
}

func TestOpenAIUsageBalanceRejectsRepeatedCursor(t *testing.T) {
	var calls atomic.Int32
	svc, _ := newUsageTestService(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"points":[],"next_cursor":"repeated"}`))
	})
	_, err := svc.QueryUsageDetails(context.Background(), 41, OpenAIUsageDetailsRequest{Kind: "balance_history", WindowMinutes: 300})
	require.ErrorContains(t, err, "cursor repeated")
	require.EqualValues(t, 2, calls.Load())
}

func TestOpenAIUsageThreadsBatchSplitAndLimit(t *testing.T) {
	var active, peak, calls atomic.Int32
	svc, _ := newUsageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		calls.Add(1)
		var body struct {
			IDs []string `json:"thread_ids"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.LessOrEqual(t, len(body.IDs), 100)
		if len(body.IDs) > 1 {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"detail":"Thread usage is incomplete."}`))
			return
		}
		if body.IDs[0] == "incomplete" {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"detail":"Thread usage is incomplete."}`))
			return
		}
		_, _ = fmt.Fprintf(w, `{"threads":[{"thread_id":%q,"estimated_usage_credits_micros":1234567,"net_new_input_tokens":10,"cached_input_tokens":20,"output_tokens":30,"groups":[]}]}`, body.IDs[0])
	})
	ids := []string{"a", "b", "incomplete", "c"}
	result, err := svc.QueryUsageDetails(context.Background(), 41, OpenAIUsageDetailsRequest{Kind: "threads", ThreadIDs: ids})
	require.NoError(t, err)
	require.False(t, result.Unavailable)
	require.EqualValues(t, 7, calls.Load())
	require.LessOrEqual(t, peak.Load(), int32(2))
	var body struct {
		Threads []struct {
			ID     string `json:"thread_id"`
			Micros int64  `json:"estimated_usage_credits_micros"`
		} `json:"threads"`
	}
	require.NoError(t, json.Unmarshal(result.Data, &body))
	require.Len(t, body.Threads, 3)
	require.EqualValues(t, 1234567, body.Threads[0].Micros)
	_, err = svc.QueryUsageDetails(context.Background(), 41, OpenAIUsageDetailsRequest{Kind: "threads", ThreadIDs: make([]string, 1001)})
	require.Error(t, err)
	_, err = svc.QueryUsageDetails(context.Background(), 41, OpenAIUsageDetailsRequest{Kind: "top_threads", Threads: []OpenAIUsageThread{{ThreadID: "root", DescendantThreadIDs: []string{"overlap"}}, {ThreadID: "overlap"}}})
	require.Error(t, err)
}

type usageRedirectDialer struct {
	target string
	calls  atomic.Int32
}

func (d *usageRedirectDialer) Dial(ctx context.Context, _ string, headers http.Header, proxy string) (openAIWSClientConn, int, http.Header, error) {
	d.calls.Add(1)
	return newDefaultOpenAIWSClientDialer().Dial(ctx, d.target, headers, proxy)
}
func (d *usageRedirectDialer) DialRouteWithProfile(ctx context.Context, target string, headers http.Header, route platformegress.Route, _ *tlsfingerprint.Profile) (openAIWSClientConn, int, http.Header, error) {
	return d.Dial(ctx, target, headers, route.ProxyURL)
}

func TestOpenAIUsageWebSocketSharedSnapshotAndFallback(t *testing.T) {
	registered, next, disconnect := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var registeredOnce sync.Once
	upgrader := websocket.Upgrader{}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Authorization"), "the signed WS URL does not receive OAuth headers")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		var writeMu sync.Mutex
		write := func(value any) { writeMu.Lock(); defer writeMu.Unlock(); _ = conn.WriteJSON(value) }
		write(map[string]any{"type": "keepalive"})
		go func() {
			<-registered
			write([]any{map[string]any{"type": "message", "topic_id": openAIUsagePushTopic, "payload": json.RawMessage(usageSnapshot(1, 1000, "user-41"))}})
			<-next
			write([]any{map[string]any{"type": "message", "topic_id": openAIUsagePushTopic, "payload": json.RawMessage(usageSnapshot(2, 2000, "user-41"))}})
			<-disconnect
			_ = conn.Close()
		}()
		for {
			var commands []struct {
				ID      int            `json:"id"`
				Command map[string]any `json:"command"`
			}
			if conn.ReadJSON(&commands) != nil {
				return
			}
			for _, command := range commands {
				reply := map[string]any{"type": command.Command["type"]}
				if command.Command["type"] == "subscribe" {
					require.Equal(t, openAIUsagePushTopic, command.Command["topic_id"])
					reply["topic_id"] = openAIUsagePushTopic
					reply["recovered"] = false
				}
				if command.Command["type"] == "presence" {
					metadata, ok := command.Command["metadata"].(map[string]any)
					require.True(t, ok)
					require.Equal(t, "electron", metadata["codex_usage_client"])
				}
				write([]any{map[string]any{"id": command.ID, "reply": reply}})
			}
		}
	}))
	defer wsServer.Close()
	svc, repo := newUsageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			_, _ = w.Write([]byte(`{"user_id":"user-41","account_id":"acct-41","plan_type":"plus","rate_limit":{"allowed":true,"primary_window":{"used_percent":10,"limit_window_seconds":18000,"reset_at":1900000000}}}`))
		case "/backend-api/wham/rate-limit-reset-credits":
			_, _ = w.Write([]byte(`{"available_count":0,"credits":[]}`))
		case "/backend-api/celsius/ws/user":
			_, _ = w.Write([]byte(`{"websocket_url":"wss://ws.chatgpt.com/fixture?signed=private"}`))
		case "/backend-api/codex/notification/subscriptions":
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "usage", r.Header.Get("X-OpenAI-Codex-Initial-Snapshot"))
			_, _ = w.Write([]byte(`{"account_id":"acct-41","account_user_id":"member-41","renewal_seconds":600}`))
			registeredOnce.Do(func() { close(registered) })
		default:
			http.NotFound(w, r)
		}
	})
	dialer := &usageRedirectDialer{target: "ws" + strings.TrimPrefix(wsServer.URL, "http")}
	svc.usagePushDialer = dialer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events, release, err := svc.SubscribeUsage(ctx, 41)
	require.NoError(t, err)
	defer release()
	await := func(events <-chan OpenAIUsagePushEvent, transport string) OpenAIUsagePushEvent {
		for {
			select {
			case event := <-events:
				if event.Transport == transport {
					return event
				}
			case <-ctx.Done():
				t.Fatal("usage event timed out")
				return OpenAIUsagePushEvent{}
			}
		}
	}
	event := await(events, "websocket")
	require.Equal(t, 25.0, event.Quota.RateLimit.PrimaryWindow.UsedPercent)
	second, releaseSecond, err := svc.SubscribeUsage(ctx, 41)
	require.NoError(t, err)
	defer releaseSecond()
	_ = await(second, "websocket")
	require.EqualValues(t, 1, dialer.calls.Load())
	close(next)
	_ = await(events, "websocket")
	repo.mu.Lock()
	saved := repo.updates[41]
	repo.mu.Unlock()
	require.Equal(t, 25.0, saved["codex_5h_used_percent"])
	close(disconnect)
	fallback := await(events, "polling")
	require.Equal(t, "fallback", fallback.Type)
	_, _, err = svc.SubscribeUsage(ctx, 41)
	require.ErrorContains(t, err, "cooling")
	quota, err := svc.QueryUsage(ctx, 41)
	require.NoError(t, err)
	require.Equal(t, 10.0, quota.RateLimit.PrimaryWindow.UsedPercent, "the existing HTTP query remains usable after WS failure")
}

func TestOpenAIUsageResetBoundariesAndSocketURL(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	quota := &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{PrimaryWindow: &OpenAIRateLimitWindow{ResetAt: now.Add(time.Hour).Unix()}}, SpendControl: json.RawMessage(fmt.Sprintf(`{"individual_limit":{"reset_at":%d}}`, now.Add(time.Minute).Unix()))}
	require.Equal(t, time.Minute+time.Second, nextUsageResetDelay(quota, now))
	for _, value := range []string{"ws://ws.chatgpt.com", "wss://evil.example", "wss://chatgpt.com.evil.example", "wss://127.0.0.1", "wss://user:pass@chatgpt.com", "wss://ws.chatgpt.com:8080"} {
		require.False(t, validUsageWebSocketURL(value), value)
	}
	require.True(t, validUsageWebSocketURL("wss://ws.chatgpt.com/signed?token=fixture"))
}

func TestOpenAIUsageReauthorizationReplacesSharedSubscription(t *testing.T) {
	svc, _ := newUsageTestService(t, func(_ http.ResponseWriter, request *http.Request) { <-request.Context().Done() })
	oldCtx, oldCancel := context.WithCancel(context.Background())
	oldEvents := make(chan OpenAIUsagePushEvent, 1)
	old := &openAIUsagePushStream{identity: "previous-credentials", cancel: oldCancel, listeners: map[chan OpenAIUsagePushEvent]struct{}{oldEvents: {}}}
	svc.usagePushStreams = map[int64]*openAIUsagePushStream{41: old}
	_, release, err := svc.SubscribeUsage(context.Background(), 41)
	require.NoError(t, err)
	defer release()
	require.ErrorIs(t, oldCtx.Err(), context.Canceled)
	require.Equal(t, "fallback", (<-oldEvents).Type)
	svc.usagePushMu.Lock()
	replacement := svc.usagePushStreams[41]
	svc.usagePushMu.Unlock()
	require.NotSame(t, old, replacement)
	_, releaseSecond, err := svc.SubscribeUsage(context.Background(), 41)
	require.NoError(t, err)
	defer releaseSecond()
	svc.usagePushMu.Lock()
	same := svc.usagePushStreams[41]
	svc.usagePushMu.Unlock()
	require.Same(t, replacement, same, "observers with the same credentials still share the connection")
}

func TestOpenAIUsagePushRejectsNonActiveAccounts(t *testing.T) {
	account := &Account{ID: 91, Status: StatusError, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "acct-91"}}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{91: account}}
	svc := &OpenAIQuotaService{accountRepo: repo}
	_, _, err := svc.SubscribeUsage(context.Background(), 91)
	require.ErrorContains(t, err, "active accounts")
}
