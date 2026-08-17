package config

import (
	"fmt"
	"log/slog"
	"math"
	"strings"
)

func validateGatewayTransport(c *Config) error {
	if c.Gateway.MaxBodySize <= 0 {
		return fmt.Errorf("gateway.max_body_size must be positive")
	}
	if c.Gateway.TextMaxBodySize <= 0 || c.Gateway.TextMaxBodySize > c.Gateway.MaxBodySize {
		return fmt.Errorf("gateway.text_max_body_size must be positive and no greater than gateway.max_body_size")
	}
	if c.Gateway.UpstreamResponseReadMaxBytes <= 0 {
		return fmt.Errorf("gateway.upstream_response_read_max_bytes must be positive")
	}
	if c.Gateway.ProxyProbeResponseReadMaxBytes <= 0 {
		return fmt.Errorf("gateway.proxy_probe_response_read_max_bytes must be positive")
	}
	if c.Gateway.ResponseHeaderTimeout < 0 {
		return fmt.Errorf("gateway.response_header_timeout must be non-negative")
	}
	if c.Gateway.OpenAIFirstOutputTimeoutSeconds < 0 || c.Gateway.OpenAIFirstOutputTimeoutSeconds > 600 ||
		(c.Gateway.OpenAIFirstOutputTimeoutSeconds > 0 && c.Gateway.OpenAIFirstOutputTimeoutSeconds < 30) {
		return fmt.Errorf("gateway.openai_first_output_timeout_seconds must be 0 or between 30-600 seconds")
	}
	if c.Gateway.OpenAIHighEffortFirstOutputTimeoutSeconds < 0 || c.Gateway.OpenAIHighEffortFirstOutputTimeoutSeconds > 1800 ||
		(c.Gateway.OpenAIHighEffortFirstOutputTimeoutSeconds > 0 && c.Gateway.OpenAIHighEffortFirstOutputTimeoutSeconds < 30) {
		return fmt.Errorf("gateway.openai_high_effort_first_output_timeout_seconds must be 0 or between 30-1800 seconds")
	}
	if strings.TrimSpace(c.Gateway.ConnectionPoolIsolation) != "" {
		switch c.Gateway.ConnectionPoolIsolation {
		case ConnectionPoolIsolationProxy, ConnectionPoolIsolationAccount, ConnectionPoolIsolationAccountProxy:
		default:
			return fmt.Errorf("gateway.connection_pool_isolation must be one of: %s/%s/%s",
				ConnectionPoolIsolationProxy, ConnectionPoolIsolationAccount, ConnectionPoolIsolationAccountProxy)
		}
	}
	if c.Gateway.ImageConcurrency.MaxConcurrentRequests < 0 {
		return fmt.Errorf("gateway.image_concurrency.max_concurrent_requests must be non-negative")
	}
	switch strings.TrimSpace(c.Gateway.ImageConcurrency.OverflowMode) {
	case "", ImageConcurrencyOverflowModeReject, ImageConcurrencyOverflowModeWait:
	default:
		return fmt.Errorf("gateway.image_concurrency.overflow_mode must be one of: %s/%s",
			ImageConcurrencyOverflowModeReject, ImageConcurrencyOverflowModeWait)
	}
	if c.Gateway.ImageConcurrency.WaitTimeoutSeconds < 0 {
		return fmt.Errorf("gateway.image_concurrency.wait_timeout_seconds must be non-negative")
	}
	if c.Gateway.ImageConcurrency.MaxWaitingRequests < 0 {
		return fmt.Errorf("gateway.image_concurrency.max_waiting_requests must be non-negative")
	}
	if c.Gateway.MaxIdleConns <= 0 {
		return fmt.Errorf("gateway.max_idle_conns must be positive")
	}
	if c.Gateway.MaxIdleConnsPerHost <= 0 {
		return fmt.Errorf("gateway.max_idle_conns_per_host must be positive")
	}
	if c.Gateway.MaxConnsPerHost < 0 {
		return fmt.Errorf("gateway.max_conns_per_host must be non-negative")
	}
	if c.Gateway.IdleConnTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.idle_conn_timeout_seconds must be positive")
	}
	if c.Gateway.IdleConnTimeoutSeconds > 180 {
		slog.Warn("gateway.idle_conn_timeout_seconds is high; consider 60-120 seconds for better connection reuse", "idle_conn_timeout_seconds", c.Gateway.IdleConnTimeoutSeconds)
	}
	if c.Gateway.MaxUpstreamClients <= 0 {
		return fmt.Errorf("gateway.max_upstream_clients must be positive")
	}
	if c.Gateway.ClientIdleTTLSeconds <= 0 {
		return fmt.Errorf("gateway.client_idle_ttl_seconds must be positive")
	}
	if c.Gateway.ConcurrencySlotTTLMinutes <= 0 {
		return fmt.Errorf("gateway.concurrency_slot_ttl_minutes must be positive")
	}
	if c.Gateway.Live.MaxSessionDurationSeconds <= 0 {
		c.Gateway.Live.MaxSessionDurationSeconds = 3600
	}
	if c.Gateway.StreamDataIntervalTimeout < 0 {
		return fmt.Errorf("gateway.stream_data_interval_timeout must be non-negative")
	}
	if c.Gateway.StreamDataIntervalTimeout != 0 &&
		(c.Gateway.StreamDataIntervalTimeout < 30 || c.Gateway.StreamDataIntervalTimeout > 300) {
		return fmt.Errorf("gateway.stream_data_interval_timeout must be 0 or between 30-300 seconds")
	}
	if c.Gateway.StreamKeepaliveInterval < 0 {
		return fmt.Errorf("gateway.stream_keepalive_interval must be non-negative")
	}
	if c.Gateway.StreamKeepaliveInterval != 0 &&
		(c.Gateway.StreamKeepaliveInterval < 5 || c.Gateway.StreamKeepaliveInterval > 30) {
		return fmt.Errorf("gateway.stream_keepalive_interval must be 0 or between 5-30 seconds")
	}
	if c.Gateway.ImageStreamDataIntervalTimeout < 0 {
		return fmt.Errorf("gateway.image_stream_data_interval_timeout must be non-negative")
	}
	if c.Gateway.ImageStreamDataIntervalTimeout != 0 &&
		(c.Gateway.ImageStreamDataIntervalTimeout < 60 || c.Gateway.ImageStreamDataIntervalTimeout > 1800) {
		return fmt.Errorf("gateway.image_stream_data_interval_timeout must be 0 or between 60-1800 seconds")
	}
	if c.Gateway.ImageStreamKeepaliveInterval < 0 {
		return fmt.Errorf("gateway.image_stream_keepalive_interval must be non-negative")
	}
	if c.Gateway.ImageStreamKeepaliveInterval != 0 &&
		(c.Gateway.ImageStreamKeepaliveInterval < 5 || c.Gateway.ImageStreamKeepaliveInterval > 60) {
		return fmt.Errorf("gateway.image_stream_keepalive_interval must be 0 or between 5-60 seconds")
	}
	if c.Gateway.ImageNonstreamKeepaliveInterval < 0 {
		return fmt.Errorf("gateway.image_nonstream_keepalive_interval must be non-negative")
	}
	if c.Gateway.ImageNonstreamKeepaliveInterval != 0 &&
		(c.Gateway.ImageNonstreamKeepaliveInterval < 5 || c.Gateway.ImageNonstreamKeepaliveInterval > 60) {
		return fmt.Errorf("gateway.image_nonstream_keepalive_interval must be 0 or between 5-60 seconds")
	}
	return nil
}

