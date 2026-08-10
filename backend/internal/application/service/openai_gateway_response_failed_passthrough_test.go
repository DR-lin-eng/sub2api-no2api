//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func buildContextLengthFailedSSE() string {
	failed := `{"type":"response.failed","response":{"id":"resp_err","object":"response","status":"failed","error":{"code":"context_length_exceeded","type":"invalid_request_error","message":"Your input exceeds the context window of this model. Please adjust your input and try again."},"output":[],"usage":{"input_tokens":100000,"output_tokens":0,"total_tokens":100000}}}`
	return fmt.Sprintf("data: %s\n\n", failed)
}

func bindPassthroughRule(c *gin.Context, platform string, keywords []string, responseCode int) {
	svc := &ErrorPassthroughService{}
	rules := make([]*cachedPassthroughRule, 0, len(keywords))
	for i, kw := range keywords {
		code := responseCode
		rules = append(rules, &cachedPassthroughRule{
			ErrorPassthroughRule: &model.ErrorPassthroughRule{
				ID:              int64(i + 1),
				Enabled:         true,
				Platforms:       []string{platform},
				MatchMode:       model.MatchModeAny,
				Keywords:        []string{kw},
				ResponseCode:    &code,
				PassthroughBody: true,
			},
			lowerKeywords:  []string{strings.ToLower(kw)},
			lowerPlatforms: []string{strings.ToLower(platform)},
		})
	}
	svc.localCacheMu.Lock()
	svc.localCache = rules
	svc.localCacheMu.Unlock()
	BindErrorPassthroughService(c, svc)
}

func TestForwardAsChatCompletions_ResponseFailed_PassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	bindPassthroughRule(c, "openai", []string{"context_length_exceeded"}, 400)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(buildContextLengthFailedSSE())),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")

	require.Error(t, err)
	require.Contains(t, err.Error(), "passthrough")
	require.Equal(t, 400, rec.Code, "passthrough rule should override 502 to 400")

	respBody := rec.Body.String()
	errType := gjson.Get(respBody, "error.type").String()
	require.Equal(t, "upstream_error", errType)
	errMsg := gjson.Get(respBody, "error.message").String()
	require.NotEmpty(t, errMsg, "passthrough should preserve error message")
	require.Contains(t, errMsg, "context window")
}

func TestForwardAsAnthropic_ResponseFailed_PassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	bindPassthroughRule(c, "openai", []string{"context_length_exceeded"}, 400)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(buildContextLengthFailedSSE())),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")

	require.Error(t, err)
	require.Contains(t, err.Error(), "passthrough")
	require.Equal(t, 400, rec.Code, "passthrough rule should override 502 to 400")
	respBody := rec.Body.String()
	errMsg := gjson.Get(respBody, "error.message").String()
	require.NotEmpty(t, errMsg, "passthrough should preserve error message")
}

func TestForwardAsChatCompletions_ResponseFailed_NoRule_Still502(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(buildContextLengthFailedSSE())),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")

	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, rec.Code, "without passthrough rule should still be 502")
}

// bindStatusCodePassthroughRule 绑定一条按错误码+关键词双条件(MatchModeAll)匹配的规则。
// 此类规则依赖语义状态码推断才能在协议转换路径命中（response.failed 无真实 HTTP 状态码）。
func bindStatusCodePassthroughRule(c *gin.Context, platform string, statusCode int, keyword string, responseCode int) {
	rule := &model.ErrorPassthroughRule{
		ID:              1,
		Name:            "status-code-rule",
		Enabled:         true,
		Priority:        1,
		Platforms:       []string{platform},
		ErrorCodes:      []int{statusCode},
		Keywords:        []string{keyword},
		MatchMode:       model.MatchModeAll,
		ResponseCode:    &responseCode,
		PassthroughBody: true,
	}
	svc := &ErrorPassthroughService{}
	svc.setLocalCache([]*model.ErrorPassthroughRule{rule})
	BindErrorPassthroughService(c, svc)
}

func TestForwardAsChatCompletions_ResponseFailed_ErrorCodeRuleMatchesViaSemanticStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	bindStatusCodePassthroughRule(c, "openai", http.StatusBadRequest, "context_length_exceeded", http.StatusBadRequest)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(buildContextLengthFailedSSE())),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code, "error-code-conditioned rule should match via semantic status inference")
	respBody := rec.Body.String()
	require.Equal(t, "upstream_error", gjson.Get(respBody, "error.type").String())
	require.Contains(t, gjson.Get(respBody, "error.message").String(), "context window")
}

func TestForwardAsAnthropic_ResponseFailed_ErrorCodeRuleMatchesViaSemanticStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	bindStatusCodePassthroughRule(c, "openai", http.StatusBadRequest, "context_length_exceeded", http.StatusBadRequest)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(buildContextLengthFailedSSE())),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code, "error-code-conditioned rule should match via semantic status inference")
	respBody := rec.Body.String()
	require.NotEmpty(t, gjson.Get(respBody, "error.message").String())
}

