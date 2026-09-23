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
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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

func TestAutomaticCodexTurnStateLengthMismatchStartsRecoveryImmediately(t *testing.T) {
	settings := automaticTurnStateTestSettings(t, "gpt-5.6-codex")
	svc := &OpenAIGatewayService{settingService: settings}
	account := automaticTurnStateTestAccount(17)
	model := "gpt-5.6-codex"
	state := strings.Repeat("s", openAICodexDefaultTurnStateCharacters)
	svc.observeOpenAICodexTurnState(context.Background(), nil, account, model, state)

	beforeMismatch := time.Now()
	svc.observeOpenAICodexTurnState(context.Background(), nil, account, model, "wrong-length")
	afterMismatch := time.Now()
	target := svc.codexAutoProbeTargets[openAICodexAutoTurnStateKey(account, model)]
	require.True(t, target.Recovering)
	require.False(t, target.NextProbeAt.Before(beforeMismatch))
	require.False(t, target.NextProbeAt.After(afterMismatch))
}

func TestAutomaticCodexTurnStateProbeStartsImmediatelyWithoutHealthyState(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := automaticTurnStateTestAccount(4)
	started := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	svc.noteOpenAICodexAutoProbeObservation(account, "gpt-5.6-codex", false, started)

	key := openAICodexAutoTurnStateKey(account, "gpt-5.6-codex")
	target := svc.codexAutoProbeTargets[key]
	require.Equal(t, started, target.NextProbeAt)

	healthyAt := started.Add(10 * time.Minute)
	svc.noteOpenAICodexAutoProbeObservation(account, "gpt-5.6-codex", true, healthyAt)
	require.True(t, svc.codexAutoProbeTargets[key].MissingSince.IsZero())
	require.Equal(t, healthyAt.Add(45*time.Minute), svc.codexAutoProbeTargets[key].NextProbeAt, "idle traffic still triggers the refresh probe")
	svc.noteOpenAICodexAutoProbeObservation(account, "gpt-5.6-codex", false, healthyAt.Add(20*time.Minute))
	target = svc.codexAutoProbeTargets[key]
	require.Equal(t, healthyAt.Add(45*time.Minute), target.NextProbeAt)

	failedAt := healthyAt.Add(46 * time.Minute)
	svc.finishOpenAICodexAutoProbe(target, false, failedAt)
	require.Equal(t, failedAt.Add(openAICodexAutoProbeRetryInterval), svc.codexAutoProbeTargets[key].NextProbeAt)
}

func TestAutomaticCodexTurnStateMissingResponseKeepsProbeBackoff(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := automaticTurnStateTestAccount(6)
	now := time.Now()
	svc.noteOpenAICodexAutoProbeObservation(account, "gpt-5.6-codex", false, now)
	key := openAICodexAutoTurnStateKey(account, "gpt-5.6-codex")
	svc.finishOpenAICodexAutoProbe(svc.codexAutoProbeTargets[key], false, now)

	svc.noteOpenAICodexAutoProbeObservation(account, "gpt-5.6-codex", false, now.Add(10*time.Second))
	require.Equal(t, now.Add(openAICodexAutoProbeRetryInterval), svc.codexAutoProbeTargets[key].NextProbeAt)
}

