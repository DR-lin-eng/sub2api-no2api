package config

import (
	"fmt"
	"strings"
)

func validateBilling(c *Config) error {
	if c.Billing.CircuitBreaker.Enabled {
		if c.Billing.CircuitBreaker.FailureThreshold <= 0 {
			return fmt.Errorf("billing.circuit_breaker.failure_threshold must be positive")
		}
		if c.Billing.CircuitBreaker.ResetTimeoutSeconds <= 0 {
			return fmt.Errorf("billing.circuit_breaker.reset_timeout_seconds must be positive")
		}
		if c.Billing.CircuitBreaker.HalfOpenRequests <= 0 {
			return fmt.Errorf("billing.circuit_breaker.half_open_requests must be positive")
		}
	}
	if c.Billing.MinimumBalanceReserve < 0 {
		return fmt.Errorf("billing.minimum_balance_reserve must be non-negative")
	}
	if c.Billing.Queue.Enabled {
		mode := c.Billing.Queue.ConsumerModeResolved()
		switch mode {
		case UsageBillingConsumerModeActive,
			UsageBillingConsumerModeAuto,
			UsageBillingConsumerModeStandby,
			UsageBillingConsumerModeProducerOnly:
		default:
			return fmt.Errorf("billing.queue.consumer_mode must be active, auto, standby, or producer_only")
		}
		if c.Billing.Queue.ConsumerCount <= 0 {
			return fmt.Errorf("billing.queue.consumer_count must be positive")
		}
		if c.Billing.Queue.MaxConsumerCount <= 0 {
			return fmt.Errorf("billing.queue.max_consumer_count must be positive")
		}
		if c.Billing.Queue.ConsumerCount > c.Billing.Queue.MaxConsumerCount {
			return fmt.Errorf("billing.queue.consumer_count cannot exceed max_consumer_count")
		}
		if c.Billing.Queue.ClusterMaxConsumers < 0 {
			return fmt.Errorf("billing.queue.cluster_max_consumers must be non-negative")
		}
		if c.Billing.Queue.ReadBatchSize <= 0 {
			return fmt.Errorf("billing.queue.read_batch_size must be positive")
		}
		if c.Billing.Queue.CleanupBatchSize <= 0 {
			return fmt.Errorf("billing.queue.cleanup_batch_size must be positive")
		}
		if c.Billing.Queue.ReadBlockMilliseconds <= 0 {
			return fmt.Errorf("billing.queue.read_block_milliseconds must be positive")
		}
		if c.Billing.Queue.CommandTimeoutSeconds <= 0 {
			return fmt.Errorf("billing.queue.command_timeout_seconds must be positive")
		}
		if c.Billing.Queue.MaxRetryDelaySeconds <= 0 {
			return fmt.Errorf("billing.queue.max_retry_delay_seconds must be positive")
		}
		if mode == UsageBillingConsumerModeAuto {
			auto := c.Billing.Queue.Auto
			if auto.MinConsumers < 0 || auto.MinConsumers > c.Billing.Queue.ConsumerCount {
				return fmt.Errorf("billing.queue.auto.min_consumers must be between 0 and consumer_count")
			}
			if auto.ScaleIntervalSeconds <= 0 || auto.ScaleCooldownSeconds <= 0 {
				return fmt.Errorf("billing.queue.auto scale intervals must be positive")
			}
			if auto.CPULowPercent < 0 || auto.CPUHighPercent <= auto.CPULowPercent || auto.CPUHighPercent > 100 {
				return fmt.Errorf("billing.queue.auto CPU thresholds are invalid")
			}
			if auto.InFlightHigh <= 0 || auto.UsageWorkerBacklogHigh <= 0 {
				return fmt.Errorf("billing.queue.auto load thresholds must be positive")
			}
			if auto.DBPoolWaitHighMilliseconds < 0 {
				return fmt.Errorf("billing.queue.auto.db_pool_wait_high_milliseconds must be non-negative")
			}
		}
		if c.Billing.Queue.Rescue.Enabled {
			rescue := c.Billing.Queue.Rescue
			if rescue.StaleAfterSeconds <= 0 || rescue.ScanIntervalSeconds <= 0 {
				return fmt.Errorf("billing.queue.rescue age and scan intervals must be positive")
			}
			if rescue.BatchSize <= 0 || rescue.CleanupBatchSize <= 0 {
				return fmt.Errorf("billing.queue.rescue batch sizes must be positive")
			}
			if rescue.ClusterMaxConcurrency <= 0 {
				return fmt.Errorf("billing.queue.rescue.cluster_max_concurrency must be positive")
			}
			if rescue.RetryAlertAttempts <= 0 || rescue.OldestAgeAlertSeconds <= 0 || rescue.ReconcileRetrySeconds <= 0 {
				return fmt.Errorf("billing.queue.rescue alert and reconcile thresholds must be positive")
			}
		}
	}
	return nil
}

