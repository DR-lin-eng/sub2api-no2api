//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain/model"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const contextWindowPassthroughMessage = "Your input exceeds the context window of this model. Please adjust your input and try again."
const contextWindowPassthroughBody = `{"error":{"type":"upstream_error","message":"` + contextWindowPassthroughMessage + `"}}`

func contextWindowPassthroughAccount(route, policy string) *Account {
	account := &Account{
		ID: 901, Name: "context-window-fixture", Platform: PlatformOpenAI,
		Type: AccountTypeAPIKey, Concurrency: 1, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://api.example.test"},
		Extra:       map[string]any{"openai_responses_mode": "force_responses"},
	}
	if route == "passthrough" {
		account.Extra["openai_passthrough"] = true
	}
	if strings.HasSuffix(route, "_chat_only") {
		account.Extra["openai_responses_mode"] = "force_chat_completions"
	}
	switch policy {
	case "custom_codes":
		account.Credentials["custom_error_codes_enabled"] = true
		account.Credentials["custom_error_codes"] = []any{float64(http.StatusTooManyRequests)}
	case "pool_retry":
		account.Credentials["pool_mode"] = true
		account.Credentials["pool_mode_retry_status_codes"] = []any{float64(http.StatusBadRequest)}
	case "temp_unschedulable":
		account.Credentials["temp_unschedulable_enabled"] = true
		account.Credentials["temp_unschedulable_rules"] = []any{map[string]any{
			"error_code": float64(http.StatusBadRequest), "keywords": []any{"context window"},
			"duration_minutes": float64(10),
		}}
	}
	return account
}

