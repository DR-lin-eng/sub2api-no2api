package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMinOutputTokensValidation(t *testing.T) {
	for _, n := range []int{-1, 0, 15} {
		err := validateGatewayTransport(&Config{Gateway: GatewayConfig{MinOutputTokens: n}})
		require.ErrorContains(t, err, "gateway.min_output_tokens")
	}
	err := validateGatewayTransport(&Config{Gateway: GatewayConfig{MinOutputTokens: 16}})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "min_output_tokens")
}