func validateGatewayCodexSimulation(c *Config) error {
	mode := strings.ToLower(strings.TrimSpace(c.Gateway.CodexSimulation.ContinuationMode))
	if mode == "" {
		mode = "off"
	}
	switch mode {
	case "off", "shadow", "enforce":
	default:
		return fmt.Errorf("gateway.codex_simulation.continuation_mode must be one of off|shadow|enforce")
	}
	c.Gateway.CodexSimulation.ContinuationMode = mode

	if c.Gateway.CodexSimulation.StateTTLSeconds <= 0 {
		return fmt.Errorf("gateway.codex_simulation.state_ttl_seconds must be positive")
	}
	if c.Gateway.CodexSimulation.FullSimulationEnabled || mode != "off" {
		secret := strings.TrimSpace(c.Gateway.CodexSimulation.IdentitySecret)
		if secret == "" {
			return fmt.Errorf("gateway.codex_simulation.identity_secret is required when Codex simulation or continuation is enabled")
		}
		if len([]byte(secret)) < 32 {
			return fmt.Errorf("gateway.codex_simulation.identity_secret must be at least 32 bytes")
		}
		c.Gateway.CodexSimulation.IdentitySecret = secret
	}
	return nil
}

func validateGatewayOpenAIWebSocket(c *Config) error {
	// 兼容旧键 sticky_previous_response_ttl_seconds
	if c.Gateway.OpenAIWS.StickyResponseIDTTLSeconds <= 0 && c.Gateway.OpenAIWS.StickyPreviousResponseTTLSeconds > 0 {
		c.Gateway.OpenAIWS.StickyResponseIDTTLSeconds = c.Gateway.OpenAIWS.StickyPreviousResponseTTLSeconds
	}
	if c.Gateway.OpenAIWS.MaxConnsPerAccount <= 0 {
		return fmt.Errorf("gateway.openai_ws.max_conns_per_account must be positive")
	}
	if c.Gateway.OpenAIWS.ClientFirstMessageTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.openai_ws.client_first_message_timeout_seconds must be positive")
	}
	if c.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds < 0 {
		return fmt.Errorf("gateway.openai_ws.ingress_inter_turn_idle_timeout_seconds must be non-negative")
	}
	if c.Gateway.OpenAIWS.MaxIngressConnectionsPerAPIKey < 0 {
		return fmt.Errorf("gateway.openai_ws.max_ingress_connections_per_api_key must be non-negative")
	}
	if c.Gateway.OpenAIWS.MinIdlePerAccount < 0 {
		return fmt.Errorf("gateway.openai_ws.min_idle_per_account must be non-negative")
	}
	if c.Gateway.OpenAIWS.MaxIdlePerAccount < 0 {
		return fmt.Errorf("gateway.openai_ws.max_idle_per_account must be non-negative")
	}
	if c.Gateway.OpenAIWS.MinIdlePerAccount > c.Gateway.OpenAIWS.MaxIdlePerAccount {
		return fmt.Errorf("gateway.openai_ws.min_idle_per_account must be <= max_idle_per_account")
	}
	if c.Gateway.OpenAIWS.MaxIdlePerAccount > c.Gateway.OpenAIWS.MaxConnsPerAccount {
		return fmt.Errorf("gateway.openai_ws.max_idle_per_account must be <= max_conns_per_account")
	}
	if c.Gateway.OpenAIWS.OAuthMaxConnsFactor <= 0 {
		return fmt.Errorf("gateway.openai_ws.oauth_max_conns_factor must be positive")
	}
	if c.Gateway.OpenAIWS.APIKeyMaxConnsFactor <= 0 {
		return fmt.Errorf("gateway.openai_ws.apikey_max_conns_factor must be positive")
	}
	if c.Gateway.OpenAIWS.DialTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.openai_ws.dial_timeout_seconds must be positive")
	}
	if c.Gateway.OpenAIWS.ReadTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.openai_ws.read_timeout_seconds must be positive")
	}
	if c.Gateway.OpenAIWS.WriteTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.openai_ws.write_timeout_seconds must be positive")
	}
	if c.Gateway.OpenAIWS.PoolTargetUtilization <= 0 || c.Gateway.OpenAIWS.PoolTargetUtilization > 1 {
		return fmt.Errorf("gateway.openai_ws.pool_target_utilization must be within (0,1]")
	}
	if c.Gateway.OpenAIWS.QueueLimitPerConn <= 0 {
		return fmt.Errorf("gateway.openai_ws.queue_limit_per_conn must be positive")
	}
	if c.Gateway.OpenAIWS.EventFlushBatchSize <= 0 {
		return fmt.Errorf("gateway.openai_ws.event_flush_batch_size must be positive")
	}
	if c.Gateway.OpenAIWS.EventFlushIntervalMS < 0 {
		return fmt.Errorf("gateway.openai_ws.event_flush_interval_ms must be non-negative")
	}
	if c.Gateway.OpenAIWS.PrewarmCooldownMS < 0 {
		return fmt.Errorf("gateway.openai_ws.prewarm_cooldown_ms must be non-negative")
	}
	if c.Gateway.OpenAIWS.ClientReadLimitBytes <= 0 {
		return fmt.Errorf("gateway.openai_ws.client_read_limit_bytes must be positive")
	}
	if c.Gateway.OpenAIWS.HTTPBridgeThresholdBytes < 0 {
		return fmt.Errorf("gateway.openai_ws.http_bridge_threshold_bytes must be non-negative")
	}
	if c.Gateway.OpenAIWS.HTTPBridgeEnabled && c.Gateway.OpenAIWS.HTTPBridgeThresholdBytes == 0 {
		return fmt.Errorf("gateway.openai_ws.http_bridge_threshold_bytes must be positive when http_bridge_enabled is true")
	}
	if c.Gateway.OpenAIWS.FallbackCooldownSeconds < 0 {
		return fmt.Errorf("gateway.openai_ws.fallback_cooldown_seconds must be non-negative")
	}
	if c.Gateway.OpenAIWS.RetryBackoffInitialMS < 0 {
		return fmt.Errorf("gateway.openai_ws.retry_backoff_initial_ms must be non-negative")
	}
	if c.Gateway.OpenAIWS.RetryBackoffMaxMS < 0 {
		return fmt.Errorf("gateway.openai_ws.retry_backoff_max_ms must be non-negative")
	}
	if c.Gateway.OpenAIWS.RetryBackoffInitialMS > 0 && c.Gateway.OpenAIWS.RetryBackoffMaxMS > 0 &&
		c.Gateway.OpenAIWS.RetryBackoffMaxMS < c.Gateway.OpenAIWS.RetryBackoffInitialMS {
		return fmt.Errorf("gateway.openai_ws.retry_backoff_max_ms must be >= retry_backoff_initial_ms")
	}
	if c.Gateway.OpenAIWS.RetryJitterRatio < 0 || c.Gateway.OpenAIWS.RetryJitterRatio > 1 {
		return fmt.Errorf("gateway.openai_ws.retry_jitter_ratio must be within [0,1]")
	}
	if c.Gateway.OpenAIWS.RetryTotalBudgetMS < 0 {
		return fmt.Errorf("gateway.openai_ws.retry_total_budget_ms must be non-negative")
	}
	if mode := strings.ToLower(strings.TrimSpace(c.Gateway.OpenAIWS.IngressModeDefault)); mode != "" {
		switch mode {
		case "off", "ctx_pool", "passthrough", "http_bridge":
		case "shared", "dedicated":
			slog.Warn("gateway.openai_ws.ingress_mode_default is deprecated, treating as ctx_pool; please update to off|ctx_pool|passthrough|http_bridge", "value", mode)
		default:
			return fmt.Errorf("gateway.openai_ws.ingress_mode_default must be one of off|ctx_pool|passthrough|http_bridge")
		}
	}
	if mode := strings.ToLower(strings.TrimSpace(c.Gateway.OpenAIWS.StoreDisabledConnMode)); mode != "" {
		switch mode {
		case "strict", "adaptive", "off":
		default:
			return fmt.Errorf("gateway.openai_ws.store_disabled_conn_mode must be one of strict|adaptive|off")
		}
	}
	if c.Gateway.OpenAIWS.PayloadLogSampleRate < 0 || c.Gateway.OpenAIWS.PayloadLogSampleRate > 1 {
		return fmt.Errorf("gateway.openai_ws.payload_log_sample_rate must be within [0,1]")
	}
	if c.Gateway.OpenAIWS.LBTopK <= 0 {
		return fmt.Errorf("gateway.openai_ws.lb_top_k must be positive")
	}
	if c.Gateway.OpenAIWS.StickySessionTTLSeconds <= 0 {
		return fmt.Errorf("gateway.openai_ws.sticky_session_ttl_seconds must be positive")
	}
	if c.Gateway.OpenAIWS.StickyResponseIDTTLSeconds <= 0 {
		return fmt.Errorf("gateway.openai_ws.sticky_response_id_ttl_seconds must be positive")
	}
	if c.Gateway.OpenAIWS.StickyPreviousResponseTTLSeconds < 0 {
		return fmt.Errorf("gateway.openai_ws.sticky_previous_response_ttl_seconds must be non-negative")
	}
	return nil
}

