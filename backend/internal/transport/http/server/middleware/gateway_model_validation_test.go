package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayModelValidationBlocksBeforeForwardingWithoutAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"/v1/responses", "/v1/chat/completions", "/v1/messages", "/v1beta/models/gemini:generateContent"} {
		t.Run(path, func(t *testing.T) {
			forwarded := false
			router := gin.New()
			router.Use(GatewayModelValidation())
			router.POST(path, func(c *gin.Context) { forwarded = true; c.Status(http.StatusOK) })
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5.6-luna","model":"gpt-6-astra"}`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), "duplicate model")
			require.False(t, forwarded)
		})
	}
}

func TestGatewayModelValidationPreservesBodyAndDecompresses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const body = `{"model":"gpt-6-astra","input":"hello","tools":[{"model":"nested"}]}`
	router := gin.New()
	router.Use(GatewayModelValidation())
	router.POST("/v1/responses", func(c *gin.Context) {
		got, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.Equal(t, body, string(got))
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code)

	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write([]byte(`{"model":"cheap","model":"expensive"}`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", &compressed)
	req.Header.Set("Content-Encoding", "gzip")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	// A reader error must preserve the existing body-size contract.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Body = http.MaxBytesReader(w, req.Body, 2)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}
