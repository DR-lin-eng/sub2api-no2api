//go:build unit

package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIStartupSSEKeepalive_AccumulatesBytesAcrossFailoverRestarts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","stream":true}`))

	for attempt := 1; attempt <= 3; attempt++ {
		stop := StartOpenAIResponsesSSEKeepalive(c, time.Hour)
		val, exists := c.Get(openAICompactSSEKeepaliveKey)
		require.True(t, exists)
		k := val.(*openAICompactSSEKeepalive)
		require.True(t, k.beat(), "attempt %d keepalive beat must succeed", attempt)
		stop()

		require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c),
			"attempt %d: keepalive-only output must keep semantic adjusted size at -1 so failover remains allowed", attempt)
		require.True(t, StopOpenAICompactSSEKeepaliveCommitted(c),
			"attempt %d: request must remember that HTTP 200 SSE headers were committed", attempt)
	}

	require.Equal(t, 3*len(": keepalive\n\n"), rec.Body.Len())

	// Once real semantic SSE output is written, adjusted size reflects only the semantic bytes.
	semantic := "event: response.created\ndata: {\"type\":\"response.created\"}\n\n"
	_, err := c.Writer.WriteString(semantic)
	require.NoError(t, err)
	require.Equal(t, len(semantic), OpenAICompactKeepaliveAdjustedWrittenSize(c))
}
