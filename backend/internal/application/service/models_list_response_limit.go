package service

import "github.com/Wei-Shaw/sub2api/internal/platform/config"

// resolveModelsListReadLimit keeps all model-catalog readers on one bounded
// policy. A nil/zero config is common in unit fixtures, so it falls back to the
// safe default instead of accidentally disabling the limit.
func resolveModelsListReadLimit(cfg *config.Config) int64 {
	if cfg != nil && cfg.Gateway.ModelsListReadMaxBytes > 0 {
		return cfg.Gateway.ModelsListReadMaxBytes
	}
	return config.DefaultModelsListReadMaxBytes
}
