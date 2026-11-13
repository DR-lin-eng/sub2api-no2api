package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIHTTP200GenericStreamFailureRecovers(t *testing.T) {
	errors := []struct{ name, frame string }{
		{"nested", `data: {"type":"error","error":{"type":"upstream_error","message":"Upstream request failed"}}`},
		{"flat", `data: {"type":"error","code":"server_error","message":"Upstream request failed"}`},
		{"named", "event: error\ndata: " + genericOpenAIUpstreamFailureBody},
		{"bare", "data: " + genericOpenAIUpstreamFailureBody},
		{"failed", `data: {"type":"response.failed","response":{"status":"failed","error":{"type":"upstream_error","message":"Upstream request failed"}}}`},
		{"json", "{\n  \"error\": {\"type\":\"upstream_error\",\"message\":\"Upstream request failed\"}\n}"},
	}
	for _, passthrough := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, tc := range errors {
				t.Run(fmt.Sprintf("pass=%v/stream=%v/%s", passthrough, stream, tc.name), func(t *testing.T) {
					upstream := &httpUpstreamRecorder{responses: []*http.Response{
						newHTTP200StreamFailure(tc.frame + "\n\n"), openAIPassthroughTransportRetrySuccessResponse(),
					}}
					if tc.name == "json" {
						upstream.responses[0].Header.Set("Content-Type", "application/json")
					}
					svc, c, rec, account, body := newOpenAIPassthroughTransportRetryFixture(t, upstream)
					account.Extra["openai_passthrough"] = passthrough
					if stream {
						body = []byte(strings.Replace(string(body), `"stream":false`, `"stream":true`, 1))
					}
					result, err := svc.Forward(context.Background(), c, account, body)
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, "resp_transport_retry", result.ResponseID)
					require.Equal(t, []int64{account.ID, account.ID}, upstream.accountIDs)
					require.Equal(t, upstream.bodies[0], upstream.bodies[1])
					for _, req := range upstream.requests {
						require.Equal(t, "zstd", req.Header.Get("Content-Encoding"))
					}
					require.NotContains(t, rec.Body.String(), "Upstream request failed")
					require.NotContains(t, rec.Body.String(), `"type":"error"`)
				})
			}
		}
	}
}

func TestOpenAIHTTP200RecoveryBoundaries(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, scenario := range []string{"exhausted", "distillation", "stored", "background", "hosted-tool", "already-tried", "canceled", "reported-usage"} {
			t.Run(fmt.Sprintf("pass=%v/%s", passthrough, scenario), func(t *testing.T) {
				frame := "data: " + genericOpenAIUpstreamFailureBody + "\n\n"
				upstream := &httpUpstreamRecorder{responses: []*http.Response{newHTTP200StreamFailure(frame), newHTTP200StreamFailure(frame)}}
				svc, c, _, account, body := newOpenAIPassthroughTransportRetryFixture(t, upstream)
				account.Extra["openai_passthrough"] = passthrough
				body = []byte(strings.Replace(string(body), `"stream":false`, `"stream":true`, 1))
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				wantCalls := 1
				switch scenario {
				case "exhausted":
					wantCalls = 2
				case "distillation":
					c.Set("api_key", &APIKey{Group: &Group{ID: 1234, IsDistillationGroup: true}})
				case "stored":
					body = []byte(`{"store":true,` + string(body[1:]))
				case "background":
					body = []byte(`{"background":true,` + string(body[1:]))
				case "hosted-tool":
					body = []byte(`{"tools":[{"type":"web_search"}],` + string(body[1:]))
				case "already-tried":
					c.Set(openAIHTTP200RecoveryKey, true)
				case "canceled":
					upstream.onDo = cancel
				case "reported-usage":
					upstream.responses[0] = newHTTP200StreamFailure(`data: {"type":"response.failed","response":{"error":{"type":"upstream_error","message":"Upstream request failed"},"usage":{"input_tokens":10,"output_tokens":1}}}` + "\n\n")
				}
				_, err := svc.Forward(ctx, c, account, body)
				require.Error(t, err)
				require.Len(t, upstream.requests, wantCalls)
			})
		}
	}
}

