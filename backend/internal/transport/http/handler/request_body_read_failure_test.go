package handler

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/httputil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestBodyReadDeadlineOverHTTP1(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := httputil.ReadRequestBodyWithPrealloc(r)
		status, message := requestBodyReadFailure(nil, r, err)
		http.Error(w, message, status)
	}))
	server.Config.ReadTimeout = 40 * time.Millisecond
	server.Start()
	defer server.Close()
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.SetDeadline(time.Now().Add(2*time.Second)))
	_, err = io.WriteString(conn, "POST /v1/responses HTTP/1.1\r\nHost: fixture.invalid\r\nContent-Length: 20\r\n\r\n{")
	require.NoError(t, err)
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusRequestTimeout, response.StatusCode)
}

func TestRequestBodyReadCancelledClientHasNoResponseBody(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx, cancel := context.WithCancel(c.Request.Context())
	cancel()
	c.Request = c.Request.WithContext(ctx)
	require.True(t, handleRequestBodyReadError(c, context.Canceled))
	require.Equal(t, 499, rec.Code)
	require.Empty(t, rec.Body.String())
}