func contextWindowPassthroughRequest(route string, stream bool) (string, []byte) {
	switch {
	case strings.HasPrefix(route, "chat"):
		return "/v1/chat/completions", []byte(fmt.Sprintf(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream))
	case strings.HasPrefix(route, "messages"):
		return "/v1/messages", []byte(fmt.Sprintf(`{"model":"gpt-5.5","max_tokens":128,"messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream))
	default:
		return "/v1/responses", []byte(fmt.Sprintf(`{"model":"gpt-5.5","input":"hello","stream":%t}`, stream))
	}
}

func forwardContextWindowRequest(svc *OpenAIGatewayService, c *gin.Context, account *Account, route string, body []byte) (*OpenAIForwardResult, error) {
	switch {
	case strings.HasPrefix(route, "chat"):
		return svc.ForwardAsChatCompletions(c.Request.Context(), c, account, body, "", "gpt-5.5")
	case strings.HasPrefix(route, "messages"):
		return svc.ForwardAsAnthropic(c.Request.Context(), c, account, body, "", "gpt-5.5")
	default:
		return svc.Forward(c.Request.Context(), c, account, body)
	}
}

func TestContextWindow400Passthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, route := range []string{"responses", "passthrough", "chat", "messages", "responses_chat_only", "chat_chat_only", "messages_chat_only"} {
		for _, stream := range []bool{false, true} {
			for _, policy := range []string{"default", "custom_codes", "pool_retry", "temp_unschedulable", "rewrite_rule"} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", route, stream, policy), func(t *testing.T) {
					path, body := contextWindowPassthroughRequest(route, stream)
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
					c.Request.Header.Set("Content-Type", "application/json")
					if policy == "rewrite_rule" {
						rules := &ErrorPassthroughService{}
						rules.setLocalCache([]*model.ErrorPassthroughRule{
							newNonFailoverPassthroughRule(http.StatusBadRequest, "context window", http.StatusBadGateway, "Upstream request failed"),
						})
						BindErrorPassthroughService(c, rules)
					}
					upstream := &httpUpstreamRecorder{resp: newOpenAIUpstreamErrorResponse(http.StatusBadRequest, contextWindowPassthroughBody)}
					repo := &modelNotFoundAccountRepoStub{}
					svc := &OpenAIGatewayService{
						cfg: &config.Config{}, httpUpstream: upstream,
						rateLimitService: &RateLimitService{accountRepo: repo},
					}
					result, err := forwardContextWindowRequest(svc, c, contextWindowPassthroughAccount(route, policy), route, body)
					var failoverErr *UpstreamFailoverError
					failover := errors.As(err, &failoverErr)
					t.Logf("OBSERVED status=%d calls=%d failover=%t pauses=%d body=%s", recorder.Code, len(upstream.requests), failover, repo.tempCalls+len(repo.modelRateLimitCalls), recorder.Body.String())
					assert.Error(t, err)
					assert.Nil(t, result)
					assert.False(t, failover)
					assert.Equal(t, http.StatusBadRequest, recorder.Code)
					assert.Equal(t, contextWindowPassthroughMessage, gjson.Get(recorder.Body.String(), "error.message").String())
					assert.Len(t, upstream.requests, 1)
					assert.Zero(t, repo.tempCalls)
					assert.Empty(t, repo.modelRateLimitCalls)
					assert.True(t, IsResponseCommitted(c))
					if strings.HasPrefix(route, "messages") {
						assert.Equal(t, "error", gjson.Get(recorder.Body.String(), "type").String())
					}
				})
			}
		}
	}
}

// Keep real HTTP boundaries between both gateway services and the provider.
// Account selection is fixed; a failover signal is surfaced as a test failure.
type contextWindowChainUpstream struct {
	endpoint *url.URL
	client   *http.Client
}

func (u *contextWindowChainUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	forwarded := req.Clone(req.Context())
	forwarded.URL.Scheme, forwarded.URL.Host = u.endpoint.Scheme, u.endpoint.Host
	forwarded.Host = u.endpoint.Host
	return u.client.Do(forwarded)
}

func (u *contextWindowChainUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func newContextWindowChainGateway(t *testing.T, upstreamURL, route string) *httptest.Server {
	t.Helper()
	endpoint, err := url.Parse(upstreamURL)
	require.NoError(t, err)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &contextWindowChainUpstream{endpoint: endpoint, client: http.DefaultClient}}
	router := gin.New()
	router.POST("/*path", func(c *gin.Context) {
		body, readErr := io.ReadAll(c.Request.Body)
		if readErr != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		_, forwardErr := forwardContextWindowRequest(svc, c, contextWindowPassthroughAccount(route, "custom_codes"), route, body)
		if !c.Writer.Written() && forwardErr != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": forwardErr.Error()}})
		}
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server
}

func TestContextWindow400TwoHop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, route := range []string{"responses", "passthrough", "chat", "messages", "responses_chat_only", "chat_chat_only", "messages_chat_only"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", route, stream), func(t *testing.T) {
				var providerCalls atomic.Int32
				provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					providerCalls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, contextWindowPassthroughBody)
				}))
				t.Cleanup(provider.Close)
				inner := newContextWindowChainGateway(t, provider.URL, "passthrough")
				outer := newContextWindowChainGateway(t, inner.URL, route)
				path, body := contextWindowPassthroughRequest(route, stream)
				resp, err := http.Post(outer.URL+path, "application/json", bytes.NewReader(body))
				require.NoError(t, err)
				defer resp.Body.Close()
				responseBody, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				t.Logf("OBSERVED status=%d calls=%d body=%s", resp.StatusCode, providerCalls.Load(), responseBody)
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
				assert.Equal(t, contextWindowPassthroughMessage, gjson.GetBytes(responseBody, "error.message").String())
				assert.EqualValues(t, 1, providerCalls.Load())
			})
		}
	}
}

func TestContextWindow400PreservesMetadata(t *testing.T) {
	const body = `{"error":{"type":"invalid_request_error","code":"context_length_exceeded","param":"input","message":"` + contextWindowPassthroughMessage + `"}}`
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprint(passthrough), func(t *testing.T) {
			c, recorder := newOpenAIUpstreamErrorTestContext(t)
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			account := contextWindowPassthroughAccount("responses", "custom_codes")
			if passthrough {
				require.Error(t, svc.handleErrorResponsePassthrough(context.Background(), newOpenAIUpstreamErrorResponse(http.StatusBadRequest, body), c, account, nil, []byte(body)))
			} else {
				_, err := svc.handleErrorResponse(context.Background(), newOpenAIUpstreamErrorResponse(http.StatusBadRequest, body), c, account, nil)
				require.Error(t, err)
			}
			var payload map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Equal(t, "context_length_exceeded", gjson.Get(recorder.Body.String(), "error.code").String())
			assert.Equal(t, "input", gjson.Get(recorder.Body.String(), "error.param").String())
			assert.Equal(t, "invalid_request_error", gjson.Get(recorder.Body.String(), "error.type").String())
		})
	}
}
