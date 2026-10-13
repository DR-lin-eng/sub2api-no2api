//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestForwardAsChatCompletions_ResponsesSupportedFallsBackWhenStructuredInputRequiresString(t *testing.T) {
	for name, rejection := range map[string]string{
		"OpenAI invalid type": `{"error":{"type":"invalid_request_error","code":"invalid_type","param":"input","message":"Invalid type for 'input': expected a string, but got an array instead."}}`,
		"vLLM validation":     `{"error":{"message":"1 validation error: [{'type':'string_type', 'loc':('body','input','str'), 'msg':'Input should be a valid string', 'input':[{'role':'user','content':'hi'}, {'role':'assistant','content':[{'type':'output_text'}]}, ResponseFunctionToolCall(type='function_call'), {'type':'function_call_output'}]}]"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)

			body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"},{"role":"assistant","content":[{"type":"text","text":"checking"},{"type":"thinking","thinking":"think"}],"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"done"},{"role":"user","content":"what now"}],"stream":false}`)
			originalBody := append([]byte(nil), body...)
			logs, releaseLogs := captureStructuredLog(t)
			defer releaseLogs()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{
					StatusCode: http.StatusBadRequest,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(
						rejection,
					)),
				},
				{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_raw_fallback"}},
					Body: io.NopCloser(strings.NewReader(
						`{"id":"chatcmpl_fallback","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
					)),
				},
			}}
			svc := &OpenAIGatewayService{
				cfg:          structuredInputRetryTestConfig(),
				httpUpstream: upstream,
			}
			account := structuredInputRetryTestAccount()

			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 2)
			require.Equal(t, "/v1/responses", upstream.requests[0].URL.Path)
			require.True(t, gjson.GetBytes(upstream.bodies[0], "input").IsArray())
			require.Equal(t, "/v1/chat/completions", upstream.requests[1].URL.Path)
			require.JSONEq(t, string(body), string(upstream.bodies[1]))
			require.Equal(t, originalBody, body)
			require.Equal(t, "/v1/chat/completions", result.UpstreamEndpoint)
			require.Equal(t, "/v1/chat/completions", GetActualOpenAIUpstreamEndpoint(c))
			require.Equal(t, "ok", gjson.Get(rec.Body.String(), "choices.0.message.content").String())
			require.True(t, logs.ContainsMessage("structured Responses input rejected"))
			logs.mu.Lock()
			defer logs.mu.Unlock()
			for _, event := range logs.events {
				require.NotContains(t, event.Fields, "upstream_message")
				require.NotContains(t, event.Fields, "upstream_body")
				require.NotContains(t, event.Message, rejection)
			}
		})
	}
}

type openAIChatFallbackBoundaryReader struct {
	io.Reader
	beforeRead func()
}

func (reader *openAIChatFallbackBoundaryReader) Read(buffer []byte) (int, error) {
	if reader.beforeRead != nil {
		reader.beforeRead()
		reader.beforeRead = nil
	}
	return reader.Reader.Read(buffer)
}

func TestForwardAsChatCompletions_StructuredInputRetrySafetyBoundaries(t *testing.T) {
	rejection := `{"error":{"code":"invalid_type","param":"input","message":"Expected a string, but got an array instead."}}`
	validation := `"message":"1 validation error: [{'type':'string_type','loc':('body','input','str'),'msg':'Input should be a valid string'}]"`
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "duplicate error", body: strings.TrimSuffix(rejection, "}") + `,"error":{"code":"rate_limit_exceeded"}}`},
		{name: "duplicate code", body: strings.Replace(rejection, `"code":"invalid_type"`, `"code":"invalid_type","code":"rate_limit_exceeded"`, 1)},
		{name: "duplicate escaped param", body: strings.Replace(rejection, `"param":"input"`, `"param":"input","p\u0061ram":"tools"`, 1)},
		{name: "duplicate message", body: strings.Replace(rejection, `"message":"Expected a string, but got an array instead."`, `"message":"Expected a string, but got an array instead.","message":"Input is too long."`, 1)},
		{name: "contradictory error type", body: strings.Replace(rejection, `"error":{`, `"error":{"type":"authentication_error",`, 1)},
		{name: "contradictory envelope type", body: strings.TrimSuffix(rejection, "}") + `,"type":"rate_limit_error"}`},
		{name: "validation contradictory type", body: `{"error":{"type":"rate_limit_error",` + validation + `}}`},
		{name: "validation contradictory envelope type", body: `{"type":"authentication_error","error":{` + validation + `}}`},
		{name: "validation contradictory code", body: `{"error":{"code":"rate_limit_exceeded",` + validation + `}}`},
		{name: "validation contradictory param", body: `{"error":{"param":"tools",` + validation + `}}`},
		{name: "contradictory envelope code", body: strings.TrimSuffix(rejection, "}") + `,"code":"rate_limit_exceeded"}`},
		{name: "contradictory envelope param", body: strings.TrimSuffix(rejection, "}") + `,"param":"tools"}`},
		{name: "type cannot override code", body: strings.Replace(rejection, `"code":"invalid_type"`, `"code":"rate_limit_exceeded","type":"invalid_type"`, 1)},
		{name: "usage", body: strings.TrimSuffix(rejection, "}") + `,"usage":{"input_tokens":12,"output_tokens":3}}`},
		{name: "error usage", body: strings.Replace(rejection, `"error":{`, `"error":{"usage":{"input_tokens":12},`, 1)},
		{name: "output", body: strings.TrimSuffix(rejection, "}") + `,"output":[{"type":"message","content":[{"type":"output_text","text":"partial"}]}]}`},
		{name: "output text", body: strings.TrimSuffix(rejection, "}") + `,"output_text":"partial"}`},
		{name: "tool calls", body: strings.TrimSuffix(rejection, "}") + `,"tool_calls":[{"id":"call_partial","type":"function","function":{"name":"lookup","arguments":"{}"}}]}`},
		{name: "choices", body: strings.TrimSuffix(rejection, "}") + `,"choices":[{"delta":{"content":"partial"}}]}`},
		{name: "nested response", body: strings.TrimSuffix(rejection, "}") + `,"response":{"status":"incomplete","usage":{"input_tokens":12},"output":[]}}`},
		{name: "partial status", body: strings.TrimSuffix(rejection, "}") + `,"status":"in_progress"}`},
		{name: "incomplete details", body: strings.TrimSuffix(rejection, "}") + `,"incomplete_details":{"reason":"max_output_tokens"}}`},
		{name: "partial event", body: strings.TrimSuffix(rejection, "}") + `,"type":"response.output_text.delta","delta":"partial"}`},
		{name: "caller canceled"},
		{name: "request canceled"},
		{name: "response committed"},
		{name: "writer committed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			requestCtx, cancelRequest := context.WithCancel(context.Background())
			defer cancelRequest()
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(requestCtx)
			errorBody := test.body
			if errorBody == "" {
				errorBody = rejection
			}
			reader := &openAIChatFallbackBoundaryReader{Reader: strings.NewReader(errorBody), beforeRead: func() {
				switch test.name {
				case "caller canceled":
					cancel()
				case "request canceled":
					cancelRequest()
				case "response committed":
					MarkResponseCommitted(c)
				case "writer committed":
					c.Writer.WriteHeaderNow()
				}
			}}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(reader),
			}}
			svc := &OpenAIGatewayService{cfg: structuredInputRetryTestConfig(), httpUpstream: upstream}
			result, err := svc.ForwardAsChatCompletions(ctx, c, structuredInputRetryTestAccount(), body, "", "")
			require.Error(t, err)
			require.Nil(t, result)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, "/v1/responses", upstream.requests[0].URL.Path)
		})
	}
}

func TestConvertedResponsesInputStringRejectionIsNarrow(t *testing.T) {
	matching := []byte(`{"error":{"code":"invalid_type","param":"input","message":"Expected a string for input, but got an object instead."}}`)
	require.True(t, isConvertedResponsesInputStringRejection(http.StatusBadRequest, matching))
	require.True(t, isConvertedResponsesInputStringRejection(http.StatusBadRequest, []byte(`{"error":{"type":"invalid_type","param":"input","message":"Invalid type for 'input': expected a string, but got an array instead."}}`)))

	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "nested content validation", status: http.StatusBadRequest, body: `{"error":{"message":"'loc':('body','input',0,'content','str'), 'msg':'Input should be a valid string'"}}`},
		{name: "other field validation", status: http.StatusBadRequest, body: `{"error":{"message":"'loc':('body','model','str'), 'msg':'Input should be a valid string'"}}`},
		{name: "generic string validation", status: http.StatusBadRequest, body: `{"error":{"message":"Input should be a valid string"}}`},
		{name: "wrong status", status: http.StatusUnprocessableEntity, body: string(matching)},
		{name: "authentication", status: http.StatusUnauthorized, body: string(matching)},
		{name: "authorization", status: http.StatusForbidden, body: string(matching)},
		{name: "rate limit", status: http.StatusTooManyRequests, body: string(matching)},
		{name: "server error", status: http.StatusInternalServerError, body: string(matching)},
		{name: "wrong parameter", status: http.StatusBadRequest, body: `{"error":{"code":"invalid_type","param":"tools","message":"Expected a string, but got an array instead."}}`},
		{name: "wrong code", status: http.StatusBadRequest, body: `{"error":{"code":"invalid_request_error","param":"input","message":"Expected a string, but got an array instead."}}`},
		{name: "unrelated input error", status: http.StatusBadRequest, body: `{"error":{"code":"invalid_type","param":"input","message":"Input is too long."}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.False(t, isConvertedResponsesInputStringRejection(tc.status, []byte(tc.body)))
		})
	}
}

