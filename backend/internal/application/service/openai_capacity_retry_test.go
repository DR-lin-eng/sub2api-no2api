package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIRequestScopedRetryDelay(t *testing.T) {
	for _, tc := range []struct {
		name, seconds, millis, body string
		want                        time.Duration
	}{
		{name: "seconds", seconds: " 6.25 ", want: 6250 * time.Millisecond},
		{name: "milliseconds", millis: "1250.5", want: 1250500 * time.Microsecond},
		{name: "header precedence", seconds: "6", millis: "7000", body: `{"error":{"headers":{"retry-after":8}}}`, want: 6 * time.Second},
		{name: "top-level body", body: `{"error":{"headers":{"retry-after":"6.5"}}}`, want: 6500 * time.Millisecond},
		{name: "response body", body: `{"response":{"error":{"headers":{"retry-after":7}}}}`, want: 7 * time.Second},
		{name: "escaped header keys", body: `{"error":{"he\u0061ders":{"retry\u002dafter":"1.25"}}}`, want: 1250 * time.Millisecond},
		{name: "top-level milliseconds", body: `{"error":{"headers":{"retry-after-ms":"1250"}}}`, want: 1250 * time.Millisecond},
		{name: "response milliseconds", body: `{"response":{"error":{"headers":{"retry-after-ms":7000}}}}`, want: 7 * time.Second},
		{name: "invalid header fallback", seconds: "invalid", body: `{"error":{"headers":{"retry-after-ms":1500}}}`, want: 1500 * time.Millisecond},
		{name: "zero fallback", seconds: "0", millis: "1000", want: time.Second},
		{name: "negative", seconds: "-1"},
		{name: "nan", seconds: "NaN"},
		{name: "infinite", seconds: "+Inf"},
		{name: "overflow", seconds: "1e500"},
		{name: "cap before conversion", seconds: "1e100", want: 8 * time.Second},
		{name: "milliseconds cap", millis: "1e100", want: 8 * time.Second},
		{name: "HTTP date cap", seconds: time.Now().Add(time.Minute).UTC().Format(http.TimeFormat), want: 8 * time.Second},
		{name: "expired HTTP date", seconds: time.Now().Add(-time.Minute).UTC().Format(http.TimeFormat)},
		{name: "invalid body", body: `invalid`},
		{name: "no hint"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{"Retry-After": []string{tc.seconds}, "Retry-After-Ms": []string{tc.millis}}
			require.Equal(t, tc.want, openAIRequestScopedRetryDelay(h, []byte(tc.body)))
		})
	}
	date := time.Now().Add(5 * time.Second).UTC().Format(http.TimeFormat)
	delay := openAIRequestScopedRetryDelay(http.Header{"Retry-After": []string{date}}, nil)
	require.Greater(t, delay, time.Duration(0))
	require.LessOrEqual(t, delay, 5*time.Second)
}

func TestOpenAICapacityRetryHintErrorScope(t *testing.T) {
	h := http.Header{"Retry-After": []string{"6"}}
	capacity := newOpenAIUpstreamFailoverError(503, h, []byte(`{"error":{"code":"server_is_overloaded"}}`), "", false)
	require.Equal(t, 6*time.Second, capacity.SameAccountRetryDelay)
	require.True(t, capacity.RetryableOnSameAccount)
	require.False(t, capacity.ShouldReportAccountScheduleFailure())
	ordinary := newOpenAIUpstreamFailoverError(400, h, []byte(`{"error":{"code":"invalid_request"}}`), "", false)
	require.Zero(t, ordinary.SameAccountRetryDelay)
	require.False(t, ordinary.RetryableOnSameAccount)
	statusOnly := newOpenAIWSOverloadFailoverError(h, []byte(`{"type":"error","status":503}`), "")
	require.Equal(t, 6*time.Second, statusOnly.SameAccountRetryDelay)
	require.Equal(t, GatewayFailureScopeRequest, statusOnly.Scope)
}

func TestOpenAICapacityRetryHintAfterEmptyEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, kind := range []string{
		"response.output_text.delta", "response.reasoning_summary_text.delta", "response.reasoning_text.delta",
		"response.function_call_arguments.delta", "response.custom_tool_call_input.delta", "response.refusal.delta",
		"response.output_text.done", "response.reasoning_summary_text.done", "response.reasoning_text.done", "response.refusal.done",
		"response.content_part.done", "response.reasoning_summary_part.done",
	} {
		for _, passthrough := range []bool{false, true} {
			name := kind + "/native"
			if passthrough {
				name = kind + "/passthrough"
			}
			t.Run(name, func(t *testing.T) {
				partType := "output_text"
				if strings.Contains(kind, "summary_part") {
					partType = "summary_text"
				}
				stream := "data: {\"type\":\"" + kind + "\",\"delta\":\"\",\"text\":\"\",\"refusal\":\"\",\"part\":{\"type\":\"" + partType + "\",\"text\":\"\"}}\n\n" +
					"data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"server_is_overloaded\",\"headers\":{\"retry-after-ms\":\"1250\"}}}}\n\n"
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}
				svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
				account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
				var err error
				if passthrough {
					_, err = svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "gpt-4.1", "gpt-4.1")
				} else {
					_, err = svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-4.1", "gpt-4.1")
				}
				var failure *UpstreamFailoverError
				require.ErrorAs(t, err, &failure)
				require.Equal(t, 1250*time.Millisecond, failure.SameAccountRetryDelay)
				require.True(t, failure.RetryableOnSameAccount)
				require.False(t, c.Writer.Written())
			})
		}
	}
}
