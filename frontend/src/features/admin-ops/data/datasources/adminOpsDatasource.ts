/**
 * Admin Ops API endpoints (vNext)
 * - Error logs list/detail
 * - Dashboard overview (raw path)
 */

import { apiClient, buildGatewayUrl } from '@/core/networks/client'
import type { PaginatedResponse } from '@/types'
import {
  getDashboardOverview,
  getDashboardSnapshotV2,
  getErrorDistribution,
  getErrorTrend,
  getLatencyHistogram,
  getSwitchTrend,
  getThroughputTrend
} from '@/features/admin-ops/data/datasources/opsDashboardQueries'
import {
  cleanupSystemLogs,
  resetRuntimeLogConfig,
  updateRuntimeLogConfig
} from '@/features/admin-ops/data/datasources/opsLogActions'
import {
  getRuntimeLogConfig,
  getSystemLogSinkHealth,
  listRequestDetails,
  listSystemLogs
} from '@/features/admin-ops/data/datasources/opsLogQueries'
import {
  getAccountAvailabilityStats,
  getConcurrencySnapshot,
  getConcurrencyStats,
  getImageGenerationStats,
  getOpenAITokenStats,
  getRealtimeTrafficSummary,
  getUserConcurrencyStats,
  getUserUsageStats
} from '@/features/admin-ops/data/datasources/opsMetricsQueries'

export * from '@/features/admin-ops/data/datasources/opsDashboardQueries'
export * from '@/features/admin-ops/data/datasources/opsLogActions'
export * from '@/features/admin-ops/data/datasources/opsLogQueries'
export * from '@/features/admin-ops/data/datasources/opsMetricsQueries'
export * from '@/features/admin-ops/data/dtos/opsDashboardDtos'
export * from '@/features/admin-ops/data/dtos/opsLogDtos'
export * from '@/features/admin-ops/data/dtos/opsMetricsDtos'

export type OpsUpstreamErrorEvent = {
  at_unix_ms?: number
  platform?: string
  account_id?: number
  account_name?: string
  upstream_status_code?: number
  upstream_request_id?: string
  kind?: string
  message?: string
  detail?: string
}

/**
 * Subscribe to realtime QPS updates via WebSocket.
 *
 * Note: browsers cannot set Authorization headers for WebSockets.
 * We authenticate via Sec-WebSocket-Protocol using a prefixed token item:
 *   ["sub2api-admin", "jwt.<token>"]
 */
export interface SubscribeQPSOptions {
  token?: string | null
  onOpen?: () => void
  onClose?: (event: CloseEvent) => void
  onError?: (event: Event) => void
  /**
   * Called when the server closes with an application close code that indicates
   * reconnecting is not useful (e.g. feature flag disabled).
   */
  onFatalClose?: (event: CloseEvent) => void
  /**
   * More granular status updates for UI (connecting/reconnecting/offline/etc).
   */
  onStatusChange?: (status: OpsWSStatus) => void
  /**
   * Called when a reconnect is scheduled (helps display "retry in Xs").
   */
  onReconnectScheduled?: (info: { attempt: number, delayMs: number }) => void
  wsBaseUrl?: string
  /**
   * Maximum reconnect attempts. Defaults to Infinity to keep the dashboard live.
   * Set to 0 to disable reconnect.
   */
  maxReconnectAttempts?: number
  reconnectBaseDelayMs?: number
  reconnectMaxDelayMs?: number
  /**
   * Stale connection detection (heartbeat-by-observation).
   * If no messages are received within this window, the socket is closed to trigger a reconnect.
   * Set to 0 to disable.
   */
  staleTimeoutMs?: number
  /**
   * How often to check staleness. Only used when `staleTimeoutMs > 0`.
   */
  staleCheckIntervalMs?: number
}

export type OpsWSStatus = 'connecting' | 'connected' | 'reconnecting' | 'offline' | 'closed'

export const OPS_WS_CLOSE_CODES = {
  REALTIME_DISABLED: 4001
} as const

const OPS_WS_BASE_PROTOCOL = 'sub2api-admin'

