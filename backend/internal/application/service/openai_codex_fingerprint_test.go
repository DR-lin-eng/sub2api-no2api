package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAccountGetCodexFingerprintModeDefaultsOff(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    codexFingerprintMode
	}{
		{name: "nil", want: codexFingerprintOff},
		{name: "api key ignored", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{codexFingerprintModeExtraKey: "full"}}, want: codexFingerprintOff},
		{name: "missing remains opt out", account: openAIFingerprintAccount(1, nil), want: codexFingerprintOff},
		{name: "invalid remains opt out", account: openAIFingerprintAccount(1, map[string]any{codexFingerprintModeExtraKey: "unexpected"}), want: codexFingerprintOff},
		{name: "explicit off", account: openAIFingerprintAccount(1, map[string]any{codexFingerprintModeExtraKey: "off"}), want: codexFingerprintOff},
		{name: "device", account: openAIFingerprintAccount(1, map[string]any{codexFingerprintModeExtraKey: "device"}), want: codexFingerprintDevice},
		{name: "session", account: openAIFingerprintAccount(1, map[string]any{codexFingerprintModeExtraKey: "SESSION"}), want: codexFingerprintSession},
		{name: "full", account: openAIFingerprintAccount(1, map[string]any{codexFingerprintModeExtraKey: " full "}), want: codexFingerprintFull},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.account.GetCodexFingerprintMode())
		})
	}
}

func TestAccountCodexThinkingTagNormalizationIsAPIKeyOptIn(t *testing.T) {
	key := CodexThinkingTagNormalizationExtraKey
	require.False(t, (&Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: map[string]any{key: true}}).IsCodexThinkingTagNormalizationEnabled())
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{key: "true"}}).IsCodexThinkingTagNormalizationEnabled())
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{key: true}}).IsCodexThinkingTagNormalizationEnabled())
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{key: "true"}}).IsCodexThinkingTagNormalizationEnabled())
	require.True(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{key: true}}).IsCodexThinkingTagNormalizationEnabled())
}

func TestResolveCodexFingerprintIDsModes(t *testing.T) {
	account := openAIFingerprintAccount(42, map[string]any{"openai_device_id": "device-from-account"})

	require.Nil(t, resolveCodexFingerprintIDs(account, "client-session", codexFingerprintOff))

	device := resolveCodexFingerprintIDs(account, "client-session", codexFingerprintDevice)
	require.Equal(t, "device-from-account", device.installationID)
	require.Empty(t, device.sessionID)
	require.Empty(t, device.threadID)

	sessionA := resolveCodexFingerprintIDs(account, "client-session", codexFingerprintSession)
	sessionB := resolveCodexFingerprintIDs(account, "client-session", codexFingerprintSession)
	require.Equal(t, sessionA.installationID, sessionB.installationID)
	require.Equal(t, sessionA.sessionID, sessionB.sessionID)
	require.Equal(t, sessionA.threadID, sessionB.threadID)
	require.NotEqual(t, sessionA.turnID, sessionB.turnID)
	require.Equal(t, sessionA.threadID+":0", sessionA.windowID)
	require.NotEqual(t, sessionA.threadID, resolveCodexFingerprintIDs(account, "other-session", codexFingerprintSession).threadID)

	full := resolveCodexFingerprintIDs(account, "client-session", codexFingerprintFull)
	require.Equal(t, full.sessionID, full.threadID)
	for _, value := range []string{sessionA.sessionID, sessionA.threadID, sessionA.turnID, full.sessionID} {
		_, err := uuid.Parse(value)
		require.NoError(t, err)
	}
}

func TestCodexFingerprintStableSeedPrefersUpstreamAccountIdentity(t *testing.T) {
	first := openAIFingerprintAccount(42, nil)
	first.Credentials = map[string]any{"chatgpt_account_id": "upstream-account"}
	second := openAIFingerprintAccount(99, nil)
	second.Credentials = map[string]any{"chatgpt_account_id": "upstream-account"}

	firstIDs := resolveCodexFingerprintIDs(first, "client-session", codexFingerprintSession)
	secondIDs := resolveCodexFingerprintIDs(second, "client-session", codexFingerprintSession)
	require.Equal(t, firstIDs.installationID, secondIDs.installationID)
	require.Equal(t, firstIDs.sessionID, secondIDs.sessionID)
	require.Equal(t, firstIDs.threadID, secondIDs.threadID)

	second.Credentials["chatgpt_account_id"] = "other-account"
	otherIDs := resolveCodexFingerprintIDs(second, "client-session", codexFingerprintSession)
	require.NotEqual(t, firstIDs.installationID, otherIDs.installationID)
	require.NotEqual(t, firstIDs.sessionID, otherIDs.sessionID)
}

