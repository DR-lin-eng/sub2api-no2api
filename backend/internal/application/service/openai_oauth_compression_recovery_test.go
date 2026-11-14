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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIOAuthCompressionRecoveryForward(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, compact := range []bool{false, true} {
				if compact && stream {
					continue
				}
				t.Run(fmt.Sprintf("passthrough=%v/stream=%v/compact=%v", passthrough, stream, compact), func(t *testing.T) {
					rejection := &passthroughCloseTrackingReadCloser{Reader: strings.NewReader(`{"detail":"Bad Request"}`)}
					upstream := &httpUpstreamRecorder{responses: []*http.Response{
						{StatusCode: 400, Header: make(http.Header), Body: rejection},
						openAIPassthroughTransportRetrySuccessResponse(),
					}}
					svc, c, rec, account, body := newOpenAIPassthroughTransportRetryFixture(t, upstream)
					account.Extra["openai_passthrough"] = passthrough
					if stream {
						body = bytes.Replace(body, []byte(`"stream":false`), []byte(`"stream":true`), 1)
						// An already-flushed comment must not block recovery.
						c.Header("Content-Type", "text/event-stream")
						n, err := c.Writer.Write([]byte(":\n\n"))
						require.NoError(t, err)
						recordOpenAIStreamKeepaliveBytes(c, n)
						c.Writer.Flush()
					}
					if compact {
						c.Request.URL.Path = "/v1/responses/compact"
						upstream.responses[1] = newOpenAIRejectedFieldTestResponse(200, `{"id":"resp_transport_retry","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)
					}
					result, err := svc.Forward(context.Background(), c, account, body)
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, "resp_transport_retry", result.ResponseID)
					require.Equal(t, 200, rec.Code)
					require.True(t, rejection.closed)
					require.Equal(t, []int64{account.ID, account.ID}, upstream.accountIDs)
					require.Equal(t, "zstd", upstream.requests[0].Header.Get("Content-Encoding"))
					require.Empty(t, upstream.requests[1].Header.Get("Content-Encoding"))
					require.Equal(t, upstream.bodies[0], upstream.bodies[1], "all normalized input/metadata bytes must survive unchanged")
					require.Equal(t, int64(len(upstream.bodies[1])), upstream.requests[1].ContentLength)
					for _, header := range []string{"Authorization", "Chatgpt-Account-Id", "session_id", "conversation_id", "User-Agent", "Accept"} {
						require.Equal(t, upstream.requests[0].Header.Values(header), upstream.requests[1].Header.Values(header), header)
					}
					require.Equal(t, upstream.requests[0].URL, upstream.requests[1].URL)
					require.Equal(t, upstream.requests[0].Host, upstream.requests[1].Host)
					replay, err := upstream.requests[1].GetBody()
					require.NoError(t, err)
					wire, err := io.ReadAll(replay)
					require.NoError(t, err)
					require.NoError(t, replay.Close())
					require.Equal(t, upstream.bodies[1], wire)
					require.True(t, isCodexRequestCompressionEnabled(account), "the account must keep compression enabled")
					if passthrough {
						used, _ := openAIPassthroughBudgetForContext(c).snapshot()
						require.Equal(t, 2, used, "the recovery must consume the existing send budget")
					}
				})
			}
		}
	}
}

func TestOpenAIOAuthCompressionRecoveryBoundaries(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, scenario := range []string{"parameter-error", "policy-error", "context-limit", "already-tried", "canceled", "semantic-output", "distillation", "disabled", "api-key", "stored", "background", "hosted-tool", "nested-hosted-tool", "native-image", "previous-response", "orphan-output", "budget-exhausted"} {
			if scenario == "budget-exhausted" && !passthrough {
				continue
			}
			t.Run(fmt.Sprintf("passthrough=%v/%s", passthrough, scenario), func(t *testing.T) {
				resp := newOpenAIRejectedFieldTestResponse(400, `{"detail":"Bad Request"}`)
				upstream := &httpUpstreamRecorder{resp: resp}
				svc, c, _, account, body := newOpenAIPassthroughTransportRetryFixture(t, upstream)
				account.Extra["openai_passthrough"] = passthrough
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				switch scenario {
				case "parameter-error":
					resp.Body = io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","param":"input","message":"Bad Request"}}`))
				case "policy-error":
					resp.Body = io.NopCloser(strings.NewReader(`{"error":{"code":"content_policy_violation","message":"policy rejected"}}`))
				case "context-limit":
					resp.Body = io.NopCloser(strings.NewReader(`{"error":{"code":"context_length_exceeded","message":"maximum context length exceeded"}}`))
				case "already-tried":
					c.Set(openAIOAuthCompressionRecoveryKey, true)
				case "canceled":
					upstream.onDo = cancel
				case "semantic-output":
					_, err := c.Writer.Write([]byte("data: output\n\n"))
					require.NoError(t, err)
				case "distillation":
					c.Set("api_key", &APIKey{Group: &Group{ID: 981001, IsDistillationGroup: true}})
				case "disabled":
					account.Extra[CodexRequestCompressionExtraKey] = false
				case "api-key":
					account.Type = AccountTypeAPIKey
					account.Credentials["api_key"] = "synthetic-key"
				case "stored":
					body = append([]byte(`{"store":true,`), body[1:]...)
				case "background":
					body = append([]byte(`{"background":true,`), body[1:]...)
				case "hosted-tool":
					body = append([]byte(`{"tools":[{"type":"mcp","server_url":"https://fixture.invalid"}],`), body[1:]...)
				case "nested-hosted-tool":
					body = append([]byte(`{"tools":[{"type":"namespace","name":"fixture","tools":[{"type":"web_search"}]}],`), body[1:]...)
				case "native-image":
					body = []byte(`{"model":"gpt-6.1-sol","stream":false,"input":[{"type":"additional_tools","tools":[{"type":"image_generation"}]}]}`)
				case "previous-response":
					body = append([]byte(`{"previous_response_id":"resp_fixture",`), body[1:]...)
				case "orphan-output":
					body = []byte(`{"model":"gpt-6.1-sol","stream":false,"input":[{"type":"function_call_output","call_id":"orphan","output":"keep"}]}`)
				case "budget-exhausted":
					openAIPassthroughBudgetForContext(c).maxAttempts = 1
				}
				_, err := svc.Forward(ctx, c, account, body)
				require.Error(t, err)
				require.Len(t, upstream.requests, 1)
			})
		}
	}
}