func TestAutomaticCodexTurnStateProbeCompletionKeepsNewerResponseState(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := automaticTurnStateTestAccount(16)
	startedAt := time.Now().Add(-time.Minute)
	svc.noteOpenAICodexAutoProbeObservation(account, "gpt-5.6-codex", false, startedAt)
	key := openAICodexAutoTurnStateKey(account, "gpt-5.6-codex")
	probed := svc.codexAutoProbeTargets[key]
	probed.InFlight = true
	svc.codexAutoProbeTargets[key] = probed
	newStateAt := time.Now()
	svc.noteOpenAICodexAutoProbeNewState(account, "gpt-5.6-codex", strings.Repeat("n", openAICodexDefaultTurnStateCharacters), newStateAt)

	svc.finishOpenAICodexAutoProbe(probed, false, newStateAt.Add(time.Second))
	result := svc.codexAutoProbeTargets[key]
	require.False(t, result.InFlight)
	require.Equal(t, newStateAt, result.LastHealthyAt)
	require.Equal(t, newStateAt.Add(openAICodexAutoProbeStaleAfter), result.NextProbeAt)
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
		codexAutoProbeLock: automaticTurnStateUnavailableLock{},
	}
	err := svc.probeOpenAICodexTurnState(context.Background(), openAICodexAutoProbeTarget{Key: "account-model"})
	require.ErrorContains(t, err, "shared lock unavailable")
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

type automaticTurnStateBlockingProbeDialer struct {
	mu      sync.Mutex
	active  int
	peak    int
	calls   int
	entered chan struct{}
	release <-chan struct{}
	state   string
}

