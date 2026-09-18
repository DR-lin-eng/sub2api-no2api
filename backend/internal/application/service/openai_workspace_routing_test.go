package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIWorkspaceRoutingUpstream struct {
	mu        sync.Mutex
	requests  []*http.Request
	responses []*http.Response
}

func (u *openAIWorkspaceRoutingUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.requests = append(u.requests, req.Clone(req.Context()))
	if len(u.responses) == 0 {
		return nil, io.EOF
	}
	response := u.responses[0]
	u.responses = u.responses[1:]
	return response, nil
}

func (u *openAIWorkspaceRoutingUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func workspaceRoutingResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func workspaceRoutingTestAccount() *Account {
	return &Account{
		ID:       71,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":       "stored-token",
			"chatgpt_account_id": "workspace-71",
		},
	}
}

func newWorkspaceRoutingTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c
}

func TestParseOpenAIWorkspaceRoutingResponse(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    openAIWorkspaceRouting
		wantErr string
	}{
		{
			name: "list us route",
			body: `{"accounts":[{"id":"workspace-71","workspace_backend_origin":"https://workspace.example.com","account_routing_override":"us"}]}`,
			want: openAIWorkspaceRouting{backendOrigin: "https://workspace.example.com", accountRoutingOverride: "us"},
		},
		{
			name: "nested map us cr route",
			body: `{"accounts":{"workspace-71":{"account":{"account_id":"workspace-71","workspace_backend_origin":"https://workspace-cr.example.com/","account_routing_override":"us_cr"}}}}`,
			want: openAIWorkspaceRouting{backendOrigin: "https://workspace-cr.example.com", accountRoutingOverride: "us_cr"},
		},
		{
			name: "no constraint",
			body: `{"accounts":[{"id":"workspace-71","workspace_backend_origin":"NO_CONSTRAINT","account_routing_override":"NO_CONSTRAINT"}]}`,
			want: openAIWorkspaceRouting{backendOrigin: "NO_CONSTRAINT", accountRoutingOverride: "NO_CONSTRAINT"},
		},
		{
			name:    "missing fields fail closed",
			body:    `{"accounts":[{"id":"workspace-71"}]}`,
			wantErr: "override is invalid",
		},
		{
			name:    "unknown override",
			body:    `{"accounts":[{"id":"workspace-71","workspace_backend_origin":"https://workspace.example.com","account_routing_override":"eu"}]}`,
			wantErr: "override is invalid",
		},
		{
			name:    "backend path rejected",
			body:    `{"accounts":[{"id":"workspace-71","workspace_backend_origin":"https://workspace.example.com/backend-api","account_routing_override":"us"}]}`,
			wantErr: "HTTPS origin",
		},
		{
			name:    "wrong workspace",
			body:    `{"accounts":[{"id":"workspace-other","workspace_backend_origin":"https://workspace.example.com","account_routing_override":"us"}]}`,
			wantErr: "selected workspace is missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOpenAIWorkspaceRoutingResponse([]byte(tt.body), "workspace-71")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBuildOpenAIRequestAppliesDiscoveredWorkspaceRouting(t *testing.T) {
	upstream := &openAIWorkspaceRoutingUpstream{responses: []*http.Response{
		workspaceRoutingResponse(http.StatusOK, `{"accounts":[{"id":"workspace-71","workspace_backend_origin":"https://workspace.example.com","account_routing_override":"us"}]}`),
	}}
	service := &OpenAIGatewayService{httpUpstream: upstream}
	service.enableOpenAIWorkspaceRouting()
	account := workspaceRoutingTestAccount()

	req, err := service.buildUpstreamRequest(
		context.Background(),
		newWorkspaceRoutingTestContext(t),
		account,
		[]byte(`{"model":"gpt-5.6-sol","input":"hello"}`),
		"request-token",
		false,
		"",
		true,
	)
	require.NoError(t, err)
	require.Equal(t, "https://workspace.example.com/backend-api/codex/responses", req.URL.String())
	require.Equal(t, req.URL.Host, req.Host, "a routed backend must use the URL host")
	require.Equal(t, "us", req.Header.Get(openAIAccountRoutingOverrideHeader))
	require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))

	upstream.mu.Lock()
	require.Len(t, upstream.requests, 1)
	discovery := upstream.requests[0]
	upstream.mu.Unlock()
	require.Equal(t, openAIWorkspaceRoutingDiscoveryURL, discovery.URL.String())
	require.Equal(t, "Bearer request-token", discovery.Header.Get("Authorization"))
	require.Equal(t, "workspace-71", discovery.Header.Get("ChatGPT-Account-Id"))
	require.NotEmpty(t, discovery.Header.Get("User-Agent"))
	require.NotEmpty(t, discovery.Header.Get("originator"))
	require.True(t, HTTPUpstreamRedirectsDisabled(discovery.Context()))
}

