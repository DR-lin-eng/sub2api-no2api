package config

import "github.com/spf13/viper"

func setBillingDefaults() {
	// Billing
	viper.SetDefault("billing.circuit_breaker.enabled", true)
	viper.SetDefault("billing.circuit_breaker.failure_threshold", 5)
	viper.SetDefault("billing.circuit_breaker.reset_timeout_seconds", 30)
	viper.SetDefault("billing.circuit_breaker.half_open_requests", 3)
	viper.SetDefault("billing.minimum_balance_reserve", 0.000001)
	viper.SetDefault("billing.user_platform_quota_cache_ttl_seconds", 86400)
	viper.SetDefault("billing.user_platform_quota_sentinel_ttl_seconds", 3600)
	// Billing jobs are committed to PostgreSQL WAL before acknowledgment; Redis
	// is only a rebuildable pending-usage overlay.
	viper.SetDefault("billing.queue.enabled", true)
	// Compatibility default: every enabled node keeps the existing fixed local
	// consumers until an operator explicitly opts into cluster-aware modes.
	viper.SetDefault("billing.queue.consumer_mode", UsageBillingConsumerModeActive)
	viper.SetDefault("billing.queue.consumer_count", 4)
	viper.SetDefault("billing.queue.max_consumer_count", 8)
	viper.SetDefault("billing.queue.cluster_max_consumers", 0)
	viper.SetDefault("billing.queue.read_batch_size", 128)
	viper.SetDefault("billing.queue.cleanup_batch_size", 128)
	viper.SetDefault("billing.queue.read_block_milliseconds", 1000)
	viper.SetDefault("billing.queue.command_timeout_seconds", 15)
	viper.SetDefault("billing.queue.max_retry_delay_seconds", 30)
	// Keep the pre-upgrade polling-only behavior by default. Redis wakeups are
	// an opt-in latency optimization and PostgreSQL polling remains the source
	// of truth when Redis is unavailable.
	viper.SetDefault("billing.queue.pubsub_wakeup_enabled", false)
	viper.SetDefault("billing.queue.auto.min_consumers", 1)
	viper.SetDefault("billing.queue.auto.scale_interval_seconds", 2)
	viper.SetDefault("billing.queue.auto.scale_cooldown_seconds", 10)
	viper.SetDefault("billing.queue.auto.cpu_high_percent", 75)
	viper.SetDefault("billing.queue.auto.cpu_low_percent", 50)
	viper.SetDefault("billing.queue.auto.in_flight_high", 128)
	viper.SetDefault("billing.queue.auto.usage_worker_backlog_high", 128)
	viper.SetDefault("billing.queue.auto.db_pool_wait_high_milliseconds", 100)
	viper.SetDefault("billing.queue.rescue.enabled", false)
	viper.SetDefault("billing.queue.rescue.stale_after_seconds", 60)
	viper.SetDefault("billing.queue.rescue.scan_interval_seconds", 5)
	viper.SetDefault("billing.queue.rescue.batch_size", 32)
	viper.SetDefault("billing.queue.rescue.cleanup_batch_size", 32)
	viper.SetDefault("billing.queue.rescue.cluster_max_concurrency", 1)
	viper.SetDefault("billing.queue.rescue.retry_alert_attempts", 10)
	viper.SetDefault("billing.queue.rescue.oldest_age_alert_seconds", 120)
	viper.SetDefault("billing.queue.rescue.reconcile_retry_seconds", 300)
}
