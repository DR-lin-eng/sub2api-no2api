package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"
)

type usageSyncRepository struct {
	AccountRepository
	mu       sync.Mutex
	accounts map[int64]*Account
	writes   atomic.Int64
}

func (r *usageSyncRepository) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.accounts[id]; a != nil {
		clone := *a
		clone.Extra = maps.Clone(a.Extra)
		clone.Credentials = maps.Clone(a.Credentials)
		return &clone, nil
	}
	return nil, ErrAccountNotFound
}

func (r *usageSyncRepository) ListByPlatform(ctx context.Context, _ string) ([]Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []Account
	for _, account := range r.accounts {
		clone := *account
		clone.Credentials = maps.Clone(account.Credentials)
		clone.Extra = maps.Clone(account.Extra)
		result = append(result, clone)
	}
	return result, ctx.Err()
}

func (r *usageSyncRepository) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.accounts[id]
	if a == nil {
		return ErrAccountNotFound
	}
	if a.Extra == nil {
		a.Extra = make(map[string]any)
	}
	maps.Copy(a.Extra, updates)
	r.writes.Add(1)
	return nil
}

func normalUsageSyncAccount(id int64) *Account {
	return &Account{ID: id, Status: StatusActive, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "acct-41", "chatgpt_user_id": "user-41", "access_token": "fixture-token"}}
}

type usageSyncFixture struct {
	syncer         *OpenAIUsageSyncService
	quota          *OpenAIQuotaService
	repo           *usageSyncRepository
	connCount      atomic.Int64
	closed         atomic.Int64
	next           chan float64
	disconnect     chan struct{}
	discoveryFails atomic.Bool
	httpCalls      atomic.Int64
}

func newUsageSyncFixture(t *testing.T) *usageSyncFixture {
	t.Helper()
	f := &usageSyncFixture{repo: &usageSyncRepository{accounts: map[int64]*Account{41: normalUsageSyncAccount(41)}}, next: make(chan float64, 4), disconnect: make(chan struct{}, 1)}
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		f.connCount.Add(1)
		defer f.closed.Add(1)
		defer func() { _ = conn.Close() }()
		var mu sync.Mutex
		write := func(v any) { mu.Lock(); defer mu.Unlock(); _ = conn.WriteJSON(v) }
		done := make(chan struct{})
		defer close(done)
		go func() {
			sequence := 0
			for {
				select {
				case <-done:
					return
				case <-f.disconnect:
					_ = conn.Close()
					return
				case percent := <-f.next:
					sequence++
					snapshot, err := sjson.SetBytes(usageSnapshot(sequence, time.Now().UnixMilli(), "user-41"), "payload.usage_by_client.electron.rate_limit.primary_window.used_percent", percent)
					if err != nil {
						t.Errorf("build usage snapshot: %v", err)
						return
					}
					write([]any{map[string]any{"type": "message", "topic_id": openAIUsagePushTopic, "payload": json.RawMessage(snapshot)}})
				}
			}
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
					reply["topic_id"] = openAIUsagePushTopic
					reply["recovered"] = false
				}
				write([]any{map[string]any{"id": command.ID, "reply": reply}})
			}
		}
	}))
	t.Cleanup(server.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.httpCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			_, _ = io.WriteString(w, `{"user_id":"user-41","account_id":"acct-41","plan_type":"plus","rate_limit":{"allowed":true,"primary_window":{"used_percent":10,"limit_window_seconds":18000,"reset_at":1900000000}}}`)
		case "/backend-api/wham/rate-limit-reset-credits":
			_, _ = io.WriteString(w, `{"available_count":0,"credits":[]}`)
		case "/backend-api/celsius/ws/user":
			if f.discoveryFails.Load() {
				w.WriteHeader(503)
				return
			}
			_, _ = io.WriteString(w, `{"websocket_url":"wss://ws.chatgpt.com/fixture"}`)
		case "/backend-api/codex/notification/subscriptions":
			_, _ = io.WriteString(w, `{"account_id":"acct-41","account_user_id":"member-41","renewal_seconds":600}`)
			f.next <- 25
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)
	tokens := &stubQuotaTokenCache{tokens: map[string]string{OpenAITokenCacheKey(f.repo.accounts[41]): "fixture-token"}}
	f.quota = NewOpenAIQuotaService(f.repo, nil, NewOpenAITokenProvider(f.repo, tokens, nil), newQuotaRedirectingFactory(api))
	f.quota.usagePushDialer = &usageRedirectDialer{target: "ws" + strings.TrimPrefix(server.URL, "http")}
	f.syncer = NewOpenAIUsageSyncService(f.quota, nil)
	f.syncer.opts.scanInterval = 25 * time.Millisecond
	f.syncer.opts.retryMin = 30 * time.Millisecond
	f.syncer.opts.retryMax = 120 * time.Millisecond
	f.syncer.opts.idleInterval = 10 * time.Millisecond
	t.Cleanup(f.syncer.Stop)
	return f
}

