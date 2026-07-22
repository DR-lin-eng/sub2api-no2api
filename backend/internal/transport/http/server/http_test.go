//go:build unit

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	clientip "github.com/Wei-Shaw/sub2api/internal/shared/ip"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAutoCompatResolverWorksWithoutGinTrustedProxyConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver, err := clientip.NewResolver(nil)
	require.NoError(t, err)

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.Use(resolver.Middleware())
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, clientip.GetClientIP(c))
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "172.18.0.1:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.42")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "203.0.113.42", recorder.Body.String())
}
