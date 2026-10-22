package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/platform/liveattestation"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type recordedAccountProof struct {
	kind  string
	scope liveattestation.RequestScope
}

type accountProofRecorder struct {
	mu    sync.Mutex
	calls []recordedAccountProof
}

type scopedProofFixture struct {
	recorder *accountProofRecorder
	key      string
}

func (p scopedProofFixture) Check(context.Context) error { return nil }
func (p scopedProofFixture) ForScope(key string) liveattestation.Provider {
	return scopedProofFixture{recorder: p.recorder, key: key}
}
func (p scopedProofFixture) record(ctx context.Context, kind string) error {
	scope, ok := liveattestation.RequestScopeFromContext(ctx)
	if !ok || scope.Key != p.key {
		return liveattestation.ErrScopeMismatch
	}
	p.recorder.mu.Lock()
	defer p.recorder.mu.Unlock()
	p.recorder.calls = append(p.recorder.calls, recordedAccountProof{kind, scope})
	return nil
}
func (p scopedProofFixture) GenerateCodexToken(ctx context.Context) (string, error) {
	return "opaque-local-fixture", p.record(ctx, "codex")
}
func (p scopedProofFixture) GenerateDeviceCheckToken(ctx context.Context) (string, error) {
	return "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", p.record(ctx, "raw")
}
func (p scopedProofFixture) Generate(ctx context.Context) (string, error) {
	return `{"v":1,"s":0,"t":"v1.Zml4dHVyZQ"}`, p.record(ctx, "live")
}

func proofBindingAccount(id int64) *Account {
	return &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": fmt.Sprintf("workspace-%d", id), "chatgpt_user_id": fmt.Sprintf("user-%d", id)},
		Extra:       map[string]any{"openai_device_id": fmt.Sprintf("installation-%d", id)}}
}

func TestOpenAIProofBindingConcurrentAccountsAndThreads(t *testing.T) {
	r := &accountProofRecorder{}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIOAuthAttestationEnabled = true
	svc := &OpenAIGatewayService{cfg: cfg, liveAttestation: scopedProofFixture{recorder: r}}
	var workers sync.WaitGroup
	for i := range 24 {
		workers.Go(func() {
			a := proofBindingAccount(int64(i + 1))
			h := make(http.Header)
			threadID := fmt.Sprintf("thread-%d", i+1)
			h.Set("thread-id", threadID)
			h.Set("x-codex-installation-id", "untrusted-device")
			identity := newOpenAIProofIdentityFromHeaders(a, nil, h)
			identity.threadID = threadID
			_, err := svc.prepareOpenAIOAuthProof(withOpenAIProofIdentity(context.Background(), identity), a, "https://chatgpt.com/backend-api/codex/responses", h)
			if err != nil || h.Get("x-oai-attestation") != `{"v":1,"s":0,"t":"opaque-local-fixture"}` {
				t.Error("account callback failed")
			}
		})
	}
	workers.Wait()
	require.Len(t, r.calls, 24)
	keys := make(map[string]bool)
	for _, call := range r.calls {
		require.False(t, keys[call.scope.Key])
		keys[call.scope.Key] = true
		var id int
		_, err := fmt.Sscanf(call.scope.InstallationID, "installation-%d", &id)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("thread-%d", id), call.scope.ThreadID)
		require.Equal(t, "codex", call.kind)
	}
}

