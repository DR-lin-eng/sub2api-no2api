//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func workspaceRoutingCompletedResponse() *http.Response {
	resp := workspaceRoutingResponse(http.StatusOK, "data: "+`{"type":"response.completed","response":{"id":"resp_routed","object":"response","status":"completed","model":"gpt-6-luna","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":4,"output_tokens":2}}}`+"\n\n")
	resp.Header.Set("Content-Type", "text/event-stream")
	return resp
}

func TestForwardWorkspaceDiscoveryDeniedReachesModel(t *testing.T) {
	for _, mode := range []string{"http", "passthrough", "prewarm", "prewarm recovery"} {
		for _, stream := range []bool{false, true} {
			// Transient WS 5xx recovery is specific to non-streaming callers.
			if mode == "prewarm recovery" && stream {
				continue
			}
			t.Run(fmt.Sprintf("%s/stream=%t", mode, stream), func(t *testing.T) {
				svc, account, c, rec, _ := newCodexPrewarmNonStreamFixture(t,
					`{"type":"response.completed","response":{"id":"resp_warm"}}`,
					`{"type":"response.completed","response":{"id":"resp_ws","status":"completed","output":[],"usage":{"input_tokens":4,"output_tokens":2}}}`,
				)
				account.Credentials["chatgpt_account_id"] = "workspace-71"
				account.Extra[CodexPrewarmContinuationExtraKey] = strings.HasPrefix(mode, "prewarm")
				account.Extra["openai_passthrough"] = mode == "passthrough"
				svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
				svc.enableOpenAIWorkspaceRouting()
				if mode == "prewarm recovery" {
					svc.openaiWSPool.setClientDialerForTest(&openAIWSRejectDialer{statusCode: http.StatusBadGateway})
				}
				upstream := &httpUpstreamRecorder{responses: []*http.Response{
					workspaceRoutingResponse(http.StatusUnauthorized, `{"detail":"Unauthorized"}`),
					workspaceRoutingCompletedResponse(),
				}}
				svc.httpUpstream = upstream

				result, err := svc.Forward(context.Background(), c, account, []byte(fmt.Sprintf(`{"model":"gpt-6-luna","stream":%t,"input":"hello"}`, stream)))
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, stream, result.Stream)
				require.Equal(t, mode == "prewarm", result.OpenAIWSMode)
				require.Equal(t, 4, result.Usage.InputTokens)
				require.Equal(t, 2, result.Usage.OutputTokens)
				require.Equal(t, http.StatusOK, rec.Code)
				if stream {
					require.Contains(t, rec.Body.String(), "response.completed")
				} else {
					require.True(t, gjson.ValidBytes(rec.Body.Bytes()))
				}
				wantRequests := 2
				if mode == "prewarm" {
					wantRequests = 1
				}
				require.Len(t, upstream.requests, wantRequests)
				for _, id := range upstream.accountIDs {
					require.Equal(t, account.ID, id)
				}
			})
		}
	}
}

