package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExplicitOpenAIHeaderSessionIDPrefersCodexHyphenHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("session-id", "codex-session")
	c.Request.Header.Set("session_id", "legacy-session")

	require.Equal(t, "codex-session", explicitOpenAIHeaderSessionID(c))
	resolution := resolveOpenAIWSSessionHeaders(c, "")
	require.Equal(t, "codex-session", resolution.SessionID)
	require.Equal(t, "header_session-id", resolution.SessionSource)
}

func TestClearActualOpenAIUpstreamEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	SetActualOpenAIUpstreamEndpoint(c, "/v1/chat/completions")
	require.Equal(t, "/v1/chat/completions", GetActualOpenAIUpstreamEndpoint(c))
	ClearActualOpenAIUpstreamEndpoint(c)
	require.Empty(t, GetActualOpenAIUpstreamEndpoint(c))
}