func TestOpenAIProofBindingFailoverShadowAndRelay(t *testing.T) {
	a, b := proofBindingAccount(31), proofBindingAccount(32)
	r := &accountProofRecorder{}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIOAuthAttestationEnabled = true
	svc := &OpenAIGatewayService{cfg: cfg, liveAttestation: scopedProofFixture{recorder: r}}
	fingerprint := &codexFingerprintIDs{mode: codexFingerprintFull, fullSimulation: true, installationID: "projected-installation", threadID: "projected-thread"}
	ctx := withOpenAIProofIdentity(context.Background(), newOpenAIProofIdentity(a, fingerprint))
	ctx, err := svc.prepareOpenAIOAuthProof(ctx, a, "https://chatgpt.com/backend-api/codex/responses", make(http.Header))
	require.NoError(t, err)
	first, ok := liveattestation.RequestScopeFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "projected-installation", first.InstallationID)
	secondHeaders := http.Header{"Thread-Id": {"thread-b"}}
	secondIdentity := newOpenAIProofIdentityFromHeaders(b, nil, secondHeaders)
	secondIdentity.threadID = "thread-b"
	ctx, err = svc.prepareOpenAIOAuthProof(withOpenAIProofIdentity(context.Background(), secondIdentity), b, "https://chatgpt.com/backend-api/codex/responses", secondHeaders)
	require.NoError(t, err)
	second, ok := liveattestation.RequestScopeFromContext(ctx)
	require.True(t, ok)
	require.NotEqual(t, first.Key, second.Key)
	require.Equal(t, "installation-32", second.InstallationID)
	require.Equal(t, "thread-b", second.ThreadID)
	ctx, err = svc.prepareOpenAIOAuthProof(ctx, b, "https://relay.example/backend-api/codex/responses", make(http.Header))
	require.NoError(t, err)
	_, ok = liveattestation.RequestScopeFromContext(ctx)
	require.False(t, ok)
	require.Len(t, r.calls, 2)

	pid := a.ID
	shadow := proofBindingAccount(33)
	shadow.ParentAccountID = &pid
	svc.accountRepo = newStubCredRepo(a)
	ctx, err = svc.prepareOpenAIOAuthProof(context.Background(), shadow, "https://chatgpt.com/backend-api/codex/responses", make(http.Header))
	require.NoError(t, err)
	scope, _ := liveattestation.RequestScopeFromContext(ctx)
	require.Equal(t, openAIProofKey(shadow, a, "https://chatgpt.com", "installation-33"), scope.Key)
	require.NotEqual(t, openAIProofKey(shadow, shadow, "https://chatgpt.com", "installation-33"), scope.Key)
}

func TestOpenAIProofIdentityReceiptUsesActualHTTPProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, suffix := range []string{"", "/compact"} {
			a := proofBindingAccount(41)
			r := &accountProofRecorder{}
			cfg := &config.Config{}
			cfg.Gateway.OpenAIOAuthAttestationEnabled = true
			svc := &OpenAIGatewayService{cfg: cfg, liveAttestation: scopedProofFixture{recorder: r}}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses"+suffix, nil)
			ids := &codexFingerprintIDs{mode: codexFingerprintFull, fullSimulation: true, installationID: "metadata-installation", sessionID: "projected-session", threadID: "projected-thread"}
			body := []byte(`{"model":"gpt-5","input":[],"client_metadata":{"x-codex-installation-id":"metadata-installation"}}`)
			var req *http.Request
			var err error
			if passthrough {
				req, err = svc.buildUpstreamRequestOpenAIPassthroughWithFingerprint(context.Background(), c, a, body, "opaque-auth-fixture", ids)
			} else {
				req, err = svc.buildUpstreamRequestWithFingerprint(context.Background(), c, a, body, "opaque-auth-fixture", false, "", true, ids)
			}
			require.NoError(t, err)
			_, err = svc.prepareOpenAIOAuthProof(req.Context(), a, req.URL.String(), req.Header)
			require.NoError(t, err)
			require.Len(t, r.calls, 1)
			require.Equal(t, "metadata-installation", r.calls[0].scope.InstallationID)
			require.Equal(t, "projected-thread", r.calls[0].scope.ThreadID)
		}
	}
}

func TestOpenAIWSProofFactorySnapshotsIdentityForDialAndPrewarm(t *testing.T) {
	a := proofBindingAccount(51)
	r := &accountProofRecorder{}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIOAuthAttestationEnabled = true
	svc := &OpenAIGatewayService{cfg: cfg, liveAttestation: scopedProofFixture{recorder: r}}
	ids := &codexFingerprintIDs{mode: codexFingerprintFull, fullSimulation: true, installationID: "ws-installation", threadID: "ws-thread"}
	url := "wss://chatgpt.com/backend-api/codex/responses"
	factory := svc.newOpenAIWSProofFactory(a, url, ids)
	ids.installationID, ids.threadID = "later-installation", "later-thread"
	dialer := &fingerprintHeaderCaptureDialer{}
	pool := &openAIWSConnPool{clientDialer: dialer}
	for range 2 {
		_, err := pool.dialConn(context.Background(), openAIWSAcquireRequest{Account: a, WSURL: url, Headers: make(http.Header), ProofFactory: factory})
		require.Error(t, err)
	}
	require.Len(t, r.calls, 2)
	for _, call := range r.calls {
		require.Equal(t, "ws-installation", call.scope.InstallationID)
		require.Equal(t, "ws-thread", call.scope.ThreadID)
	}
}