func TestForwardWorkspaceDiscoveryDeniedModel401SetsAccountError(t *testing.T) {
	for _, mode := range []string{"http", "passthrough", "prewarm recovery"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", mode, stream), func(t *testing.T) {
				svc, account, c, _, _ := newCodexPrewarmNonStreamFixture(t)
				account.Credentials["chatgpt_account_id"] = "workspace-71"
				account.Extra[CodexPrewarmContinuationExtraKey] = mode == "prewarm recovery"
				account.Extra["openai_passthrough"] = mode == "passthrough"
				if mode == "prewarm recovery" {
					svc.openaiWSPool.setClientDialerForTest(&openAIWSRejectDialer{statusCode: http.StatusUnauthorized})
				}
				svc.enableOpenAIWorkspaceRouting()
				upstream := &httpUpstreamRecorder{responses: []*http.Response{
					workspaceRoutingResponse(http.StatusUnauthorized, `{"detail":"Unauthorized"}`),
					workspaceRoutingResponse(http.StatusUnauthorized, `{"detail":"Unauthorized"}`),
				}}
				svc.httpUpstream = upstream
				repo := &rateLimitAccountRepoStub{}
				svc.rateLimitService = NewRateLimitService(repo, nil, svc.cfg, nil, nil)
				svc.rateLimitService.SetAccountRuntimeBlocker(svc)

				result, err := svc.Forward(context.Background(), c, account, []byte(fmt.Sprintf(`{"model":"gpt-6-luna","stream":%t,"input":"hello"}`, stream)))
				require.Nil(t, result)
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Equal(t, http.StatusUnauthorized, failover.StatusCode, "confirmed model 401 must not become a synthetic routing 502")
				require.Equal(t, 1, repo.setErrorCalls)
				require.Equal(t, account.ID, repo.lastErrorID)
				require.Contains(t, repo.lastErrorMsg, "401")
				require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "failed credentials must leave scheduling immediately")
				require.Len(t, upstream.requests, 2, "one discovery and one model call")
			})
		}
	}
}

func TestForwardWorkspaceDiscoveryDeniedModel401RefreshesOnce(t *testing.T) {
	for _, recovered := range []bool{false, true} {
		t.Run(fmt.Sprintf("recovered=%t", recovered), func(t *testing.T) {
			svc, account, c, rec, _ := newCodexPrewarmNonStreamFixture(t)
			account.Extra[CodexPrewarmContinuationExtraKey] = false
			account.Credentials["chatgpt_account_id"] = "workspace-71"
			account.Credentials["refresh_token"] = "fixture-refresh"
			account.Credentials["expires_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
			svc.enableOpenAIWorkspaceRouting()
			finalResponse := workspaceRoutingResponse(http.StatusUnauthorized, `{"detail":"Unauthorized"}`)
			if recovered {
				finalResponse = workspaceRoutingCompletedResponse()
			}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				workspaceRoutingResponse(http.StatusUnauthorized, `{"detail":"Unauthorized"}`),
				workspaceRoutingResponse(http.StatusUnauthorized, `{"detail":"Unauthorized"}`),
				workspaceRoutingResponse(http.StatusUnauthorized, `{"detail":"Unauthorized"}`),
				finalResponse,
			}}
			svc.httpUpstream = upstream
			repo := &refreshAPIAccountRepo{account: account}
			cache := newOpenAITokenCacheStub()
			oauthClient := &tokenExpiredRecoveryOAuthClient{}
			oauthService := NewOpenAIOAuthService(nil, oauthClient)
			t.Cleanup(oauthService.Stop)
			tokenProvider := NewOpenAITokenProvider(repo, cache, oauthService)
			tokenProvider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewOpenAITokenRefresher(oauthService, repo))
			svc.openAITokenProvider = tokenProvider
			errorRepo := &rateLimitAccountRepoStub{}
			svc.rateLimitService = NewRateLimitService(errorRepo, nil, svc.cfg, nil, nil)
			svc.rateLimitService.SetAccountRuntimeBlocker(svc)

			result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-6-luna","stream":false,"input":"hello"}`))
			if recovered {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, http.StatusOK, rec.Code)
				require.Equal(t, 0, errorRepo.setErrorCalls)
			} else {
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Equal(t, http.StatusUnauthorized, failover.StatusCode)
				require.Equal(t, 1, errorRepo.setErrorCalls)
				require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
			}
			require.Equal(t, int32(1), atomic.LoadInt32(&oauthClient.refreshCalls))
			require.Len(t, upstream.requests, 4, "each bearer gets one discovery and one model request")
			require.Equal(t, "Bearer fixture-token", upstream.requests[1].Header.Get("Authorization"))
			require.Equal(t, "Bearer refreshed-access-token", upstream.requests[3].Header.Get("Authorization"))
		})
	}
}
