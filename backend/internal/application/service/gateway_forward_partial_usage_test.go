package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newForwardPartialUsageServiceForTest(upstream *anthropicHTTPUpstreamRecorder) *GatewayService {
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	return &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
		rateLimitService:     &RateLimitService{},
		deferredService:      &DeferredService{},
	}
}

func newAnthropicOAuthAccountForPartialUsageTest() *Account {
	return &Account{
		ID:          501,
		Name:        "anthropic-oauth-partial-usage",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token"},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func TestGatewayService_Forward_StreamMissingTerminalPreservesPartialUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(`{"model":"claude-3-5-sonnet-latest","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)

	upstreamSSE := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":11,"cache_read_input_tokens":7}}}`,
		"",
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":null},"usage":{"output_tokens":5}}`,
		"",
		"",
	}, "\n")
	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"rid-partial"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamSSE)),
	}}

	result, err := newForwardPartialUsageServiceForTest(upstream).Forward(
		context.Background(), c, newAnthropicOAuthAccountForPartialUsageTest(), parsed,
	)
	require.ErrorContains(t, err, "missing terminal event")
	require.NotNil(t, result)
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.CacheReadInputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Equal(t, "rid-partial", result.RequestID)
}

func TestGatewayService_Forward_StreamErrorWithoutUsageReturnsNilResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	body := []byte(`{"model":"claude-3-5-sonnet-latest","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)
	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("event: ping\ndata: {\"type\":\"ping\"}\n\n")),
	}}

	result, err := newForwardPartialUsageServiceForTest(upstream).Forward(
		context.Background(), c, newAnthropicOAuthAccountForPartialUsageTest(), parsed,
	)
	require.ErrorContains(t, err, "missing terminal event")
	require.Nil(t, result)
}

func TestGatewayService_AnthropicAPIKeyPassthrough_StreamMissingTerminalPreservesPartialUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	body := []byte(`{"model":"claude-3-7-sonnet-20250219","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-3-7-sonnet-20250219", Stream: true}
	upstreamSSE := strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":9,"cache_read_input_tokens":2}}}`,
		"",
		`data: {"type":"message_delta","usage":{"output_tokens":3}}`,
		"",
	}, "\n")
	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
	}}

	result, err := newForwardPartialUsageServiceForTest(upstream).Forward(
		context.Background(), c, newAnthropicAPIKeyAccountForTest(), parsed,
	)
	require.ErrorContains(t, err, "missing terminal event")
	require.NotNil(t, result)
	require.Equal(t, 9, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.CacheReadInputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
}