func TestDesktopDeviceCheckCookieAndGeneratorShareAccountScope(t *testing.T) {
	r := &accountProofRecorder{}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIDesktopDeviceCheckEnabled = true
	u := &integrityWireUpstream{}
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: u, liveAttestation: scopedProofFixture{recorder: r}}
	a, b := proofBindingAccount(61), proofBindingAccount(62)
	// Even duplicated device IDs and absent upstream principal claims must not
	// cause unrelated credential owners to inherit each other's cookie/state.
	a.Credentials, b.Credentials = nil, nil
	a.Extra, b.Extra = map[string]any{"openai_device_id": "shared-device"}, map[string]any{"openai_device_id": "shared-device"}
	for _, account := range []*Account{a, b, a} {
		req, err := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/messaging/rooms/fixture/messages", nil)
		require.NoError(t, err)
		req.Header.Set("oai-did", "untrusted-device")
		resp, err := svc.doAccountHTTPUpstream(req, "", account)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	require.Len(t, svc.openAIDeviceCookies, 2)
	require.Len(t, u.requests, 5, "two registrations and three messages")
	require.Len(t, r.calls, 3)
	require.NotEqual(t, r.calls[0].scope.Key, r.calls[1].scope.Key)
	require.Equal(t, r.calls[0].scope.Key, r.calls[2].scope.Key)
	for _, req := range u.requests {
		require.Equal(t, "shared-device", req.Header.Get("oai-did"))
	}
}

func TestOpenAIProofNamespaceSurvivesRefreshAndIsolatesUnknownUsers(t *testing.T) {
	a := proofBindingAccount(71)
	before := openAIIntegrityKey(a, a, "https://chatgpt.com")
	a.Credentials["access_token"] = "new-opaque-auth-fixture"
	require.Equal(t, before, openAIIntegrityKey(a, a, "https://chatgpt.com"))
	b := proofBindingAccount(72)
	a.Credentials, b.Credentials = map[string]any{"chatgpt_account_id": "shared-workspace"}, map[string]any{"chatgpt_account_id": "shared-workspace"}
	a.Extra, b.Extra = map[string]any{"openai_device_id": "shared-installation"}, map[string]any{"openai_device_id": "shared-installation"}
	require.NotEqual(t, openAIIntegrityKey(a, a, "https://chatgpt.com"), openAIIntegrityKey(b, b, "https://chatgpt.com"))
}

type unavailableScopedProof struct{ liveAttestationStub }

func (unavailableScopedProof) ForScope(string) liveattestation.Provider { return nil }

func TestCreateLiveCallScopeFailureReleasesAccountBeforeLiveLease(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	var acquired, released []int64
	cache := &liveConcurrencyRecordingCache{schedulerTestConcurrencyCache: schedulerTestConcurrencyCache{acquiredIDs: &acquired, releasedIDs: &released}, acquireResult: true}
	concurrency := NewConcurrencyService(cache)
	concurrency.SetStandaloneRequestSlots(true)
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	cfg.JWT.Secret = "scope-failure-test-secret"
	a := proofBindingAccount(81)
	a.Status, a.Schedulable, a.Concurrency = StatusActive, true, 1
	u := &liveHTTPUpstreamStub{}
	svc := &OpenAIGatewayService{
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{*a}},
		cache:       &liveTestStore{GatewayCache: &schedulerTestGatewayCache{}}, cfg: cfg,
		concurrencyService: concurrency, httpUpstream: u,
		liveAttestation: unavailableScopedProof{}, liveAttestationCipher: newLiveAttestationCipher(cfg),
	}
	_, err := svc.CreateLiveCall(context.Background(), &LiveCallRequest{SDP: "v=0\r\n", Session: json.RawMessage(`{"model":"gpt-live-test"}`)}, LiveCallIdentity{UserID: 20, APIKeyID: 30}, 1, 1)
	var unavailable *LiveAttestationUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.Equal(t, []int64{81}, acquired)
	require.Equal(t, []int64{81}, released)
	acquireCalls, releaseCalls := cache.callCounts()
	require.Zero(t, acquireCalls)
	require.Zero(t, releaseCalls)
	require.Nil(t, u.request)
}
