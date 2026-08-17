package egress

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type heControlStoreStub struct {
	snapshot HETunnelControlSnapshot
	saved    HETunnelConfig
	action   string
}

func (s *heControlStoreStub) Load(context.Context) (*HETunnelControlSnapshot, error) {
	copy := s.snapshot
	return &copy, nil
}

func (s *heControlStoreStub) SaveConfig(_ context.Context, value HETunnelConfig) error {
	s.saved = value
	s.snapshot.Config = value
	return nil
}

func (s *heControlStoreStub) Request(_ context.Context, action string) (string, error) {
	s.action = action
	return "request-1", nil
}

func enabledHEControlConfig() *config.Config {
	return &config.Config{IPv6Egress: config.IPv6EgressConfig{Enabled: true, ControlEnabled: true}}
}

func validHEControlInput() SaveHETunnelConfigInput {
	return SaveHETunnelConfigInput{
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
	}
}

func TestHETunnelControlSavePreservesAndMasksUpdateKey(t *testing.T) {
	store := &heControlStoreStub{snapshot: HETunnelControlSnapshot{Config: HETunnelConfig{UpdateKey: "existing-key"}}}
	service := NewHETunnelControlService(store, enabledHEControlConfig())
	input := validHEControlInput()
	input.UpdateEnabled = true
	input.TunnelID = "12345"
	input.Username = "operator"

	snapshot, err := service.Save(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, "existing-key", store.saved.UpdateKey)
	require.True(t, snapshot.Config.UpdateKeyConfigured)
	require.Empty(t, snapshot.Config.UpdateKey)
}

func TestHETunnelControlRejectsTunnelAndRoutedPrefixOverlap(t *testing.T) {
	store := &heControlStoreStub{}
	service := NewHETunnelControlService(store, enabledHEControlConfig())
	input := validHEControlInput()
	input.PoolCIDR = "2001:470:1::/64"

	_, err := service.Save(t.Context(), input)
	require.ErrorContains(t, err, "different from the tunnel /64")
}

func TestHETunnelControlRequestRequiresEnabledCompleteConfig(t *testing.T) {
	store := &heControlStoreStub{}
	service := NewHETunnelControlService(store, enabledHEControlConfig())

	_, err := service.Request(t.Context(), HETunnelActionApply)
	require.ErrorContains(t, err, "must be enabled")
	require.Empty(t, store.action)

	store.snapshot.Config = HETunnelConfig{Enabled: true}
	_, err = service.Request(t.Context(), HETunnelActionApply)
	require.ErrorContains(t, err, "is required")
	require.Empty(t, store.action)
}

func TestHETunnelControlUnavailableDoesNotTouchStore(t *testing.T) {
	store := &heControlStoreStub{}
	service := NewHETunnelControlService(store, &config.Config{})

	snapshot, err := service.Get(t.Context())
	require.NoError(t, err)
	require.False(t, snapshot.Available)
	require.Equal(t, "unavailable", snapshot.Agent.State)

	_, err = service.Save(t.Context(), validHEControlInput())
	require.ErrorIs(t, err, ErrHETunnelControlUnavailable)
}
