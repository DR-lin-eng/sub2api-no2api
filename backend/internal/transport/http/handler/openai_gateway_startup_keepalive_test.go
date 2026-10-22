package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesSlowAdmissionSendsHeadersAndTerminalFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(ctx context.Context, _ int64, _ int, _ string) (bool, error) {
			timer := time.NewTimer(1600 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-timer.C:
				return false, errors.New("fixture admission unavailable")
			}
		},
	}
	h := newOpenAIHandlerForPreviousResponseIDValidation(t, cache)
	cfg := &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 1}}
	h.gatewayService = service.NewOpenAIGatewayService(
		nil, nil, nil, nil, nil, nil, nil, cfg,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	router := gin.New()
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
			ID: 101, User: &service.User{ID: 1}, Group: &service.Group{Platform: service.PlatformOpenAI},
		})
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 1})
		h.Responses(c)
	})
	gateway := httptest.NewServer(router)
	defer gateway.Close()
	transport := &http.Transport{ResponseHeaderTimeout: 1300 * time.Millisecond}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	resp, err := client.Post(gateway.URL+"/v1/responses", "application/json", strings.NewReader(`{"model":"gpt-5.5","stream":true,"input":"hello"}`))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	wire, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(wire), `"type":"response.failed"`)
	require.NotContains(t, string(wire), "event: error")
}
