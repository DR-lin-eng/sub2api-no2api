package config

import (
	"fmt"
	"strings"
)

func validateOperations(c *Config) error {
	if c.Ops.MetricsCollectorCache.TTL < 0 {
		return fmt.Errorf("ops.metrics_collector_cache.ttl must be non-negative")
	}
	if c.Ops.Cleanup.ErrorLogRetentionDays < 0 {
		return fmt.Errorf("ops.cleanup.error_log_retention_days must be non-negative")
	}
	if c.Ops.Cleanup.MinuteMetricsRetentionDays < 0 {
		return fmt.Errorf("ops.cleanup.minute_metrics_retention_days must be non-negative")
	}
	if c.Ops.Cleanup.HourlyMetricsRetentionDays < 0 {
		return fmt.Errorf("ops.cleanup.hourly_metrics_retention_days must be non-negative")
	}
	if c.Ops.Cleanup.Enabled && strings.TrimSpace(c.Ops.Cleanup.Schedule) == "" {
		return fmt.Errorf("ops.cleanup.schedule is required when ops.cleanup.enabled=true")
	}
	return nil
}

func validateConcurrency(c *Config) error {
	if c.Concurrency.PingInterval != 0 && (c.Concurrency.PingInterval < 5 || c.Concurrency.PingInterval > 30) {
		return fmt.Errorf("concurrency.ping_interval must be 0 (disabled) or between 5-30 seconds")
	}
	return nil
}

func validateDingTalkConnect(c *Config) error {
	if err := ValidateDingTalkConfig(c.DingTalk); err != nil {
		return fmt.Errorf("dingtalk_connect: %w", err)
	}
	return nil
}
