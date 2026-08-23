//go:build unit

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultOAuthModelSyncConfig(t *testing.T) {
	resetViperWithJWTSecret(t)
	for _, key := range []string{
		"OAUTH_MODEL_SYNC_ENABLED",
		"OAUTH_MODEL_SYNC_INTERVAL_MINUTES",
		"OAUTH_MODEL_SYNC_ACCOUNT_TIMEOUT_SECONDS",
		"OAUTH_MODEL_SYNC_MAX_CONCURRENT",
	} {
		t.Setenv(key, "")
	}
	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.OAuthModelSync.Enabled)
	require.Equal(t, 60, cfg.OAuthModelSync.IntervalMinutes)
	require.Equal(t, 20, cfg.OAuthModelSync.AccountTimeoutSeconds)
	require.Equal(t, 2, cfg.OAuthModelSync.MaxConcurrent)
}

func TestValidateOAuthModelSyncConfigBounds(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "interval too small", mutate: func(c *Config) { c.OAuthModelSync.IntervalMinutes = 0 }, want: "oauth_model_sync.interval_minutes"},
		{name: "timeout too large", mutate: func(c *Config) { c.OAuthModelSync.AccountTimeoutSeconds = 301 }, want: "oauth_model_sync.account_timeout_seconds"},
		{name: "concurrency too large", mutate: func(c *Config) { c.OAuthModelSync.MaxConcurrent = 33 }, want: "oauth_model_sync.max_concurrent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{OAuthModelSync: OAuthModelSyncConfig{
				Enabled:               true,
				IntervalMinutes:       60,
				AccountTimeoutSeconds: 20,
				MaxConcurrent:         2,
			}}
			tc.mutate(cfg)
			err := validateOAuthModelSync(cfg)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}