func TestOpenAIOAuthCompressionRecoveryIsRequestScoped(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprint(passthrough), func(t *testing.T) {
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				newOpenAIRejectedFieldTestResponse(400, `{"detail":"Bad Request"}`),
				newOpenAIRejectedFieldTestResponse(400, `{"detail":"Bad Request"}`),
			}}
			svc, c, _, account, body := newOpenAIPassthroughTransportRetryFixture(t, upstream)
			account.Extra["openai_passthrough"] = passthrough
			_, err := svc.Forward(context.Background(), c, account, body)
			require.Error(t, err)
			require.Len(t, upstream.requests, 2, "even an ambiguous error is retried at most once")
			// A new logical request uses zstd again, even on the same account.
			next, _ := gin.CreateTestContext(httptest.NewRecorder())
			next.Request = httptest.NewRequest("POST", "/v1/responses", bytes.NewReader(body))
			upstream.responses = []*http.Response{openAIPassthroughTransportRetrySuccessResponse()}
			result, err := svc.Forward(context.Background(), next, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 3)
			require.Equal(t, "zstd", upstream.requests[2].Header.Get("Content-Encoding"))
		})
	}
}

func TestOpenAIOAuthCompressionRejectionIsNarrow(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   bool
	}{
		{400, `{"detail":"Bad Request"}`, true},
		{415, `{"detail":"Unsupported Content-Encoding: zstd"}`, true},
		{415, `{"detail":"Unsupported media type application/xml"}`, false},
		{400, `{"error":{"message":"Bad Request"}}`, false},
		{400, `{"detail":"Bad Request","error":{"param":"input"}}`, false},
		{400, `{"detail":"Bad Request","detail":"Bad Request"}`, false},
		{400, `{"detail":"Bad Request","usage":{"output_tokens":1}}`, false},
		{400, `{"detail":"Invalid prompt"}`, false},
		{401, `{"detail":"Bad Request"}`, false},
		{500, `{"detail":"Bad Request"}`, false},
		{400, `{"detail":"Bad Request"`, false},
		{400, `["Bad Request"]`, false},
	} {
		t.Run(fmt.Sprintf("%d/%s", tc.status, tc.body), func(t *testing.T) {
			require.Equal(t, tc.want, isOpenAIOAuthCompressionRejection(tc.status, []byte(tc.body)))
		})
	}
}

func TestOpenAIOAuthCompressionRecoveryTransportBoundaries(t *testing.T) {
	for _, scenario := range []string{"no-replay-body", "broken-replay-body", "broken-zstd", "response-read-error", "415", "transport-error", "budget", "request-canceled", "paired-tool-history"} {
		t.Run(scenario, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				newOpenAIRejectedFieldTestResponse(400, `{"detail":"Bad Request"}`),
				newOpenAIRejectedFieldTestResponse(200, `{"status":"completed"}`),
			}}
			svc, c, _, account, body := newOpenAIPassthroughTransportRetryFixture(t, upstream)
			ctx := context.Background()
			if scenario == "paired-tool-history" {
				body = []byte(`{"model":"gpt-6.1-sol","store":false,"input":[{"type":"function_call","name":"shell","call_id":"paired","arguments":"{}"},{"type":"function_call_output","call_id":"paired","output":"historical result"}],"tools":[{"type":"function","name":"shell","parameters":{"type":"object"}}],"metadata":{"counter":9007199254740993}}`)
			}
			req, err := svc.buildUpstreamRequest(ctx, c, account, body, "synthetic", false, "session", true)
			require.NoError(t, err)
			var budget *openAIPassthroughRetryBudget
			wantCalls, wantStatus := 1, 400
			switch scenario {
			case "no-replay-body":
				req.GetBody = nil
			case "broken-replay-body":
				req.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("fixture") }
			case "broken-zstd":
				req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("broken")), nil }
			case "response-read-error":
				upstream.responses[0].Body = &passthroughErrReadCloser{err: io.ErrUnexpectedEOF}
			case "415":
				upstream.responses[0] = newOpenAIRejectedFieldTestResponse(415, `{"detail":"Unsupported Content-Encoding: zstd"}`)
				wantCalls, wantStatus = 2, 200
			case "transport-error":
				upstream.err = errors.New("fixture TLS EOF")
			case "budget":
				budget = &openAIPassthroughRetryBudget{maxAttempts: 1, used: 1}
			case "request-canceled":
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				c.Request = c.Request.WithContext(canceled)
			case "paired-tool-history":
				wantCalls, wantStatus = 2, 200
			}
			resp, err := svc.doOpenAIResponsesWithCompressionRecovery(ctx, c, req, "fixture-proxy", account, false, body, budget)
			if scenario == "transport-error" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, wantStatus, resp.StatusCode)
				require.NoError(t, resp.Body.Close())
			}
			require.Len(t, upstream.requests, wantCalls)
			if wantCalls == 2 {
				require.Equal(t, upstream.bodies[0], upstream.bodies[1])
				require.Equal(t, "fixture-proxy", upstream.lastProxyURL)
			}
		})
	}
}