func validateGatewayReliability(c *Config) error {
	if c.Gateway.OpenAIHTTP2.FallbackErrorThreshold < 0 {
		return fmt.Errorf("gateway.openai_http2.fallback_error_threshold must be non-negative")
	}
	if c.Gateway.OpenAIHTTP2.FallbackWindowSeconds < 0 {
		return fmt.Errorf("gateway.openai_http2.fallback_window_seconds must be non-negative")
	}
	if c.Gateway.OpenAIHTTP2.FallbackTTLSeconds < 0 {
		return fmt.Errorf("gateway.openai_http2.fallback_ttl_seconds must be non-negative")
	}
	if c.Gateway.OpenAIProxyStreamCircuit.FailureThreshold < 0 {
		return fmt.Errorf("gateway.openai_proxy_stream_circuit.failure_threshold must be non-negative")
	}
	if c.Gateway.OpenAIProxyStreamCircuit.WindowSeconds < 0 {
		return fmt.Errorf("gateway.openai_proxy_stream_circuit.window_seconds must be non-negative")
	}
	if c.Gateway.OpenAIProxyStreamCircuit.TTLSeconds < 0 {
		return fmt.Errorf("gateway.openai_proxy_stream_circuit.ttl_seconds must be non-negative")
	}
	if c.Gateway.OpenAIProxyStreamCircuit.Enabled &&
		(c.Gateway.OpenAIProxyStreamCircuit.FailureThreshold == 0 ||
			c.Gateway.OpenAIProxyStreamCircuit.WindowSeconds == 0 ||
			c.Gateway.OpenAIProxyStreamCircuit.TTLSeconds == 0) {
		return fmt.Errorf("gateway.openai_proxy_stream_circuit values must be positive when enabled")
	}
	weights := c.Gateway.OpenAIWS.SchedulerScoreWeights
	for _, weight := range []float64{
		weights.Priority, weights.Load, weights.Queue, weights.ErrorRate, weights.TTFT,
		weights.Reset, weights.QuotaHeadroom, weights.UpstreamCost,
		weights.PreviousResponse, weights.SessionSticky,
	} {
		if weight < 0 || math.IsNaN(weight) || math.IsInf(weight, 0) {
			return fmt.Errorf("gateway.openai_ws.scheduler_score_weights.* must be non-negative and finite")
		}
	}
	weightSum := weights.BaseWeightSum()
	if weightSum <= 0 {
		return fmt.Errorf("gateway.openai_ws.scheduler_score_weights must not all be zero")
	}
	if math.IsNaN(weightSum) || math.IsInf(weightSum, 0) {
		return fmt.Errorf("gateway.openai_ws.scheduler_score_weights base-weight sum must be finite")
	}
	if totalWeightSum := weights.TotalWeightSum(); math.IsNaN(totalWeightSum) || math.IsInf(totalWeightSum, 0) {
		return fmt.Errorf("gateway.openai_ws.scheduler_score_weights total-weight sum must be finite")
	}
	return nil
}

