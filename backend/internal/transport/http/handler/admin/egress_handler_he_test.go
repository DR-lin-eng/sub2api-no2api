package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	moduleegress "github.com/Wei-Shaw/sub2api/internal/modules/egress"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type heHandlerControlStore struct {
	snapshot moduleegress.HETunnelControlSnapshot
	action   string
}

func (s *heHandlerControlStore) Load(context.Context) (*moduleegress.HETunnelControlSnapshot, error) {
	copy := s.snapshot
	return &copy, nil
}

func (s *heHandlerControlStore) SaveConfig(_ context.Context, value moduleegress.HETunnelConfig) error {
	s.snapshot.Config = value
	return nil
}

func (s *heHandlerControlStore) Request(_ context.Context, action string) (string, error) {
	s.action = action
	return "request-1", nil
}

func newHETunnelHandlerTestRouter(store *heHandlerControlStore) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{IPv6Egress: config.IPv6EgressConfig{Enabled: true, ControlEnabled: true}}
	control := moduleegress.NewHETunnelControlService(store, cfg)
	handler := NewEgressHandler(nil, control, cfg)
	router := gin.New()
	router.GET("/he-tunnel", handler.GetHETunnel)
	router.PUT("/he-tunnel", handler.SaveHETunnel)
	router.POST("/he-tunnel/:action", handler.HETunnelAction)
	return router
}

func TestEgressHandlerHETunnelSaveMasksUpdateKey(t *testing.T) {
	store := &heHandlerControlStore{}
	router := newHETunnelHandlerTestRouter(store)
	body := `{
      "enabled":true,
      "server_ipv4":"216.66.80.30",
      "client_ipv6":"2001:470:1::2/64",
      "server_ipv6":"2001:470:1::1",
      "pool_cidr":"2001:470:2::/64",
      "mtu":1480,
      "route_metric":2048,
      "probe_ipv6":"2606:4700:4700::1111",
      "probe_timeout_seconds":5,
      "allow_private_ipv4":true,
      "update_enabled":true,
      "tunnel_id":"12345",
      "username":"operator",
      "update_key":"secret-update-key"
    }`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/he-tunnel", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "secret-update-key")
	require.Contains(t, recorder.Body.String(), `"update_key_configured":true`)
	require.Equal(t, "secret-update-key", store.snapshot.Config.UpdateKey)
}

func TestEgressHandlerHETunnelActionAccepted(t *testing.T) {
	store := &heHandlerControlStore{snapshot: moduleegress.HETunnelControlSnapshot{Config: moduleegress.HETunnelConfig{
		Enabled:             true,
		ServerIPv4:          "216.66.80.30",
		ClientIPv6:          "2001:470:1::2/64",
		ServerIPv6:          "2001:470:1::1",
		PoolCIDR:            "2001:470:2::/64",
		MTU:                 1480,
		RouteMetric:         2048,
		ProbeIPv6:           "2606:4700:4700::1111",
		ProbeTimeoutSeconds: 5,
		AllowPrivateIPv4:    true,
	}}}
	router := newHETunnelHandlerTestRouter(store)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/he-tunnel/apply", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.Equal(t, moduleegress.HETunnelActionApply, store.action)
	require.NotContains(t, recorder.Body.String(), "secret-update-key")
}