export function subscribeQPS(onMessage: (data: any) => void, options: SubscribeQPSOptions = {}): () => void {
  let ws: WebSocket | null = null
  let reconnectAttempts = 0
  const maxReconnectAttempts = Number.isFinite(options.maxReconnectAttempts as number)
    ? (options.maxReconnectAttempts as number)
    : Infinity
  const baseDelayMs = options.reconnectBaseDelayMs ?? 1000
  const maxDelayMs = options.reconnectMaxDelayMs ?? 30000
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let shouldReconnect = true
  let isConnecting = false
  let hasConnectedOnce = false
  let lastMessageAt = 0
  const staleTimeoutMs = options.staleTimeoutMs ?? 120_000
  const staleCheckIntervalMs = options.staleCheckIntervalMs ?? 30_000
  let staleTimer: ReturnType<typeof setInterval> | null = null

  const setStatus = (status: OpsWSStatus) => {
    options.onStatusChange?.(status)
  }

  const clearReconnectTimer = () => {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  const clearStaleTimer = () => {
    if (staleTimer) {
      clearInterval(staleTimer)
      staleTimer = null
    }
  }

  const startStaleTimer = () => {
    clearStaleTimer()
    if (!staleTimeoutMs || staleTimeoutMs <= 0) return
    staleTimer = setInterval(() => {
      if (!shouldReconnect) return
      if (!ws || ws.readyState !== WebSocket.OPEN) return
      if (!lastMessageAt) return
      const ageMs = Date.now() - lastMessageAt
      if (ageMs > staleTimeoutMs) {
        // Treat as a half-open connection; closing triggers the normal reconnect path.
        ws.close()
      }
    }, staleCheckIntervalMs)
  }

  const scheduleReconnect = () => {
    if (!shouldReconnect) return
    if (hasConnectedOnce && reconnectAttempts >= maxReconnectAttempts) return

    // If we're offline, wait for the browser to come back online.
    if (typeof navigator !== 'undefined' && 'onLine' in navigator && !navigator.onLine) {
      setStatus('offline')
      return
    }

    const expDelay = baseDelayMs * Math.pow(2, reconnectAttempts)
    const delay = Math.min(expDelay, maxDelayMs)
    const jitter = Math.floor(Math.random() * 250)
    clearReconnectTimer()
    reconnectTimer = setTimeout(() => {
      reconnectAttempts++
      connect()
    }, delay + jitter)
    options.onReconnectScheduled?.({ attempt: reconnectAttempts + 1, delayMs: delay + jitter })
  }

  const handleOnline = () => {
    if (!shouldReconnect) return
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return
    connect()
  }

  const handleOffline = () => {
    setStatus('offline')
  }

  const connect = () => {
    if (!shouldReconnect) return
    if (isConnecting) return
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return
    if (hasConnectedOnce && reconnectAttempts >= maxReconnectAttempts) return

    isConnecting = true
    setStatus(hasConnectedOnce ? 'reconnecting' : 'connecting')
    const wsBaseUrl = options.wsBaseUrl || import.meta.env.VITE_WS_BASE_URL
    const wsURL = wsBaseUrl
      ? new URL(`${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${wsBaseUrl}/api/v1/admin/ops/ws/qps`)
      : new URL(buildGatewayUrl('/api/v1/admin/ops/ws/qps').replace(/^http/, 'ws'))

    // Do NOT put admin JWT in the URL query string (it can leak via access logs, proxies, etc).
    // Browsers cannot set Authorization headers for WebSockets, so we pass the token via
    // Sec-WebSocket-Protocol (subprotocol list): ["sub2api-admin", "jwt.<token>"].
    const rawToken = String(options.token ?? localStorage.getItem('auth_token') ?? '').trim()
    const protocols: string[] = [OPS_WS_BASE_PROTOCOL]
    if (rawToken) protocols.push(`jwt.${rawToken}`)

    ws = new WebSocket(wsURL.toString(), protocols)

    ws.onopen = () => {
      reconnectAttempts = 0
      isConnecting = false
      hasConnectedOnce = true
      clearReconnectTimer()
      lastMessageAt = Date.now()
      startStaleTimer()
      setStatus('connected')
      options.onOpen?.()
    }

    ws.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data)
        lastMessageAt = Date.now()
        onMessage(data)
      } catch (err) {
        console.warn('[OpsWS] Failed to parse message:', err)
      }
    }

    ws.onerror = (error) => {
      console.error('[OpsWS] Connection error:', error)
      options.onError?.(error)
    }

    ws.onclose = (event) => {
      isConnecting = false
      options.onClose?.(event)
      clearStaleTimer()
      ws = null

      // If the server explicitly tells us to stop reconnecting, honor it.
      if (event && typeof event.code === 'number' && event.code === OPS_WS_CLOSE_CODES.REALTIME_DISABLED) {
        shouldReconnect = false
        clearReconnectTimer()
        setStatus('closed')
        options.onFatalClose?.(event)
        return
      }

      scheduleReconnect()
    }
  }

  window.addEventListener('online', handleOnline)
  window.addEventListener('offline', handleOffline)
  connect()

  return () => {
    shouldReconnect = false
    window.removeEventListener('online', handleOnline)
    window.removeEventListener('offline', handleOffline)
    clearReconnectTimer()
    clearStaleTimer()
    if (ws) ws.close()
    ws = null
    setStatus('closed')
  }
}

