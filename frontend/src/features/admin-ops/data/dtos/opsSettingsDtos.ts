import type { AlertSeverity } from '@/features/admin-ops/data/dtos/opsAlertDtos'
import type { OpsSeverity } from '@/features/admin-ops/data/dtos/opsErrorDtos'

export interface EmailNotificationConfig {
  alert: {
    enabled: boolean
    recipients: string[]
    min_severity: AlertSeverity | ''
    rate_limit_per_hour: number
    batching_window_seconds: number
    include_resolved_alerts: boolean
  }
  report: {
    enabled: boolean
    recipients: string[]
    daily_summary_enabled: boolean
    daily_summary_schedule: string
    weekly_summary_enabled: boolean
    weekly_summary_schedule: string
    error_digest_enabled: boolean
    error_digest_schedule: string
    error_digest_min_count: number
    account_health_enabled: boolean
    account_health_schedule: string
    account_health_error_rate_threshold: number
  }
}

export interface OpsMetricThresholds {
  sla_percent_min?: number | null
  ttft_p99_ms_max?: number | null
  request_error_rate_percent_max?: number | null
  upstream_error_rate_percent_max?: number | null
}

export interface OpsDistributedLockSettings {
  enabled: boolean
  key: string
  ttl_seconds: number
}

export interface OpsAlertRuntimeSettings {
  evaluation_interval_seconds: number
  distributed_lock: OpsDistributedLockSettings
  silencing: {
    enabled: boolean
    global_until_rfc3339: string
    global_reason: string
    entries?: Array<{
      rule_id?: number
      severities?: Array<OpsSeverity | string>
      until_rfc3339: string
      reason: string
    }>
  }
  thresholds: OpsMetricThresholds
}

export interface OpsOpenAIAccountQuotaAutoPauseSettings {
  default_threshold_5h: number
  default_threshold_7d: number
}

export interface OpsAdvancedSettings {
  data_retention: OpsDataRetentionSettings
  aggregation: OpsAggregationSettings
  openai_account_quota_auto_pause: OpsOpenAIAccountQuotaAutoPauseSettings
  ignore_count_tokens_errors: boolean
  ignore_context_canceled: boolean
  ignore_no_available_accounts: boolean
  ignore_invalid_api_key_errors: boolean
  ignore_insufficient_balance_errors: boolean
  record_business_limited_429: boolean
  display_openai_token_stats: boolean
  display_user_usage_stats: boolean
  display_alert_events: boolean
  display_system_logs: boolean
  display_concurrency: boolean
  display_switch_rate_trend: boolean
  display_throughput_trend: boolean
  display_network_bandwidth: boolean
  display_latency_histogram: boolean
  display_error_distribution: boolean
  display_error_trend: boolean
  display_image_generation_stats: boolean
  auto_refresh_enabled: boolean
  auto_refresh_interval_seconds: number
}

export interface OpsSettingsSnapshot {
  runtime: OpsAlertRuntimeSettings
  email: EmailNotificationConfig
  advanced: OpsAdvancedSettings
  metric_thresholds: OpsMetricThresholds
}

export interface OpsDataRetentionSettings {
  user_request_log_retention_days: number
  cleanup_enabled: boolean
  cleanup_schedule: string
  error_log_retention_days: number
  minute_metrics_retention_days: number
  hourly_metrics_retention_days: number
}

export interface OpsAggregationSettings {
  aggregation_enabled: boolean
}
