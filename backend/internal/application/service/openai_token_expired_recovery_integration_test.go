//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type tokenExpiredRecoveryOAuthClient struct {
	refreshCalls int32
	response     *openai.TokenResponse
}

func (c *tokenExpiredRecoveryOAuthClient) ExchangeCode(context.Context, string, string, string, string, string) (*openai.TokenResponse, error) {
	return nil, nil
}

func (c *tokenExpiredRecoveryOAuthClient) RefreshToken(context.Context, string, string) (*openai.TokenResponse, error) {
	return c.RefreshTokenWithClientID(context.Background(), "", "", "")
}

func (c *tokenExpiredRecoveryOAuthClient) RefreshTokenWithClientID(context.Context, string, string, string) (*openai.TokenResponse, error) {
	atomic.AddInt32(&c.refreshCalls, 1)
	if c.response != nil {
		return c.response, nil
	}
	return &openai.TokenResponse{
		AccessToken:  "refreshed-access-token",
		RefreshToken: "refreshed-refresh-token",
		ExpiresIn:    3600,
	}, nil
}

func runOpenAIGatewayServiceAuthRecovery(t *testing.T, errorBody string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusUnauthorized,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(errorBody)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp-after-refresh","model":"gpt-5.5","status":"completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)),
		},
	}}
	account := &Account{
		ID:          903,
		Name:        "codex-oauth-token-expired",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "expired-access-token",
			"refresh_token":      "refresh-token",
			"expires_at":         time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
			"chatgpt_account_id": "chatgpt-account-903",
			"base_url":           "https://example.invalid",
		},
	}
	repo := &refreshAPIAccountRepo{account: account}
	cache := newOpenAITokenCacheStub()
	oauthClient := &tokenExpiredRecoveryOAuthClient{}
	oauthService := NewOpenAIOAuthService(nil, oauthClient)
	defer oauthService.Stop()
	tokenProvider := NewOpenAITokenProvider(repo, cache, oauthService)
	tokenProvider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewOpenAITokenRefresher(oauthService, repo))

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{
		cfg:                 cfg,
		httpUpstream:        upstream,
		openAITokenProvider: tokenProvider,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.5","stream":false,"input":"hello"}`))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp-after-refresh", result.ResponseID)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, int32(1), atomic.LoadInt32(&oauthClient.refreshCalls))
	require.Equal(t, "refreshed-access-token", account.GetOpenAIAccessToken())
	require.Equal(t, "refreshed-access-token", cache.tokens[OpenAITokenCacheKey(account)])
}

func TestOpenAIGatewayService_TokenExpiredRefreshesAndReplaysSameAccount(t *testing.T) {
	runOpenAIGatewayServiceAuthRecovery(t, `{
  "detail": {"code": "token_expired", "message": "Your authentication token has expired. Please try refreshing it."},
  "error": {"code": "token_expired", "message": "Your authentication token has expired.", "param": null, "type": "invalid_request_error"},
  "status": 401
}`)
}

func TestOpenAIGatewayService_TeamGenericUnauthorizedRefreshesAndReplaysSameAccount(t *testing.T) {
	runOpenAIGatewayServiceAuthRecovery(t, `{"detail":"Unauthorized","status":401}`)
}

