package service

import (
	"context"
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

func TestOpenAIStreamPoolRecoveryBudgetDoesNotRefillSameAccount(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ConfigureOpenAIPassthroughAttemptBudget(c, 1)
	for accountID := int64(1); accountID <= 7; accountID++ {
		UseOpenAIStreamPoolAccount(c, accountID)
		budget := openAIPassthroughBudgetForContext(c)
		for i := 0; i < defaultOpenAIPassthroughAttemptBudget(); i++ {
			_, _, ok := budget.reserve()
			require.True(t, ok)
		}
		UseOpenAIStreamPoolAccount(c, accountID)
		_, _, ok := budget.reserve()
		require.False(t, ok)
		require.True(t, budget.exhaustedError().ShouldRetryNextAccount())
	}
	require.False(t, newOpenAIPassthroughAttemptBudgetError().ShouldRetryNextAccount())
}

func TestOpenAIStreamPoolRecoveryHeaderHeartbeatSurvivesShortAttempts(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 1}},
		httpUpstream: &httpUpstreamRecorder{
			onDo: func() { time.Sleep(450 * time.Millisecond) },
			resp: &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))},
		},
	}
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/responses", nil)
		resp, err := svc.doOpenAIResponsesUpstream(context.Background(), c, req, "", &Account{ID: 1, Platform: PlatformOpenAI}, true)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	require.Contains(t, recorder.Body.String(), ":\n\n", "short attempts must not reset the request heartbeat")
	require.False(t, openAIStreamClientOutputStarted(c, false))
}