func validateDataStores(c *Config) error {
	if c.Database.MaxOpenConns <= 0 {
		return fmt.Errorf("database.max_open_conns must be positive")
	}
	if c.Database.MaxIdleConns < 0 {
		return fmt.Errorf("database.max_idle_conns must be non-negative")
	}
	if c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		return fmt.Errorf("database.max_idle_conns cannot exceed database.max_open_conns")
	}
	if c.Database.ConnMaxLifetimeMinutes < 0 {
		return fmt.Errorf("database.conn_max_lifetime_minutes must be non-negative")
	}
	if c.Database.ConnMaxIdleTimeMinutes < 0 {
		return fmt.Errorf("database.conn_max_idle_time_minutes must be non-negative")
	}
	if c.Redis.DialTimeoutSeconds <= 0 {
		return fmt.Errorf("redis.dial_timeout_seconds must be positive")
	}
	if c.Redis.ReadTimeoutSeconds <= 0 {
		return fmt.Errorf("redis.read_timeout_seconds must be positive")
	}
	if c.Redis.WriteTimeoutSeconds <= 0 {
		return fmt.Errorf("redis.write_timeout_seconds must be positive")
	}
	if c.Redis.PoolSize <= 0 {
		return fmt.Errorf("redis.pool_size must be positive")
	}
	if c.Redis.MinIdleConns < 0 {
		return fmt.Errorf("redis.min_idle_conns must be non-negative")
	}
	if c.Redis.MinIdleConns > c.Redis.PoolSize {
		return fmt.Errorf("redis.min_idle_conns cannot exceed redis.pool_size")
	}
	if c.Redis.MaxIdleConns < 0 {
		return fmt.Errorf("redis.max_idle_conns must be non-negative")
	}
	return nil
}

func validateBatchImage(c *Config) error {
	if c.BatchImage.QueueEnabled {
		if strings.TrimSpace(c.BatchImage.QueueReadyKey) == "" {
			return fmt.Errorf("batch_image.queue_ready_key must not be empty")
		}
		if strings.TrimSpace(c.BatchImage.QueueDelayedKey) == "" {
			return fmt.Errorf("batch_image.queue_delayed_key must not be empty")
		}
		if strings.TrimSpace(c.BatchImage.QueueActiveKey) == "" {
			return fmt.Errorf("batch_image.queue_active_key must not be empty")
		}
		if strings.TrimSpace(c.BatchImage.InflightKeyPrefix) == "" {
			return fmt.Errorf("batch_image.inflight_key_prefix must not be empty")
		}
		if strings.TrimSpace(c.BatchImage.LockKeyPrefix) == "" {
			return fmt.Errorf("batch_image.lock_key_prefix must not be empty")
		}
		if c.BatchImage.InflightTTLSeconds <= 0 {
			return fmt.Errorf("batch_image.inflight_ttl_seconds must be positive")
		}
		if c.BatchImage.JobLockTTLSeconds <= 0 {
			return fmt.Errorf("batch_image.job_lock_ttl_seconds must be positive")
		}
		if c.BatchImage.StaleActiveAfterSeconds <= 0 {
			return fmt.Errorf("batch_image.stale_active_after_seconds must be positive")
		}
		if c.BatchImage.DelayedMoveLimit <= 0 {
			return fmt.Errorf("batch_image.delayed_move_limit must be positive")
		}
		if c.BatchImage.RecoverLimit <= 0 {
			return fmt.Errorf("batch_image.recover_limit must be positive")
		}
	}
	if c.BatchImage.VertexEnabled {
		if strings.TrimSpace(c.BatchImage.VertexManagedGCSBucket) == "" {
			return fmt.Errorf("batch_image.vertex_managed_gcs_bucket must not be empty when vertex is enabled")
		}
		if strings.Contains(c.BatchImage.VertexManagedGCSBucket, "://") {
			return fmt.Errorf("batch_image.vertex_managed_gcs_bucket must be a bucket name, not a URI")
		}
		if strings.TrimSpace(c.BatchImage.VertexLocation) == "" {
			return fmt.Errorf("batch_image.vertex_location must not be empty when vertex is enabled")
		}
		if strings.TrimSpace(c.BatchImage.VertexManagedGCSPrefix) == "" {
			return fmt.Errorf("batch_image.vertex_managed_gcs_prefix must not be empty when vertex is enabled")
		}
		if !strings.Contains(c.BatchImage.VertexManagedGCSPrefix, "{batch_id}") {
			return fmt.Errorf("batch_image.vertex_managed_gcs_prefix must contain {batch_id}")
		}
		if c.BatchImage.VertexInputRetentionHours <= 0 {
			return fmt.Errorf("batch_image.vertex_input_retention_hours must be positive")
		}
		if c.BatchImage.VertexOutputRetentionHours <= 0 {
			return fmt.Errorf("batch_image.vertex_output_retention_hours must be positive")
		}
	}
	return nil
}