func TestCodexFingerprintHeadersAndMetadataStayConsistent(t *testing.T) {
	account := openAIFingerprintAccount(7, nil)
	ids := resolveCodexFingerprintIDs(account, "client-session", codexFingerprintSession)
	headers := http.Header{
		"X-Codex-Turn-Metadata": {`{"sandbox":"workspace-write","turn_id":"client-turn"}`},
		"X-Client-Request-Id":   {"client-request"},
	}
	body := map[string]any{
		"client_metadata": map[string]any{
			"preserved":             true,
			"x-codex-turn-metadata": `{"sandbox":"workspace-write","turn_id":"client-turn"}`,
		},
	}

	applyCodexFingerprintHeaders(headers, ids)
	require.True(t, applyCodexFingerprintClientMetadata(body, ids))
	require.Equal(t, ids.turnID, headers.Get("X-Client-Request-ID"), "request tracing must remain unique per request")
	require.Equal(t, ids.installationID, headers.Get("X-Codex-Installation-ID"))
	require.Equal(t, ids.sessionID, headers.Get("Session-Id"))
	require.Equal(t, ids.threadID, headers.Get("Thread-Id"))

	clientMetadata, ok := body["client_metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, clientMetadata["preserved"])
	require.Equal(t, ids.installationID, clientMetadata["x-codex-installation-id"])
	require.Equal(t, ids.turnID, clientMetadata["turn_id"])
	require.Equal(t, ids.threadID, clientMetadata["thread_id"])

	headerMetadata := decodeFingerprintMetadata(t, headers.Get("X-Codex-Turn-Metadata"))
	rawBodyMetadata, ok := clientMetadata["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	bodyMetadata := decodeFingerprintMetadata(t, rawBodyMetadata)
	require.Equal(t, "workspace-write", headerMetadata["sandbox"])
	require.Equal(t, headerMetadata["turn_id"], bodyMetadata["turn_id"])
	require.Equal(t, headerMetadata["turn_started_at_unix_ms"], bodyMetadata["turn_started_at_unix_ms"])
}

func TestCodexFingerprintDeviceModeDoesNotInventSessionFields(t *testing.T) {
	ids := resolveCodexFingerprintIDs(openAIFingerprintAccount(8, nil), "client-session", codexFingerprintDevice)
	headers := http.Header{
		"Session-Id":            {"client-session"},
		"X-Client-Request-Id":   {"client-request"},
		"X-Codex-Turn-Metadata": {`{"session_id":"client-session"}`},
	}
	body := map[string]any{"client_metadata": map[string]any{"session_id": "client-session"}}

	applyCodexFingerprintHeaders(headers, ids)
	require.True(t, applyCodexFingerprintClientMetadata(body, ids))
	require.Equal(t, "client-session", headers.Get("Session-Id"))
	require.Equal(t, "client-request", headers.Get("X-Client-Request-Id"))
	clientMetadata, ok := body["client_metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "client-session", clientMetadata["session_id"])
	require.Equal(t, "client-session", decodeFingerprintMetadata(t, headers.Get("X-Codex-Turn-Metadata"))["session_id"])

	wsHeaders := headers.Clone()
	applyCodexFingerprintWSHeaders(wsHeaders, ids)
	require.Equal(t, ids.installationID, wsHeaders.Get("X-Codex-Installation-ID"))
	require.Equal(t, "client-session", decodeFingerprintMetadata(t, wsHeaders.Get("X-Codex-Turn-Metadata"))["session_id"])
}

func TestOpenAIBuildUpstreamRequestFingerprintIsAttemptLocal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.0")
	c.Request.Header.Set("Session-Id", "client-session")
	c.Request.Header.Set("X-Codex-Installation-ID", "client-installation")

	service := &OpenAIGatewayService{}
	enabledAccount := openAIFingerprintAccount(10, map[string]any{codexFingerprintModeExtraKey: "session"})
	enabledAccount.Credentials = map[string]any{"chatgpt_account_id": "enabled"}
	disabledAccount := openAIFingerprintAccount(11, nil)
	disabledAccount.Credentials = map[string]any{"chatgpt_account_id": "disabled"}
	enabledIDs := resolveCodexFingerprintIDsFromRequest(enabledAccount, c.Request.Header)

	enabledRequest, err := service.buildUpstreamRequestWithFingerprint(context.Background(), c, enabledAccount, []byte(`{"model":"gpt-5"}`), "token", false, "", true, enabledIDs)
	require.NoError(t, err)
	require.Equal(t, enabledIDs.installationID, enabledRequest.Header.Get("X-Codex-Installation-ID"))
	require.Equal(t, enabledIDs.sessionID, enabledRequest.Header.Get("Session-Id"))

	disabledRequest, err := service.buildUpstreamRequestWithFingerprint(context.Background(), c, disabledAccount, []byte(`{"model":"gpt-5"}`), "token", false, "", true, nil)
	require.NoError(t, err)
	require.Equal(t, "client-installation", disabledRequest.Header.Get("X-Codex-Installation-ID"))
	require.NotEqual(t, enabledIDs.sessionID, disabledRequest.Header.Get("Session-Id"))
}

func TestCodexFullSimulationProjectsCustomMetadataIntoBoundedExtra(t *testing.T) {
	ids := &codexFingerprintIDs{
		mode:           codexFingerprintFull,
		fullSimulation: true,
		installationID: "installation",
		sessionID:      "session",
		threadID:       "thread",
		turnID:         "turn",
		windowID:       "thread:0",
	}
	body := []byte(`{"model":"gpt-5.5","client_metadata":{"valid_key":"value","bad/key":"drop","too_large":"` + strings.Repeat("x", codexExtraMetadataMaxValueBytes+1) + `"}}`)
	rewritten, changed, err := applyCodexFingerprintClientMetadataToBody(body, ids)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(rewritten, "client_metadata.valid_key").Exists())
	require.False(t, gjson.GetBytes(rewritten, "client_metadata.bad/key").Exists())
	turnMetadata := decodeFingerprintMetadata(t, gjson.GetBytes(rewritten, "client_metadata.x-codex-turn-metadata").String())
	require.Equal(t, "value", turnMetadata["valid_key"])
	require.NotContains(t, turnMetadata, "bad/key")
	require.NotContains(t, turnMetadata, "too_large")
}

func TestCodexFullSimulationRewritesParentAndSubagentCompatibilityFields(t *testing.T) {
	ids := &codexFingerprintIDs{
		mode:           codexFingerprintFull,
		fullSimulation: true,
		identitySecret: "identity-secret-for-parent-test",
		principalKey:   "principal-key",
		installationID: "installation",
		sessionID:      "session",
		threadID:       "thread",
		turnID:         "turn",
		windowID:       "thread:0",
	}
	headers := http.Header{
		"X-Codex-Parent-Thread-Id": {"forged-parent"},
		"X-Openai-Subagent":        {"forged-subagent"},
		"X-Codex-Turn-Metadata":    {`{"parent_thread_id":"forged-parent","subagent_kind":"review"}`},
	}
	applyCodexFingerprintHeaders(headers, ids)
	rewritten := decodeFingerprintMetadata(t, headers.Get("x-codex-turn-metadata"))
	require.NotEqual(t, "forged-parent", rewritten["parent_thread_id"])
	require.Equal(t, rewritten["parent_thread_id"], headers.Get("x-codex-parent-thread-id"))
	require.Equal(t, "review", headers.Get("x-openai-subagent"))
}

func TestOpenAIPassthroughFingerprintBodyAndHeadersStayConsistent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.0")
	c.Request.Header.Set("Session-Id", "client-session")
	c.Request.Header.Set("X-Client-Request-Id", "client-request")
	c.Request.Header.Set("X-Codex-Turn-Metadata", `{"sandbox":"workspace-write","turn_id":"client-turn"}`)

	account := openAIFingerprintAccount(20, map[string]any{codexFingerprintModeExtraKey: "session"})
	account.Credentials = map[string]any{"chatgpt_account_id": "passthrough-enabled"}
	ids := resolveCodexFingerprintIDsFromRequest(account, c.Request.Header)
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"message","content":"<tag>&value</tag>"}],"client_metadata":{"preserved":true,"x-codex-turn-metadata":"{\"sandbox\":\"workspace-write\",\"turn_id\":\"client-turn\"}"}}`)

	rewrittenBody, changed, err := applyCodexFingerprintClientMetadataToBody(body, ids)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, string(rewrittenBody), `<tag>&value</tag>`, "large/raw input fields must not be JSON re-encoded")

	service := &OpenAIGatewayService{}
	request, err := service.buildUpstreamRequestOpenAIPassthroughWithFingerprint(context.Background(), c, account, rewrittenBody, "token", ids)
	require.NoError(t, err)
	require.Equal(t, ids.installationID, request.Header.Get("X-Codex-Installation-ID"))
	require.Equal(t, ids.sessionID, request.Header.Get("Session-Id"))
	require.Equal(t, ids.turnID, request.Header.Get("X-Client-Request-ID"))

	var decodedBody map[string]any
	require.NoError(t, json.Unmarshal(rewrittenBody, &decodedBody))
	clientMetadata, ok := decodedBody["client_metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, clientMetadata["preserved"])
	require.Equal(t, request.Header.Get("X-Codex-Installation-ID"), clientMetadata["x-codex-installation-id"])
	require.Equal(t, request.Header.Get("Session-Id"), clientMetadata["session_id"])
	require.Equal(t, request.Header.Get("Thread-Id"), clientMetadata["thread_id"])
	require.Equal(t, request.Header.Get("X-Client-Request-ID"), clientMetadata["turn_id"])

	headerMetadata := decodeFingerprintMetadata(t, request.Header.Get("X-Codex-Turn-Metadata"))
	rawBodyMetadata, ok := clientMetadata["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	bodyMetadata := decodeFingerprintMetadata(t, rawBodyMetadata)
	require.Equal(t, headerMetadata["turn_id"], bodyMetadata["turn_id"])
	require.Equal(t, headerMetadata["turn_started_at_unix_ms"], bodyMetadata["turn_started_at_unix_ms"])
}

func TestOpenAIPassthroughFingerprintDisabledAndCompactRemainCompatible(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &OpenAIGatewayService{}
	body := []byte(`{"model":"gpt-5.5","client_metadata":{"session_id":"client-body-session"}}`)

	newContext := func(path string) *gin.Context {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, path, nil)
		c.Request.Header.Set("Session-Id", "client-header-session")
		c.Request.Header.Set("X-Codex-Installation-ID", "client-installation")
		return c
	}

	disabledAccount := openAIFingerprintAccount(21, nil)
	disabledAccount.Credentials = map[string]any{"chatgpt_account_id": "passthrough-disabled"}
	disabledContext := newContext("/v1/responses")
	disabledBody, changed, err := applyCodexFingerprintClientMetadataToBody(body, resolveCodexFingerprintIDsFromRequest(disabledAccount, disabledContext.Request.Header))
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, disabledBody)
	disabledRequest, err := service.buildUpstreamRequestOpenAIPassthroughWithFingerprint(context.Background(), disabledContext, disabledAccount, disabledBody, "token", nil)
	require.NoError(t, err)
	require.Equal(t, "client-installation", disabledRequest.Header.Get("X-Codex-Installation-ID"))
	require.NotEqual(t, resolveConvergedSessionID(disabledAccount), disabledRequest.Header.Get("Session-Id"))

	compactAccount := openAIFingerprintAccount(22, map[string]any{codexFingerprintModeExtraKey: "session"})
	compactAccount.Credentials = map[string]any{"chatgpt_account_id": "passthrough-compact"}
	compactContext := newContext("/v1/responses/compact")
	compactIDs := resolveCodexFingerprintIDsFromRequest(compactAccount, compactContext.Request.Header)
	compactRequest, err := service.buildUpstreamRequestOpenAIPassthroughWithFingerprint(context.Background(), compactContext, compactAccount, body, "token", compactIDs)
	require.NoError(t, err)
	require.Equal(t, "client-installation", compactRequest.Header.Get("X-Codex-Installation-ID"))
	require.NotEqual(t, compactIDs.sessionID, compactRequest.Header.Get("Session-Id"))
}

func TestCodexFingerprintWSCompatibilityKeyIsStableAndInternal(t *testing.T) {
	account := openAIFingerprintAccount(12, nil)
	first := resolveCodexFingerprintIDs(account, "client-session", codexFingerprintSession)
	second := resolveCodexFingerprintIDs(account, "client-session", codexFingerprintSession)
	other := resolveCodexFingerprintIDs(account, "other-session", codexFingerprintSession)

	firstHeaders := make(http.Header)
	secondHeaders := make(http.Header)
	otherHeaders := make(http.Header)
	applyCodexFingerprintWSHeaders(firstHeaders, first)
	applyCodexFingerprintWSHeaders(secondHeaders, second)
	applyCodexFingerprintWSHeaders(otherHeaders, other)
	require.Equal(t, normalizeOpenAIWSHandshakeCompatibility(firstHeaders), normalizeOpenAIWSHandshakeCompatibility(secondHeaders))
	require.NotEqual(t, normalizeOpenAIWSHandshakeCompatibility(firstHeaders), normalizeOpenAIWSHandshakeCompatibility(otherHeaders))

	pool := &openAIWSConnPool{clientDialer: &fingerprintHeaderCaptureDialer{}}
	_, err := pool.dialConn(context.Background(), openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
		Headers: firstHeaders,
	})
	require.Error(t, err)
	dialer, ok := pool.clientDialer.(*fingerprintHeaderCaptureDialer)
	require.True(t, ok)
	captured := dialer.headers
	require.Empty(t, captured.Get(codexFingerprintWSKeyHeader))
	require.Equal(t, first.installationID, captured.Get("X-Codex-Installation-ID"))
}

func BenchmarkCodexFingerprintModeOff(b *testing.B) {
	account := openAIFingerprintAccount(13, nil)
	headers := http.Header{"Session-Id": {"client-session"}}
	b.ReportAllocs()
	for range b.N {
		if resolveCodexFingerprintIDsFromRequest(account, headers) != nil {
			b.Fatal("off mode returned a plan")
		}
	}
}

func BenchmarkCodexFingerprintSessionPlan(b *testing.B) {
	account := openAIFingerprintAccount(13, map[string]any{codexFingerprintModeExtraKey: "session"})
	headers := http.Header{"Session-Id": {"client-session"}}
	b.ReportAllocs()
	for range b.N {
		ids := resolveCodexFingerprintIDsFromRequest(account, headers)
		if ids == nil {
			b.Fatal("session mode returned no plan")
		}
	}
}

func BenchmarkCodexFingerprintPassthroughBodyRewrite(b *testing.B) {
	account := openAIFingerprintAccount(14, map[string]any{codexFingerprintModeExtraKey: "session"})
	ids := resolveCodexFingerprintIDs(account, "client-session", codexFingerprintSession)
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"message","content":"hello"}],"client_metadata":{"preserved":true}}`)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	for range b.N {
		rewritten, changed, err := applyCodexFingerprintClientMetadataToBody(body, ids)
		if err != nil || !changed || len(rewritten) == 0 {
			b.Fatalf("rewrite failed: changed=%v err=%v", changed, err)
		}
	}
}

func openAIFingerprintAccount(id int64, extra map[string]any) *Account {
	return &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}
}

func decodeFingerprintMetadata(t *testing.T, raw string) map[string]any {
	t.Helper()
	var value map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &value))
	return value
}

type fingerprintHeaderCaptureDialer struct {
	headers http.Header
}

func (d *fingerprintHeaderCaptureDialer) Dial(_ context.Context, _ string, headers http.Header, _ string) (openAIWSClientConn, int, http.Header, error) {
	d.headers = cloneHeader(headers)
	return nil, http.StatusBadGateway, nil, context.Canceled
}
