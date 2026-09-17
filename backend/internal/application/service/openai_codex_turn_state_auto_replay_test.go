package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func automaticTurnStateTestSettings(t *testing.T, models ...string) *SettingService {
	t.Helper()
	settings := NewSettingService(newCodexSimulationSettingRepo(), &config.Config{})
	_, err := settings.SetCodexSimulationSettings(context.Background(), &CodexSimulationSettings{
		TurnStateAutoReplayEnabled: true,
		TurnStateWatchModels:       models,
		ContinuationMode:           "off",
		StateTTLSeconds:            60,
	})
	require.NoError(t, err)
	return settings
}

func automaticTurnStateTestAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "turn-state-test",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "chatgpt-account-" + string(rune('a'+id)),
			"model_mapping":      map[string]any{"gpt-5.6-codex": "gpt-5.6-codex"},
		},
	}
}

func TestOpenAICodexTurnStateCharacterCountUsesUnicodeCharacters(t *testing.T) {
	state := strings.Repeat("状", openAICodexDefaultTurnStateCharacters)
	require.Greater(t, len(state), openAICodexDefaultTurnStateCharacters)
	require.Equal(t, openAICodexDefaultTurnStateCharacters, openAICodexTurnStateCharacterCount(state))
}

func TestCaptureOpenAICodexTurnStateMetadata(t *testing.T) {
	state := strings.Repeat("s", openAICodexDefaultTurnStateCharacters)
	headers := make(http.Header)
	payload := []byte(`{"type":"codex.response.metadata","headers":{"x-codex-turn-state":"` + state + `"}}`)

	require.Equal(t, state, captureOpenAICodexTurnStateMetadata(headers, payload))
	require.Equal(t, state, headers.Get(openAICodexTurnStateHeader))

	headers = make(http.Header)
	sse := "data: " + string(payload) + "\n\ndata: {\"type\":\"response.completed\"}\n\n"
	require.Equal(t, state, captureOpenAICodexTurnStateFromSSE(headers, sse))
	require.Equal(t, state, headers.Get(openAICodexTurnStateHeader))
	require.Empty(t, captureOpenAICodexTurnStateMetadata(headers, []byte(`{"type":"codex.response.metadata","headers":{"x-codex-turn-state":"bad\r\nvalue"}}`)))
	require.Equal(t, state, headers.Get(openAICodexTurnStateHeader))
}

func TestAutomaticCodexTurnStateReplayUsesConfiguredCharacterLength(t *testing.T) {
	settings := NewSettingService(newCodexSimulationSettingRepo(), &config.Config{})
	_, err := settings.SetCodexSimulationSettings(context.Background(), &CodexSimulationSettings{
		TurnStateAutoReplayEnabled: true,
		TurnStateTargetLength:      3,
		TurnStateWatchModels:       []string{"gpt-5.6-codex"},
		ContinuationMode:           "off",
		StateTTLSeconds:            60,
	})
	require.NoError(t, err)
	svc := &OpenAIGatewayService{settingService: settings}
	account := automaticTurnStateTestAccount(9)
	svc.observeOpenAICodexTurnState(context.Background(), nil, account, "gpt-5.6-codex", "abc")
	require.Equal(t, "abc", svc.loadOpenAICodexAutoTurnState(context.Background(), account, "gpt-5.6-codex"))
	svc.observeOpenAICodexTurnState(context.Background(), nil, account, "gpt-5.6-codex", "x")
	require.Equal(t, "abc", svc.loadOpenAICodexAutoTurnState(context.Background(), account, "gpt-5.6-codex"))
	svc.observeOpenAICodexTurnState(context.Background(), nil, account, "gpt-5.6-codex", "a\nb")
	require.Equal(t, "abc", svc.loadOpenAICodexAutoTurnState(context.Background(), account, "gpt-5.6-codex"))
}

