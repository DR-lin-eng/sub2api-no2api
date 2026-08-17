package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const codexSimulationTestSecret = "codex-simulation-test-secret-32-bytes"

func TestCodexSimulationRootUsesWholeTupleAndKeepsCanonicalBodyImmutable(t *testing.T) {
	svc := newCodexSimulationTestService(true, codexContinuationOff)
	body := []byte(`{"model":"gpt-5.4","input":[{"type":"message","content":"hello"}]}`)
	groupID := int64(29)
	c := newCodexSimulationTestContext("/v1/responses")
	c.Request.Header.Set("thread-id", "thread-from-client")
	c.Request.Header.Set(CodexProjectIDHeader, "project-a")

	svc.PrepareCodexSimulationRequest(c, 17, &groupID, body)
	state, ok := codexSimulationRequestStateFromGin(c)
	require.True(t, ok)
	expected := codexSimulationHMAC(
		codexSimulationTestSecret,
		"root:v1",
		"api_key:17:group:29",
		"project-a",
		string(codexConversationThreadHeader),
		"thread-from-client",
	)
	require.Equal(t, hex.EncodeToString(expected[:]), state.root.rootKey)
	require.Equal(t, sha256.Sum256(body), state.root.canonicalBodyHash)

	originalRoot := state.root
	body[0] = '['
	svc.PrepareCodexSimulationRequest(c, 99, &groupID, body)
	require.Equal(t, originalRoot, state.root, "an established request root must be immutable")

	projectB := newCodexSimulationTestContext("/v1/responses")
	projectB.Request.Header.Set("thread-id", "thread-from-client")
	projectB.Request.Header.Set(CodexProjectIDHeader, "project-b")
	svc.PrepareCodexSimulationRequest(projectB, 17, &groupID, []byte(`{"model":"gpt-5.4"}`))
	projectBState, ok := codexSimulationRequestStateFromGin(projectB)
	require.True(t, ok)
	require.NotEqual(t, state.root.rootKey, projectBState.root.rootKey)
}

func TestCodexSimulationConversationSignalPriority(t *testing.T) {
	svc := newCodexSimulationTestService(true, codexContinuationOff)
	tests := []struct {
		name       string
		headers    map[string]string
		body       string
		wantSource codexConversationSignalSource
	}{
		{name: "thread", headers: map[string]string{"thread-id": "thread", "session-id": "session"}, body: `{"prompt_cache_key":"cache"}`, wantSource: codexConversationThreadHeader},
		{name: "session", headers: map[string]string{"session-id": "session", "session_id": "legacy"}, body: `{"prompt_cache_key":"cache"}`, wantSource: codexConversationSessionHeader},
		{name: "legacy", headers: map[string]string{"session_id": "legacy"}, body: `{"prompt_cache_key":"cache"}`, wantSource: codexConversationLegacyHeader},
		{name: "prompt cache", body: `{"prompt_cache_key":"cache","input":"hello"}`, wantSource: codexConversationPromptCache},
		{name: "content", body: `{"input":[{"role":"user","content":"hello"}]}`, wantSource: codexConversationContent},
		{name: "request local", body: `{}`, wantSource: codexConversationRequestLocal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := newCodexSimulationTestContext("/v1/responses")
			for key, value := range test.headers {
				c.Request.Header.Set(key, value)
			}
			svc.PrepareCodexSimulationRequest(c, 1, nil, []byte(test.body))
			state, ok := codexSimulationRequestStateFromGin(c)
			require.True(t, ok)
			require.Equal(t, test.wantSource, state.root.conversationSource)
		})
	}
}