func (d *automaticTurnStateBlockingProbeDialer) Dial(ctx context.Context, _ string, _ http.Header, _ string) (openAIWSClientConn, int, http.Header, error) {
	d.mu.Lock()
	d.active++
	d.calls++
	if d.active > d.peak {
		d.peak = d.active
	}
	d.mu.Unlock()

	select {
	case d.entered <- struct{}{}:
	case <-ctx.Done():
		d.leave()
		return nil, 0, nil, ctx.Err()
	}
	select {
	case <-d.release:
	case <-ctx.Done():
		d.leave()
		return nil, 0, nil, ctx.Err()
	}
	d.leave()
	conn := &openAIWSCaptureConn{events: [][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_probe"}}`)}}
	return conn, http.StatusSwitchingProtocols, http.Header{"X-Codex-Turn-State": []string{d.state}}, nil
}

func (d *automaticTurnStateBlockingProbeDialer) leave() {
	d.mu.Lock()
	d.active--
	d.mu.Unlock()
}

func (d *automaticTurnStateBlockingProbeDialer) snapshot() (calls, peak int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls, d.peak
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

func TestAutomaticCodexTurnStateProbeRacesAccountRouteAndUsesZeroOutputPing(t *testing.T) {
	account := automaticTurnStateTestAccount(5)
	repo := &stubOpenAIAccountRepo{accounts: []Account{*account}}
	previousState := strings.Repeat("x", openAICodexDefaultTurnStateCharacters)
	healthyState := strings.Repeat("y", openAICodexDefaultTurnStateCharacters)
	dialer := &automaticTurnStateProbeDialer{states: []string{"wrong-one", healthyState, "wrong-three", "wrong-four"}}
	svc := &OpenAIGatewayService{
		accountRepo:               repo,
		settingService:            automaticTurnStateTestSettings(t, "gpt-5.6-codex"),
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
	require.Equal(t, []string{"", "", "", ""}, dialer.proxies)
	require.Len(t, dialer.payloads, openAICodexAutoProbeAttemptsPerTarget)
	for _, payload := range dialer.payloads {
		require.Equal(t, false, payload["generate"])
		require.Equal(t, []any{}, payload["input"])
		require.Equal(t, openai.CodexBaseInstructionsForModel("gpt-5.6-codex"), payload["instructions"])
	}
	require.Equal(t, healthyState, svc.loadOpenAICodexAutoTurnState(context.Background(), account, "gpt-5.6-codex"))
}

func TestAutomaticCodexTurnStateProbeRetriesUntilCorrectThenMonitorReplaces(t *testing.T) {
	account := automaticTurnStateTestAccount(18)
	model := "gpt-5.6-codex"
	probeState := strings.Repeat("p", openAICodexDefaultTurnStateCharacters)
	monitorState := strings.Repeat("m", openAICodexDefaultTurnStateCharacters)
	dialer := &automaticTurnStateProbeDialer{states: []string{
		"short-1", "short-2", "short-3", "short-4",
		"short-5", "short-6", "short-7", "short-8",
		probeState, "short-10", "short-11", "short-12",
	}}
	svc := &OpenAIGatewayService{
		accountRepo:               &stubOpenAIAccountRepo{accounts: []Account{*account}},
		settingService:            automaticTurnStateTestSettings(t, model),
		cfg:                       &config.Config{},
		openaiWSPassthroughDialer: dialer,
	}
	svc.noteOpenAICodexAutoProbeInvalidObservation(account, model, time.Now())
	key := openAICodexAutoTurnStateKey(account, model)

	for round := 0; round < 2; round++ {
		probed := svc.codexAutoProbeTargets[key]
		err := svc.probeOpenAICodexTurnState(context.Background(), probed)
		require.ErrorContains(t, err, "turn state has")
		finishedAt := time.Now()
		svc.finishOpenAICodexAutoProbe(probed, false, finishedAt)
		result := svc.codexAutoProbeTargets[key]
		require.True(t, result.Recovering)
		require.Equal(t, finishedAt.Add(openAICodexAutoProbeRetryInterval), result.NextProbeAt)
	}

	probed := svc.codexAutoProbeTargets[key]
	require.NoError(t, svc.probeOpenAICodexTurnState(context.Background(), probed))
	svc.finishOpenAICodexAutoProbe(probed, true, time.Now())
	require.Equal(t, probeState, svc.loadOpenAICodexAutoTurnState(context.Background(), account, model))
	require.False(t, svc.codexAutoProbeTargets[key].Recovering)

	svc.observeOpenAICodexTurnState(context.Background(), nil, account, model, monitorState)
	require.Equal(t, monitorState, svc.loadOpenAICodexAutoTurnState(context.Background(), account, model))
	require.False(t, svc.codexAutoProbeTargets[key].Recovering)
}

func TestAutomaticCodexTurnStateProbeAcceptsCorrectPreviousState(t *testing.T) {
	account := automaticTurnStateTestAccount(19)
	model := "gpt-5.6-codex"
	state := strings.Repeat("s", openAICodexDefaultTurnStateCharacters)
	dialer := &automaticTurnStateProbeDialer{states: []string{state}}
	svc := &OpenAIGatewayService{
		accountRepo:               &stubOpenAIAccountRepo{accounts: []Account{*account}},
		settingService:            automaticTurnStateTestSettings(t, model),
		cfg:                       &config.Config{},
		openaiWSPassthroughDialer: dialer,
	}
	svc.storeOpenAICodexAutoTurnState(context.Background(), account, model, state)
	capturedAt := svc.openAICodexAutoTurnStateCapturedAt(account, model)
	svc.noteOpenAICodexAutoProbeNewState(account, model, state, capturedAt)
	key := openAICodexAutoTurnStateKey(account, model)
	target := svc.codexAutoProbeTargets[key]
	target.Recovering = true
	target.MissingSince = capturedAt
	target.NextProbeAt = capturedAt
	svc.codexAutoProbeTargets[key] = target

	require.NoError(t, svc.probeOpenAICodexTurnState(context.Background(), target))
	require.Equal(t, state, svc.loadOpenAICodexAutoTurnState(context.Background(), account, model))
	require.False(t, svc.codexAutoProbeTargets[key].Recovering)
}

func TestAutomaticCodexTurnStateHTTPProbeUsesAccountRouteAndModelInstructions(t *testing.T) {
	account := automaticTurnStateTestAccount(25)
	model := "gpt-5.6-sol"
	account.Credentials["model_mapping"] = map[string]any{model: model}
	state := strings.Repeat("h", openAICodexDefaultTurnStateCharacters)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{http.CanonicalHeaderKey(openAICodexTurnStateHeader): []string{state}},
		Body:       io.NopCloser(strings.NewReader("")),
	}}
	svc := &OpenAIGatewayService{
		accountRepo:    &stubOpenAIAccountRepo{accounts: []Account{*account}},
		httpUpstream:   upstream,
		settingService: automaticTurnStateTestSettings(t, model),
		cfg:            &config.Config{},
	}

	result, err := svc.probeOpenAICodexTurnStateViaHTTP(context.Background(), account, model)
	require.NoError(t, err)
	require.Empty(t, upstream.lastProxyURL)
	require.NotNil(t, upstream.lastReq)
	require.Empty(t, upstream.lastReq.Header.Get(openAICodexTurnStateHeader))
	require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, openai.CodexBaseInstructionsForModel(model), gjson.GetBytes(upstream.lastBody, "instructions").String())
	require.Equal(t, state, result)
}

type turnStateProxyRepoForTest struct {
	ProxyRepository
	proxies []Proxy
}

func (r *turnStateProxyRepoForTest) ListActive(context.Context) ([]Proxy, error) {
	return append([]Proxy(nil), r.proxies...), nil
}

func TestAutomaticCodexTurnStateDedicatedProxyProbeUsesOnlyConfiguredProxy(t *testing.T) {
	account := automaticTurnStateTestAccount(26)
	model := "gpt-5.6-codex"
	proxyID := int64(42)
	state := strings.Repeat("d", openAICodexDefaultTurnStateCharacters)
	dialer := &automaticTurnStateProbeDialer{states: []string{state, "bad", "bad", "bad"}}
	settings := automaticTurnStateTestSettings(t, model)
	settingsValue := settings.CodexSimulationSettingsSnapshot(context.Background())
	settingsValue.TurnStateProxyProbeEnabled = true
	settingsValue.TurnStateProbeProxyID = &proxyID
	_, err := settings.SetCodexSimulationSettings(context.Background(), &settingsValue)
	require.NoError(t, err)
	svc := &OpenAIGatewayService{
		accountRepo:               &stubOpenAIAccountRepo{accounts: []Account{*account}},
		proxyRepo:                 &turnStateProxyRepoForTest{proxies: []Proxy{{ID: proxyID, Protocol: "http", Host: "dedicated.test", Port: 8080}, {ID: 99, Protocol: "http", Host: "other.test", Port: 8080}}},
		settingService:            settings,
		cfg:                       &config.Config{},
		openaiWSPassthroughDialer: dialer,
	}
	require.NoError(t, svc.probeOpenAICodexTurnState(context.Background(), openAICodexAutoProbeTarget{Key: openAICodexAutoTurnStateKey(account, model), AccountID: account.ID, Model: model}))
	require.Equal(t, []string{"http://dedicated.test:8080", "http://dedicated.test:8080", "http://dedicated.test:8080", "http://dedicated.test:8080"}, dialer.proxies)
}

func TestAutomaticCodexTurnStateProxyPoolProbeRequiresExplicitSwitch(t *testing.T) {
	account := automaticTurnStateTestAccount(27)
	model := "gpt-5.6-codex"
	state := strings.Repeat("p", openAICodexDefaultTurnStateCharacters)
	dialer := &automaticTurnStateProbeDialer{states: []string{"bad", state, "bad", "bad"}}
	settings := automaticTurnStateTestSettings(t, model)
	value := settings.CodexSimulationSettingsSnapshot(context.Background())
	value.TurnStateProxyProbeEnabled = true
	_, err := settings.SetCodexSimulationSettings(context.Background(), &value)
	require.NoError(t, err)
	svc := &OpenAIGatewayService{
		accountRepo:               &stubOpenAIAccountRepo{accounts: []Account{*account}},
		proxyRepo:                 &turnStateProxyRepoForTest{proxies: []Proxy{{ID: 1, Protocol: "http", Host: "pool-one.test", Port: 8080}, {ID: 2, Protocol: "http", Host: "pool-two.test", Port: 8080}}},
		settingService:            settings,
		cfg:                       &config.Config{},
		openaiWSPassthroughDialer: dialer,
	}
	require.NoError(t, svc.probeOpenAICodexTurnState(context.Background(), openAICodexAutoProbeTarget{Key: openAICodexAutoTurnStateKey(account, model), AccountID: account.ID, Model: model}))
	require.Len(t, dialer.proxies, 2)
	require.Contains(t, dialer.proxies, "http://pool-one.test:8080")
	require.Contains(t, dialer.proxies, "http://pool-two.test:8080")
}

func TestAutomaticCodexTurnStateCorrectResponseStopsRecoveryWithoutRefreshingStableState(t *testing.T) {
	settings := automaticTurnStateTestSettings(t, "gpt-5.6-codex")
	svc := &OpenAIGatewayService{settingService: settings}
	account := automaticTurnStateTestAccount(20)
	model := "gpt-5.6-codex"
	state := strings.Repeat("s", openAICodexDefaultTurnStateCharacters)
	svc.observeOpenAICodexTurnState(context.Background(), nil, account, model, state)
	key := openAICodexAutoTurnStateKey(account, model)
	firstCapturedAt := svc.codexAutoTurnStates[key].CapturedAt
	svc.noteOpenAICodexAutoProbeInvalidObservation(account, model, firstCapturedAt.Add(time.Minute))
	beforeRecoveryResponse := time.Now()
	svc.observeOpenAICodexTurnState(context.Background(), nil, account, model, state)
	afterRecoveryResponse := time.Now()
	require.False(t, svc.codexAutoProbeTargets[key].Recovering)
	require.False(t, svc.codexAutoProbeTargets[key].LastHealthyAt.Before(beforeRecoveryResponse))
	require.False(t, svc.codexAutoProbeTargets[key].LastHealthyAt.After(afterRecoveryResponse))
	require.Equal(t, firstCapturedAt, svc.codexAutoTurnStates[key].CapturedAt)
}

func TestAutomaticCodexTurnStateProbeDoesNotStoreAfterMonitoringDisabled(t *testing.T) {
	account := automaticTurnStateTestAccount(8)
	settings := automaticTurnStateTestSettings(t, "gpt-5.6-codex")
	release := make(chan struct{})
	dialer := &automaticTurnStateBlockingProbeDialer{
		entered: make(chan struct{}, openAICodexAutoProbeAttemptsPerTarget),
		release: release,
		state:   strings.Repeat("p", openAICodexDefaultTurnStateCharacters),
	}
	svc := &OpenAIGatewayService{
		accountRepo:               &stubOpenAIAccountRepo{accounts: []Account{*account}},
		settingService:            settings,
		cfg:                       &config.Config{},
		openaiWSPassthroughDialer: dialer,
	}
	result := make(chan error, 1)
	go func() {
		result <- svc.probeOpenAICodexTurnState(context.Background(), openAICodexAutoProbeTarget{
			Key: openAICodexAutoTurnStateKey(account, "gpt-5.6-codex"), AccountID: account.ID, Model: "gpt-5.6-codex",
		})
	}()
	select {
	case <-dialer.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not start")
	}
	_, err := settings.SetCodexSimulationSettings(context.Background(), &CodexSimulationSettings{ContinuationMode: "off", StateTTLSeconds: 60})
	require.NoError(t, err)
	close(release)
	select {
	case err := <-result:
		require.ErrorContains(t, err, "monitoring was disabled")
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not finish")
	}
	require.Empty(t, svc.loadOpenAICodexAutoTurnState(context.Background(), account, "gpt-5.6-codex"))
}

func TestAutomaticCodexTurnStateSweepRunsTargetsAndAccountRouteAttemptsInParallel(t *testing.T) {
	const extraTarget = 1
	require.Equal(t, 32, openAICodexAutoProbeMaxConcurrent)
	targetCount := openAICodexAutoProbeMaxConcurrent + extraTarget
	accounts := make([]Account, 0, targetCount)
	for id := 1; id <= targetCount; id++ {
		accounts = append(accounts, *automaticTurnStateTestAccount(int64(id)))
	}
	release := make(chan struct{})
	entered := make(chan struct{}, openAICodexAutoProbeMaxConcurrent*openAICodexAutoProbeAttemptsPerTarget)
	dialer := &automaticTurnStateBlockingProbeDialer{
		entered: entered,
		release: release,
		state:   strings.Repeat("p", openAICodexDefaultTurnStateCharacters),
	}
	svc := &OpenAIGatewayService{
		accountRepo:               &stubOpenAIAccountRepo{accounts: accounts},
		settingService:            automaticTurnStateTestSettings(t, "gpt-5.6-codex"),
		cfg:                       &config.Config{},
		openaiWSPassthroughDialer: dialer,
	}
	dueAt := time.Now().Add(-time.Minute)
	for i := range accounts {
		svc.noteOpenAICodexAutoProbeObservation(&accounts[i], "gpt-5.6-codex", false, dueAt)
	}

	svc.runOpenAICodexAutoProbeSweep(context.Background(), time.Now())
	wantConcurrentAttempts := openAICodexAutoProbeMaxConcurrent * openAICodexAutoProbeAttemptsPerTarget
	for range wantConcurrentAttempts {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %d concurrent account-route attempts", wantConcurrentAttempts)
		}
	}
	calls, peak := dialer.snapshot()
	require.Equal(t, wantConcurrentAttempts, calls)
	require.Equal(t, wantConcurrentAttempts, peak)

	svc.codexAutoProbeMu.Lock()
	inFlight := 0
	for _, target := range svc.codexAutoProbeTargets {
		if target.InFlight {
			inFlight++
		}
	}
	svc.codexAutoProbeMu.Unlock()
	require.Equal(t, openAICodexAutoProbeMaxConcurrent, inFlight)

	close(release)
	svc.codexAutoProbeWorkers.Wait()
	svc.runOpenAICodexAutoProbeSweep(context.Background(), time.Now())
	svc.codexAutoProbeWorkers.Wait()
	calls, _ = dialer.snapshot()
	require.GreaterOrEqual(t, calls, wantConcurrentAttempts+1)
	require.LessOrEqual(t, calls, targetCount*openAICodexAutoProbeAttemptsPerTarget)
}

