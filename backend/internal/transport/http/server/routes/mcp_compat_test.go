package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMCPDisabledNeedsNoRedisAndPreservesExistingIssuer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/.well-known/oauth-authorization-server", func(c *gin.Context) { c.JSON(200, gin.H{"issuer": "existing-oauth2"}) })
	h := mcpRouteHandlers()
	h.MCPOAuth = handler.NewMCPOAuthHandler(nil, nil, nil, nil, &config.Config{})
	v1 := r.Group("/api/v1")
	RegisterMCPRoutes(v1, h, middleware.AuditLogMiddleware(mcpRouteNext), middleware.StepUpAuthMiddleware(mcpRouteNext), nil, nil, nil, nil)
	RegisterMCPOAuthRoutes(r, v1, h, nil)
	for _, path := range []string{"/api/v1/mcp", "/api/v1/mcp/oauth/register", "/api/v1/mcp/oauth/token", "/api/v1/mcp/oauth/revoke"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
		require.Equal(t, http.StatusServiceUnavailable, w.Code, path)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"issuer":"existing-oauth2"}`, w.Body.String())
}