func TestConvertedResponsesInputStringRejectionValidationBoundaries(t *testing.T) {
	validation := `{'type': 'string_type', 'loc': ('body', 'input', 'str'), 'msg': 'Input should be a valid string', 'input': [{'role': 'user', 'content': 'hello'}]}`
	for _, test := range []struct {
		name    string
		message string
		want    bool
	}{
		{name: "complete Pydantic error", message: "1 validation error: [" + validation + "]", want: true},
		{name: "multiple Pydantic errors", message: "2 validation errors for Request: [" + strings.Replace(validation, "'input', 'str'", "'model', 'str'", 1) + ", " + validation + "]", want: true},
		{name: "double quoted keys", message: "1 validation error: [" + strings.ReplaceAll(validation, "'", `"`) + "]", want: true},
		{name: "surrounding whitespace", message: " \n1 validation error: [" + validation + "]\n ", want: true},
		{name: "escaped quote in history", message: `1 validation error: [{'type':'string_type','loc':('body','input','str'),'msg':'Input should be a valid string','input':[{'content':'can\'t [close] this }'}]}]`, want: true},
		{name: "bare loc and msg", message: `'loc':('body','input','str'), 'msg':'Input should be a valid string'`},
		{name: "echoed dictionary in unrelated error", message: "Invalid history: " + validation},
		{name: "echoed fake loc and msg in input", message: `1 validation error: [{'type':'string_type','loc':('body','model','str'),'msg':'Input should be a valid string','input':{'loc':('body','input','str'),'msg':'Input should be a valid string'}}]`},
		{name: "echoed fake loc and msg in quoted history", message: `1 validation error: [{'type':'string_type','loc':('body','input',0,'content','str'),'msg':'Input should be a valid string','input':"'loc':('body','input','str'), 'msg':'Input should be a valid string'"}]`},
		{name: "echoed complete error in quoted history", message: `1 validation error: [{'type':'string_type','loc':('body','model','str'),'msg':'Input should be a valid string','input':"1 validation error: [{'type':'string_type','loc':('body','input','str'),'msg':'Input should be a valid string'}]"}]`},
		{name: "truncated error list", message: "1 validation error: [" + validation},
		{name: "truncated quoted input", message: `1 validation error: [{'type':'string_type','loc':('body','input','str'),'msg':'Input should be a valid string','input':'history}]`},
		{name: "mismatched closing bracket", message: "1 validation error: [" + strings.TrimSuffix(validation, "}") + ")]"},
		{name: "elided validation list", message: "240 validation errors: ... " + validation},
		{name: "incorrect error count", message: "2 validation errors: [" + validation + "]"},
		{name: "overflowed error count", message: "999999999999999999999999 validation errors: [" + validation + "]"},
		{name: "duplicate location", message: "1 validation error: [" + strings.Replace(validation, "'input':", "'loc': ('body', 'model', 'str'), 'input':", 1) + "]"},
		{name: "wrong validation type", message: "1 validation error: [" + strings.Replace(validation, "string_type", "value_error", 1) + "]"},
		{name: "missing validation type", message: "1 validation error: [" + strings.Replace(validation, "'type': 'string_type', ", "", 1) + "]"},
		{name: "location and message in different errors", message: "2 validation errors: [" + strings.Replace(validation, "Input should be a valid string", "Invalid history", 1) + ", " + strings.Replace(validation, "'input', 'str'", "'model', 'str'", 1) + "]"},
		{name: "trailing echoed text", message: "1 validation error: [" + validation + "] echoed history"},
		{name: "structured string array", message: "Expected a string, but got an array instead.", want: true},
		{name: "structured object", message: "Invalid type for 'input': expected a string, but got an object instead.", want: true},
		{name: "structured echoed history", message: "Invalid history containing 'expected a string' and 'got an array'"},
		{name: "structured truncated message", message: "Expected a string, but got an array"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"error": map[string]string{"message": test.message, "code": "invalid_type", "param": "input"}})
			require.NoError(t, err)
			require.Equal(t, test.want, isConvertedResponsesInputStringRejection(http.StatusBadRequest, body))
			if test.want {
				require.False(t, isConvertedResponsesInputStringRejection(http.StatusBadRequest, body[:len(body)-1]))
				require.False(t, isConvertedResponsesInputStringRejection(http.StatusBadRequest, append(body, []byte(" trailing")...)))
			}
		})
	}
}

