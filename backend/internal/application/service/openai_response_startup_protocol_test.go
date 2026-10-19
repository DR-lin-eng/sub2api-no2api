package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesStartupKeepaliveTerminalErrorAndReuse(t *testing.T) {
	c, rec := newOpenAIPassthroughResilienceContext(nil)
	original := c.Writer
	stop := StartOpenAIResponsesSSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	wrapped := c.Writer
	nestedStop := StartOpenAIResponsesSSEKeepalive(c, keepaliveTestInterval)
	nestedStop()
	require.Same(t, wrapped, c.Writer, "Forward must reuse its handler's heartbeat")
	require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond)
	require.True(t, StopOpenAICompactSSEKeepaliveCommitted(c))
	require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c))
	require.False(t, openAIStreamClientOutputStarted(c, false))
	require.True(t, writeOpenAIResponsesErrorAfterKeepalive(c, http.StatusBadGateway, "upstream_error", "fixture error"))
	require.Contains(t, rec.Body.String(), `"type":"response.failed"`)
	require.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"response.failed"`))
	_, marked := GetOpsStreamError(c)
	require.True(t, marked, "a failure after heartbeat must still reach Ops")
	stop()
	require.Same(t, original, c.Writer)
}

func TestOpenAIResponsesStartupKeepalivePreservesFastHTTPError(t *testing.T) {
	c, rec := newOpenAIPassthroughResilienceContext(nil)
	stop := StartOpenAIResponsesSSEKeepalive(c, time.Hour)
	defer stop()
	require.False(t, StopOpenAICompactSSEKeepaliveCommitted(c))
	c.JSON(http.StatusTooManyRequests, gin.H{"error": gin.H{"message": "fixture limit"}})
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.NotContains(t, rec.Body.String(), ": keepalive")
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
}

func TestOpenAIResponsePreparationKeepaliveScope(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 1}}}
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		account *Account
		stream  bool
	}{
		{"nonstream", context.Background(), &Account{Platform: PlatformOpenAI}, false},
		{"image", WithOpenAIImageGenerationIntent(context.Background()), &Account{Platform: PlatformOpenAI}, true},
		{"other_provider", context.Background(), &Account{Platform: PlatformGrok}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			original := c.Writer
			stop := svc.startOpenAIResponsePreparationKeepalive(tc.ctx, c, tc.account, tc.stream)
			defer stop()
			require.Same(t, original, c.Writer)
			require.False(t, c.Writer.Written())
		})
	}
}