func validateGatewayRouting(c *Config) error {
	if c.Gateway.OpenAIScheduler.StickyEscapeTTFTMs <= 0 {
		return fmt.Errorf("gateway.openai_scheduler.sticky_escape_ttft_ms must be positive")
	}
	if c.Gateway.OpenAIScheduler.StickyEscapeErrorRate < 0 || c.Gateway.OpenAIScheduler.StickyEscapeErrorRate > 1 {
		return fmt.Errorf("gateway.openai_scheduler.sticky_escape_error_rate must be between 0 and 1")
	}
	if c.Gateway.MaxLineSize < 0 {
		return fmt.Errorf("gateway.max_line_size must be non-negative")
	}
	if c.Gateway.MaxLineSize != 0 && c.Gateway.MaxLineSize < 1024*1024 {
		return fmt.Errorf("gateway.max_line_size must be at least 1MB")
	}
	return nil
}

func validateGatewayUsageRecord(c *Config) error {
	if c.Gateway.UsageRecord.WorkerCount <= 0 {
		return fmt.Errorf("gateway.usage_record.worker_count must be positive")
	}
	if c.Gateway.UsageRecord.QueueSize <= 0 {
		return fmt.Errorf("gateway.usage_record.queue_size must be positive")
	}
	if c.Gateway.UsageRecord.TaskTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.usage_record.task_timeout_seconds must be positive")
	}
	switch strings.ToLower(strings.TrimSpace(c.Gateway.UsageRecord.OverflowPolicy)) {
	case UsageRecordOverflowPolicyDrop, UsageRecordOverflowPolicySample, UsageRecordOverflowPolicySync:
	default:
		return fmt.Errorf("gateway.usage_record.overflow_policy must be one of: %s/%s/%s",
			UsageRecordOverflowPolicyDrop, UsageRecordOverflowPolicySample, UsageRecordOverflowPolicySync)
	}
	if c.Gateway.UsageRecord.OverflowSamplePercent < 0 || c.Gateway.UsageRecord.OverflowSamplePercent > 100 {
		return fmt.Errorf("gateway.usage_record.overflow_sample_percent must be between 0-100")
	}
	if strings.EqualFold(strings.TrimSpace(c.Gateway.UsageRecord.OverflowPolicy), UsageRecordOverflowPolicySample) &&
		c.Gateway.UsageRecord.OverflowSamplePercent <= 0 {
		return fmt.Errorf("gateway.usage_record.overflow_sample_percent must be positive when overflow_policy=sample")
	}
	if c.Gateway.UsageRecord.AutoScaleEnabled {
		if c.Gateway.UsageRecord.AutoScaleMinWorkers <= 0 {
			return fmt.Errorf("gateway.usage_record.auto_scale_min_workers must be positive")
		}
		if c.Gateway.UsageRecord.AutoScaleMaxWorkers <= 0 {
			return fmt.Errorf("gateway.usage_record.auto_scale_max_workers must be positive")
		}
		if c.Gateway.UsageRecord.AutoScaleMaxWorkers < c.Gateway.UsageRecord.AutoScaleMinWorkers {
			return fmt.Errorf("gateway.usage_record.auto_scale_max_workers must be >= auto_scale_min_workers")
		}
		if c.Gateway.UsageRecord.WorkerCount < c.Gateway.UsageRecord.AutoScaleMinWorkers ||
			c.Gateway.UsageRecord.WorkerCount > c.Gateway.UsageRecord.AutoScaleMaxWorkers {
			return fmt.Errorf("gateway.usage_record.worker_count must be between auto_scale_min_workers and auto_scale_max_workers")
		}
		if c.Gateway.UsageRecord.AutoScaleUpQueuePercent <= 0 || c.Gateway.UsageRecord.AutoScaleUpQueuePercent > 100 {
			return fmt.Errorf("gateway.usage_record.auto_scale_up_queue_percent must be between 1-100")
		}
		if c.Gateway.UsageRecord.AutoScaleDownQueuePercent < 0 || c.Gateway.UsageRecord.AutoScaleDownQueuePercent >= 100 {
			return fmt.Errorf("gateway.usage_record.auto_scale_down_queue_percent must be between 0-99")
		}
		if c.Gateway.UsageRecord.AutoScaleDownQueuePercent >= c.Gateway.UsageRecord.AutoScaleUpQueuePercent {
			return fmt.Errorf("gateway.usage_record.auto_scale_down_queue_percent must be less than auto_scale_up_queue_percent")
		}
		if c.Gateway.UsageRecord.AutoScaleUpStep <= 0 {
			return fmt.Errorf("gateway.usage_record.auto_scale_up_step must be positive")
		}
		if c.Gateway.UsageRecord.AutoScaleDownStep <= 0 {
			return fmt.Errorf("gateway.usage_record.auto_scale_down_step must be positive")
		}
		if c.Gateway.UsageRecord.AutoScaleCheckIntervalSeconds <= 0 {
			return fmt.Errorf("gateway.usage_record.auto_scale_check_interval_seconds must be positive")
		}
		if c.Gateway.UsageRecord.AutoScaleCooldownSeconds < 0 {
			return fmt.Errorf("gateway.usage_record.auto_scale_cooldown_seconds must be non-negative")
		}
	}
	return nil
}