func TestOpenAIGatewayService_RefreshPrincipalChangeDoesNotReplayCurrentTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, changed := range []string{"account", "user"} {
		for _, passthrough := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/passthrough=%t/stream=%t", changed, passthrough, stream), func(t *testing.T) {
					upstream := &httpUpstreamRecorder{responses: []*http.Response{
						workspaceRoutingResponse(http.StatusUnauthorized, `{"detail":"Unauthorized"}`),
						openAICompatSSECompletedResponse("resp-unsafe-replay", "gpt-5.5"),
					}}
					account := &Account{ID: 904, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
						Status: StatusActive, Schedulable: true, Concurrency: 1,
						Credentials: map[string]any{
							"access_token":       workspaceRoutingToken("workspace-a", "user-a"),
							"refresh_token":      "fixture-refresh-token",
							"expires_at":         time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
							"chatgpt_account_id": "workspace-a", "chatgpt_user_id": "user-a",
						},
						Extra: map[string]any{"openai_passthrough": passthrough},
					}
					newAccount, newUser := "workspace-a", "user-a"
					if changed == "account" {
						newAccount = "workspace-b"
					} else {
						newUser = "user-b"
					}
					repo := &refreshAPIAccountRepo{account: account}
					cache := newOpenAITokenCacheStub()
					oauthClient := &tokenExpiredRecoveryOAuthClient{response: &openai.TokenResponse{
						AccessToken: workspaceRoutingToken(newAccount, newUser), RefreshToken: "rotated-fixture-refresh", ExpiresIn: 3600,
					}}
					oauthService := NewOpenAIOAuthService(nil, oauthClient)
					t.Cleanup(oauthService.Stop)
					provider := NewOpenAITokenProvider(repo, cache, oauthService)
					provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewOpenAITokenRefresher(oauthService, repo))
					svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, openAITokenProvider: provider}
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					body := []byte(fmt.Sprintf(`{"model":"gpt-5.5","stream":%t,"input":"hello"}`, stream))
					result, err := svc.Forward(context.Background(), c, account, body)
					require.Nil(t, result)
					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
					require.Equal(t, GatewayFailureReason("openai_oauth_account_changed"), failover.Reason)
					require.Len(t, upstream.requests, 1, "the original turn must not be replayed under the new principal")
					require.Equal(t, int32(1), atomic.LoadInt32(&oauthClient.refreshCalls))
					require.Equal(t, newAccount, repo.account.GetChatGPTAccountID(), "persist the refreshed identity for subsequent requests")
					require.Equal(t, newUser, repo.account.GetChatGPTUserID())
				})
			}
		}
	}
}

func TestOpenAIGatewayService_WSRefreshPrincipalChangeDoesNotFallbackToHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stage := range []string{"handshake", "event"} {
		t.Run(stage, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			account := &Account{ID: 905, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Status: StatusActive, Schedulable: true, Concurrency: 1,
				Credentials: map[string]any{
					"access_token": workspaceRoutingToken("workspace-a", "user-a"), "refresh_token": "fixture-refresh",
					"expires_at":         time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
					"chatgpt_account_id": "workspace-a", "chatgpt_user_id": "user-a",
				}, Extra: map[string]any{"responses_websockets_v2_enabled": true},
			}
			repo := &refreshAPIAccountRepo{account: account}
			cache := newOpenAITokenCacheStub()
			oauthClient := &tokenExpiredRecoveryOAuthClient{response: &openai.TokenResponse{
				AccessToken: workspaceRoutingToken("workspace-a", "user-b"), RefreshToken: "rotated-fixture-refresh", ExpiresIn: 3600,
			}}
			oauthService := NewOpenAIOAuthService(nil, oauthClient)
			t.Cleanup(oauthService.Stop)
			provider := NewOpenAITokenProvider(repo, cache, oauthService)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewOpenAITokenRefresher(oauthService, repo))
			pool := newOpenAIWSConnPool(cfg)
			t.Cleanup(pool.Close)
			var dialCount func() int
			if stage == "handshake" {
				dialer := &openAIWSRejectDialer{statusCode: http.StatusUnauthorized, responseBody: []byte(`{"detail":"Unauthorized"}`)}
				pool.setClientDialerForTest(dialer)
				dialCount = func() int { return int(dialer.dialCount.Load()) }
			} else {
				dialer := &openAIWSCaptureDialer{conn: &openAIWSCaptureConn{events: [][]byte{
					[]byte(`{"type":"error","error":{"code":"token_expired"}}`),
				}}}
				pool.setClientDialerForTest(dialer)
				dialCount = dialer.DialCount
			}
			upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp-unsafe-http-fallback", "gpt-5.5")}
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, openAITokenProvider: provider,
				openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), openaiWSPool: pool, toolCorrector: NewCodexToolCorrector()}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.5","stream":true,"input":"hello"}`))
			require.Nil(t, result)
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.Equal(t, GatewayFailureReason("openai_oauth_account_changed"), failover.Reason)
			require.Equal(t, int32(1), atomic.LoadInt32(&oauthClient.refreshCalls))
			require.Equal(t, 1, dialCount(), "a new principal must not reconnect the original turn")
			require.Empty(t, upstream.requests, "a principal change must not trigger HTTP replay")
		})
	}
}