func TestOpenAIHTTP200RecoveryKeepsNegotiatedEncoding(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprint(passthrough), func(t *testing.T) {
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				newOpenAIRejectedFieldTestResponse(400, `{"detail":"Bad Request"}`),
				newHTTP200StreamFailure("data: " + genericOpenAIUpstreamFailureBody + "\n\n"),
				openAIPassthroughTransportRetrySuccessResponse(),
			}}
			svc, c, _, account, body := newOpenAIPassthroughTransportRetryFixture(t, upstream)
			account.Extra["openai_passthrough"] = passthrough
			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 3)
			require.Equal(t, "zstd", upstream.requests[0].Header.Get("Content-Encoding"))
			require.Empty(t, upstream.requests[1].Header.Get("Content-Encoding"))
			require.Empty(t, upstream.requests[2].Header.Get("Content-Encoding"))
			require.Equal(t, upstream.bodies[0], upstream.bodies[2])
			require.True(t, isCodexRequestCompressionEnabled(account))
		})
	}
}

func TestOpenAIHTTP200TruncatedStreamHasClassifiedTerminal(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, networkError := range []bool{false, true} {
			t.Run(fmt.Sprintf("pass=%v/error=%v", passthrough, networkError), func(t *testing.T) {
				first := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial answer\"}\n\n"
				response := newHTTP200StreamFailure(first)
				code := OpenAIUpstreamStreamTruncatedCode
				if networkError {
					response.Body = io.NopCloser(io.MultiReader(strings.NewReader(first), passthroughErrReadCloser{err: io.ErrUnexpectedEOF}))
					code = OpenAIUpstreamStreamReadErrorCode
				}
				upstream := &httpUpstreamRecorder{resp: response}
				svc, c, rec, account, body := newOpenAIPassthroughTransportRetryFixture(t, upstream)
				account.Extra["openai_passthrough"] = passthrough
				body = []byte(strings.Replace(string(body), `"stream":false`, `"stream":true`, 1))
				_, err := svc.Forward(context.Background(), c, account, body)
				require.Error(t, err)
				require.Len(t, upstream.requests, 1)
				require.Contains(t, rec.Body.String(), "partial answer")
				require.Contains(t, rec.Body.String(), `"type":"response.failed"`)
				require.Contains(t, rec.Body.String(), code)
				require.NotContains(t, rec.Body.String(), `"type":"error"`)
				require.NotContains(t, rec.Body.String(), "Upstream request failed")
			})
		}
	}
}

func TestOpenAIHTTP200FailureAfterOutputHasTerminalEvent(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, frame := range []string{
			`data: {"type":"error","code":"server_error","message":"Upstream request failed"}`,
			"event: error\ndata: " + genericOpenAIUpstreamFailureBody,
		} {
			t.Run(fmt.Sprintf("pass=%v/%s", passthrough, frame), func(t *testing.T) {
				upstream := &httpUpstreamRecorder{resp: newHTTP200StreamFailure(
					"data: {\"type\":\"response.output_text.delta\",\"delta\":\"already delivered\"}\n\n" + frame + "\n\n")}
				svc, c, rec, account, body := newOpenAIPassthroughTransportRetryFixture(t, upstream)
				account.Extra["openai_passthrough"] = passthrough
				body = []byte(strings.Replace(string(body), `"stream":false`, `"stream":true`, 1))
				_, err := svc.Forward(context.Background(), c, account, body)
				require.Error(t, err)
				require.Len(t, upstream.requests, 1)
				require.Contains(t, rec.Body.String(), "already delivered")
				require.Contains(t, rec.Body.String(), `"type":"response.failed"`)
				require.NotContains(t, rec.Body.String(), `"type":"error"`)
				require.NotContains(t, rec.Body.String(), "Upstream request failed")
			})
		}
	}
}

func TestOpenAIGenericFailureFlatEnvelopeIsNarrow(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"type":"error","code":"server_error","message":"Upstream request failed"}`, true},
		{`{"type":"error","code":null,"message":"Upstream request failed"}`, true},
		{`{"type":"error","code":"invalid_request_error","message":"Upstream request failed"}`, false},
		{`{"type":"error","code":"context_length_exceeded","message":"Upstream request failed"}`, false},
		{`{"type":"error","code":"content_policy_violation","message":"Upstream request failed"}`, false},
		{`{"type":"response.output_text.delta","message":"Upstream request failed"}`, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			require.Equal(t, tc.want, IsOpenAIGenericUpstreamFailureBody([]byte(tc.body)))
		})
	}
	code, _, message := parseOpenAIWSErrorEventFields([]byte(`{"type":"error","code":"server_error","message":"Upstream request failed"}`))
	require.Equal(t, "server_error", code)
	require.Equal(t, "Upstream request failed", message)
}

func newHTTP200StreamFailure(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(body))}
}
