package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func openAIOAuthCustomRelayTestService() *OpenAIGatewayService {
	return &OpenAIGatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
			Enabled:           false,
			AllowInsecureHTTP: true,
		}},
	}}
}

func openAIOAuthCustomRelayAccount() *Account {
	return &Account{
		ID:       901,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"custom_base_url_enabled": true,
			"custom_base_url":         "https://codex-relay.oaifree.com/backend-api/codex",
		},
		Credentials: map[string]any{"access_token": "oauth-token"},
	}
}

func TestOpenAIOAuthCustomRelayTargetUsesResponsesPathWithoutChatGPTHost(t *testing.T) {
	svc := openAIOAuthCustomRelayTestService()
	account := openAIOAuthCustomRelayAccount()

	target, official, err := svc.openAIOAuthCodexTargetURL(account)
	require.NoError(t, err)
	require.False(t, official)
	require.Equal(t, "https://codex-relay.oaifree.com/backend-api/codex/responses", target)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req, err := svc.buildUpstreamRequest(context.Background(), c, account, []byte(`{"model":"gpt-5.4","input":"hello"}`), "oauth-token", false, "", true)
	require.NoError(t, err)
	require.Equal(t, target, req.URL.String())
	require.NotEqual(t, "chatgpt.com", req.Host, "custom relays must use their own Host instead of chatgpt.com")
}

func TestOpenAIOAuthCustomRelayPassthroughAndWebSocketTargets(t *testing.T) {
	svc := openAIOAuthCustomRelayTestService()
	account := openAIOAuthCustomRelayAccount()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)

	req, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, []byte(`{"model":"gpt-5.4"}`), "oauth-token")
	require.NoError(t, err)
	require.Equal(t, "https://codex-relay.oaifree.com/backend-api/codex/responses/compact", req.URL.String())
	require.NotEqual(t, "chatgpt.com", req.Host)

	wsURL, err := svc.buildOpenAIResponsesWSURL(account)
	require.NoError(t, err)
	require.Equal(t, "wss://codex-relay.oaifree.com/backend-api/codex/responses", wsURL)
}

func TestOpenAIOAuthCustomRelayInputTokensUseLocalEstimate(t *testing.T) {
	require.True(t, shouldEstimateOpenAIInputTokensLocally(openAIOAuthCustomRelayAccount()))
	official := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}}
	require.False(t, shouldEstimateOpenAIInputTokensLocally(official))
}

func TestOpenAIOAuthCustomRelayModelsManifestUsesRelayModelsPath(t *testing.T) {
	var gotPath, gotVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotVersion = r.URL.Query().Get("client_version")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"slug":"gpt-5.4"}]}`))
	}))
	defer server.Close()

	svc := openAIOAuthCustomRelayTestService()
	account := openAIOAuthCustomRelayAccount()
	account.Extra["custom_base_url"] = strings.TrimRight(server.URL, "/") + "/backend-api/codex"
	manifest, err := svc.FetchCodexModelsManifest(context.Background(), account, "0.146.0", "")
	require.NoError(t, err)
	require.NotNil(t, manifest)
	require.Equal(t, "/backend-api/codex/models", gotPath)
	require.Equal(t, "0.146.0", gotVersion)
}
