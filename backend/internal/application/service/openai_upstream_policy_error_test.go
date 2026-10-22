//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIUpstreamPolicyErrorDetails(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		policy     bool
	}{
		{"upstream_session_block", `{"error":{"type":"permission_error","code":"session_blocked_by_cyber_policy","message":"Please start a new session"}}`, true},
		{"cyber", `{"error":{"code":"cyber_policy"}}`, true},
		{"content", `{"error":{"code":"content_policy_violation"}}`, true},
		{"filter_type", `{"error":{"type":"content_filter"}}`, true},
		{"nested", `{"response":{"error":{"code":"safety_policy_violation"}}}`, true},
		{"safety_message", `{"error":{"message":"Your request was rejected as a result of our safety system."}}`, true},
		{"content_message", `{"error":{"message":"Content policy violation"}}`, true},
		{"chinese_message", `{"error":{"message":"请求被安全策略拦截"}}`, true},
		{"precedence", `{"error":{"code":"content_policy_violation","type":"server_error","message":"try again"}}`, true},
		{"normal_text", `{"output":[{"content":[{"text":"Request blocked by content policy"}]}]}`, false},
		{"quoted_tool", `{"output":[{"arguments":"{\"error\":{\"code\":\"cyber_policy\"}}"}]}`, false},
		{"ordinary_auth", `{"error":{"code":"invalid_api_key","message":"Access forbidden"}}`, false},
		{"transport", `{"error":{"code":"server_error","message":"Transport error: network error: error decoding response body"}}`, false},
		{"policy_docs", `{"error":{"message":"See the security policy documentation"}}`, false},
		{"unrelated_policy", `{"error":{"message":"Retry policy rejected this connection"}}`, false},
		{"security_code", `{"error":{"code":"security_policy_violation"}}`, true},
		{"account_policy_diagnostic", `{"error":{"code":"account_deactivated","message":"Account blocked for usage policy violations"}}`, false},
		{"schema_quotes_policy", `{"error":{"code":"invalid_function_parameters","message":"Invalid enum: request blocked by content policy"}}`, false},
		{"quota_policy_diagnostic", `{"error":{"code":"rate_limit_exceeded","message":"Request blocked by usage policy rate limits"}}`, false},
		{"malformed", `{"error":{"code":"cyber_policy"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, policy := openAIUpstreamPolicyErrorDetails([]byte(tc.body))
			require.Equal(t, tc.policy, policy)
		})
	}
}

func TestOpenAIPolicyErrorPreservesFieldsAndFiltersBody(t *testing.T) {
	c, recorder := newOpenAIUpstreamErrorTestContext(t)
	body := []byte(`{"error":{"type":"invalid_request_error","code":"content_policy_violation","param":"input","message":"Blocked by content policy"},"internal_diagnostics":"private-fixture"}`)
	response := newOpenAIUpstreamErrorResponse(403, string(body))
	response.Header.Set("Content-Encoding", "gzip")
	response.Header.Set("Content-Length", "9999")
	svc := &OpenAIGatewayService{}
	err := svc.handleErrorResponsePassthrough(context.Background(), response, c, newOpenAIUpstreamErrorTestAccount(), nil, body)
	require.Error(t, err)
	require.Equal(t, 403, recorder.Code)
	require.Empty(t, recorder.Header().Get("Content-Encoding"))
	require.Empty(t, recorder.Header().Get("Content-Length"))
	require.Equal(t, "content_policy_violation", gjson.Get(recorder.Body.String(), "error.code").String())
	require.Equal(t, "input", gjson.Get(recorder.Body.String(), "error.param").String())
	require.NotContains(t, recorder.Body.String(), "private-fixture")
}

func TestOpenAIPolicyErrorKeepsCyberUsage(t *testing.T) {
	c, _ := newOpenAIUpstreamErrorTestContext(t)
	payload := []byte(`{"type":"error","error":{"code":"cyber_policy","message":"Blocked by security policy"},"usage":{"input_tokens":9,"output_tokens":2,"output_tokens_details":{"reasoning_tokens":1}}}`)
	normalized, changed := normalizeOpenAIPolicyErrorEvent(payload)
	require.True(t, changed)
	require.Equal(t, "response.failed", gjson.GetBytes(normalized, "type").String())
	_, handled := writeOpenAIUpstreamPolicyError(c, 200, normalized)
	require.True(t, handled)
	mark := GetOpsCyberPolicy(c)
	require.NotNil(t, mark)
	require.Equal(t, 9, mark.UpstreamInTok)
	require.Equal(t, 2, mark.UpstreamOutTok)
	require.NotNil(t, mark.ReasoningTokens)
	require.Equal(t, 1, *mark.ReasoningTokens)
	require.Equal(t, 200, mark.UpstreamStatus)
}

func TestOpenAIPolicyErrorAfterKeepalive(t *testing.T) {
	c, recorder := newOpenAIUpstreamErrorTestContext(t)
	MarkOpenAICompactClientStream(c)
	stop := StartOpenAICompactSSEKeepalive(c, time.Hour)
	keeper, ok := c.Get(openAICompactSSEKeepaliveKey)
	require.True(t, ok)
	require.True(t, keeper.(*openAICompactSSEKeepalive).beat())
	stop()
	require.Contains(t, recorder.Body.String(), ":")
	_, handled := writeOpenAIUpstreamPolicyError(c, 403, []byte(`{"error":{"code":"content_policy_violation","message":"Blocked by content policy"}}`))
	require.True(t, handled)
	require.Equal(t, 200, recorder.Code)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"response.failed"`))
	require.Contains(t, recorder.Body.String(), `"code":"content_policy_violation"`)
	require.Contains(t, recorder.Body.String(), "Blocked by content policy")
}

func TestOpenAIPolicyCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const message = "Request blocked by content policy"
	const policy = `{"type":"invalid_request_error","code":"content_policy_violation","message":"` + message + `"}`
	for _, anthropic := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, envelope := range []string{"http", "failed", "bare"} {
				t.Run(fmt.Sprintf("anthropic=%v/stream=%v/%s", anthropic, stream, envelope), func(t *testing.T) {
					body := []byte(fmt.Sprintf(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":%v}`, stream))
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					path := "/v1/chat/completions"
					if anthropic {
						path = "/v1/messages"
					}
					c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
					status, contentType, payload := 403, "application/json", `{"error":`+policy+`}`
					if envelope != "http" {
						status, contentType = 200, "text/event-stream"
						if envelope == "failed" {
							payload = "data: " + `{"type":"response.failed","response":{"id":"resp_policy","object":"response","status":"failed","error":` + policy + `,"output":[]}}` + "\n\n"
						} else {
							payload = "event: error\ndata: " + `{"type":"error","error":` + policy + `}` + "\n\n"
						}
					}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
						StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(payload)),
					}}}
					var err error
					if anthropic {
						_, err = svc.ForwardAsAnthropic(context.Background(), c, rawChatCompletionsTestAccount(), body, "", "")
					} else {
						_, err = svc.ForwardAsChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), body, "", "")
					}
					require.Error(t, err)
					var failoverErr *UpstreamFailoverError
					require.False(t, errors.As(err, &failoverErr))
					require.NotEqual(t, 502, recorder.Code)
					require.Contains(t, recorder.Body.String(), message)
					require.Equal(t, 1, strings.Count(recorder.Body.String(), message))
				})
			}
		}
	}
}

func TestOpenAIPolicyUntypedStreamError(t *testing.T) {
	payload := []byte(`{"error":{"type":"permission_error","code":"session_blocked_by_cyber_policy","message":"Please start a new session"}}`)
	normalized, changed := normalizeOpenAIPolicyErrorEvent(payload)
	require.True(t, changed)
	require.Equal(t, "response.failed", gjson.GetBytes(normalized, "type").String())
	require.Equal(t, "session_blocked_by_cyber_policy", gjson.GetBytes(normalized, "response.error.code").String())
	for _, event := range []string{
		`{"type":"response.output_text.delta","delta":"blocked by safety policy"}`,
		`{"type":"response.completed","response":{"output":[],"error":null}}`,
		`{"error":{"code":"rate_limit_exceeded","message":"Request blocked by usage policy rate limits"}}`,
	} {
		got, changed := normalizeOpenAIPolicyErrorEvent([]byte(event))
		require.False(t, changed)
		require.Equal(t, event, string(got))
	}
}
