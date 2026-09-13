//go:build unit

package service

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDetectGeminiResponseSignal(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		hit    bool
		kind   geminiResponseSignalKind
		reason string
		status int
	}{
		{name: "google error envelope", body: `{"error":{"code":503,"status":"UNAVAILABLE","message":"try later"}}`, hit: true, kind: geminiSignalError, reason: "UNAVAILABLE", status: 503},
		{name: "code assist wrapper", body: `{"response":{"promptFeedback":{"blockReason":"SAFETY"}}}`, hit: true, kind: geminiSignalPromptBlocked, reason: "SAFETY", status: 400},
		{name: "content filter finish", body: `{"candidates":[{"finishReason":"SAFETY"}]}`, hit: true, kind: geminiSignalContentFilter, reason: "SAFETY", status: 400},
		{name: "normal stop", body: `{"candidates":[{"finishReason":"STOP"}]}`, hit: false},
		{name: "model malformed call is not provider failure", body: `{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}]}`, hit: false},
		{name: "invalid json", body: `{"finishReason":`, hit: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sig, ok := detectGeminiResponseSignal([]byte(tc.body))
			require.Equal(t, tc.hit, ok)
			if tc.hit {
				require.Equal(t, tc.kind, sig.Kind)
				require.Equal(t, tc.reason, sig.Reason)
				require.Equal(t, tc.status, sig.Status)
			}
		})
	}
}

func TestGeminiResponseSignalBodyAndEmptySemantics(t *testing.T) {
	sig, ok := detectGeminiResponseSignalInBody([]byte(`[ {"candidates":[{"finishReason":"STOP"}]}, {"candidates":[{"finishReason":"SAFETY"}]} ]`))
	require.True(t, ok)
	require.Equal(t, geminiSignalContentFilter, sig.Kind)

	for _, body := range []string{"", "{}", `{"candidates":[]}`, `{"response":{"candidates":[]}}`, `[]`} {
		require.True(t, isGeminiEmptyResponseBody([]byte(body)), body)
	}
	for _, body := range []string{`{"promptFeedback":{"blockReason":"SAFETY"}}`, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`} {
		require.False(t, isGeminiEmptyResponseBody([]byte(body)), body)
	}
}

func TestGeminiSSEFallbackBodyBounded(t *testing.T) {
	fallback := &geminiSSEFallbackBody{}
	fallback.AddLine("  {\"error\":{\"status\":\"UNAVAILABLE\"}}  ")
	require.Contains(t, string(fallback.Bytes()), `"UNAVAILABLE"`)
	fallback.AddLine(string(bytes.Repeat([]byte{'x'}, geminiSSEFallbackBodyLimit)))
	require.True(t, fallback.Truncated())
}

func TestGeminiNativeNonStreamSignalIsRecordedWithoutChangingBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"candidates":[{"finishReason":"SAFETY"}]}`
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-test:generateContent", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(body))}
	svc := &GeminiMessagesCompatService{}
	_, err := svc.handleNativeNonStreamingResponseObserved(c, resp, false, &Account{ID: 7, Platform: PlatformGemini}, "req-1")
	require.NoError(t, err)
	require.JSONEq(t, body, rec.Body.String())
	streamErr, ok := GetOpsStreamError(c)
	require.True(t, ok)
	require.True(t, streamErr.RequestScoped)
	require.True(t, streamErr.NonStream)
	require.Equal(t, http.StatusBadRequest, streamErr.IntendedStatus)
}

func TestGeminiNativeStreamSignalPreservesWireAndMarksProviderFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := "data: {\"error\":{\"code\":503,\"status\":\"UNAVAILABLE\",\"message\":\"try later\"}}\n\n"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-test:streamGenerateContent", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(bytes.NewBufferString(body))}
	svc := &GeminiMessagesCompatService{}
	_, err := svc.handleNativeStreamingResponseObserved(c, resp, time.Now(), false, &Account{ID: 8, Platform: PlatformGemini}, "req-2")
	require.NoError(t, err)
	require.Contains(t, rec.Body.String(), body)
	streamErr, ok := GetOpsStreamError(c)
	require.True(t, ok)
	require.False(t, streamErr.RequestScoped)
	require.False(t, streamErr.NonStream)
	require.True(t, streamErr.CountTowardsSLA)
	require.Equal(t, http.StatusServiceUnavailable, streamErr.IntendedStatus)
	var upstream []*OpsUpstreamErrorEvent
	if value, exists := c.Get(OpsUpstreamErrorsKey); exists {
		upstream, _ = value.([]*OpsUpstreamErrorEvent)
	}
	require.Len(t, upstream, 1)
}
