package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const streamControlPreamble = "data: {\"type\":\"codex.rate_limits\",\"rate_limits\":{\"allowed\":true}}\n\n" +
	"data: {\"type\":\"codex.response.metadata\",\"headers\":{\"x-codex-turn-state\":\"fixture-state\"}}\n\n" +
	"data: {\"type\":\"response.in_progress\",\"response\":{\"id\":\"resp_control\"}}\n\n"

func runStreamControlResponse(svc *OpenAIGatewayService, mode string, c *gin.Context, body io.ReadCloser) (*OpenAIUsage, *int, error) {
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}
	account := &Account{ID: 81, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	if mode == "native" {
		result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-5.6-sol", "gpt-5.6-sol")
		return result.usage, result.firstTokenMs, err
	}
	result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "gpt-5.6-sol", "gpt-5.6-sol")
	return result.usage, result.firstTokenMs, err
}

func TestOpenAIStreamControlClassification(t *testing.T) {
	for _, eventType := range []string{"codex.rate_limits", "codex.response.metadata", "response.created", "response.in_progress"} {
		t.Run(eventType, func(t *testing.T) {
			payload := `{"type":"` + eventType + `"}`
			require.False(t, openAIStreamDataStartsClientOutput(payload, eventType))
			require.False(t, openAIStreamDataSignalsOutputProgressTrimmed(payload, eventType))
		})
	}
	require.True(t, openAIStreamDataStartsClientOutput(`{"type":"response.future_event"}`, "response.future_event"))
}

func TestOpenAIStreamControlServerErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		code    string
		message string
		retry   bool
	}{
		{"server_error", "Upstream service temporarily unavailable. Please retry your request.", true},
		{"server_error", "Transport error: network error: error decoding response body", true},
		{"invalid_request_error", "Invalid input", false},
		{"server_error", "Your input exceeds the context window of this model", false},
		{"server_error", "Request blocked by content policy", false},
		{"custom_notice", "An ordinary compatibility event", false},
	} {
		t.Run(tc.code+"/"+tc.message, func(t *testing.T) {
			payload := []byte(`{"type":"error","error":{"type":"upstream_error","code":"` + tc.code + `","message":"` + tc.message + `"}}`)
			require.Equal(t, tc.retry, openAIStreamFailureIsExplicitlyRetryable(payload, tc.message))
		})
	}
}