func (f *usageSyncFixture) storedPercent() float64 {
	a, _ := f.repo.GetByID(context.Background(), 41)
	if a != nil && a.Extra != nil {
		if value, ok := a.Extra["codex_5h_used_percent"].(float64); ok {
			return value
		}
	}
	return -1
}

func TestOpenAIUsageBackgroundFirstCallSurvivesBrowserExitAndReconnects(t *testing.T) {
	f := newUsageSyncFixture(t)
	f.syncer.Start()
	// Merely visiting a page before any call must not create an upstream WS.
	_, releasePage, err := f.quota.SubscribeUsage(context.Background(), 41)
	require.NoError(t, err)
	releasePage()
	require.Zero(t, f.connCount.Load())
	a, _ := f.repo.GetByID(context.Background(), 41)
	gateway := &OpenAIGatewayService{usageSync: f.syncer}
	gateway.notifyOpenAIUsageActivity(a)
	require.Eventually(t, func() bool { return f.storedPercent() == 25 }, 3*time.Second, 10*time.Millisecond)
	_, releasePage, err = f.quota.SubscribeUsage(context.Background(), 41)
	require.NoError(t, err)
	releasePage()
	f.next <- 73
	// Projection flushes are batched for at most five seconds, with no page.
	require.Eventually(t, func() bool { return f.storedPercent() == 73 }, 6*time.Second, 20*time.Millisecond)
	require.EqualValues(t, 1, f.connCount.Load())
	f.disconnect <- struct{}{}
	require.Eventually(t, func() bool { return f.connCount.Load() >= 2 && f.storedPercent() == 25 }, 4*time.Second, 10*time.Millisecond)
	f.syncer.Stop()
	require.Eventually(t, func() bool { return f.closed.Load() == f.connCount.Load() }, time.Second, 10*time.Millisecond)
}

func TestOpenAIUsageBackgroundRestoresRecentAccountsAndExpiresIdle(t *testing.T) {
	f := newUsageSyncFixture(t)
	f.syncer.opts.idleWindow = 300 * time.Millisecond
	now := time.Now()
	f.repo.accounts[41].LastUsedAt = &now
	f.syncer.Start()
	require.Eventually(t, func() bool { return f.connCount.Load() == 1 }, 2*time.Second, 5*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	a, _ := f.repo.GetByID(context.Background(), 41)
	f.syncer.NotifyAccountUsed(a) // Extend the sliding window.
	require.Eventually(t, func() bool { return f.closed.Load() == 1 }, 2*time.Second, 10*time.Millisecond)
	count := f.connCount.Load()
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, count, f.connCount.Load(), "quota queries and WS traffic must not extend activity")
	f.syncer.NotifyAccountUsed(a)
	require.Eventually(t, func() bool { return f.connCount.Load() == count+1 }, 2*time.Second, 5*time.Millisecond)
}

func TestOpenAIUsageBackgroundThousandAccountPoolDoesNotHandshakeIdleAccounts(t *testing.T) {
	f := newUsageSyncFixture(t)
	for id := int64(42); id <= 1040; id++ {
		f.repo.accounts[id] = normalUsageSyncAccount(id)
	}
	old := time.Now().Add(-11 * time.Minute)
	f.repo.accounts[42].LastUsedAt = &old
	f.repo.accounts[43].Status = StatusError
	f.repo.accounts[44].Status = "inactive"
	f.syncer.Start()
	require.Never(t, func() bool { return f.httpCalls.Load() > 0 }, 100*time.Millisecond, 10*time.Millisecond)
	for _, id := range []int64{43, 44} {
		a, _ := f.repo.GetByID(context.Background(), id)
		f.syncer.NotifyAccountUsed(a)
	}
	require.Zero(t, f.httpCalls.Load())
	a, _ := f.repo.GetByID(context.Background(), 41)
	f.syncer.NotifyAccountUsed(a)
	require.Eventually(t, func() bool { return f.storedPercent() == 25 }, 2*time.Second, 5*time.Millisecond)
	require.EqualValues(t, 1, f.connCount.Load(), "only the first-called account should connect in a pool of 1000")
	f.repo.mu.Lock()
	f.repo.accounts[41].Status = StatusError
	f.repo.mu.Unlock()
	require.Eventually(t, func() bool { return f.closed.Load() == 1 }, 2*time.Second, 10*time.Millisecond)
}

func TestOpenAIUsageBackgroundDiscoveryFailurePersistsHTTPFallback(t *testing.T) {
	f := newUsageSyncFixture(t)
	f.discoveryFails.Store(true)
	f.syncer.Start()
	a, _ := f.repo.GetByID(context.Background(), 41)
	f.syncer.NotifyAccountUsed(a)
	require.Eventually(t, func() bool { return f.storedPercent() == 10 }, 2*time.Second, 5*time.Millisecond)
	require.Zero(t, f.connCount.Load())
	f.discoveryFails.Store(false)
	require.Eventually(t, func() bool { return f.storedPercent() == 25 }, 2*time.Second, 5*time.Millisecond)
}