func validateDashboard(c *Config) error {
	if c.Dashboard.Enabled {
		if c.Dashboard.StatsFreshTTLSeconds <= 0 {
			return fmt.Errorf("dashboard_cache.stats_fresh_ttl_seconds must be positive")
		}
		if c.Dashboard.StatsTTLSeconds <= 0 {
			return fmt.Errorf("dashboard_cache.stats_ttl_seconds must be positive")
		}
		if c.Dashboard.StatsRefreshTimeoutSeconds <= 0 {
			return fmt.Errorf("dashboard_cache.stats_refresh_timeout_seconds must be positive")
		}
		if c.Dashboard.StatsFreshTTLSeconds > c.Dashboard.StatsTTLSeconds {
			return fmt.Errorf("dashboard_cache.stats_fresh_ttl_seconds must be <= dashboard_cache.stats_ttl_seconds")
		}
	} else {
		if c.Dashboard.StatsFreshTTLSeconds < 0 {
			return fmt.Errorf("dashboard_cache.stats_fresh_ttl_seconds must be non-negative")
		}
		if c.Dashboard.StatsTTLSeconds < 0 {
			return fmt.Errorf("dashboard_cache.stats_ttl_seconds must be non-negative")
		}
		if c.Dashboard.StatsRefreshTimeoutSeconds < 0 {
			return fmt.Errorf("dashboard_cache.stats_refresh_timeout_seconds must be non-negative")
		}
	}
	if c.DashboardAgg.Enabled {
		if c.DashboardAgg.IntervalSeconds <= 0 {
			return fmt.Errorf("dashboard_aggregation.interval_seconds must be positive")
		}
		if c.DashboardAgg.LookbackSeconds < 0 {
			return fmt.Errorf("dashboard_aggregation.lookback_seconds must be non-negative")
		}
		if c.DashboardAgg.BackfillMaxDays < 0 {
			return fmt.Errorf("dashboard_aggregation.backfill_max_days must be non-negative")
		}
		if c.DashboardAgg.BackfillEnabled && c.DashboardAgg.BackfillMaxDays == 0 {
			return fmt.Errorf("dashboard_aggregation.backfill_max_days must be positive")
		}
		if c.DashboardAgg.Retention.UsageLogsDays <= 0 {
			return fmt.Errorf("dashboard_aggregation.retention.usage_logs_days must be positive")
		}
		if c.DashboardAgg.Retention.UsageBillingDedupDays <= 0 {
			return fmt.Errorf("dashboard_aggregation.retention.usage_billing_dedup_days must be positive")
		}
		if c.DashboardAgg.Retention.UsageBillingDedupDays < c.DashboardAgg.Retention.UsageLogsDays {
			return fmt.Errorf("dashboard_aggregation.retention.usage_billing_dedup_days must be greater than or equal to usage_logs_days")
		}
		if c.DashboardAgg.Retention.HourlyDays <= 0 {
			return fmt.Errorf("dashboard_aggregation.retention.hourly_days must be positive")
		}
		if c.DashboardAgg.Retention.DailyDays <= 0 {
			return fmt.Errorf("dashboard_aggregation.retention.daily_days must be positive")
		}
		if c.DashboardAgg.RecomputeDays < 0 {
			return fmt.Errorf("dashboard_aggregation.recompute_days must be non-negative")
		}
	} else {
		if c.DashboardAgg.IntervalSeconds < 0 {
			return fmt.Errorf("dashboard_aggregation.interval_seconds must be non-negative")
		}
		if c.DashboardAgg.LookbackSeconds < 0 {
			return fmt.Errorf("dashboard_aggregation.lookback_seconds must be non-negative")
		}
		if c.DashboardAgg.BackfillMaxDays < 0 {
			return fmt.Errorf("dashboard_aggregation.backfill_max_days must be non-negative")
		}
		if c.DashboardAgg.Retention.UsageLogsDays < 0 {
			return fmt.Errorf("dashboard_aggregation.retention.usage_logs_days must be non-negative")
		}
		if c.DashboardAgg.Retention.UsageBillingDedupDays < 0 {
			return fmt.Errorf("dashboard_aggregation.retention.usage_billing_dedup_days must be non-negative")
		}
		if c.DashboardAgg.Retention.UsageBillingDedupDays > 0 &&
			c.DashboardAgg.Retention.UsageLogsDays > 0 &&
			c.DashboardAgg.Retention.UsageBillingDedupDays < c.DashboardAgg.Retention.UsageLogsDays {
			return fmt.Errorf("dashboard_aggregation.retention.usage_billing_dedup_days must be greater than or equal to usage_logs_days")
		}
		if c.DashboardAgg.Retention.HourlyDays < 0 {
			return fmt.Errorf("dashboard_aggregation.retention.hourly_days must be non-negative")
		}
		if c.DashboardAgg.Retention.DailyDays < 0 {
			return fmt.Errorf("dashboard_aggregation.retention.daily_days must be non-negative")
		}
		if c.DashboardAgg.RecomputeDays < 0 {
			return fmt.Errorf("dashboard_aggregation.recompute_days must be non-negative")
		}
	}
	return nil
}