export type OpsSeverity = string
export type OpsPhase = string

export type AlertSeverity = 'critical' | 'warning' | 'info'
export type ThresholdMode = 'count' | 'percentage' | 'both'
export type MetricType =
  | 'success_rate'
  | 'error_rate'
  | 'upstream_error_rate'
  | 'cpu_usage_percent'
  | 'memory_usage_percent'
  | 'concurrency_queue_depth'
  | 'group_available_accounts'
  | 'group_available_ratio'
  | 'group_rate_limit_ratio'
  | 'account_rate_limited_count'
  | 'account_error_count'
  | 'account_error_ratio'
  | 'account_temp_unscheduled_count'
  | 'overload_account_count'
export type Operator = '>' | '>=' | '<' | '<=' | '==' | '!='

export interface AlertRule {
  id?: number
  name: string
  description?: string
  enabled: boolean
  metric_type: MetricType
  operator: Operator
  threshold: number
  window_minutes: number
  sustained_minutes: number
  severity: OpsSeverity
  cooldown_minutes: number
  notify_email: boolean
  filters?: Record<string, any>
  created_at?: string
  updated_at?: string
  last_triggered_at?: string | null
}

export interface AlertEvent {
  id: number
  rule_id: number
  severity: OpsSeverity | string
  status: 'firing' | 'resolved' | 'manual_resolved' | string
  title?: string
  description?: string
  metric_value?: number
  threshold_value?: number
  dimensions?: Record<string, any>
  fired_at: string
  resolved_at?: string | null
  email_sent: boolean
  created_at: string
}

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
  sla_percent_min?: number | null                 // SLA低于此值变红
  ttft_p99_ms_max?: number | null                 // TTFT P99高于此值变红
  request_error_rate_percent_max?: number | null  // 请求错误率高于此值变红
  upstream_error_rate_percent_max?: number | null // 上游错误率高于此值变红
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
  thresholds: OpsMetricThresholds // 指标阈值配置
}

export interface OpsOpenAIAccountQuotaAutoPauseSettings {
  default_threshold_5h: number // 0~1，0 表示不启用全局默认 5h 阈值
  default_threshold_7d: number // 0~1，0 表示不启用全局默认 7d 阈值
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

export interface OpsErrorLog {
  id: number
  created_at: string

  // Standardized classification
  phase: OpsPhase
  type: string
  error_owner: 'client' | 'provider' | 'platform' | string
  error_source: 'client_request' | 'upstream_http' | 'gateway' | string

  severity: OpsSeverity
  status_code: number
  platform: string
  model: string

  resolved: boolean
  resolved_at?: string | null
  resolved_by_user_id?: number | null

  client_request_id: string
  request_id: string
  message: string

  user_id?: number | null
  user_email: string
  api_key_id?: number | null
  // 关联 api_key 名称（后端 LEFT JOIN api_keys；软删保留 name，故已删 key 仍有原名）。
  api_key_name?: string
  api_key_deleted?: boolean
  account_id?: number | null
  account_name: string
  group_id?: number | null
  group_name: string

  client_ip?: string | null
  request_path?: string
  stream?: boolean

  // Error observability context (endpoint + model mapping)
  inbound_endpoint?: string
  upstream_endpoint?: string
  requested_model?: string
  upstream_model?: string
  request_type?: number | null
  user_agent?: string

}

export interface OpsErrorDetail extends OpsErrorLog {
  error_body: string

  // Upstream context (optional; enriched by gateway services)
  upstream_status_code?: number | null
  upstream_error_message?: string
  upstream_error_detail?: string
  upstream_errors?: string

  auth_latency_ms?: number | null
  routing_latency_ms?: number | null
  upstream_latency_ms?: number | null
  response_latency_ms?: number | null
  time_to_first_token_ms?: number | null

  is_business_limited: boolean

