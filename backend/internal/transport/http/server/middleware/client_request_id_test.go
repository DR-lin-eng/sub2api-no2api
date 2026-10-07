package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestClientRequestIDIssuesPrivateBillingIDForEveryIngress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestLogger(), ClientRequestID())
	var billingIDs []string
	router.POST("/v1/responses", func(c *gin.Context) {
		billingID, _ := c.Request.Context().Value(ctxkey.UsageBillingRequestID).(string)
		_, err := uuid.Parse(billingID)
		require.NoError(t, err)
		require.Equal(t, "anything", c.Request.Context().Value(ctxkey.RequestID))
		require.Equal(t, "anything", c.Request.Context().Value(ctxkey.ClientRequestID))
		billingIDs = append(billingIDs, billingID)
		c.Status(http.StatusOK)
	})
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`))
		req.Header.Set("X-Request-ID", "anything")
		req.Header.Set(clientRequestIDHeader, "anything")
		// An inherited correlation ID and even a previous private ID cannot
		// cause a separate ingress to reuse a settlement identity.
		ctx := context.WithValue(req.Context(), ctxkey.ClientRequestID, "anything")
		ctx = context.WithValue(ctx, ctxkey.UsageBillingRequestID, "previous-request")
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "anything", w.Header().Get("X-Request-ID"))
		require.Equal(t, "anything", w.Header().Get(clientRequestIDHeader))
	}
	require.Len(t, billingIDs, 2)
	require.NotEqual(t, billingIDs[0], billingIDs[1])
}

func TestClientRequestIDGeneratesAndExposesID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ClientRequestID())
	router.GET("/", func(c *gin.Context) {
		value, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string)
		c.String(http.StatusOK, value)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotEmpty(t, w.Body.String())
	require.Equal(t, w.Body.String(), w.Header().Get(clientRequestIDHeader))
}

func TestClientRequestIDBoundsExistingContextID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ClientRequestID())
	router.GET("/", func(c *gin.Context) {
		value, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string)
		c.String(http.StatusOK, value)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.ClientRequestID, strings.Repeat("x", 200)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Len(t, w.Body.String(), 36)
	require.NotEqual(t, strings.Repeat("x", maxPersistentRequestIDBytes), w.Body.String())
	require.Equal(t, w.Body.String(), w.Header().Get(clientRequestIDHeader))
}

func TestClientRequestIDPreservesExistingContextID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ClientRequestID())
	router.GET("/", func(c *gin.Context) {
		value, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string)
		c.String(http.StatusOK, value)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.ClientRequestID, "existing-client-request-id"))
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "existing-client-request-id", w.Body.String())
	require.Equal(t, "existing-client-request-id", w.Header().Get(clientRequestIDHeader))
}