func TestForwardAsChatCompletions_StructuredInputRetryRequiresCompleteErrorBody(t *testing.T) {
	rejection := `{"error":{"code":"invalid_type","param":"input","message":"Expected a string, but got an array instead."}}`
	limit := int(openAIUpstreamErrorBodyReadLimitForConfig(structuredInputRetryTestConfig()))
	for _, test := range []struct {
		name    string
		payload string
		readErr error
		unknown bool
		want    bool
	}{
		{name: "complete JSON followed by read failure", payload: rejection, readErr: io.ErrUnexpectedEOF},
		{name: "truncated JSON", payload: rejection[:len(rejection)-1]},
		{name: "exact read limit", payload: rejection + strings.Repeat(" ", limit-len(rejection)), want: true},
		{name: "one byte beyond read limit", payload: rejection + strings.Repeat(" ", limit-len(rejection)+1)},
		{name: "valid JSON prefix before discarded suffix", payload: rejection + strings.Repeat(" ", limit-len(rejection)) + "trailing"},
		{name: "unknown capability still retries after read failure", payload: rejection, readErr: io.ErrUnexpectedEOF, unknown: true, want: true},
		{name: "unknown capability still retries beyond read limit", payload: rejection + strings.Repeat(" ", limit-len(rejection)+1), unknown: true, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}]}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			errorBody := io.NopCloser(strings.NewReader(test.payload))
			if test.readErr != nil {
				errorBody = &openAIChatStreamReadErrorCloser{payload: []byte(test.payload), err: test.readErr}
			}
			account := structuredInputRetryTestAccount()
			status := http.StatusBadRequest
			if test.unknown {
				delete(account.Extra, openai_compat.ExtraKeyResponsesSupported)
				status = http.StatusNotFound
			}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: errorBody},
				{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"chatcmpl_retry","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))},
			}}
			svc := &OpenAIGatewayService{cfg: structuredInputRetryTestConfig(), httpUpstream: upstream}
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			if test.want {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Len(t, upstream.requests, 2)
			} else {
				require.Error(t, err)
				require.Nil(t, result)
				require.Len(t, upstream.requests, 1)
			}
		})
	}
}

