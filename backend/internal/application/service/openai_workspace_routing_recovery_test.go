package service

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIWorkspaceRoutingDiscoveryDeniedUsesDefaultRoute(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := &openAIWorkspaceRoutingUpstream{responses: []*http.Response{
				workspaceRoutingResponse(status, `{"detail":"Unauthorized"}`),
			}}
			svc := &OpenAIGatewayService{httpUpstream: upstream}
			svc.enableOpenAIWorkspaceRouting()
			account := workspaceRoutingTestAccount()
			ctx := context.Background()
			c := newWorkspaceRoutingTestContext(t)
			body := []byte(`{"model":"gpt-6-luna","stream":false,"input":"hello"}`)

			// WS constructs its URL and headers separately; an HTTP recovery must
			// share the same short-lived discovery result with both operations.
			wsURL, err := svc.buildOpenAIResponsesWSURLWithContext(ctx, account, "stored-token")
			require.NoError(t, err)
			require.Equal(t, "wss://chatgpt.com/backend-api/codex/responses", wsURL)
			headers, _, err := svc.buildOpenAIWSHeaders(ctx, c, account, "stored-token", OpenAIWSProtocolDecision{}, false, "", "", "", "gpt-6-luna", "")
			require.NoError(t, err)
			require.Equal(t, "workspace-71", headers.Get("chatgpt-account-id"))
			require.Empty(t, headers.Get(openAIAccountRoutingOverrideHeader))
			req, err := svc.buildUpstreamRequest(ctx, c, account, body, "stored-token", false, "", false)
			require.NoError(t, err)
			require.Equal(t, chatgptCodexURL, req.URL.String())
			require.Equal(t, "workspace-71", req.Header.Get("chatgpt-account-id"))
			require.Empty(t, req.Header.Get(openAIAccountRoutingOverrideHeader))
			require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))
			require.Len(t, upstream.requests, 1, "WS and HTTP must not repeatedly probe a denied discovery endpoint")
		})
	}
}

func TestOpenAIWorkspaceRoutingDiscoveryDeniedCacheExpires(t *testing.T) {
	upstream := &openAIWorkspaceRoutingUpstream{responses: []*http.Response{
		workspaceRoutingResponse(http.StatusUnauthorized, `{"detail":"Unauthorized"}`),
		workspaceRoutingResponse(http.StatusOK, `{"accounts":[{"id":"workspace-71","workspace_backend_origin":"https://workspace.example.com","account_routing_override":"us"}]}`),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	svc.enableOpenAIWorkspaceRouting()
	account := workspaceRoutingTestAccount()

	routing, err := svc.resolveOpenAIWorkspaceRouting(context.Background(), account, "stored-token")
	require.NoError(t, err)
	require.Equal(t, "NO_CONSTRAINT", routing.backendOrigin)
	cachedValue, ok := svc.openAIWorkspaceRoutingCache.Load(account.ID)
	require.True(t, ok)
	cached, ok := cachedValue.(cachedOpenAIWorkspaceRouting)
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(time.Minute), cached.expiresAt, 5*time.Second)
	cached.expiresAt = time.Now().Add(-time.Second)
	svc.openAIWorkspaceRoutingCache.Store(account.ID, cached)

	routing, err = svc.resolveOpenAIWorkspaceRouting(context.Background(), account, "stored-token")
	require.NoError(t, err)
	require.Equal(t, "https://workspace.example.com", routing.backendOrigin)
	require.Equal(t, "us", routing.accountRoutingOverride)
	require.Len(t, upstream.requests, 2)
}

func TestOpenAIWorkspaceRoutingDiscoveryDeniedCacheRechecksCredentials(t *testing.T) {
	upstream := &openAIWorkspaceRoutingUpstream{responses: []*http.Response{
		workspaceRoutingResponse(http.StatusUnauthorized, `{"detail":"Unauthorized"}`),
		workspaceRoutingResponse(http.StatusOK, `{"accounts":[{"id":"workspace-71","workspace_backend_origin":"https://workspace.example.com","account_routing_override":"us"}]}`),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	svc.enableOpenAIWorkspaceRouting()
	account := workspaceRoutingTestAccount()
	account.Credentials["chatgpt_user_id"] = "user-a"
	token := workspaceRoutingToken("workspace-71", "user-a")

	_, err := svc.resolveOpenAIWorkspaceRouting(context.Background(), account, token)
	require.NoError(t, err)
	account.Credentials["chatgpt_user_id"] = "user-b"
	_, err = svc.resolveOpenAIWorkspaceRouting(context.Background(), account, token)
	require.Error(t, err, "cached default routing must not bypass principal validation")
	require.Len(t, upstream.requests, 1)

	newToken := workspaceRoutingToken("workspace-71", "user-b")
	routing, err := svc.resolveOpenAIWorkspaceRouting(context.Background(), account, newToken)
	require.NoError(t, err)
	require.Equal(t, "https://workspace.example.com", routing.backendOrigin)
	require.Len(t, upstream.requests, 2, "a refreshed bearer must rediscover immediately")
}

func TestOpenAIWorkspaceRoutingDiscoveryDeniedCoalescesConcurrentRequests(t *testing.T) {
	upstream := &openAIWorkspaceRoutingUpstream{responses: []*http.Response{
		workspaceRoutingResponse(http.StatusUnauthorized, `{"detail":"Unauthorized"}`),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	svc.enableOpenAIWorkspaceRouting()
	account := workspaceRoutingTestAccount()
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			routing, err := svc.resolveOpenAIWorkspaceRouting(context.Background(), account, "stored-token")
			if err != nil || routing == nil || routing.backendOrigin != "NO_CONSTRAINT" {
				t.Errorf("concurrent discovery should use default routing: %v", err)
			}
		})
	}
	wg.Wait()
	require.Len(t, upstream.requests, 1)
}

func TestOpenAIWSNonStreamDoesNotRecoverWorkspaceRoutingFailureThroughHTTP(t *testing.T) {
	svc, account, c, _, _ := newCodexPrewarmNonStreamFixture(t)
	svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
	body := []byte(`{"stream":false,"input":"hello"}`)
	err := newOpenAIWorkspaceRoutingFailoverError(errOpenAIWorkspaceRoutingUnavailable)
	require.False(t, svc.prepareOpenAIWSHTTPFallback(context.Background(), c, account, err, body, body), "a local routing failure is not an upstream WS 5xx")
}
