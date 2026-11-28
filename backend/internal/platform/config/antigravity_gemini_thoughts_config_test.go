package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_AntigravityGeminiOmitIncludeThoughtsEnv(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")

	cfgDefault, err := Load()
	require.NoError(t, err)
	require.False(t, cfgDefault.Gateway.AntigravityGeminiOmitIncludeThoughts)

	t.Setenv("GATEWAY_ANTIGRAVITY_GEMINI_OMIT_INCLUDE_THOUGHTS", "true")
	cfgEnabled, err := Load()
	require.NoError(t, err)
	require.True(t, cfgEnabled.Gateway.AntigravityGeminiOmitIncludeThoughts)
}
