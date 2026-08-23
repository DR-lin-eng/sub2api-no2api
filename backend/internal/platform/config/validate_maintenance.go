package config

import "fmt"

func validateOAuthModelSync(c *Config) error {
	if c == nil || !c.OAuthModelSync.Enabled {
		return nil
	}
	if c.OAuthModelSync.IntervalMinutes < 1 || c.OAuthModelSync.IntervalMinutes > 7*24*60 {
		return fmt.Errorf("oauth_model_sync.interval_minutes must be between 1 and %d", 7*24*60)
	}
	if c.OAuthModelSync.AccountTimeoutSeconds < 1 || c.OAuthModelSync.AccountTimeoutSeconds > 300 {
		return fmt.Errorf("oauth_model_sync.account_timeout_seconds must be between 1 and 300")
	}
	if c.OAuthModelSync.MaxConcurrent < 1 || c.OAuthModelSync.MaxConcurrent > 32 {
		return fmt.Errorf("oauth_model_sync.max_concurrent must be between 1 and 32")
	}
	return nil
}