type openAIChatFallbackDelayReader struct {
	io.Reader
	elapsed time.Duration
}

func (reader *openAIChatFallbackDelayReader) Read(buffer []byte) (int, error) {
	if reader.elapsed == 0 {
		started := time.Now()
		time.Sleep(30 * time.Millisecond)
		reader.elapsed = time.Since(started)
	}
	return reader.Reader.Read(buffer)
}

func TestForwardAsChatCompletions_RawFallbackPreservesResponsesTiming(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("unknown=%t/stream=%t", unknown, stream), func(t *testing.T) {
				body := []byte(fmt.Sprintf(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream))
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
				account := structuredInputRetryTestAccount()
				status := http.StatusBadRequest
				if unknown {
					delete(account.Extra, openai_compat.ExtraKeyResponsesSupported)
					status = http.StatusNotFound
				}
				initialBody := &openAIChatFallbackDelayReader{Reader: strings.NewReader(`{"error":{"code":"invalid_type","param":"input","message":"Expected a string, but got an array instead."}}`)}
				responseBody := `{"id":"chatcmpl_retry","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
				contentType := "application/json"
				if stream {
					responseBody = "data: {\"id\":\"chatcmpl_retry\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n"
					contentType = "text/event-stream"
				}
				upstream := &httpUpstreamRecorder{responses: []*http.Response{
					{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(initialBody)},
					{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(responseBody))},
				}}
				svc := &OpenAIGatewayService{cfg: structuredInputRetryTestConfig(), httpUpstream: upstream}
				result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Len(t, upstream.requests, 2)
				require.Equal(t, "/v1/responses", upstream.requests[0].URL.Path)
				require.Equal(t, "/v1/chat/completions", upstream.requests[1].URL.Path)
				require.GreaterOrEqual(t, result.Duration, initialBody.elapsed)
				if stream {
					require.NotNil(t, result.FirstTokenMs)
					require.GreaterOrEqual(t, *result.FirstTokenMs, int(initialBody.elapsed.Milliseconds()))
				}
			})
		}
	}
}

func TestForwardAsChatCompletions_StructuredInputRejectionDoesNotRetryIneligibleRequests(t *testing.T) {
	rejection := `{"error":{"code":"invalid_type","param":"input","message":"Expected a string, but got an array instead."}}`
	for _, test := range []struct {
		name          string
		mode          string
		unknown       bool
		platform      string
		protocol      string
		endpoint      string
		accountType   string
		responsesBody bool
		nativeIngress bool
		status        int
		rejection     string
	}{
		{name: "forced Responses", mode: "force_responses"},
		{name: "forced Responses without probe", mode: "force_responses", unknown: true},
		{name: "unknown support", unknown: true},
		{name: "Responses shaped Chat ingress", responsesBody: true},
		{name: "native Responses ingress", responsesBody: true, nativeIngress: true},
		{name: "native CN Responses protocol", platform: PlatformDeepseek},
		{name: "adaptive Chat ingress stays raw", platform: PlatformKimi, protocol: APIProtocolAdaptive, endpoint: "/chat/completions"},
		{name: "adaptive Responses shaped ingress stays Responses", platform: PlatformKimi, protocol: APIProtocolAdaptive, responsesBody: true},
		{name: "adaptive Responses shaped ingress converts to Chat", platform: PlatformZhipu, protocol: APIProtocolAdaptive, responsesBody: true, endpoint: "/chat/completions"},
		{name: "model routed Responses is not OpenAI retry eligible", platform: PlatformMiniMax, protocol: APIProtocolResponses},
		{name: "OAuth", accountType: AccountTypeOAuth},
		{name: "authentication", status: http.StatusUnauthorized},
		{name: "authorization", status: http.StatusForbidden},
		{name: "rate limit", status: http.StatusTooManyRequests},
		{name: "unrelated validation", rejection: `{"error":{"code":"invalid_type","param":"tools","message":"Expected a string, but got an array instead."}}`},
		{name: "nested validation", rejection: `{"error":{"message":"'loc':('body','input',0,'content','str'), 'msg':'Input should be a valid string'"}}`},
		{name: "echoed validation", rejection: `{"error":{"message":"1 validation error: [{'type':'string_type','loc':('body','model','str'),'msg':'Input should be a valid string','input':{'loc':('body','input','str'),'msg':'Input should be a valid string'}}]"}}`},
		{name: "echoed structured error", rejection: `{"error":{"code":"invalid_type","param":"input","message":"Invalid history containing expected a string but got an array"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
			if test.responsesBody {
				body = []byte(`{"model":"gpt-5.4","input":[{"role":"user","content":"hello"}],"stream":false}`)
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			account := structuredInputRetryTestAccount()
			account.Extra = map[string]any{openai_compat.ExtraKeyResponsesSupported: true, openai_compat.ExtraKeyResponsesMode: test.mode}
			if test.unknown {
				delete(account.Extra, openai_compat.ExtraKeyResponsesSupported)
			}
			if test.platform != "" {
				account.Platform = test.platform
				account.Credentials["api_protocol"] = APIProtocolResponses
			}
			if test.protocol != "" {
				account.Credentials["api_protocol"] = test.protocol
			}
			if test.accountType != "" {
				account.Type = test.accountType
				account.Credentials["access_token"] = "test-token"
			}
			status := test.status
			if status == 0 {
				status = http.StatusBadRequest
			}
			errorBody := rejection
			if test.rejection != "" {
				errorBody = test.rejection
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(errorBody)),
			}}
			svc := &OpenAIGatewayService{cfg: structuredInputRetryTestConfig(), httpUpstream: upstream}
			var result *OpenAIForwardResult
			var err error
			if test.nativeIngress {
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
				result, err = svc.Forward(context.Background(), c, account, body)
			} else {
				result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			}
			require.Error(t, err)
			require.Nil(t, result)
			require.Len(t, upstream.requests, 1)
			endpoint := test.endpoint
			if endpoint == "" {
				endpoint = "/responses"
			}
			require.True(t, strings.HasSuffix(upstream.requests[0].URL.Path, endpoint))
			var failover *UpstreamFailoverError
			if status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests {
				require.ErrorAs(t, err, &failover)
				require.Equal(t, status, failover.StatusCode)
			} else {
				require.False(t, errors.As(err, &failover))
				require.True(t, c.Writer.Written())
			}
		})
	}
}