func TestOpenAIUsageBackgroundHandshakeLimitAndNoObserverCap(t *testing.T) {
	repo := &usageSyncRepository{accounts: make(map[int64]*Account)}
	now := time.Now()
	for id := int64(1); id <= 1001; id++ {
		a := normalUsageSyncAccount(id)
		a.LastUsedAt = &now
		repo.accounts[id] = a
	}
	var inflight, peak atomic.Int64
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inflight.Add(1)
		defer inflight.Add(-1)
		for prior := peak.Load(); n > prior && !peak.CompareAndSwap(prior, n); prior = peak.Load() {
		}
		<-r.Context().Done()
	}))
	defer api.Close()
	tokens := make(map[string]string, len(repo.accounts))
	for _, account := range repo.accounts {
		tokens[OpenAITokenCacheKey(account)] = "fixture-token"
	}
	quota := NewOpenAIQuotaService(repo, nil, NewOpenAITokenProvider(repo, &stubQuotaTokenCache{tokens: tokens}, nil), newQuotaRedirectingFactory(api))
	svc := NewOpenAIUsageSyncService(quota, nil)
	require.Equal(t, 1000, svc.opts.handshakes)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workers := make(map[int64]*openAIUsageSyncWorker)
	svc.reconcile(ctx, workers, make(chan struct{}, svc.opts.handshakes))
	require.Len(t, workers, 1001, "accounts beyond the handshake limit must wait in the queue")
	require.Eventually(t, func() bool { return peak.Load() == 1000 }, 15*time.Second, 10*time.Millisecond)
	require.LessOrEqual(t, peak.Load(), int64(1000))
	cancel()
	for _, worker := range workers {
		<-worker.done
	}
	require.Eventually(t, func() bool { return inflight.Load() == 0 }, time.Second, 5*time.Millisecond)
}

type usageSyncLease struct {
	mu       sync.Mutex
	owner    string
	rejected string
}

func (l *usageSyncLease) TryAcquireLeaderLock(ctx context.Context, _, owner string, _ time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if l.owner != "" {
		return false, nil
	}
	l.owner = owner
	return true, nil
}
func (l *usageSyncLease) RenewLeaderLock(_ context.Context, _, owner string, _ time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if owner == l.rejected {
		return false, errors.New("lease unavailable")
	}
	return owner == l.owner, nil
}
func (l *usageSyncLease) ReleaseLeaderLock(_ context.Context, _, owner string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.owner == owner {
		l.owner = ""
	}
	return nil
}

func TestOpenAIUsageBackgroundSingleOwnerStopsOnLeaseLoss(t *testing.T) {
	a, b := newUsageSyncFixture(t), newUsageSyncFixture(t)
	lease := &usageSyncLease{}
	now := time.Now()
	for _, f := range []*usageSyncFixture{a, b} {
		f.syncer.locks = lease
		f.syncer.opts.leaseInterval = 20 * time.Millisecond
		f.repo.accounts[41].LastUsedAt = &now
	}
	a.syncer.Start()
	require.Eventually(t, func() bool { return a.storedPercent() == 25 }, 2*time.Second, 10*time.Millisecond)
	b.syncer.Start()
	require.Never(t, func() bool { return b.connCount.Load() != 0 }, 100*time.Millisecond, 10*time.Millisecond)
	lease.mu.Lock()
	lease.rejected = lease.owner
	lease.owner = ""
	lease.mu.Unlock()
	require.Eventually(t, func() bool { return a.closed.Load() >= 1 }, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return b.storedPercent() == 25 || a.connCount.Load() > 1 }, 2*time.Second, 10*time.Millisecond)
	a.syncer.Stop()
	b.syncer.Stop()
	require.Eventually(t, func() bool { return a.closed.Load() == a.connCount.Load() && b.closed.Load() == b.connCount.Load() }, time.Second, 10*time.Millisecond)
}

func TestOpenAIUsageActivityOnlyModelSendTriggersImmediateSync(t *testing.T) {
	f := newUsageSyncFixture(t)
	s := &OpenAIGatewayService{accountRepo: f.repo, usageSync: f.syncer, httpUpstream: &quotaAccountTransportCapture{responseBody: `{"ok":true}`}}
	a, err := f.repo.GetByID(context.Background(), 41)
	require.NoError(t, err)
	for _, path := range []string{"/backend-api/wham/usage", "/backend-api/codex/models"} {
		req, err := http.NewRequest(http.MethodGet, "https://chatgpt.com"+path, nil)
		require.NoError(t, err)
		resp, err := s.doAccountHTTPUpstream(req, "", a)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.True(t, f.syncer.lastActivity(41, time.Time{}).IsZero())
	}
	req, err := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", strings.NewReader(`{"model":"fixture"}`))
	require.NoError(t, err)
	resp, err := s.doAccountHTTPUpstream(req, "", a)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.False(t, f.syncer.lastActivity(41, time.Time{}).IsZero(), "activity starts on send, before completion/billing")
}
