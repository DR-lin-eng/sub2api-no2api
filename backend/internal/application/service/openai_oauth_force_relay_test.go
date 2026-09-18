package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIOAuthForceRelayFailingRepo struct {
	*openAIWSModeRouterSettingRepo
}

func (r *openAIOAuthForceRelayFailingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("database unavailable")
}

func openAIOAuthForceRelayTestService(baseURL string) *OpenAIGatewayService {
	svc := openAIOAuthCustomRelayTestService()
	svc.settingService = NewSettingService(&openAIWSModeRouterSettingRepo{values: map[string]string{
		SettingKeyOpenAIOAuthForceRelayEnabled: "true",
		SettingKeyOpenAIOAuthForceRelayBaseURL: baseURL,
	}}, svc.cfg)
	return svc
}

func TestOpenAIOAuthForceRelayCoversHTTPCompactWSAndSearch(t *testing.T) {
	svc := openAIOAuthForceRelayTestService("https://global-relay.example/backend-api/codex")
	account := openAIOAuthCustomRelayAccount()
	account.Extra["custom_base_url"] = "https://account-relay.example/backend-api/codex"
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	ctx := context.Background()

	req, err := svc.buildUpstreamRequest(ctx, c, account, []byte(`{"model":"gpt-5.4","input":"hi"}`), "token", false, "", true)
	require.NoError(t, err)
	require.Equal(t, "https://global-relay.example/backend-api/codex/responses/compact", req.URL.String())
	require.Equal(t, "global-relay.example", req.Host)

	req, err = svc.buildUpstreamRequestOpenAIPassthrough(ctx, c, account, []byte(`{"model":"gpt-5.4"}`), "token")
	require.NoError(t, err)
	require.Equal(t, "https://global-relay.example/backend-api/codex/responses/compact", req.URL.String())
	require.Equal(t, "global-relay.example", req.Host)

	wsURL, err := svc.buildOpenAIResponsesWSURLWithContext(ctx, account)
	require.NoError(t, err)
	require.Equal(t, "wss://global-relay.example/backend-api/codex/responses", wsURL)
	searchURL, err := svc.openAIAlphaSearchURLWithContext(ctx, account)
	require.NoError(t, err)
	require.Equal(t, "https://global-relay.example/backend-api/codex/alpha/search", searchURL)
	require.True(t, svc.shouldEstimateOpenAIInputTokensLocally(ctx, account))
}

func TestOpenAIOAuthForceRelayManifestAndLiveUseGlobalTarget(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"slug":"gpt-5.4"}]}`))
	}))
	defer server.Close()
	svc := openAIOAuthForceRelayTestService(strings.TrimRight(server.URL, "/") + "/backend-api/codex")
	account := openAIOAuthCustomRelayAccount()
	account.Extra["custom_base_url"] = "https://account-relay.example/backend-api/codex"
	manifest, err := svc.FetchCodexModelsManifest(context.Background(), account, "0.146.0", "")
	require.NoError(t, err)
	require.NotNil(t, manifest)
	require.Equal(t, "/backend-api/codex/models", gotPath)

	target, err := svc.openAIOAuthLiveSidebandURL(context.Background(), account, "call/one")
	require.NoError(t, err)
	require.Equal(t, "ws"+strings.TrimPrefix(server.URL, "http")+"/backend-api/codex/call%2Fone", target)

	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "key"}}
	wsURL, err := svc.buildOpenAIResponsesWSURLWithContext(context.Background(), apiKey)
	require.NoError(t, err)
	require.Equal(t, "wss://api.openai.com/v1/responses", wsURL)
}

func TestOpenAIOAuthForceRelaySettingsFailClosedAndSaveValidation(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled: true, UpstreamHosts: []string{"allowed.example"},
	}}}
	for _, raw := range []string{"", "relay.example", "https://denied.example/backend-api/codex"} {
		t.Run(raw, func(t *testing.T) {
			repo := &openAIWSModeRouterSettingRepo{values: map[string]string{}}
			settings := NewSettingService(repo, cfg)
			before := settings.parseSettings(repo.values)
			before.OpenAIOAuthForceRelayEnabled = true
			before.OpenAIOAuthForceRelayBaseURL = raw
			require.Error(t, settings.UpdateSettings(ctx, before))
			require.Empty(t, repo.values)
		})
	}
	repo := &openAIWSModeRouterSettingRepo{values: map[string]string{}}
	settings := NewSettingService(repo, cfg)
	// A disabled prefill outside the allowlist must not block unrelated saves.
	require.NoError(t, settings.UpdateSettings(ctx, settings.parseSettings(repo.values)))
	require.Equal(t, "false", repo.values[SettingKeyOpenAIOAuthForceRelayEnabled])

	settings = NewSettingService(&openAIWSModeRouterSettingRepo{values: map[string]string{
		SettingKeyOpenAIOAuthForceRelayEnabled: "true",
		SettingKeyOpenAIOAuthForceRelayBaseURL: "https://denied.example/backend-api/codex",
	}}, cfg)
	_, _, err := settings.GetOpenAIOAuthForceRelaySettings(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not allowed")

	settings = NewSettingService(&openAIOAuthForceRelayFailingRepo{
		openAIWSModeRouterSettingRepo: &openAIWSModeRouterSettingRepo{values: map[string]string{}},
	}, cfg)
	_, _, err = settings.GetOpenAIOAuthForceRelaySettings(ctx)
	require.ErrorContains(t, err, "database unavailable")
}