func TestOpenAIStreamControlDeterministicErrorAfterHeaderKeepalive(t *testing.T) {
	for _, mode := range []string{"native", "passthrough"} {
		t.Run(mode, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Header("Content-Type", "text/event-stream")
			n, err := c.Writer.WriteString(":\n\n")
			require.NoError(t, err)
			recordOpenAIStreamKeepaliveBytes(c, n)
			c.Writer.Flush()
			body := []byte(`{"error":{"code":"context_length_exceeded","type":"invalid_request_error","message":"Your input exceeds the context window of this model"}}`)
			resp := &http.Response{StatusCode: http.StatusBadRequest, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
			if mode == "native" {
				_, err = svc.handleErrorResponse(context.Background(), resp, c, account, nil)
			} else {
				err = svc.handleErrorResponsePassthrough(context.Background(), resp, c, account, nil, body)
			}
			require.Error(t, err)
			require.Equal(t, "text/event-stream", recorder.Result().Header.Get("Content-Type"))
			require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"response.failed"`))
			require.Contains(t, recorder.Body.String(), "context window")
			require.True(t, IsResponseCommitted(c))
		})
	}
}

func TestOpenAIStreamControlFailureSampleCanFailoverAfterKeepalive(t *testing.T) {
	sample, err := os.ReadFile("testdata/openai_control_failure.sse")
	require.NoError(t, err)
	for _, mode := range []string{"native", "passthrough"} {
		t.Run(mode, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			n, err := c.Writer.WriteString(":\n\n")
			require.NoError(t, err)
			recordOpenAIStreamKeepaliveBytes(c, n)
			c.Writer.Flush()
			_, ttft, err := runStreamControlResponse(svc, mode, c, io.NopCloser(strings.NewReader(string(sample))))
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.Equal(t, http.StatusBadGateway, failover.StatusCode)
			require.Nil(t, ttft)
			require.Equal(t, ":\n\n", recorder.Body.String(), "failed attempt must remain private")
			require.False(t, openAIStreamClientOutputStarted(c, false))

			healthy := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"recovered\"}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_healthy\",\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n"
			usage, _, err := runStreamControlResponse(svc, mode, c, io.NopCloser(strings.NewReader(healthy)))
			require.NoError(t, err)
			require.Equal(t, 3, usage.InputTokens)
			require.Equal(t, 1, usage.OutputTokens)
			require.Contains(t, recorder.Body.String(), "recovered")
			require.NotContains(t, recorder.Body.String(), "fixture-turn-state")
			require.NotContains(t, recorder.Body.String(), "response.failed")
		})
	}
}

func TestOpenAIStreamControlEOFBeforeOutputCanFailover(t *testing.T) {
	for _, mode := range []string{"native", "passthrough"} {
		for _, readErr := range []error{io.ErrUnexpectedEOF, context.DeadlineExceeded} {
			for _, prefix := range []string{streamControlPreamble, "data: {\"type\":\"codex.response.metadata\",\"headers\":{}}\n\n"} {
				t.Run(mode+"/"+readErr.Error()+"/"+gjson.Get(strings.TrimPrefix(prefix, "data: "), "type").String(), func(t *testing.T) {
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					body := &openAIResponseFlushReadError{payload: []byte(prefix), err: readErr}
					_, ttft, err := runStreamControlResponse(&OpenAIGatewayService{cfg: &config.Config{}}, mode, c, body)
					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
					require.Nil(t, ttft)
					require.Empty(t, recorder.Body.String())
				})
			}
		}
	}
}

func TestOpenAIStreamControlWatchdogKeepsHeartbeatsWithoutSemanticOutput(t *testing.T) {
	for _, mode := range []string{"native", "passthrough"} {
		t.Run(mode, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
				OpenAIFirstOutputTimeoutSeconds: 2, StreamKeepaliveInterval: 1,
			}}}
			body := newOpenAICompatBlockingReadCloser([]byte(streamControlPreamble + "event: response.in_progress\n"))
			defer func() { _ = body.Close() }()
			done := make(chan error, 1)
			go func() {
				_, _, err := runStreamControlResponse(svc, mode, c, body)
				done <- err
			}()
			var err error
			select {
			case err = <-done:
			case <-time.After(3 * time.Second):
				_ = body.Close()
				<-done
				t.Fatal("control frames disabled the first-output watchdog")
			}
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.Equal(t, http.StatusGatewayTimeout, failover.StatusCode)
			require.Contains(t, recorder.Body.String(), ":\n\n")
			require.NotContains(t, recorder.Body.String(), "data:")
			require.False(t, openAIStreamClientOutputStarted(c, false))
		})
	}
}

func TestOpenAIStreamControlNativeDataTimeoutBeforeOutputCanFailover(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	body := newOpenAICompatBlockingReadCloser([]byte(streamControlPreamble))
	defer func() { _ = body.Close() }()
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1, StreamKeepaliveInterval: 1}}}
	_, _, err := runStreamControlResponse(svc, "native", c, body)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.False(t, openAIStreamClientOutputStarted(c, false))
}

func TestOpenAIStreamControlFailureEndsHTTPWithoutWaitingForEOF(t *testing.T) {
	for _, mode := range []string{"native", "passthrough"} {
		for _, http2 := range []bool{false, true} {
			name := mode + "/http1"
			if http2 {
				name = mode + "/http2"
			}
			t.Run(name, func(t *testing.T) {
				// The upstream stays open after failure and keeps claiming progress.
				body := newOpenAICompatBlockingReadCloser([]byte(
					"data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n" +
						"data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_wire\",\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"generation failed\"},\"usage\":{\"input_tokens\":7,\"output_tokens\":2}}}\n\n" +
						":\n\ndata: {\"type\":\"response.in_progress\",\"response\":{\"id\":\"resp_wire\"}}\n\n"))
				defer func() { _ = body.Close() }()
				type outcome struct {
					usage *OpenAIUsage
					err   error
				}
				done := make(chan outcome, 1)
				router := gin.New()
				router.POST("/v1/responses", func(c *gin.Context) {
					defer func() { _ = body.Close() }()
					svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 1}}}
					usage, _, err := runStreamControlResponse(svc, mode, c, body)
					done <- outcome{usage, err}
				})
				server := httptest.NewUnstartedServer(router)
				server.EnableHTTP2 = http2
				server.StartTLS()
				defer server.Close()
				defer func() { _ = body.Close() }()
				ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
				defer cancel()
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v1/responses", nil)
				require.NoError(t, err)
				response, err := server.Client().Do(req)
				require.NoError(t, err)
				defer func() { _ = response.Body.Close() }()
				wire, readErr := io.ReadAll(response.Body)
				_ = body.Close()
				result := <-done
				require.NoError(t, readErr, "failure must end the HTTP body cleanly before another heartbeat")
				require.ErrorContains(t, result.err, "upstream response failed:")
				var failover *UpstreamFailoverError
				require.False(t, errors.As(result.err, &failover), "partial output must not be replayed")
				require.Equal(t, 7, result.usage.InputTokens)
				require.Equal(t, 2, result.usage.OutputTokens)
				require.Equal(t, 1, strings.Count(string(wire), `"type":"response.failed"`))
				require.NotContains(t, string(wire), "response.in_progress")
				require.True(t, strings.HasSuffix(string(wire), "\n\n"))
				if http2 {
					require.Equal(t, 2, response.ProtoMajor)
				}
			})
		}
	}
}

func TestOpenAIStreamControlErrorPairRetainsTerminalUsage(t *testing.T) {
	for _, mode := range []string{"native", "passthrough"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			body := io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n" +
					"data: {\"type\":\"error\",\"error\":{\"code\":\"server_error\",\"message\":\"temporary failure\"}}\n\n" +
					"data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_pair\",\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"temporary failure\"},\"usage\":{\"input_tokens\":9,\"output_tokens\":2}}}\n\n"))
			usage, _, err := runStreamControlResponse(&OpenAIGatewayService{cfg: &config.Config{}}, mode, c, body)
			require.ErrorContains(t, err, "upstream response failed:")
			require.Equal(t, 9, usage.InputTokens)
			require.Equal(t, 2, usage.OutputTokens)
		})
	}
}