func TestAutomaticCodexTurnStateReplayMessagesBridgeRejectsOldSessionAndCapturesMetadata(t *testing.T) {
	settings := automaticTurnStateTestSettings(t, "gpt-5.4")
	metadataState := strings.Repeat("m", openAICodexDefaultTurnStateCharacters)
	first := openAICompatSSECompletedResponse("resp_state_bad", "gpt-5.4")
	first.Header.Set(openAICodexTurnStateHeader, "bad-session-state")
	second := openAICompatSSECompletedResponse("resp_state_good", "gpt-5.4")
	secondBody, err := io.ReadAll(second.Body)
	require.NoError(t, err)
	_ = second.Body.Close()
	metadata := []byte(`data: {"type":"codex.response.metadata","headers":{"x-codex-turn-state":"` + metadataState + `"}}` + "\n\n")
	second.Body = io.NopCloser(bytes.NewReader(append(metadata, secondBody...)))
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		first, second, openAICompatSSECompletedResponse("resp_state_replay", "gpt-5.4"),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream:   upstream,
		settingService: settings,
		cfg: &config.Config{Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		}},
	}
	account := automaticTurnStateTestAccount(11)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"hi"}],"stream":false}`)
	for i := 0; i < 3; i++ {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
		result, forwardErr := svc.ForwardAsAnthropic(context.Background(), c, account, body, "same-session", "gpt-5.4")
		require.NoError(t, forwardErr)
		require.NotNil(t, result)
	}
	require.Empty(t, upstream.requests[0].Header.Get(openAICodexTurnStateHeader))
	require.Empty(t, upstream.requests[1].Header.Get(openAICodexTurnStateHeader), "the old short session state must not bypass the automatic replay gate")
	require.Equal(t, metadataState, upstream.requests[2].Header.Get(openAICodexTurnStateHeader))
}

func TestAutomaticCodexTurnStateReplayScopesByAccountAndWatchedModel(t *testing.T) {
	settings := automaticTurnStateTestSettings(t, "gpt-5.6-codex")
	svc := &OpenAIGatewayService{settingService: settings}
	account := automaticTurnStateTestAccount(1)
	state := strings.Repeat("s", openAICodexDefaultTurnStateCharacters)
	c, _ := newOpenAICodexTurnStateTestContext(t, 7, "auto-replay")
	stageOpenAICodexTurnStateModel(c, "gpt-5.6-codex")

	headers := make(http.Header)
	headers.Set(openAICodexTurnStateHeader, "unverified-client-state")
	svc.applyConfiguredCodexTurnStateReplay(c, account, headers)
	require.Empty(t, headers.Get(openAICodexTurnStateHeader), "watched models must not replay an unverified inbound state")

	svc.observeOpenAICodexTurnState(context.Background(), nil, account, "gpt-5.6-codex", state)
	key := openAICodexAutoTurnStateKey(account, "gpt-5.6-codex")
	firstCapturedAt := svc.codexAutoTurnStates[key].CapturedAt
	headers = make(http.Header)
	svc.applyConfiguredCodexTurnStateReplay(c, account, headers)
	require.Equal(t, state, headers.Get(openAICodexTurnStateHeader))

	svc.observeOpenAICodexTurnState(context.Background(), nil, account, "gpt-5.6-codex", state)
	require.Equal(t, firstCapturedAt, svc.codexAutoTurnStates[key].CapturedAt, "the same state must not refresh its one-hour lifetime")
	require.Equal(t, firstCapturedAt, svc.codexAutoProbeTargets[key].MissingSince, "the 45-minute window starts at the last new state")

	otherAccountContext, _ := newOpenAICodexTurnStateTestContext(t, 7, "other-account")
	stageOpenAICodexTurnStateModel(otherAccountContext, "gpt-5.6-codex")
	otherAccountHeaders := make(http.Header)
	svc.applyConfiguredCodexTurnStateReplay(otherAccountContext, automaticTurnStateTestAccount(2), otherAccountHeaders)
	require.Empty(t, otherAccountHeaders.Get(openAICodexTurnStateHeader))
	sameUpstreamIdentity := automaticTurnStateTestAccount(2)
	sameUpstreamIdentity.Credentials["chatgpt_account_id"] = account.GetChatGPTAccountID()
	require.NotEqual(t, key, openAICodexAutoTurnStateKey(sameUpstreamIdentity, "gpt-5.6-codex"))
	require.Empty(t, svc.loadOpenAICodexAutoTurnState(context.Background(), sameUpstreamIdentity, "gpt-5.6-codex"))

	unwatchedContext, _ := newOpenAICodexTurnStateTestContext(t, 7, "unwatched-model")
	stageOpenAICodexTurnStateModel(unwatchedContext, "gpt-5.5-codex")
	unwatchedHeaders := make(http.Header)
	unwatchedHeaders.Set(openAICodexTurnStateHeader, "client-state")
	svc.applyConfiguredCodexTurnStateReplay(unwatchedContext, account, unwatchedHeaders)
	require.Equal(t, "client-state", unwatchedHeaders.Get(openAICodexTurnStateHeader))
}

func TestAutomaticCodexTurnStateReplayExpiresAfterOneHour(t *testing.T) {
	settings := automaticTurnStateTestSettings(t, "gpt-5.6-codex")
	svc := &OpenAIGatewayService{settingService: settings}
	account := automaticTurnStateTestAccount(3)
	key := openAICodexAutoTurnStateKey(account, "gpt-5.6-codex")
	svc.codexAutoTurnStates = map[string]openAICodexAutoTurnStateBinding{
		key: {
			State:      strings.Repeat("s", openAICodexDefaultTurnStateCharacters),
			CapturedAt: time.Now().Add(-openAICodexAutoTurnStateTTL - time.Minute),
			ExpiresAt:  time.Now().Add(-time.Minute),
		},
	}
	require.Empty(t, svc.loadOpenAICodexAutoTurnState(context.Background(), account, "gpt-5.6-codex"))
}

func TestAutomaticCodexTurnStateProbeStartsAfterFortyFiveMinutesWithoutHealthyState(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := automaticTurnStateTestAccount(4)
	started := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	svc.noteOpenAICodexAutoProbeObservation(account, "gpt-5.6-codex", false, started)

	key := openAICodexAutoTurnStateKey(account, "gpt-5.6-codex")
	target := svc.codexAutoProbeTargets[key]
	require.Equal(t, started.Add(45*time.Minute), target.NextProbeAt)

	healthyAt := started.Add(10 * time.Minute)
	svc.noteOpenAICodexAutoProbeObservation(account, "gpt-5.6-codex", true, healthyAt)
	require.True(t, svc.codexAutoProbeTargets[key].MissingSince.IsZero())
	require.Equal(t, healthyAt.Add(45*time.Minute), svc.codexAutoProbeTargets[key].NextProbeAt, "idle traffic still triggers the refresh probe")
	svc.noteOpenAICodexAutoProbeObservation(account, "gpt-5.6-codex", false, healthyAt.Add(20*time.Minute))
	target = svc.codexAutoProbeTargets[key]
	require.Equal(t, healthyAt.Add(45*time.Minute), target.NextProbeAt)
}

func TestAutomaticCodexTurnStateReplaySharesFreshStateAcrossNodes(t *testing.T) {
	cache := &stubOpenAIWSSharedCache{}
	account := automaticTurnStateTestAccount(10)
	state := strings.Repeat("n", openAICodexDefaultTurnStateCharacters)
	first := &OpenAIGatewayService{cache: cache, settingService: automaticTurnStateTestSettings(t, "gpt-5.6-codex")}
	second := &OpenAIGatewayService{cache: cache, settingService: automaticTurnStateTestSettings(t, "gpt-5.6-codex")}
	first.storeOpenAICodexAutoTurnState(context.Background(), account, "gpt-5.6-codex", state)
	sharedKey := openAIWSSessionTurnStateCacheKey(0, openAICodexAutoTurnStateKey(account, "gpt-5.6-codex"))
	require.Eventually(t, func() bool {
		_, found, err := cache.GetOpenAIWSState(context.Background(), sharedKey)
		return err == nil && found
	}, time.Second, time.Millisecond)
	require.Equal(t, state, second.loadOpenAICodexAutoTurnState(context.Background(), account, "gpt-5.6-codex"))
	require.Empty(t, second.loadOpenAICodexAutoTurnState(context.Background(), account, "gpt-5.5-codex"))
}

type automaticTurnStateUnavailableLock struct{}

func (automaticTurnStateUnavailableLock) TryAcquireLeaderLock(context.Context, string, string, time.Duration) (bool, error) {
	return false, errors.New("shared lock unavailable")
}

func (automaticTurnStateUnavailableLock) ReleaseLeaderLock(context.Context, string, string) error {
	return nil
}

func TestAutomaticCodexTurnStateProbeSkipsWhenSharedLockUnavailable(t *testing.T) {
	svc := &OpenAIGatewayService{
		accountRepo:        &stubOpenAIAccountRepo{},
		proxyRepo:          &automaticTurnStateProxyRepo{},
		codexAutoProbeLock: automaticTurnStateUnavailableLock{},
	}
	err := svc.probeOpenAICodexTurnState(context.Background(), openAICodexAutoProbeTarget{Key: "account-model"})
	require.ErrorContains(t, err, "shared lock unavailable")
}

type automaticTurnStateProxyRepo struct {
	ProxyRepository
	proxies []Proxy
}

func (r *automaticTurnStateProxyRepo) ListActive(context.Context) ([]Proxy, error) {
	return append([]Proxy(nil), r.proxies...), nil
}

type automaticTurnStateProbeDialer struct {
	mu       sync.Mutex
	states   []string
	proxies  []string
	payloads []map[string]any
}

func (d *automaticTurnStateProbeDialer) Dial(_ context.Context, _ string, _ http.Header, proxyURL string) (openAIWSClientConn, int, http.Header, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	index := len(d.proxies)
	d.proxies = append(d.proxies, proxyURL)
	state := ""
	if index < len(d.states) {
		state = d.states[index]
	}
	conn := &openAIWSCaptureConn{events: [][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_probe"}}`)}}
	return &automaticTurnStateProbeConn{openAIWSCaptureConn: conn, owner: d}, http.StatusSwitchingProtocols, http.Header{
		"X-Codex-Turn-State": []string{state},
	}, nil
}