func TestForwardAsChatCompletions_StructuredInputRetryPreservesStreamingAndMapping(t *testing.T) {
	for _, useAccountMapping := range []bool{false, true} {
		t.Run(fmt.Sprintf("account_mapping=%t", useAccountMapping), func(t *testing.T) {
			body := []byte(`{"model":"client-model","messages":[{"role":"user","content":"hello"}],"stream":true,"stream_options":{"include_usage":false},"vendor_option":"preserve"}`)
			originalBody := append([]byte(nil), body...)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			account := structuredInputRetryTestAccount()
			account.Extra = map[string]any{openai_compat.ExtraKeyResponsesSupported: true, openai_compat.ExtraKeyResponsesMode: "auto"}
			mappedModel := "gpt-5.4"
			if useAccountMapping {
				account.Credentials["model_mapping"] = map[string]any{"client-model": mappedModel}
			}
			stream := "data: {\"id\":\"chatcmpl_retry\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-5.4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":4,\"total_tokens\":13}}\n\ndata: [DONE]\n\n"
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_type","param":"input","message":"Expected a string, but got an array instead."}}`))},
				{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))},
			}}
			svc := &OpenAIGatewayService{cfg: structuredInputRetryTestConfig(), httpUpstream: upstream}
			defaultMappedModel := mappedModel
			if useAccountMapping {
				defaultMappedModel = ""
			}
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", defaultMappedModel)
			require.NoError(t, err)
			require.Len(t, upstream.requests, 2)
			for _, outbound := range upstream.bodies {
				require.Equal(t, mappedModel, gjson.GetBytes(outbound, "model").String())
				require.True(t, gjson.GetBytes(outbound, "stream").Bool())
			}
			require.JSONEq(t, gjson.GetBytes(body, "messages").Raw, gjson.GetBytes(upstream.bodies[1], "messages").Raw)
			require.Equal(t, "preserve", gjson.GetBytes(upstream.bodies[1], "vendor_option").String())
			require.True(t, gjson.GetBytes(upstream.bodies[1], "stream_options.include_usage").Bool())
			require.Equal(t, originalBody, body)
			require.True(t, result.Stream)
			require.Equal(t, mappedModel, result.BillingModel)
			require.Equal(t, mappedModel, result.UpstreamModel)
			require.Equal(t, 9, result.Usage.InputTokens)
			require.Equal(t, 4, result.Usage.OutputTokens)
			require.Equal(t, "/v1/chat/completions", result.UpstreamEndpoint)
			require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
			require.Contains(t, rec.Body.String(), "data: [DONE]")
		})
	}
}