func TestAutomaticCodexTurnStateShutdownWaitsForInFlightProbes(t *testing.T) {
	account := automaticTurnStateTestAccount(7)
	release := make(chan struct{})
	entered := make(chan struct{}, openAICodexAutoProbeAttemptsPerTarget)
	dialer := &automaticTurnStateBlockingProbeDialer{
		entered: entered,
		release: release,
		state:   strings.Repeat("p", openAICodexDefaultTurnStateCharacters),
	}
	svc := &OpenAIGatewayService{
		accountRepo:               &stubOpenAIAccountRepo{accounts: []Account{*account}},
		settingService:            automaticTurnStateTestSettings(t, "gpt-5.6-codex"),
		cfg:                       &config.Config{},
		openaiWSPassthroughDialer: dialer,
	}
	svc.StartCodexTurnStateAutoProbe(context.Background())
	svc.noteOpenAICodexAutoProbeObservation(account, "gpt-5.6-codex", false, time.Now())
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not start")
	}
	closed := make(chan struct{})
	go func() {
		svc.CloseOpenAIWSPool()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not wait for the probe")
	}
	key := openAICodexAutoTurnStateKey(account, "gpt-5.6-codex")
	svc.codexAutoProbeMu.Lock()
	inFlight := svc.codexAutoProbeTargets[key].InFlight
	svc.codexAutoProbeMu.Unlock()
	require.False(t, inFlight)
}