type automaticTurnStateProbeConn struct {
	*openAIWSCaptureConn
	owner *automaticTurnStateProbeDialer
}

func (c *automaticTurnStateProbeConn) WriteJSON(ctx context.Context, value any) error {
	if err := c.openAIWSCaptureConn.WriteJSON(ctx, value); err != nil {
		return err
	}
	if payload, ok := value.(map[string]any); ok {
		c.owner.mu.Lock()
		c.owner.payloads = append(c.owner.payloads, cloneMapStringAny(payload))
		c.owner.mu.Unlock()
	}
	return nil
}

func TestAutomaticCodexTurnStateProbeRotatesActiveProxiesAndUsesZeroOutputPing(t *testing.T) {
	account := automaticTurnStateTestAccount(5)
	repo := &stubOpenAIAccountRepo{accounts: []Account{*account}}
	previousState := strings.Repeat("x", openAICodexDefaultTurnStateCharacters)
	healthyState := strings.Repeat("y", openAICodexDefaultTurnStateCharacters)
	dialer := &automaticTurnStateProbeDialer{states: []string{previousState, healthyState}}
	svc := &OpenAIGatewayService{
		accountRepo: repo,
		proxyRepo: &automaticTurnStateProxyRepo{proxies: []Proxy{
			{ID: 2, Protocol: "http", Host: "proxy-two.test", Port: 8080, Status: StatusActive},
			{ID: 1, Protocol: "http", Host: "proxy-one.test", Port: 8080, Status: StatusActive},
		}},
		cfg:                       &config.Config{},
		openaiWSPassthroughDialer: dialer,
	}
	svc.storeOpenAICodexAutoTurnState(context.Background(), account, "gpt-5.6-codex", previousState)
	missingSince := svc.openAICodexAutoTurnStateCapturedAt(account, "gpt-5.6-codex")

	err := svc.probeOpenAICodexTurnState(context.Background(), openAICodexAutoProbeTarget{
		Key:           openAICodexAutoTurnStateKey(account, "gpt-5.6-codex"),
		AccountID:     account.ID,
		Model:         "gpt-5.6-codex",
		MissingSince:  missingSince,
		LastHealthyAt: missingSince,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"http://proxy-one.test:8080", "http://proxy-two.test:8080"}, dialer.proxies)
	require.Len(t, dialer.payloads, 2)
	for _, payload := range dialer.payloads {
		require.Equal(t, false, payload["generate"])
		require.Equal(t, []any{}, payload["input"])
	}
	require.Equal(t, healthyState, svc.loadOpenAICodexAutoTurnState(context.Background(), account, "gpt-5.6-codex"))
}