func TestCodexSimulationPrincipalAndTurnMapping(t *testing.T) {
	svc := newCodexSimulationTestService(true, codexContinuationOff)
	c := newCodexSimulationTestContext("/v1/responses")
	c.Request.Header.Set("thread-id", "shared-thread")
	body := []byte(`{"model":"gpt-5.4","input":"hello"}`)
	svc.PrepareCodexSimulationRequest(c, 5, nil, body)

	first := openAIFingerprintAccount(10, map[string]any{codexFingerprintModeExtraKey: "full"})
	first.Credentials = map[string]any{"chatgpt_account_id": "upstream-principal"}
	secondRecord := openAIFingerprintAccount(11, map[string]any{codexFingerprintModeExtraKey: "full"})
	secondRecord.Credentials = map[string]any{"chatgpt_account_id": "upstream-principal"}
	otherPrincipal := openAIFingerprintAccount(12, map[string]any{codexFingerprintModeExtraKey: "full"})
	otherPrincipal.Credentials = map[string]any{"chatgpt_account_id": "other-principal"}

	_, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, first, body)
	require.NoError(t, err)
	firstAttempt, ok := codexSimulationAttemptFromGin(c)
	require.True(t, ok)
	firstIDs := *firstAttempt.fingerprint

	_, err = svc.PrepareCodexSimulationAttempt(context.Background(), c, first, body)
	require.NoError(t, err)
	retryAttempt, ok := codexSimulationAttemptFromGin(c)
	require.True(t, ok)
	require.Equal(t, firstIDs.turnID, retryAttempt.fingerprint.turnID, "same-principal retry must reuse the turn ID")

	_, err = svc.PrepareCodexSimulationAttempt(context.Background(), c, secondRecord, body)
	require.NoError(t, err)
	secondAttempt, ok := codexSimulationAttemptFromGin(c)
	require.True(t, ok)
	require.Equal(t, firstAttempt.principal.key, secondAttempt.principal.key)
	require.Equal(t, firstIDs, *secondAttempt.fingerprint, "local records for one upstream principal must share identity")

	_, err = svc.PrepareCodexSimulationAttempt(context.Background(), c, otherPrincipal, body)
	require.NoError(t, err)
	otherAttempt, ok := codexSimulationAttemptFromGin(c)
	require.True(t, ok)
	require.NotEqual(t, firstAttempt.principal.key, otherAttempt.principal.key)
	require.NotEqual(t, firstIDs.turnID, otherAttempt.fingerprint.turnID)
	require.NotEqual(t, firstIDs.sessionID, otherAttempt.fingerprint.sessionID)

	missingFirst := openAIFingerprintAccount(21, map[string]any{codexFingerprintModeExtraKey: "full"})
	missingSecond := openAIFingerprintAccount(22, map[string]any{codexFingerprintModeExtraKey: "full"})
	require.NotEqual(t, svc.resolveCodexSimulationPrincipal(missingFirst).key, svc.resolveCodexSimulationPrincipal(missingSecond).key)
	stats := svc.CodexPrincipalSourceStats()
	require.GreaterOrEqual(t, stats.ChatGPTAccountID, uint64(4))
	require.Equal(t, uint64(2), stats.LocalAccountID)

	require.Equal(t, firstIDs.sessionID, firstIDs.threadID)
	require.Equal(t, firstIDs.sessionID, firstIDs.promptCacheKey)
	require.Contains(t, firstIDs.profile.userAgent, runtimeProfileOSFragment())
}

func TestCodexFullSimulationStillRequiresAccountFullMode(t *testing.T) {
	svc := newCodexSimulationTestService(true, codexContinuationOff)
	body := []byte(`{"model":"gpt-5.4","input":"hello"}`)

	fullContext := newCodexSimulationTestContext("/v1/responses")
	fullAccount := openAIFingerprintAccount(25, map[string]any{codexFingerprintModeExtraKey: "full"})
	fullAccount.Credentials = map[string]any{"chatgpt_account_id": "full-principal"}
	_, err := svc.PrepareCodexSimulationAttempt(context.Background(), fullContext, fullAccount, body)
	require.NoError(t, err)
	fullAttempt, ok := codexSimulationAttemptFromGin(fullContext)
	require.True(t, ok)
	require.NotNil(t, fullAttempt.fingerprint)
	require.True(t, fullAttempt.fingerprint.fullSimulation)

	sessionContext := newCodexSimulationTestContext("/v1/responses")
	sessionAccount := openAIFingerprintAccount(26, map[string]any{codexFingerprintModeExtraKey: "session"})
	sessionAccount.Credentials = map[string]any{"chatgpt_account_id": "session-principal"}
	_, err = svc.PrepareCodexSimulationAttempt(context.Background(), sessionContext, sessionAccount, body)
	require.NoError(t, err)
	sessionAttempt, ok := codexSimulationAttemptFromGin(sessionContext)
	require.True(t, ok)
	require.Nil(t, sessionAttempt.fingerprint, "global A must not widen device/session accounts into full simulation")
}