  // Bound (non-deleted) key prefix, snapshotted at error time
  api_key_prefix?: string | null
}

export type OpsErrorLogsResponse = PaginatedResponse<OpsErrorLog>

export type OpsErrorListView = 'errors' | 'excluded' | 'all'

export type OpsErrorListQueryParams = {
  page?: number
  page_size?: number
  time_range?: string
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  account_id?: number | null
  user_id?: number
  api_key_id?: number
  // 模型过滤：后端以 COALESCE(requested_model, model) 精确匹配（admin 路径）。
  model?: string

  phase?: string
  // 分类(用户侧粗分类码,如 auth/rate_limit/upstream),后端反查为 phase/type ANY 条件
  category?: string
  error_owner?: string
  error_source?: string
  resolved?: string
  view?: OpsErrorListView

  q?: string
  status_codes?: string
  status_codes_other?: string

  // 服务端排序,列白名单见后端 opsErrorLogsOrderBy(created_at/model/status_code)
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

// Legacy unified endpoints
export async function listErrorLogs(params: OpsErrorListQueryParams): Promise<OpsErrorLogsResponse> {
  const { data } = await apiClient.get<OpsErrorLogsResponse>('/admin/ops/errors', { params })
  return data
}

export async function getErrorLogDetail(id: number): Promise<OpsErrorDetail> {
  const { data } = await apiClient.get<OpsErrorDetail>(`/admin/ops/errors/${id}`)
  return data
}

export async function updateErrorResolved(errorId: number, resolved: boolean): Promise<void> {
  await apiClient.put(`/admin/ops/errors/${errorId}/resolve`, { resolved })
}

// New split endpoints
export async function listRequestErrors(params: OpsErrorListQueryParams): Promise<OpsErrorLogsResponse> {
  const { data } = await apiClient.get<OpsErrorLogsResponse>('/admin/ops/request-errors', { params })
  return data
}

export async function listUpstreamErrors(params: OpsErrorListQueryParams): Promise<OpsErrorLogsResponse> {
  const { data } = await apiClient.get<OpsErrorLogsResponse>('/admin/ops/upstream-errors', { params })
  return data
}

export async function getRequestErrorDetail(id: number): Promise<OpsErrorDetail> {
  const { data } = await apiClient.get<OpsErrorDetail>(`/admin/ops/request-errors/${id}`)
  return data
}

export async function getUpstreamErrorDetail(id: number): Promise<OpsErrorDetail> {
  const { data } = await apiClient.get<OpsErrorDetail>(`/admin/ops/upstream-errors/${id}`)
  return data
}

export async function updateRequestErrorResolved(errorId: number, resolved: boolean): Promise<void> {
  await apiClient.put(`/admin/ops/request-errors/${errorId}/resolve`, { resolved })
}

export async function updateUpstreamErrorResolved(errorId: number, resolved: boolean): Promise<void> {
  await apiClient.put(`/admin/ops/upstream-errors/${errorId}/resolve`, { resolved })
}

export async function listRequestErrorUpstreamErrors(
  id: number,
  params: OpsErrorListQueryParams = {},
  options: { include_detail?: boolean } = {}
): Promise<PaginatedResponse<OpsErrorDetail>> {
  const query: Record<string, any> = { ...params }
  if (options.include_detail) query.include_detail = '1'
  const { data } = await apiClient.get<PaginatedResponse<OpsErrorDetail>>(`/admin/ops/request-errors/${id}/upstream-errors`, { params: query })
  return data
}

// Alert rules
export async function listAlertRules(): Promise<AlertRule[]> {
  const { data } = await apiClient.get<AlertRule[]>('/admin/ops/alert-rules')
  return data
}

export async function createAlertRule(rule: AlertRule): Promise<AlertRule> {
  const { data } = await apiClient.post<AlertRule>('/admin/ops/alert-rules', rule)
  return data
}

export async function updateAlertRule(id: number, rule: Partial<AlertRule>): Promise<AlertRule> {
  const { data } = await apiClient.put<AlertRule>(`/admin/ops/alert-rules/${id}`, rule)
  return data
}

export async function deleteAlertRule(id: number): Promise<void> {
  await apiClient.delete(`/admin/ops/alert-rules/${id}`)
}

export interface AlertEventsQuery {
  limit?: number
  status?: string
  severity?: string
  email_sent?: boolean
  time_range?: string
  start_time?: string
  end_time?: string
  before_fired_at?: string
  before_id?: number
  platform?: string
  group_id?: number
}

export async function listAlertEvents(params: AlertEventsQuery = {}): Promise<AlertEvent[]> {
  const { data } = await apiClient.get<AlertEvent[]>('/admin/ops/alert-events', { params })
  return data
}

export async function getAlertEvent(id: number): Promise<AlertEvent> {
  const { data } = await apiClient.get<AlertEvent>(`/admin/ops/alert-events/${id}`)
  return data
}

export async function updateAlertEventStatus(id: number, status: 'resolved' | 'manual_resolved'): Promise<void> {
  await apiClient.put(`/admin/ops/alert-events/${id}/status`, { status })
}

export async function createAlertSilence(payload: {
  rule_id: number
  platform: string
  group_id?: number | null
  region?: string | null
  until: string
  reason?: string
}): Promise<void> {
  await apiClient.post('/admin/ops/alert-silences', payload)
}

// Email notification config
export async function getEmailNotificationConfig(): Promise<EmailNotificationConfig> {
  const { data } = await apiClient.get<EmailNotificationConfig>('/admin/ops/email-notification/config')
  return data
}

export async function getSettingsSnapshot(): Promise<OpsSettingsSnapshot> {
  const { data } = await apiClient.get<OpsSettingsSnapshot>('/admin/ops/settings/snapshot')
  return data
}

export async function updateEmailNotificationConfig(config: EmailNotificationConfig): Promise<EmailNotificationConfig> {
  const { data } = await apiClient.put<EmailNotificationConfig>('/admin/ops/email-notification/config', config)
  return data
}

// Runtime settings (DB-backed)
export async function getAlertRuntimeSettings(): Promise<OpsAlertRuntimeSettings> {
  const { data } = await apiClient.get<OpsAlertRuntimeSettings>('/admin/ops/runtime/alert')
  return data
}

export async function updateAlertRuntimeSettings(config: OpsAlertRuntimeSettings): Promise<OpsAlertRuntimeSettings> {
  const { data } = await apiClient.put<OpsAlertRuntimeSettings>('/admin/ops/runtime/alert', config)
  return data
}

// Advanced settings (DB-backed)
export async function getAdvancedSettings(): Promise<OpsAdvancedSettings> {
  const { data } = await apiClient.get<OpsAdvancedSettings>('/admin/ops/advanced-settings')
  return data
}

export async function updateAdvancedSettings(config: OpsAdvancedSettings): Promise<OpsAdvancedSettings> {
  const { data } = await apiClient.put<OpsAdvancedSettings>('/admin/ops/advanced-settings', config)
  return data
}

// ==================== Metric Thresholds ====================

async function getMetricThresholds(): Promise<OpsMetricThresholds> {
  const { data } = await apiClient.get<OpsMetricThresholds>('/admin/ops/settings/metric-thresholds')
  return data
}

async function updateMetricThresholds(thresholds: OpsMetricThresholds): Promise<void> {
  await apiClient.put('/admin/ops/settings/metric-thresholds', thresholds)
}

export const opsAPI = {
  getDashboardSnapshotV2,
  getDashboardOverview,
  getThroughputTrend,
  getSwitchTrend,
  getLatencyHistogram,
  getErrorTrend,
  getErrorDistribution,
  getImageGenerationStats,
  getOpenAITokenStats,
  getUserUsageStats,
  getConcurrencyStats,
  getConcurrencySnapshot,
  getUserConcurrencyStats,
  getAccountAvailabilityStats,
  getRealtimeTrafficSummary,
  subscribeQPS,

  // Legacy unified endpoints
  listErrorLogs,
  getErrorLogDetail,
  updateErrorResolved,

  // New split endpoints
  listRequestErrors,
  listUpstreamErrors,
  getRequestErrorDetail,
  getUpstreamErrorDetail,
  updateRequestErrorResolved,
  updateUpstreamErrorResolved,
  listRequestErrorUpstreamErrors,

  listRequestDetails,
  listAlertRules,
  createAlertRule,
  updateAlertRule,
  deleteAlertRule,
  listAlertEvents,
  getAlertEvent,
  updateAlertEventStatus,
  createAlertSilence,
  getSettingsSnapshot,
  getEmailNotificationConfig,
  updateEmailNotificationConfig,
  getAlertRuntimeSettings,
  updateAlertRuntimeSettings,
  getRuntimeLogConfig,
  updateRuntimeLogConfig,
  resetRuntimeLogConfig,
  getAdvancedSettings,
  updateAdvancedSettings,
  getMetricThresholds,
  updateMetricThresholds,
  listSystemLogs,
  cleanupSystemLogs,
  getSystemLogSinkHealth
}

export default opsAPI
