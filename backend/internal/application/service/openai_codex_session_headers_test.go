package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newCodexSessionHeaderTestContext(t *testing.T, path string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	c.Set("api_key", &APIKey{ID: 41})
	return c
}

func TestBuildUpstreamRequestSynthesizesIsolatedCodexSessionHeaders(t *testing.T) {
	c := newCodexSessionHeaderTestContext(t, "/v1/responses")
	c.Request.Header.Set("session-id", "client-session")
	c.Request.Header.Set("thread-id", "client-thread")
	c.Request.Header.Set("x-client-request-id", "client-request")
	body := []byte(`{"model":"gpt-5.5","prompt_cache_key":"cache-key","input":"hello"}`)
	account := &Account{
		ID:          12,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "chatgpt-account", "access_token": "token"},
	}

	req, err := (&OpenAIGatewayService{}).buildUpstreamRequest(
		context.Background(), c, account, body, "token", true, "cache-key", true,
	)
	require.NoError(t, err)
	require.NotEmpty(t, req.Header.Get("session-id"))
	require.NotEmpty(t, req.Header.Get("thread-id"))
	require.Equal(t, req.Header.Get("thread-id"), req.Header.Get("x-client-request-id"))
	require.NotEqual(t, "client-session", req.Header.Get("session-id"))
	require.NotEqual(t, "client-thread", req.Header.Get("thread-id"))
	require.NotEqual(t, "client-request", req.Header.Get("x-client-request-id"))
	require.NotEmpty(t, req.Header.Get("session_id"), "legacy gateway session projection remains available")
}

func TestCodexSessionHeadersAreAccountScoped(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","prompt_cache_key":"same-cache","input":"hello"}`)
	accountA := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "acct-a"}}
	accountB := &Account{ID: 13, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "acct-b"}}
	c := newCodexSessionHeaderTestContext(t, "/v1/responses")

	first, err := (&OpenAIGatewayService{}).buildUpstreamRequest(context.Background(), c, accountA, body, "token", true, "same-cache", true)
	require.NoError(t, err)
	second, err := (&OpenAIGatewayService{}).buildUpstreamRequest(context.Background(), c, accountB, body, "token", true, "same-cache", true)
	require.NoError(t, err)
	require.NotEqual(t, first.Header.Get("session-id"), second.Header.Get("session-id"))
	require.NotEqual(t, first.Header.Get("thread-id"), second.Header.Get("thread-id"))
}

func TestBuildOpenAIWSHeadersSynthesizesCodexSessionHeaders(t *testing.T) {
	c := newCodexSessionHeaderTestContext(t, "/v1/responses")
	c.Request.Header.Set("thread-id", "ws-thread")
	account := &Account{
		ID:          21,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "ws-account"},
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	headers, _, err := svc.buildOpenAIWSHeaders(
		context.Background(), c, account, "token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		true, "", "", "cache-key", "gpt-5.5", "",
	)
	require.NoError(t, err)
	require.NotEmpty(t, headers.Get("session-id"))
	require.NotEmpty(t, headers.Get("thread-id"))
	require.Equal(t, headers.Get("thread-id"), headers.Get("x-client-request-id"))
	require.NotEqual(t, "ws-thread", headers.Get("thread-id"))
}

func TestBuildOpenAIWSHeadersUsesClientMetadataSessionSignals(t *testing.T) {
	c := newCodexSessionHeaderTestContext(t, "/v1/responses")
	stageCodexOutboundSessionBody(c, []byte(`{"model":"gpt-5.5","client_metadata":{"session_id":"body-session","thread_id":"body-thread"}}`))
	account := &Account{ID: 22, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "body-account"}}
	svc := &OpenAIGatewayService{}
	headers, _, err := svc.buildOpenAIWSHeaders(
		context.Background(), c, account, "token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		true, "", "", "", "gpt-5.5", "",
	)
	require.NoError(t, err)
	require.NotEmpty(t, headers.Get("session-id"))
	require.NotEmpty(t, headers.Get("thread-id"))
	require.NotEqual(t, "body-session", headers.Get("session-id"))
	require.NotEqual(t, "body-thread", headers.Get("thread-id"))
}

func TestRewriteCodexOutboundSessionMetadataMatchesHeaderProjection(t *testing.T) {
	c := newCodexSessionHeaderTestContext(t, "/v1/responses")
	body := []byte(`{"model":"gpt-5.5","client_metadata":{"session_id":"raw-session","thread_id":"raw-thread","preserved":"yes"}}`)
	account := &Account{ID: 33, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "metadata-account"}}
	ids := resolveCodexOutboundSessionIDs(c, account, body, "")

	rewritten, err := rewriteCodexOutboundSessionMetadata(body, ids)
	require.NoError(t, err)
	require.Equal(t, ids.sessionID, gjson.GetBytes(rewritten, "client_metadata.session_id").String())
	require.Equal(t, ids.threadID, gjson.GetBytes(rewritten, "client_metadata.thread_id").String())
	require.Equal(t, "yes", gjson.GetBytes(rewritten, "client_metadata.preserved").String())
}