func TestForwardAsChatCompletions_StructuredInputRetryIsBoundedAndPreservesRawErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusTooManyRequests} {
		t.Run(fmt.Sprintf("status=%d", status), func(t *testing.T) {
			rejection := `{"error":{"code":"invalid_type","param":"input","message":"Expected a string, but got an array instead."}}`
			response := func(status int) *http.Response {
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(rejection))}
			}
			body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
			account := structuredInputRetryTestAccount()
			account.Extra = map[string]any{openai_compat.ExtraKeyResponsesSupported: true}
			forward := func(retry bool) (*httptest.ResponseRecorder, error) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
				upstream := &httpUpstreamRecorder{responses: []*http.Response{response(status)}}
				if retry {
					upstream.responses = append([]*http.Response{response(http.StatusBadRequest)}, upstream.responses...)
				}
				svc := &OpenAIGatewayService{cfg: structuredInputRetryTestConfig(), httpUpstream: upstream}
				var result *OpenAIForwardResult
				var err error
				if retry {
					result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
					require.Len(t, upstream.requests, 2)
					require.Equal(t, "/v1/chat/completions", upstream.requests[1].URL.Path)
				} else {
					result, err = svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
					require.Len(t, upstream.requests, 1)
				}
				require.Nil(t, result)
				require.Error(t, err)
				return rec, err
			}
			baseline, baselineErr := forward(false)
			retried, retryErr := forward(true)
			require.Equal(t, baseline.Code, retried.Code)
			require.Equal(t, baseline.Body.String(), retried.Body.String())
			require.Equal(t, baselineErr.Error(), retryErr.Error())
			require.IsType(t, baselineErr, retryErr)
		})
	}
}

func structuredInputRetryTestConfig() *config.Config {
	return &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled:           false,
		AllowInsecureHTTP: true,
	}}}
}

func structuredInputRetryTestAccount() *Account {
	return &Account{
		ID:          101,
		Name:        "raw-openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "http://upstream.example"},
		Extra:       map[string]any{openai_compat.ExtraKeyResponsesSupported: true},
	}
}
