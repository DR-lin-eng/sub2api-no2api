//go:build unit

package handler

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Each gateway uses its API Key account and real HTTP transport to call the
// next server. No upstream response is injected into the forwarding service.
type policyChainHTTPUpstream struct {
	service.HTTPUpstream
	client *http.Client
	calls  atomic.Int32
}

func (u *policyChainHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	return u.client.Do(req)
}

func (u *policyChainHTTPUpstream) DoWithTLS(req *http.Request, proxy string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, accountID, concurrency)
}

func newPolicyChainAPIServer(t *testing.T, upstreamURL, mode string) (*httptest.Server, *policyChainHTTPUpstream) {
	t.Helper()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(transport.CloseIdleConnections)
	upstream := &policyChainHTTPUpstream{client: &http.Client{Transport: transport, Timeout: 3 * time.Second}}
	var fixtureAccounts []service.Account
	var fixtureConfig *config.Config
	h := newOpenAIResponsesFailoverTestHandler(t, upstream, func(accounts *[]service.Account, cfg *config.Config) {
		cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 0
		cfg.Security.URLAllowlist.AllowInsecureHTTP = true
		for i := range *accounts {
			(*accounts)[i].Type = service.AccountTypeAPIKey
			(*accounts)[i].Credentials = map[string]any{"api_key": "fixture-chain-key", "base_url": upstreamURL}
			(*accounts)[i].Extra = map[string]any{"openai_responses_mode": "force_responses"}
			if mode == "passthrough" {
				(*accounts)[i].Extra["openai_passthrough"] = true
			}
			if mode == "chat_only" {
				(*accounts)[i].Extra["openai_responses_mode"] = "force_chat_completions"
			}
		}
		fixtureAccounts, fixtureConfig = *accounts, cfg
	})
	h.gatewayService = service.NewOpenAIGatewayService(
		openAIImagesFailoverAccountRepo{accounts: fixtureAccounts}, nil, nil, nil, nil, nil, nil, fixtureConfig,
		nil, nil, service.NewBillingService(fixtureConfig, nil), nil, nil, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		fixture, _ := newOpenAIResponsesFailoverTestContext(t, context.Background())
		for key, value := range fixture.Keys {
			if apiKey, ok := value.(*service.APIKey); ok && apiKey.Group != nil {
				apiKey.Group.AllowMessagesDispatch = true
			}
			c.Set(key, value)
		}
		c.Next()
	})
	router.POST("/v1/responses", h.Responses)
	router.POST("/v1/chat/completions", h.ChatCompletions)
	router.POST("/v1/messages", h.Messages)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server, upstream
}

func TestOpenAIPolicyAPIChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name, errJSON, code, message      string
		status                            int
		sse, untyped, compressed, partial bool
	}{
		{"http_policy", `{"type":"invalid_request_error","code":"content_policy_violation","message":"Request blocked by content policy"}`, "content_policy_violation", "Request blocked by content policy", 403, false, false, false, false},
		{"http_session", `{"type":"permission_error","code":"session_blocked_by_cyber_policy","message":"Please start a new session"}`, "session_blocked_by_cyber_policy", "Please start a new session", 403, false, false, false, false},
		{"http_gzip", `{"type":"invalid_request_error","code":"content_policy_violation","message":"Request blocked by content policy"}`, "content_policy_violation", "Request blocked by content policy", 403, false, false, true, false},
		{"sse_policy", `{"type":"invalid_request_error","code":"content_policy_violation","message":"Request blocked by content policy"}`, "content_policy_violation", "Request blocked by content policy", 200, true, false, false, false},
		{"sse_untyped", `{"type":"invalid_request_error","code":"content_policy_violation","message":"Request blocked by content policy"}`, "content_policy_violation", "Request blocked by content policy", 200, true, true, false, false},
		{"sse_partial", `{"type":"invalid_request_error","code":"content_policy_violation","message":"Request blocked by content policy"}`, "content_policy_violation", "Request blocked by content policy", 200, true, true, false, true},
	}
	for _, mode := range []string{"native", "passthrough", "chat_only"} {
		for _, endpoint := range []string{"responses", "chat/completions", "messages"} {
			for _, stream := range []bool{false, true} {
				for _, tc := range tests {
					if tc.partial && !stream {
						continue
					}
					t.Run(fmt.Sprintf("%s/%s/stream=%v/%s", mode, endpoint, stream, tc.name), func(t *testing.T) {
						var providerCalls atomic.Int32
						provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							providerCalls.Add(1)
							if r.Header.Get("Authorization") != "Bearer fixture-chain-key" {
								http.Error(w, "missing API Key", 401)
								return
							}
							payload := `{"error":` + tc.errJSON + `}`
							contentType := "application/json"
							if tc.sse {
								contentType = "text/event-stream"
								if !tc.untyped {
									payload = `{"type":"error","error":` + tc.errJSON + `}`
								}
								payload = "event: error\ndata: " + payload + "\n\n"
								if tc.partial {
									if strings.Contains(r.URL.Path, "chat/completions") {
										payload = "data: {\"id\":\"chat_fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-5.4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n" + payload
									} else {
										payload = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n" + payload
									}
								}
							}
							w.Header().Set("Content-Type", contentType)
							w.Header().Set("X-Request-Id", "api-chain-fixture")
							if tc.compressed {
								var compressed bytes.Buffer
								zipper := gzip.NewWriter(&compressed)
								_, _ = zipper.Write([]byte(payload))
								_ = zipper.Close()
								w.Header().Set("Content-Encoding", "gzip")
								w.WriteHeader(tc.status)
								_, _ = w.Write(compressed.Bytes())
								return
							}
							w.WriteHeader(tc.status)
							_, _ = io.WriteString(w, payload)
						}))
						defer provider.Close()
						upstreamServer, upstreamTransport := newPolicyChainAPIServer(t, provider.URL, mode)
						entryServer, entryTransport := newPolicyChainAPIServer(t, upstreamServer.URL, mode)
						body := fmt.Sprintf(`{"model":"gpt-5.4","stream":%v,"input":"hello","instructions":"fixture"}`, stream)
						if endpoint != "responses" {
							body = fmt.Sprintf(`{"model":"gpt-5.4","stream":%v,"max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`, stream)
						}
						client := &http.Client{Timeout: 5 * time.Second}
						response, err := client.Post(entryServer.URL+"/v1/"+endpoint, "application/json", strings.NewReader(body))
						require.NoError(t, err)
						defer func() { _ = response.Body.Close() }()
						wire, readErr := io.ReadAll(response.Body)
						t.Logf("status=%d entry_calls=%d upstream_calls=%d provider_calls=%d read_error=%v body=%s", response.StatusCode, entryTransport.calls.Load(), upstreamTransport.calls.Load(), providerCalls.Load(), readErr, wire)
						require.NoError(t, readErr)
						require.Equal(t, int32(1), entryTransport.calls.Load())
						require.Equal(t, int32(1), upstreamTransport.calls.Load())
						require.Equal(t, int32(1), providerCalls.Load())
						require.NotEqual(t, 502, response.StatusCode)
						require.Equal(t, 1, strings.Count(string(wire), tc.message))
						// Anthropic's error object has no code field. OpenAI clients
						// must retain the original policy code over both HTTP hops.
						if endpoint != "messages" {
							require.Contains(t, string(wire), `"code":"`+tc.code+`"`)
						}
						if !stream || tc.status >= 400 {
							require.GreaterOrEqual(t, response.StatusCode, 400)
						}
						if stream && response.StatusCode == 200 && endpoint == "responses" {
							terminal := 0
							for _, line := range strings.Split(string(wire), "\n") {
								if strings.HasPrefix(line, "data:") && gjson.Get(strings.TrimSpace(strings.TrimPrefix(line, "data:")), "type").String() == "response.failed" {
									terminal++
								}
							}
							require.Equal(t, 1, terminal)
							require.NotContains(t, string(wire), `"type":"response.completed"`)
						}
						require.NotContains(t, string(wire), "Upstream request failed")
						require.NotContains(t, string(wire), "upstream_stream_truncated")
					})
				}
			}
		}
	}
}
