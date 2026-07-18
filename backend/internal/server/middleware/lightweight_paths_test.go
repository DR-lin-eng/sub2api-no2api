package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCredentialKeyPathSkipsRequestLoggingAndSessionBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := initMiddlewareTestLogger(t)
	router := gin.New()
	router.Use(RequestLogger())
	router.Use(SessionBindingContext(true))
	router.Use(Logger())
	router.GET(credentialKeyRequestPath, func(c *gin.Context) {
		_, hasRequestID := c.Request.Context().Value(ctxkey.RequestID).(string)
		require.False(t, hasRequestID)
		require.Nil(t, service.SessionBindingFromContext(c.Request.Context()))
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, credentialKeyRequestPath, nil))

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Empty(t, w.Header().Get(requestIDHeader))
	require.Empty(t, sink.list())
}