func validateUsageCleanup(c *Config) error {
	if c.UsageCleanup.Enabled {
		if c.UsageCleanup.MaxRangeDays <= 0 {
			return fmt.Errorf("usage_cleanup.max_range_days must be positive")
		}
		if c.UsageCleanup.BatchSize <= 0 {
			return fmt.Errorf("usage_cleanup.batch_size must be positive")
		}
		if c.UsageCleanup.WorkerIntervalSeconds <= 0 {
			return fmt.Errorf("usage_cleanup.worker_interval_seconds must be positive")
		}
		if c.UsageCleanup.TaskTimeoutSeconds <= 0 {
			return fmt.Errorf("usage_cleanup.task_timeout_seconds must be positive")
		}
	} else {
		if c.UsageCleanup.MaxRangeDays < 0 {
			return fmt.Errorf("usage_cleanup.max_range_days must be non-negative")
		}
		if c.UsageCleanup.BatchSize < 0 {
			return fmt.Errorf("usage_cleanup.batch_size must be non-negative")
		}
		if c.UsageCleanup.WorkerIntervalSeconds < 0 {
			return fmt.Errorf("usage_cleanup.worker_interval_seconds must be non-negative")
		}
		if c.UsageCleanup.TaskTimeoutSeconds < 0 {
			return fmt.Errorf("usage_cleanup.task_timeout_seconds must be non-negative")
		}
	}
	return nil
}

func validateIdempotency(c *Config) error {
	if c.Idempotency.DefaultTTLSeconds <= 0 {
		return fmt.Errorf("idempotency.default_ttl_seconds must be positive")
	}
	if c.Idempotency.SystemOperationTTLSeconds <= 0 {
		return fmt.Errorf("idempotency.system_operation_ttl_seconds must be positive")
	}
	if c.Idempotency.ProcessingTimeoutSeconds <= 0 {
		return fmt.Errorf("idempotency.processing_timeout_seconds must be positive")
	}
	if c.Idempotency.FailedRetryBackoffSeconds <= 0 {
		return fmt.Errorf("idempotency.failed_retry_backoff_seconds must be positive")
	}
	if c.Idempotency.MaxStoredResponseLen <= 0 {
		return fmt.Errorf("idempotency.max_stored_response_len must be positive")
	}
	if c.Idempotency.CleanupIntervalSeconds <= 0 {
		return fmt.Errorf("idempotency.cleanup_interval_seconds must be positive")
	}
	if c.Idempotency.CleanupBatchSize <= 0 {
		return fmt.Errorf("idempotency.cleanup_batch_size must be positive")
	}
	return nil
}