func buildCapacityFailedResponsesSSE(includeReplaySafePreamble bool, includeOutput bool, message string) string {
	lines := make([]string, 0, 18)
	if includeReplaySafePreamble {
		lines = append(lines,
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_capacity"}}`,
			"",
			"event: response.in_progress",
			`data: {"type":"response.in_progress","response":{"id":"resp_capacity"}}`,
			"",
			"event: response.output_item.added",
			`data: {"type":"response.output_item.added","item":{"type":"message","content":[]}}`,
			"",
			`data: {"type":"response.output_text.delta","delta":""}`,
			"",
			`data: {"type":"response.metadata","metadata":{"trace_id":"safe"}}`,
			"",
		)
	}
	if includeOutput {
		lines = append(lines,
			`data: {"type":"response.output_text.delta","delta":"partial"}`,
			"",
		)
	}
	lines = append(lines,
		"event: response.failed",
		fmt.Sprintf(`data: {"type":"response.failed","response":{"id":"resp_capacity","status":"failed","error":{"type":"invalid_request_error","message":%q},"output":[]}}`, message),
		"",
	)
	return strings.Join(lines, "\n")
}

func openAIResponsesCapacityFailureCases() []struct {
	name    string
	message string
	keyword string
} {
	return []struct {
		name    string
		message string
		keyword string
	}{
		{
			name:    "selected_model_at_capacity",
			message: "Selected model is at capacity. Please try a different model.",
			keyword: "Selected model is at capacity",
		},
		{
			name:    "servers_currently_overloaded",
			message: "Our servers are currently overloaded. Please try again later.",
			keyword: "Our servers are currently overloaded",
		},
	}
}

func TestOpenAIResponsesCapacityFailureWinsOverPassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, failureCase := range openAIResponsesCapacityFailureCases() {
		t.Run(failureCase.name, func(t *testing.T) {
			for _, passthrough := range []bool{false, true} {
				t.Run(map[bool]string{false: "native", true: "passthrough"}[passthrough], func(t *testing.T) {
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					bindPassthroughRule(c, "openai", []string{failureCase.keyword}, http.StatusBadRequest)

					resp := &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"rid-capacity-rule"}},
						Body:       io.NopCloser(strings.NewReader(buildCapacityFailedResponsesSSE(true, false, failureCase.message))),
					}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
					account := rawChatCompletionsTestAccount()

					var err error
					if passthrough {
						_, err = svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now(), "model", "model")
					} else {
						_, err = svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "model", "model")
					}
					var failoverErr *UpstreamFailoverError
					require.ErrorAs(t, err, &failoverErr)
					require.False(t, c.Writer.Written(), "capacity failover must happen before the passthrough rule commits a response")
					require.Empty(t, rec.Body.String())
					require.NotContains(t, rec.Body.String(), failureCase.message)
				})
			}
		})
	}
}

func TestOpenAIResponsesCapacityFailureSSEToJSONReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, failureCase := range openAIResponsesCapacityFailureCases() {
		t.Run(failureCase.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"rid-capacity-json"}},
			}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
			account := rawChatCompletionsTestAccount()

			result, err := svc.handleSSEToJSONWithAccount(
				resp,
				c,
				[]byte(buildCapacityFailedResponsesSSE(true, false, failureCase.message)),
				"model",
				"model",
				account,
				false,
			)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Nil(t, result)
			require.False(t, c.Writer.Written())
			require.Empty(t, rec.Body.String())
		})
	}
}

func TestOpenAIResponsesCapacityFailureJSONEnvelopeReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, failureCase := range openAIResponsesCapacityFailureCases() {
		t.Run(failureCase.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"rid-capacity-envelope"}},
				Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
					`{"error":{"type":"invalid_request_error","message":%q}}`, failureCase.message,
				))),
			}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
			result, err := svc.handleNonStreamingResponse(context.Background(), resp, c, rawChatCompletionsTestAccount(), "model", "model")
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Nil(t, result)
			require.False(t, c.Writer.Written())
			require.Empty(t, rec.Body.String())
		})
	}
}

func TestOpenAIResponsesCapacityFailureAfterOutputDoesNotFailOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, failureCase := range openAIResponsesCapacityFailureCases() {
		t.Run(failureCase.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"rid-capacity-after-output"}},
				Body:       io.NopCloser(strings.NewReader(buildCapacityFailedResponsesSSE(false, true, failureCase.message))),
			}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
			_, err := svc.handleStreamingResponse(context.Background(), resp, c, rawChatCompletionsTestAccount(), time.Now(), "model", "model")
			var failoverErr *UpstreamFailoverError
			require.Error(t, err)
			require.NotErrorAs(t, err, &failoverErr)
			require.Contains(t, rec.Body.String(), "response.failed")
			require.Contains(t, rec.Body.String(), "Upstream service temporarily unavailable")
			require.NotContains(t, rec.Body.String(), failureCase.message)
		})
	}
}