func TestCodexFullSimulationRebuildsReservedIdentityConsistently(t *testing.T) {
	svc := newCodexSimulationTestService(true, codexContinuationOff)
	c := newCodexSimulationTestContext("/v1/responses")
	c.Request.Header.Set("thread-id", "conversation-a")
	c.Request.Header.Set(CodexProjectIDHeader, "project-a")
	body := []byte(`{"model":"gpt-5.4","input":"hello","client_metadata":{"preserved":true}}`)
	account := openAIFingerprintAccount(31, map[string]any{codexFingerprintModeExtraKey: "full"})
	account.Credentials = map[string]any{"chatgpt_account_id": "principal-a"}

	svc.PrepareCodexSimulationRequest(c, 7, nil, body)
	prepared, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, account, body)
	require.NoError(t, err)
	attempt, ok := codexSimulationAttemptFromGin(c)
	require.True(t, ok)
	ids := attempt.fingerprint
	require.NotNil(t, ids)

	headers := http.Header{
		"Originator":                 {"forged"},
		"Session-Id":                 {"forged-session"},
		"Session_Id":                 {"forged-session-alias"},
		"Thread-Id":                  {"forged-thread"},
		"User-Agent":                 {"forged-agent"},
		"Version":                    {"forged-version"},
		"X-Client-Request-Id":        {"forged-request"},
		"X-Codex-Installation-Id":    {"forged-installation"},
		"X-Codex-Window-Id":          {"forged-window"},
		"X-Sub2api-Codex-Project-Id": {"project-a"},
		"Openai-Beta":                {"responses=experimental"},
	}
	applyCodexFingerprintHeaders(headers, ids)
	require.Equal(t, ids.installationID, headers.Get("x-codex-installation-id"))
	require.Equal(t, ids.sessionID, headers.Get("session-id"))
	require.Equal(t, ids.threadID, headers.Get("thread-id"))
	require.Equal(t, ids.threadID, headers.Get("x-client-request-id"))
	require.Empty(t, headers.Get("session_id"))
	require.Empty(t, headers.Get(CodexProjectIDHeader))
	require.Equal(t, ids.profile.userAgent, headers.Get("user-agent"))
	require.Equal(t, ids.profile.originator, headers.Get("originator"))
	require.Equal(t, "responses=experimental", headers.Get("openai-beta"))

	rewritten, changed, err := applyCodexFingerprintClientMetadataToBody(prepared, ids)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, ids.promptCacheKey, gjson.GetBytes(rewritten, "prompt_cache_key").String())
	require.Equal(t, ids.installationID, gjson.GetBytes(rewritten, "client_metadata.x-codex-installation-id").String())
	require.Equal(t, ids.sessionID, gjson.GetBytes(rewritten, "client_metadata.session_id").String())
	require.Equal(t, ids.threadID, gjson.GetBytes(rewritten, "client_metadata.thread_id").String())
	require.Equal(t, ids.turnID, gjson.GetBytes(rewritten, "client_metadata.turn_id").String())

	request, err := svc.buildUpstreamRequestWithFingerprint(context.Background(), c, account, rewritten, "token", false, "", true, ids)
	require.NoError(t, err)
	require.Empty(t, request.Header.Get(CodexProjectIDHeader))
	require.Empty(t, request.Header.Get("session_id"))
	require.Equal(t, ids.threadID, request.Header.Get("x-client-request-id"))
	require.Equal(t, ids.profile.userAgent, request.Header.Get("user-agent"))
}

func TestCodexCompactGenerationAdvancesOnlyAfterSuccess(t *testing.T) {
	svc := newCodexSimulationTestService(true, codexContinuationOff)
	account := openAIFingerprintAccount(41, map[string]any{codexFingerprintModeExtraKey: "full"})
	account.Credentials = map[string]any{"chatgpt_account_id": "compact-principal"}
	body := []byte(`{"model":"gpt-5.4","input":"compact me"}`)

	prepare := func() (*gin.Context, *codexFingerprintIDs) {
		c := newCodexSimulationTestContext("/v1/responses/compact")
		c.Request.Header.Set("thread-id", "compact-thread")
		svc.PrepareCodexSimulationRequest(c, 8, nil, body)
		_, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, account, body)
		require.NoError(t, err)
		attempt, ok := codexSimulationAttemptFromGin(c)
		require.True(t, ok)
		return c, attempt.fingerprint
	}

	failedContext, beforeFailure := prepare()
	require.Zero(t, beforeFailure.generation)
	_, afterFailure := prepare()
	require.Zero(t, afterFailure.generation, "a failed Compact must not advance generation")

	svc.completeCodexSimulationSuccess(context.Background(), failedContext, account, "resp_compact", "")
	svc.completeCodexSimulationSuccess(context.Background(), failedContext, account, "resp_compact", "")
	_, afterSuccess := prepare()
	require.Equal(t, uint64(1), afterSuccess.generation, "success completion must be idempotent for one attempt")
	require.NotEqual(t, beforeFailure.windowID, afterSuccess.windowID)
}