func TestBuildOpenAIRequestNoConstraintOmitsHeaderButRejectsRedirects(t *testing.T) {
	upstream := &openAIWorkspaceRoutingUpstream{responses: []*http.Response{
		workspaceRoutingResponse(http.StatusOK, `{"accounts":[{"id":"workspace-71","workspace_backend_origin":"NO_CONSTRAINT","account_routing_override":"NO_CONSTRAINT"}]}`),
	}}
	service := &OpenAIGatewayService{httpUpstream: upstream}
	service.enableOpenAIWorkspaceRouting()

	req, err := service.buildUpstreamRequest(
		context.Background(),
		newWorkspaceRoutingTestContext(t),
		workspaceRoutingTestAccount(),
		[]byte(`{"model":"gpt-5.6-sol"}`),
		"request-token",
		false,
		"",
		true,
	)
	require.NoError(t, err)
	require.Equal(t, chatgptCodexURL, req.URL.String())
	require.Equal(t, "chatgpt.com", req.Host)
	require.Empty(t, req.Header.Get(openAIAccountRoutingOverrideHeader))
	require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))
}

func TestOpenAIWorkspaceRoutingCacheKeysByToken(t *testing.T) {
	response := `{"accounts":[{"id":"workspace-71","workspace_backend_origin":"https://workspace.example.com","account_routing_override":"us"}]}`
	upstream := &openAIWorkspaceRoutingUpstream{responses: []*http.Response{
		workspaceRoutingResponse(http.StatusOK, response),
		workspaceRoutingResponse(http.StatusOK, response),
	}}
	service := &OpenAIGatewayService{httpUpstream: upstream}
	service.enableOpenAIWorkspaceRouting()
	account := workspaceRoutingTestAccount()

	_, err := service.resolveOpenAIWorkspaceRouting(context.Background(), account, "token-a")
	require.NoError(t, err)
	_, err = service.resolveOpenAIWorkspaceRouting(context.Background(), account, "token-a")
	require.NoError(t, err)
	_, err = service.resolveOpenAIWorkspaceRouting(context.Background(), account, "token-b")
	require.NoError(t, err)

	upstream.mu.Lock()
	require.Len(t, upstream.requests, 2, "same token should hit cache; a refreshed token must rediscover routing")
	upstream.mu.Unlock()
}

func TestBuildOpenAIRequestWorkspaceRoutingFailureIsAccountFailover(t *testing.T) {
	upstream := &openAIWorkspaceRoutingUpstream{responses: []*http.Response{
		workspaceRoutingResponse(http.StatusOK, `{"accounts":[{"id":"workspace-71"}]}`),
	}}
	service := &OpenAIGatewayService{httpUpstream: upstream}
	service.enableOpenAIWorkspaceRouting()

	_, err := service.buildUpstreamRequest(
		context.Background(),
		newWorkspaceRoutingTestContext(t),
		workspaceRoutingTestAccount(),
		[]byte(`{"model":"gpt-5.6-sol"}`),
		"request-token",
		false,
		"",
		true,
	)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Equal(t, GatewayFailureScopeAccount, failover.Scope)
	require.Equal(t, GatewayFailureReason("openai_workspace_routing_unavailable"), failover.Reason)
	require.Equal(t, NextAccountRetry, failover.NextAccountAction)
}

func TestResolveOpenAIWorkspaceRoutingSkipsAccountWithoutSelectedWorkspace(t *testing.T) {
	service := &OpenAIGatewayService{httpUpstream: &openAIWorkspaceRoutingUpstream{}}
	service.enableOpenAIWorkspaceRouting()
	account := &Account{ID: 72, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	routing, err := service.resolveOpenAIWorkspaceRouting(context.Background(), account, "opaque-token")
	require.NoError(t, err)
	require.Nil(t, routing)
}