func validateGatewayCaches(c *Config) error {
	if c.Gateway.UserGroupRateCacheTTLSeconds <= 0 {
		return fmt.Errorf("gateway.user_group_rate_cache_ttl_seconds must be positive")
	}
	if c.Gateway.ModelsListCacheTTLSeconds < 10 || c.Gateway.ModelsListCacheTTLSeconds > 30 {
		return fmt.Errorf("gateway.models_list_cache_ttl_seconds must be between 10-30")
	}
	return nil
}

func validateGatewayScheduling(c *Config) error {
	if c.Gateway.Scheduling.StickySessionMaxWaiting <= 0 {
		return fmt.Errorf("gateway.scheduling.sticky_session_max_waiting must be positive")
	}
	if c.Gateway.Scheduling.StickySessionWaitTimeout <= 0 {
		return fmt.Errorf("gateway.scheduling.sticky_session_wait_timeout must be positive")
	}
	if c.Gateway.Scheduling.FallbackWaitTimeout <= 0 {
		return fmt.Errorf("gateway.scheduling.fallback_wait_timeout must be positive")
	}
	if c.Gateway.Scheduling.FallbackMaxWaiting <= 0 {
		return fmt.Errorf("gateway.scheduling.fallback_max_waiting must be positive")
	}
	if c.Gateway.Scheduling.LoadBatchCacheTTLMS < 0 {
		return fmt.Errorf("gateway.scheduling.load_batch_cache_ttl_ms must be non-negative")
	}
	if c.Gateway.Scheduling.SnapshotMGetChunkSize <= 0 {
		return fmt.Errorf("gateway.scheduling.snapshot_mget_chunk_size must be positive")
	}
	if c.Gateway.Scheduling.SnapshotWriteChunkSize <= 0 {
		return fmt.Errorf("gateway.scheduling.snapshot_write_chunk_size must be positive")
	}
	if c.Gateway.Scheduling.SlotCleanupInterval < 0 {
		return fmt.Errorf("gateway.scheduling.slot_cleanup_interval must be non-negative")
	}
	if c.Gateway.Scheduling.DbFallbackTimeoutSeconds < 0 {
		return fmt.Errorf("gateway.scheduling.db_fallback_timeout_seconds must be non-negative")
	}
	if c.Gateway.Scheduling.DbFallbackMaxQPS < 0 {
		return fmt.Errorf("gateway.scheduling.db_fallback_max_qps must be non-negative")
	}
	if c.Gateway.Scheduling.OutboxPollIntervalSeconds <= 0 {
		return fmt.Errorf("gateway.scheduling.outbox_poll_interval_seconds must be positive")
	}
	if c.Gateway.Scheduling.OutboxLagWarnSeconds < 0 {
		return fmt.Errorf("gateway.scheduling.outbox_lag_warn_seconds must be non-negative")
	}
	if c.Gateway.Scheduling.OutboxLagRebuildSeconds < 0 {
		return fmt.Errorf("gateway.scheduling.outbox_lag_rebuild_seconds must be non-negative")
	}
	if c.Gateway.Scheduling.OutboxLagRebuildFailures <= 0 {
		return fmt.Errorf("gateway.scheduling.outbox_lag_rebuild_failures must be positive")
	}
	if c.Gateway.Scheduling.OutboxBacklogRebuildRows < 0 {
		return fmt.Errorf("gateway.scheduling.outbox_backlog_rebuild_rows must be non-negative")
	}
	if c.Gateway.Scheduling.FullRebuildIntervalSeconds < 0 {
		return fmt.Errorf("gateway.scheduling.full_rebuild_interval_seconds must be non-negative")
	}
	if c.Gateway.Scheduling.OutboxLagWarnSeconds > 0 &&
		c.Gateway.Scheduling.OutboxLagRebuildSeconds > 0 &&
		c.Gateway.Scheduling.OutboxLagRebuildSeconds < c.Gateway.Scheduling.OutboxLagWarnSeconds {
		return fmt.Errorf("gateway.scheduling.outbox_lag_rebuild_seconds must be >= outbox_lag_warn_seconds")
	}
	return nil
}