func TestCodexSimulationDisabledUsesHardNoopRequestGate(t *testing.T) {
	svc := newCodexSimulationTestService(false, codexContinuationOff)
	c := newCodexSimulationTestContext("/v1/responses")
	body := []byte(`{"model":"gpt-5.4","input":"unchanged"}`)
	account := openAIFingerprintAccount(51, map[string]any{codexFingerprintModeExtraKey: "full"})
	account.Credentials = map[string]any{"chatgpt_account_id": "disabled-principal"}

	svc.PrepareCodexSimulationRequest(c, 9, nil, body)
	require.False(t, svc.CodexSimulationRequestEnabled(c))
	_, hasRequestState := codexSimulationRequestStateFromGin(c)
	require.False(t, hasRequestState)

	prepared, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, account, body)
	require.NoError(t, err)
	require.Equal(t, body, prepared)
	_, hasAttempt := codexSimulationAttemptFromGin(c)
	require.False(t, hasAttempt)
}

func TestCodexFullSimulationHeaderRefreshDoesNotWidenLegacyFingerprintModes(t *testing.T) {
	legacyDirectHeaders := make(http.Header)
	headers := http.Header{"X-Codex-Installation-Id": {"client-installation"}}
	legacyIDs := &codexFingerprintIDs{
		mode:           codexFingerprintSession,
		installationID: "legacy-installation",
		sessionID:      "legacy-session",
		threadID:       "legacy-thread",
		windowID:       "legacy-window",
	}
	applyCodexFingerprintWSHeaders(legacyDirectHeaders, legacyIDs)
	require.Empty(t, legacyDirectHeaders.Get("x-codex-turn-metadata"))

	applyCodexFullSimulationWSHeaders(headers, legacyIDs)

	require.Equal(t, "client-installation", headers.Get("x-codex-installation-id"))
	require.Empty(t, headers.Get("session-id"))
	require.Empty(t, headers.Get("thread-id"))
}

func TestForceDisableEndsExistingCodexSimulationAtNextWSTurn(t *testing.T) {
	repo := newCodexSimulationSettingRepo()
	cfg := &config.Config{}
	cfg.Gateway.CodexSimulation = config.GatewayCodexSimulationConfig{
		FullSimulationEnabled: true,
		IdentitySecret:        codexSimulationTestSecret,
		ContinuationMode:      "off",
		StateTTLSeconds:       7 * 24 * 60 * 60,
	}
	settingService := NewSettingService(repo, cfg)
	gateway := &OpenAIGatewayService{
		cfg:                cfg,
		settingService:     settingService,
		openaiWSStateStore: NewOpenAIWSStateStore(nil),
	}
	c := newCodexSimulationTestContext("/v1/responses")
	body := []byte(`{"model":"gpt-5.4","input":"turn"}`)
	account := openAIFingerprintAccount(52, map[string]any{codexFingerprintModeExtraKey: "full"})
	account.Credentials = map[string]any{"chatgpt_account_id": "active-principal"}

	gateway.PrepareCodexSimulationRequest(c, 10, nil, body)
	_, err := gateway.prepareCodexSimulationAttemptForTurn(context.Background(), c, account, body, 1)
	require.NoError(t, err)
	require.True(t, gateway.CodexSimulationRequestEnabled(c))

	_, err = settingService.ForceDisableCodexSimulationSettings(context.Background())
	require.NoError(t, err)
	_, err = gateway.prepareCodexSimulationAttemptForTurn(context.Background(), c, account, body, 2)
	var terminalErr *CodexContinuationTerminalError
	require.ErrorAs(t, err, &terminalErr)
	require.Contains(t, terminalErr.Message, "reconnect")
}

func newCodexSimulationTestService(full bool, mode codexContinuationMode) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.CodexSimulation = config.GatewayCodexSimulationConfig{
		FullSimulationEnabled: full,
		IdentitySecret:        codexSimulationTestSecret,
		ContinuationMode:      string(mode),
		StateTTLSeconds:       7 * 24 * 60 * 60,
	}
	return &OpenAIGatewayService{
		cfg:                cfg,
		openaiWSStateStore: NewOpenAIWSStateStore(nil),
	}
}

func newCodexSimulationTestContext(path string) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	return c
}

func runtimeProfileOSFragment() string {
	switch runtime.GOOS {
	case "darwin":
		return "Mac OS"
	case "windows":
		return "Windows"
	default:
		return "Ubuntu"
	}
}
