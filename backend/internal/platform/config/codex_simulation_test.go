package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadGatewayCodexSimulationDefaults(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.Gateway.CodexSimulation.FullSimulationEnabled)
	require.Equal(t, "off", cfg.Gateway.CodexSimulation.ContinuationMode)
	require.Empty(t, cfg.Gateway.CodexSimulation.IdentitySecret)
	require.Equal(t, 7*24*60*60, cfg.Gateway.CodexSimulation.StateTTLSeconds)
}

func TestLoadGatewayCodexSimulationFromEnvironment(t *testing.T) {
	resetViperWithJWTSecret(t)
	secret := strings.Repeat("e", 32)
	t.Setenv("GATEWAY_CODEX_SIMULATION_FULL_SIMULATION_ENABLED", "true")
	t.Setenv("GATEWAY_CODEX_SIMULATION_IDENTITY_SECRET", secret)
	t.Setenv("GATEWAY_CODEX_SIMULATION_CONTINUATION_MODE", "shadow")
	t.Setenv("GATEWAY_CODEX_SIMULATION_STATE_TTL_SECONDS", "3600")

	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.Gateway.CodexSimulation.FullSimulationEnabled)
	require.Equal(t, secret, cfg.Gateway.CodexSimulation.IdentitySecret)
	require.Equal(t, "shadow", cfg.Gateway.CodexSimulation.ContinuationMode)
	require.Equal(t, 3600, cfg.Gateway.CodexSimulation.StateTTLSeconds)
}

func TestValidateGatewayCodexSimulation(t *testing.T) {
	validSecret := strings.Repeat("s", 32)
	tests := []struct {
		name      string
		value     GatewayCodexSimulationConfig
		wantMode  string
		wantError string
	}{
		{name: "disabled", value: GatewayCodexSimulationConfig{ContinuationMode: "off", StateTTLSeconds: 1}, wantMode: "off"},
		{name: "empty mode normalizes off", value: GatewayCodexSimulationConfig{StateTTLSeconds: 1}, wantMode: "off"},
		{name: "shadow", value: GatewayCodexSimulationConfig{ContinuationMode: " SHADOW ", IdentitySecret: validSecret, StateTTLSeconds: 60}, wantMode: "shadow"},
		{name: "enforce", value: GatewayCodexSimulationConfig{ContinuationMode: "enforce", IdentitySecret: validSecret, StateTTLSeconds: 60}, wantMode: "enforce"},
		{name: "full simulation", value: GatewayCodexSimulationConfig{FullSimulationEnabled: true, ContinuationMode: "off", IdentitySecret: validSecret, StateTTLSeconds: 60}, wantMode: "off"},
		{name: "invalid mode", value: GatewayCodexSimulationConfig{ContinuationMode: "on", StateTTLSeconds: 60}, wantError: "off|shadow|enforce"},
		{name: "nonpositive ttl", value: GatewayCodexSimulationConfig{ContinuationMode: "off"}, wantError: "must be positive"},
		{name: "missing secret", value: GatewayCodexSimulationConfig{ContinuationMode: "shadow", StateTTLSeconds: 60}, wantError: "identity_secret is required"},
		{name: "short secret", value: GatewayCodexSimulationConfig{FullSimulationEnabled: true, ContinuationMode: "off", IdentitySecret: "short", StateTTLSeconds: 60}, wantError: "at least 32 bytes"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := &Config{Gateway: GatewayConfig{CodexSimulation: test.value}}
			err := validateGatewayCodexSimulation(cfg)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.wantMode, cfg.Gateway.CodexSimulation.ContinuationMode)
		})
	}
}
