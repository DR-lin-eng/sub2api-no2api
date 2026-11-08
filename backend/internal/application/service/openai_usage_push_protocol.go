package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
)

const openAIUsagePushTopic = "codex-work-usage-push-v1"

type openAIUsageSnapshotState struct {
	stream           string
	sequence         int64
	generated        int64
	published        int64
	minimumGenerated int64
}

func (s *openAIUsageSnapshotState) parse(raw []byte, expected *OpenAIQuotaUsage) (*OpenAIQuotaUsage, error) {
	var message struct {
		Type    string `json:"type"`
		Payload struct {
			Version   int                        `json:"version"`
			Stream    string                     `json:"stream_id"`
			Sequence  int64                      `json:"sequence"`
			Generated *int64                     `json:"generated_at_ms"`
			Published *int64                     `json:"change_published_at_ms"`
			Usage     map[string]json.RawMessage `json:"usage_by_client"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		return nil, fmt.Errorf("invalid usage snapshot")
	}
	p := message.Payload
	if message.Type != "usage.snapshot" || p.Version != 1 || p.Stream == "" || p.Sequence <= 0 || p.Generated == nil || *p.Generated < 0 || p.Published != nil && *p.Published < 0 {
		return nil, fmt.Errorf("unsupported usage snapshot")
	}
	var usage OpenAIQuotaUsage
	if err := json.Unmarshal(p.Usage["electron"], &usage); err != nil {
		return nil, fmt.Errorf("missing electron usage")
	}
	if usage.UserID == "" || usage.UserID != expected.UserID || usage.AccountID != expected.AccountID || usage.PlanType == "" || usage.PlanType != expected.PlanType {
		return nil, fmt.Errorf("usage snapshot identity mismatch")
	}
	if usage.RateLimit == nil && len(usage.RateLimitsByLimitID) == 0 && len(usage.SpendControl) == 0 {
		return nil, fmt.Errorf("usage snapshot contains no quota")
	}
	if *p.Generated < s.minimumGenerated {
		return nil, nil
	}
	if s.stream != "" {
		if p.Stream != s.stream {
			return nil, fmt.Errorf("usage stream changed")
		}
		if p.Sequence <= s.sequence || *p.Generated <= s.generated || p.Published != nil && *p.Published < s.published {
			return nil, nil
		}
		if p.Sequence != s.sequence+1 {
			return nil, fmt.Errorf("usage snapshot gap")
		}
	}
	s.stream, s.sequence, s.generated = p.Stream, p.Sequence, *p.Generated
	if p.Published != nil {
		s.published = *p.Published
	}
	usage.FetchedAt = time.Now().Unix()
	return &usage, nil
}

func validUsageWebSocketURL(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "wss" || u.User != nil || u.Fragment != "" || u.Port() != "" && u.Port() != "443" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "chatgpt.com" || strings.HasSuffix(host, ".chatgpt.com") || host == "openai.com" || strings.HasSuffix(host, ".openai.com")
}

func (s *OpenAIQuotaService) registerUsagePush(ctx context.Context, accountID int64, expected *OpenAIQuotaUsage) (time.Duration, int64, error) {
	status, headers, body, err := s.usageHTTP(ctx, accountID, http.MethodPost, "/codex/notification/subscriptions", nil, nil)
	if err != nil || status < 200 || status >= 300 {
		return 0, 0, fmt.Errorf("usage subscription unavailable")
	}
	var result struct {
		AccountID      string `json:"account_id"`
		AccountUserID  string `json:"account_user_id"`
		RenewalSeconds int64  `json:"renewal_seconds"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.AccountID != expected.AccountID || result.AccountUserID == "" || result.RenewalSeconds <= 0 || result.RenewalSeconds > 86400 {
		return 0, 0, fmt.Errorf("invalid usage subscription")
	}
	account, err := s.quotaCredentialAccount(ctx, accountID)
	if err != nil {
		return 0, 0, err
	}
	if id := account.GetCredential("account_user_id"); id != "" && id != result.AccountUserID {
		return 0, 0, fmt.Errorf("usage subscription identity mismatch")
	}
	principal := account.GetCredential("chatgpt_account_id")
	if principal == "" {
		principal = account.GetCredential("organization_id")
	}
	if principal != expected.AccountID {
		return 0, 0, fmt.Errorf("usage account changed")
	}
	watermark, _ := strconv.ParseInt(headers.Get("X-OpenAI-Codex-Initial-Snapshot-At-Ms"), 10, 64)
	return min(time.Duration(result.RenewalSeconds)*time.Second*9/10, 5*time.Minute), watermark, nil
}

func (s *OpenAIQuotaService) runUsagePush(ctx context.Context, accountID int64, emit func(OpenAIUsagePushEvent)) error {
	setupCtx, setupCancel := context.WithTimeout(ctx, 35*time.Second)
	defer setupCancel()
	requested, err := s.accountRepo.GetByID(setupCtx, accountID)
	if err != nil || requested == nil || requested.Status != StatusActive || !requested.IsOpenAIOAuth() {
		return fmt.Errorf("usage account is not active")
	}
	shadow := requested.IsShadow()
	// Retain the existing HTTP read as a baseline while the first push is
	// pending, and establish the authoritative account/user/plan identity.
	baseline, err := s.QueryUsage(setupCtx, accountID)
	if err != nil {
		return err
	}
	if baseline.UserID == "" || baseline.AccountID == "" || baseline.PlanType == "" {
		return fmt.Errorf("usage push identity unavailable")
	}
	// Persist the HTTP baseline even when discovery/WS is unavailable. The
	// background retry then keeps quota available without a browser request.
	if err := s.persistUsagePushSnapshot(setupCtx, accountID, shadow, baseline); err != nil {
		return err
	}
	emit(OpenAIUsagePushEvent{Type: "quota", Transport: "polling", Quota: baseline})
	status, _, discovery, err := s.usageHTTP(setupCtx, accountID, http.MethodGet, "/celsius/ws/user", nil, nil)
	if err != nil || status != 200 {
		return fmt.Errorf("usage push discovery unavailable")
	}
	var endpoint struct {
		URL string `json:"websocket_url"`
	}
	if err := json.Unmarshal(discovery, &endpoint); err != nil || !validUsageWebSocketURL(endpoint.URL) {
		return fmt.Errorf("invalid usage push endpoint")
	}
	account, err := s.quotaCredentialAccount(setupCtx, accountID)
	if err != nil {
		return err
	}
	var route platformegress.Route
	if _, _, _, _, err := s.prepareUpstreamCall(setupCtx, accountID, &route); err != nil {
		return err
	}
	dialer := s.usagePushDialer
	if dialer == nil {
		dialer = newConfiguredOpenAIWSClientDialer(s.cfg)
	}
	// The discovery URL carries its own short-lived authorization. The App
	// opens it without OAuth headers; never forward bearer tokens to it.
	conn, _, _, err := dialOpenAIWSRouteWithProfile(dialer, setupCtx, endpoint.URL, http.Header{}, route, s.quotaTLSProfile(account))
	if err != nil {
		return fmt.Errorf("usage push connection failed")
	}
	readCtx, cancelRead := context.WithCancel(ctx)
	readerDone := make(chan struct{})
	defer func() { cancelRead(); _ = conn.Close(); <-readerDone }()
	type frame struct {
		data []byte
		err  error
	}
	frames := make(chan frame, 1)
	go func() {
		defer close(readerDone)
		for {
			data, err := conn.ReadMessage(readCtx)
			if len(data) > 1<<20 {
				err = fmt.Errorf("usage push frame exceeds size limit")
			}
			select {
			case frames <- frame{data, err}:
			case <-readCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	command := func(id int, value any) error {
		writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return conn.WriteJSON(writeCtx, []any{map[string]any{"id": id, "command": value}})
	}
	if err := command(1, map[string]any{"type": "connect", "presence": map[string]any{"type": "presence", "state": "foreground"}}); err != nil {
		return err
	}
	if err := command(2, map[string]any{"type": "subscribe", "topic_id": openAIUsagePushTopic}); err != nil {
		return err
	}
	presence := map[string]any{"type": "presence", "state": "foreground", "metadata": map[string]any{"codex_usage_push": true, "codex_usage_client": "electron"}}
	connected, subscribed, present := false, false, false
	registered, hasSnapshot := false, false
	lastPresence := time.Now()
	requestID := 3
	state := &openAIUsageSnapshotState{}
	var latest *OpenAIQuotaUsage
	var lastSaved time.Time
	dirty := false
	persist := func() {
		if latest != nil && dirty {
			if s.persistUsagePushSnapshot(ctx, accountID, shadow, latest) == nil {
				dirty = false
			}
			lastSaved = time.Now()
		}
	}
	defer persist()
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	flush := time.NewTicker(5 * time.Second)
	defer flush.Stop()
	renew := time.NewTimer(24 * time.Hour)
	defer renew.Stop()
	reset := time.NewTimer(24 * time.Hour)
	defer reset.Stop()
	resetTimer := func(usage *OpenAIQuotaUsage) {
		reset.Reset(nextUsageResetDelay(usage, time.Now()))
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-setupCtx.Done():
			if !hasSnapshot {
				return fmt.Errorf("initial usage snapshot timed out")
			}
			// Once initialized, use the lifetime context, not the setup deadline.
			setupCtx = context.Background()
		case <-flush.C:
			if latest != nil && time.Since(lastSaved) >= 5*time.Second {
				persist()
			}
		case <-heartbeat.C:
			if time.Since(lastPresence) > 35*time.Second {
				return fmt.Errorf("usage push presence lost")
			}
			requestID++
			if err := command(requestID, presence); err != nil {
				return err
			}
		case <-renew.C:
			fresh, err := s.quotaCredentialAccount(ctx, accountID)
			if err != nil {
				return err
			}
			var freshRoute platformegress.Route
			if _, _, _, _, err := s.prepareUpstreamCall(ctx, accountID, &freshRoute); err != nil {
				return err
			}
			if fresh.GetCredential("chatgpt_account_id") != account.GetCredential("chatgpt_account_id") || fresh.GetChatGPTUserID() != account.GetChatGPTUserID() || fmt.Sprint(freshRoute) != fmt.Sprint(route) {
				return fmt.Errorf("usage push identity or route changed")
			}
			delay, _, err := s.registerUsagePush(ctx, accountID, baseline)
			if err != nil {
				return err
			}
			renew.Reset(delay)
		case <-reset.C:
			usage, err := s.QueryUsage(ctx, accountID)
			if err != nil {
				return fmt.Errorf("usage reset refresh failed")
			}
			if usage.UserID != baseline.UserID || usage.AccountID != baseline.AccountID || usage.PlanType != baseline.PlanType {
				return fmt.Errorf("usage identity changed")
			}
			latest = usage
			dirty = true
			persist()
			emit(OpenAIUsagePushEvent{Type: "quota", Transport: "websocket", Quota: usage})
			resetTimer(usage)
		case frame := <-frames:
			if frame.err != nil {
				return fmt.Errorf("usage push disconnected")
			}
			var messages []struct {
				ID      int             `json:"id"`
				Reply   json.RawMessage `json:"reply"`
				Type    string          `json:"type"`
				Topic   string          `json:"topic_id"`
				Payload json.RawMessage `json:"payload"`
			}
			if err := json.Unmarshal(frame.data, &messages); err != nil {
				var control struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(frame.data, &control) == nil && control.Type != "" && control.Type != "error" {
					continue
				}
				return fmt.Errorf("invalid usage push frame")
			}
			for _, message := range messages {
				if message.ID != 0 {
					var reply struct {
						Type  string `json:"type"`
						Topic string `json:"topic_id"`
					}
					if json.Unmarshal(message.Reply, &reply) != nil {
						return fmt.Errorf("invalid usage push reply")
					}
					switch message.ID {
					case 1:
						if reply.Type != "connect" {
							return fmt.Errorf("usage connect rejected")
						}
						connected = true
						if err := command(3, presence); err != nil {
							return err
						}
					case 2:
						if reply.Type != "subscribe" || reply.Topic != openAIUsagePushTopic {
							return fmt.Errorf("usage subscribe rejected")
						}
						subscribed = true
					default:
						if reply.Type == "presence" {
							present = true
							lastPresence = time.Now()
						}
					}
					if connected && subscribed && present && !registered {
						delay, watermark, err := s.registerUsagePush(setupCtx, accountID, baseline)
						if err != nil {
							return err
						}
						registered = true
						state.minimumGenerated = watermark
						renew.Reset(delay)
					}
					continue
				}
				if message.Type != "message" || message.Topic != openAIUsagePushTopic {
					continue
				}
				if !registered {
					continue
				}
				usage, err := state.parse(message.Payload, baseline)
				if err != nil {
					return err
				}
				if usage == nil {
					continue
				}
				hasSnapshot = true
				setupCancel()
				setupCtx = context.Background()
				if usage.RateLimitResetCredits == nil {
					usage.RateLimitResetCredits = baseline.RateLimitResetCredits
				}
				latest = usage
				dirty = true
				if lastSaved.IsZero() {
					persist()
				}
				emit(OpenAIUsagePushEvent{Type: "quota", Transport: "websocket", Quota: usage})
				resetTimer(usage)
			}
		}
	}
}

func nextUsageResetDelay(usage *OpenAIQuotaUsage, now time.Time) time.Duration {
	next := 24 * time.Hour
	consider := func(at int64) {
		if delay := time.Unix(at, 0).Add(time.Second).Sub(now); delay > 0 && delay < next {
			next = delay
		}
	}
	limits := []*OpenAIRateLimit{usage.RateLimit, usage.CodeReviewRateLimit}
	for _, additional := range usage.AdditionalRateLimits {
		limits = append(limits, additional.RateLimit)
	}
	for _, limit := range limits {
		if limit == nil {
			continue
		}
		for _, window := range []*OpenAIRateLimitWindow{limit.PrimaryWindow, limit.SecondaryWindow} {
			if window != nil {
				consider(window.ResetAt)
			}
		}
	}
	for _, bucket := range usage.RateLimitsByLimitID {
		consider(bucket.ResetsAt)
		for _, window := range []*OpenAIAppServerRateLimitWindow{bucket.Primary, bucket.Secondary} {
			if window != nil {
				consider(window.ResetsAt)
			}
		}
	}
	var spend struct {
		Individual struct {
			ResetAt int64 `json:"reset_at"`
		} `json:"individual_limit"`
	}
	if json.Unmarshal(usage.SpendControl, &spend) == nil {
		consider(spend.Individual.ResetAt)
	}
	return next
}

func (s *OpenAIQuotaService) persistUsagePushSnapshot(ctx context.Context, accountID int64, shadow bool, usage *OpenAIQuotaUsage) error {
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	account, err := s.accountRepo.GetByID(writeCtx, accountID)
	if err != nil || account == nil || account.Status != StatusActive {
		return fmt.Errorf("usage account is not active")
	}
	if shadow {
		account, err = s.quotaCredentialAccount(writeCtx, accountID)
		if err != nil {
			return err
		}
	}
	principal := account.GetCredential("chatgpt_account_id")
	if principal == "" {
		principal = account.GetCredential("organization_id")
	}
	if principal != usage.AccountID || account.GetChatGPTUserID() != "" && account.GetChatGPTUserID() != usage.UserID {
		return fmt.Errorf("usage account identity changed")
	}
	updates := make(map[string]any)
	if snapshot := rateLimitSnapshotFromUsage(usage); snapshot != nil {
		updates[openAIQuotaRateLimitSnapshotKey] = snapshot
	}
	now := time.Now()
	if shadow {
		for key, value := range buildCodexSparkWindowExtraUpdates(usage, now) {
			updates[key] = value
		}
	} else if limit := usage.RateLimit; limit != nil {
		snapshot := &OpenAICodexUsageSnapshot{UpdatedAt: now.Format(time.RFC3339)}
		for i, window := range []*OpenAIRateLimitWindow{limit.PrimaryWindow, limit.SecondaryWindow} {
			if window == nil {
				continue
			}
			percent, minutes, seconds := window.UsedPercent, int(window.LimitWindowSeconds/60), int(window.ResetAfterSeconds)
			if window.ResetAt > 0 {
				seconds = max(0, int(window.ResetAt-now.Unix()))
			}
			if i == 0 {
				snapshot.PrimaryUsedPercent, snapshot.PrimaryWindowMinutes, snapshot.PrimaryResetAfterSeconds = &percent, &minutes, &seconds
			} else {
				snapshot.SecondaryUsedPercent, snapshot.SecondaryWindowMinutes, snapshot.SecondaryResetAfterSeconds = &percent, &minutes, &seconds
			}
		}
		for key, value := range buildCodexUsageExtraUpdates(snapshot, now) {
			updates[key] = value
		}
	}
	if len(updates) == 0 {
		return nil
	}
	return s.accountRepo.UpdateExtra(writeCtx, accountID, updates)
}
