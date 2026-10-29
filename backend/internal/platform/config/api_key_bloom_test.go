package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAPIKeyBloomDefaultAndEnvironment(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "environment"}[enabled], func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("API_KEY_AUTH_CACHE_BLOOM_ENABLED", map[bool]string{false: "false", true: "true"}[enabled])
			cfg, err := Load()
			require.NoError(t, err)
			require.Equal(t, enabled, cfg.APIKeyAuth.Bloom.Enabled)
		})
	}
}
