package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGroupModelAllowlistRejectsAndRestoresBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	apiKey := &service.APIKey{Group: &service.Group{
		ID: 1, Platform: service.PlatformOpenAI,
		ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.4"}},
	}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), apiKey)
		c.Next()
	})
	router.Use(GroupModelAllowlist())
	router.POST("/v1/responses", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		require.Contains(t, string(body), "gpt-5.4")
		c.Status(http.StatusOK)
	})

	allowed := httptest.NewRecorder()
	router.ServeHTTP(allowed, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4"}`)))
	require.Equal(t, http.StatusOK, allowed.Code)

	blocked := httptest.NewRecorder()
	router.ServeHTTP(blocked, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-4.1"}`)))
	require.Equal(t, http.StatusNotFound, blocked.Code)
	require.Contains(t, blocked.Body.String(), "gpt-4.1")
}

func TestGroupModelAllowlistChecksGeminiPathAndQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	apiKey := &service.APIKey{Group: &service.Group{
		ID: 1, Platform: service.PlatformGemini,
		ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"gemini-2.5-*"}},
	}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), apiKey)
		c.Next()
	})
	router.Use(GroupModelAllowlist())
	router.GET("/v1beta/models/:model", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/v1/realtime", func(c *gin.Context) { c.Status(http.StatusOK) })

	allowed := httptest.NewRecorder()
	router.ServeHTTP(allowed, httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-2.5-pro", nil))
	require.Equal(t, http.StatusOK, allowed.Code)
	blocked := httptest.NewRecorder()
	router.ServeHTTP(blocked, httptest.NewRequest(http.MethodGet, "/v1/realtime?model=grok-4.1", nil))
	require.Equal(t, http.StatusNotFound, blocked.Code)
}
