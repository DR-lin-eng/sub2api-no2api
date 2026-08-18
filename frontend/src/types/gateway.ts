import type { Group } from './group'

export * from './group'
export type {
  AdminGroup,
  CompositeModelRoute,
  CompositeModelRouteInput,
  CompositeRouteDecision,
  CompositeRouteEndpoint,
  CompositeRouteMatchType,
  CompositeRoutePreviewRequest,
  CompositeRouteSource,
  CreateGroupRequest,
  UpdateGroupRequest,
} from '@/features/admin-groups/data/dtos/adminGroupDtos'

export interface ApiKey {
  id: number
  user_id: number
  key: string
  name: string
  group_id: number | null
  group_bindings: ApiKeyGroupBinding[]
  status: 'active' | 'inactive' | 'quota_exhausted' | 'expired'
  ip_whitelist: string[]
  ip_blacklist: string[]
  last_used_at: string | null
  last_used_ip: string | null
  quota: number // Quota limit in USD (0 = unlimited)
  quota_used: number // Used quota amount in USD
  expires_at: string | null // Expiration time (null = never expires)
  created_at: string
  updated_at: string
  concurrency_limit: number // Maximum concurrent requests (0 = unlimited)
  current_concurrency: number
  group?: Group
  rate_limit_5h: number
  rate_limit_1d: number
  rate_limit_7d: number
  usage_5h: number
  usage_1d: number
  usage_7d: number
  window_5h_start: string | null
  window_1d_start: string | null
  window_7d_start: string | null
  reset_5h_at: string | null
  reset_1d_at: string | null
  reset_7d_at: string | null
}

export interface ApiKeyGroupBinding {
  group_id: number
  max_rate_multiplier: number | null
}

export interface CreateApiKeyRequest {
  name: string
  group_id?: number | null
  group_bindings?: ApiKeyGroupBinding[]
  custom_key?: string // Optional custom API Key
  ip_whitelist?: string[]
  ip_blacklist?: string[]
  quota?: number // Quota limit in USD (0 = unlimited)
  expires_in_days?: number // Days until expiry (null = never expires)
  rate_limit_5h?: number
  rate_limit_1d?: number
  rate_limit_7d?: number
}

export interface UpdateApiKeyRequest {
  name?: string
  group_id?: number | null
  group_bindings?: ApiKeyGroupBinding[]
  status?: 'active' | 'inactive'
  ip_whitelist?: string[]
  ip_blacklist?: string[]
  quota?: number // Quota limit in USD (null = no change, 0 = unlimited)
  expires_at?: string | null // Expiration time (null = no change)
  reset_quota?: boolean // Reset quota_used to 0
  concurrency_limit?: number // Maximum concurrent requests (0 = unlimited)
  rate_limit_5h?: number
  rate_limit_1d?: number
  rate_limit_7d?: number
  reset_rate_limit_usage?: boolean
}

// ==================== Account & Proxy Types ====================

export type AccountPlatform = 'anthropic' | 'openai' | 'gemini' | 'antigravity' | 'grok'
export type AccountType = 'oauth' | 'setup-token' | 'apikey' | 'upstream' | 'bedrock' | 'service_account'
export type OAuthAddMethod = 'oauth' | 'setup-token'
export type ProxyProtocol = 'http' | 'https' | 'socks5' | 'socks5h'

export interface Proxy {
  id: number
  name: string
  protocol: ProxyProtocol
  host: string
  port: number
  username: string | null
  password?: string | null
  status: 'active' | 'inactive' | 'expired'
  account_count?: number // Number of accounts using this proxy
  latency_ms?: number
  latency_status?: 'success' | 'failed'
  latency_message?: string
  ip_address?: string
  country?: string
  country_code?: string
  region?: string
  city?: string
  quality_status?: 'healthy' | 'warn' | 'challenge' | 'failed'
  quality_score?: number
  quality_grade?: string
  quality_summary?: string
  quality_checked?: number
  expires_at: string | null
  fallback_mode: 'none' | 'proxy' | 'direct'
  backup_proxy_id?: number | null
  expiry_warn_days: number
  created_at: string
  updated_at: string
}

export interface ProxyAccountSummary {
  id: number
  name: string
  platform: AccountPlatform
  type: AccountType
  notes?: string | null
}

export interface ProxyQualityCheckItem {
  target: string
  status: 'pass' | 'warn' | 'fail' | 'challenge'
  http_status?: number
  latency_ms?: number
  message?: string
  cf_ray?: string
}

export interface ProxyQualityCheckResult {
  proxy_id: number
  score: number
  grade: string
  summary: string
  exit_ip?: string
  country?: string
  country_code?: string
  base_latency_ms?: number
  passed_count: number
  warn_count: number
  failed_count: number
  challenge_count: number
  checked_at: number
  items: ProxyQualityCheckItem[]
}

// Gemini credentials structure for OAuth and API Key authentication
export interface GeminiCredentials {
  // API Key authentication
  api_key?: string

  // OAuth authentication
  access_token?: string
  refresh_token?: string
  oauth_type?: 'code_assist' | 'google_one' | 'ai_studio' | string
  tier_id?:
    | 'google_one_free'
    | 'google_ai_pro'
    | 'google_ai_ultra'
    | 'gcp_standard'
    | 'gcp_enterprise'
    | 'aistudio_free'
    | 'aistudio_paid'
    | 'LEGACY'
    | 'PRO'
    | 'ULTRA'
    | string
  project_id?: string
  token_type?: string
  scope?: string
  expires_at?: string
  model_mapping?: Record<string, string>
}

export interface UpstreamBillingData {
  object: 'sub2api.key_billing'
  schema_version: 1
  billing_scope: 'token'
  group_rate_multiplier: number
  user_rate_multiplier?: number
  resolved_rate_multiplier: number
  peak_rate_enabled: boolean
  peak_start?: string
  peak_end?: string
  peak_rate_multiplier?: number
  applied_peak_multiplier?: number
  effective_rate_multiplier: number
  timezone?: string
  observed_at: string
}

export type UpstreamBillingProbeStatus = 'ok' | 'unsupported' | 'failed'

export interface UpstreamBillingProbeSnapshot {
  status: UpstreamBillingProbeStatus
  data?: UpstreamBillingData
  received_at?: string
  fresh_until?: string
  last_attempt_at: string
  next_probe_at: string
  failure_count?: number
  http_status?: number
  last_error?: string
  synced_rate_multiplier?: number
}

export interface UpstreamBillingProbeSettings {
  enabled: boolean
  interval_minutes: number
}

export interface UpstreamBillingProbeResult {
  account_id: number
  snapshot?: UpstreamBillingProbeSnapshot
  error?: string
}

export interface UpstreamBillingRateSnapshotItem {
  account_id: number
  snapshot?: UpstreamBillingProbeSnapshot | null
}

export interface UpstreamBillingRatesResponse {
  items: UpstreamBillingRateSnapshotItem[]
  total: number
  page: number
  page_size: number
}

export type UpstreamQuotaProvider = 'sub2api' | 'new_api'
export type UpstreamQuotaMode = 'balance' | 'quota' | 'subscription' | 'rate_limits'
export type UpstreamQuotaUnit = 'USD' | 'CNY' | 'TOKENS'

export interface UpstreamQuotaWindow {
  name: string
  used?: number | null
  limit?: number | null
  remaining?: number | null
  reset_at?: string | null
}

export interface UpstreamSubscriptionInfo {
  plan_name: string
  remaining?: number | null
  unlimited?: boolean
  expires_at: string
  windows?: UpstreamQuotaWindow[]
}

export interface UpstreamQuotaInfo {
  provider: UpstreamQuotaProvider
  mode: UpstreamQuotaMode
  unit?: UpstreamQuotaUnit
  remaining?: number | null
  used?: number | null
  total?: number | null
  expires_at?: string | null
  windows?: UpstreamQuotaWindow[]
  subscription?: UpstreamSubscriptionInfo | null
}

export interface UpstreamQuotaQueryResult {
  account_id: number
  observed_at: string
  quota: UpstreamQuotaInfo | null
}

export interface AccountHourlyUsageStats {
  total_requests: number
  successful_requests: number
  success_rate: number
  avg_first_token_ms: number | null
  error_4xx: number
  error_5xx: number
}

export type OllamaCloudUsageStatus = 'ok' | 'unauthorized' | 'failed'

export interface OllamaCloudUsageWindow {
  used_percent: number
  reset_at?: string
  reset_text?: string
}

export interface OllamaCloudUsageModel {
  model: string
  window: 'five_hour' | 'seven_day'
  requests: number
}

export interface OllamaCloudUsageData {
  plan?: string
  five_hour?: OllamaCloudUsageWindow
  seven_day?: OllamaCloudUsageWindow
  balance?: string
  models?: OllamaCloudUsageModel[]
}

export interface OllamaCloudUsageSnapshot {
  status: OllamaCloudUsageStatus
  data?: OllamaCloudUsageData
  fetched_at?: string
  last_attempt_at: string
  next_refresh_at: string
  failure_count?: number
  http_status?: number
  last_error?: string
}

export interface OllamaCloudUsageState {
  account_id: number
  eligible: boolean
  configured: boolean
  auto_refresh_enabled: boolean
  encryption_key_configured: boolean
  snapshot?: OllamaCloudUsageSnapshot
}

export interface OllamaCloudUsageSettings {
  enabled: boolean
  /** Max wait while model requests keep arriving (minutes). */
  interval_minutes: number
  /** Trailing quiet period after the latest model request (minutes). */
  debounce_minutes: number
}

export interface AccountEgressBinding {
  id: number
  account_id: number
  account_name?: string
  pool_id: number
  pool_name?: string
  pool_status?: 'active' | 'disabled'
  source_ipv6: string
  status: 'active' | 'disabled'
  version: number
  rotated_at?: string | null
  created_at: string
  updated_at: string
}

export interface Account {
  id: number
  name: string
  notes?: string | null
  platform: AccountPlatform
  type: AccountType
  // 后端响应里 credentials 已脱敏：access_token / refresh_token / id_token /
  // api_key / session_key / cookie / cpa_management_key / aws_secret_access_key / aws_session_token /
  // service_account_json / service_account / private_key 不会出现，
  // 改为通过 credentials_status.has_<key> 暴露存在性。
  credentials?: Record<string, unknown>
  credentials_status?: Record<string, boolean>
  ollama_cloud_usage?: OllamaCloudUsageState
  // Extra fields including Codex usage, OpenAI compact capability, and model-level rate limits.
  extra?: (CodexUsageSnapshot & OpenAICompactState & {
    model_rate_limits?: Record<string, { rate_limited_at: string; rate_limit_reset_at: string }>
    antigravity_credits_overages?: Record<string, { activated_at: string; active_until: string }>
    upstream_billing_probe_enabled?: boolean
    upstream_billing_rate_sync_enabled?: boolean
    upstream_billing_probe?: UpstreamBillingProbeSnapshot
    auto_disable_on_upstream_insufficient_balance?: boolean
    codex_reset_credit_snapshot?: {
      available_count?: number
      credits?: { expires_at?: string }[]
    }
  } & Record<string, unknown>)
  proxy_id: number | null
  egress_mode?: 'inherit' | 'direct' | 'external_proxy' | 'ipv6_pool'
  egress_binding?: AccountEgressBinding | null
  proxy_fallback_origin_id?: number | null
  proxy_fallback_origin_name?: string | null
  concurrency: number
  load_factor?: number | null
  current_concurrency?: number // Real-time concurrency count from Redis
  cpa_capacity?: CPACapacityStatus | null
  stream_degraded?: boolean
  stream_degradation_level?: number
  stream_degradation_timeouts?: number
  stream_degraded_since?: string | null
  stream_next_probe_at?: string | null
  scheduler_score?: {
    base_score: number
    sticky_score?: number
    sticky_score_infinity?: boolean
    sticky_weighted_enabled: boolean
  } | null
  scheduler_scores?: AccountSchedulerGroupScore[] | null
  priority: number
  rate_multiplier?: number // Account billing multiplier (>=0, 0 means free)
  status: 'active' | 'inactive' | 'error'
  error_message: string | null
  last_used_at: string | null
  expires_at: number | null
  auto_pause_on_expired: boolean
  created_at: string
  updated_at: string
  proxy?: Proxy
  group_ids?: number[] // Groups this account belongs to
  groups?: Group[] // Preloaded group objects

  // Rate limit & scheduling fields
  schedulable: boolean
  rate_limited_at: string | null
  rate_limit_reset_at: string | null
  overload_until: string | null
  temp_unschedulable_until: string | null
  temp_unschedulable_reason: string | null

  // Session window fields (5-hour window)
  session_window_start: string | null
  session_window_end: string | null
  session_window_status: 'allowed' | 'allowed_warning' | 'rejected' | null

  // 5h窗口费用控制（仅 Anthropic OAuth/SetupToken 账号有效）
  window_cost_limit?: number | null
  window_cost_sticky_reserve?: number | null

  // 会话数量控制（仅 Anthropic OAuth/SetupToken 账号有效）
  max_sessions?: number | null
  session_idle_timeout_minutes?: number | null

  // RPM 限制（仅 Anthropic OAuth/SetupToken 账号有效）
  base_rpm?: number | null
  rpm_strategy?: string | null
  rpm_sticky_buffer?: number | null
  user_msg_queue_mode?: string | null  // "serialize" | "throttle" | null

  // TLS指纹伪装（仅 Anthropic OAuth/SetupToken 账号有效）
  enable_tls_fingerprint?: boolean | null
  tls_fingerprint_profile_id?: number | null

  // 会话ID伪装（仅 Anthropic OAuth/SetupToken 账号有效）
  // 启用后将在15分钟内固定 metadata.user_id 中的 session ID
  session_id_masking_enabled?: boolean | null

  // 缓存 TTL 强制替换（仅 Anthropic OAuth/SetupToken 账号有效）
  cache_ttl_override_enabled?: boolean | null
  cache_ttl_override_target?: string | null

  // 自定义 Base URL 中继转发（仅 Anthropic OAuth/SetupToken 账号有效）
  custom_base_url_enabled?: boolean | null
  custom_base_url?: string | null

  // API Key 账号配额限制
  quota_limit?: number | null
  quota_used?: number | null
  quota_daily_limit?: number | null
  quota_daily_used?: number | null
  quota_weekly_limit?: number | null
  quota_weekly_used?: number | null

  // 配额固定时间重置配置
  quota_daily_reset_mode?: 'rolling' | 'fixed' | null
  quota_daily_reset_hour?: number | null
  quota_weekly_reset_mode?: 'rolling' | 'fixed' | null
  quota_weekly_reset_day?: number | null
  quota_weekly_reset_hour?: number | null
  quota_reset_timezone?: string | null
  quota_daily_reset_at?: string | null
  quota_weekly_reset_at?: string | null

  // 运行时状态（仅当启用对应限制时返回）
  current_window_cost?: number | null // 当前窗口费用
  active_sessions?: number | null // 当前活跃会话数
  current_rpm?: number | null // 当前分钟 RPM 计数
  hourly_usage?: AccountHourlyUsageStats | null // 最近一小时滚动使用情况（按需返回）

  // 影子账号关系（spark 维度影子）
  parent_account_id?: number | null
  quota_dimension?: string
  // 影子账号回填的母账号信息（仅影子非空）
  parent_email?: string
  parent_plan_type?: string
  parent_privacy_mode?: string
  parent_subscription_expires_at?: string
  parent_chatgpt_account_id?: string
}

export interface CPACapacityStatus {
  total_credentials: number
  enabled_credentials: number
  abnormal_credentials: number
  available_credentials: number
  capacity_credentials?: number
  effective_concurrency: number
  concurrency_per_credential: number
  exclude_abnormal_credentials?: boolean
  fetched_at?: string
  state: 'fresh' | 'stale' | 'unavailable'
}

export interface AccountSchedulerGroupScore {
  group_id?: number | null
  group_name?: string
  group_priority?: number | null
  base_score: number
  sticky_score?: number
  sticky_score_infinity?: boolean
  sticky_weighted_enabled: boolean
}

// Account Usage types
export interface WindowStats {
  requests: number
  tokens: number
  cost: number // Account cost (account multiplier)
  standard_cost?: number
  user_cost?: number
}

export interface UsageProgress {
  utilization: number // Percentage (0-100+, 100 = 100%)
  resets_at: string | null
  remaining_seconds: number
  window_stats?: WindowStats | null // 窗口期统计（从窗口开始到当前的使用量）
  used_requests?: number
  limit_requests?: number
}

// Antigravity 单个模型的配额信息
export interface AntigravityModelQuota {
  utilization: number // 使用率 0-100
  reset_time: string  // 重置时间 ISO8601
}

export interface GrokQuotaWindow {
  limit?: number | null
  remaining?: number | null
  reset_unix?: number | null
  reset_at?: string | null
}

export interface GrokBillingProductUsage {
  product: string
  usage_percent?: number | null
}

export interface GrokBillingSummary {
  period_type?: string
  usage_percent?: number | null
  period_start?: string
  period_end?: string
  product_usage?: GrokBillingProductUsage[]
  monthly_limit_cents?: number | null
  used_cents?: number | null
  included_used_cents?: number | null
  billing_period_start?: string
  billing_period_end?: string
  used_percent?: number | null
  plan?: string
  status_code?: number
  source?: string
  fetched_at?: string
  updated_at?: string
  weekly_updated_at?: string
  monthly_updated_at?: string
  partial?: boolean
  failed_windows?: string[]
}

export interface AccountUsageInfo {
  source?: 'passive' | 'active'
  updated_at: string | null
  five_hour: UsageProgress | null
  seven_day: UsageProgress | null
  seven_day_sonnet: UsageProgress | null
  seven_day_fable?: UsageProgress | null
  gemini_shared_daily?: UsageProgress | null
  gemini_pro_daily?: UsageProgress | null
  gemini_flash_daily?: UsageProgress | null
  gemini_shared_minute?: UsageProgress | null
  gemini_pro_minute?: UsageProgress | null
  gemini_flash_minute?: UsageProgress | null
  antigravity_quota?: Record<string, AntigravityModelQuota> | null
  grok_request_quota?: GrokQuotaWindow | null
  grok_token_quota?: GrokQuotaWindow | null
  grok_retry_after_seconds?: number | null
  grok_entitlement_status?: string
  grok_quota_snapshot_state?: string
  grok_last_quota_probe_at?: string
  grok_last_headers_seen_at?: string
  grok_last_status_code?: number
  grok_free_token_limit?: number
  grok_local_usage?: WindowStats | null
  grok_local_usage_24h?: WindowStats | null
  grok_local_usage_7d?: WindowStats | null
  grok_local_usage_monthly?: WindowStats | null
  grok_billing?: GrokBillingSummary | null
  subscription_tier?: string
  subscription_tier_raw?: string
  ai_credits?: Array<{
    credit_type?: string
    amount?: number
    minimum_balance?: number
  }> | null
  // Antigravity 403 forbidden 状态
  is_forbidden?: boolean
  forbidden_reason?: string
  forbidden_type?: string   // "validation" | "violation" | "forbidden"
  validation_url?: string   // 验证/申诉链接

  // 状态标记（后端自动推导）
  needs_verify?: boolean    // 需要人工验证（forbidden_type=validation）
  is_banned?: boolean       // 账号被封（forbidden_type=violation）
  needs_reauth?: boolean    // token 失效需重新授权（401）

  // 机器可读错误码：forbidden / unauthenticated / rate_limited / network_error
  error_code?: string

  error?: string            // usage 获取失败时的错误信息
}

// OpenAI Codex usage snapshot (from response headers)
export interface CodexUsageSnapshot {
  // Legacy fields (kept for backwards compatibility)
  // NOTE: The naming is ambiguous - actual window type is determined by window_minutes value
  codex_primary_used_percent?: number // Usage percentage (check window_minutes for actual window type)
  codex_primary_reset_after_seconds?: number // Seconds until reset
  codex_primary_window_minutes?: number // Window in minutes
  codex_secondary_used_percent?: number // Usage percentage (check window_minutes for actual window type)
  codex_secondary_reset_after_seconds?: number // Seconds until reset
  codex_secondary_window_minutes?: number // Window in minutes
  codex_primary_over_secondary_percent?: number // Overflow ratio

  // Canonical fields (normalized by backend, use these preferentially)
  codex_5h_used_percent?: number // 5-hour window usage percentage
  codex_5h_reset_after_seconds?: number // Seconds until 5h window reset
  codex_5h_reset_at?: string // 5-hour window absolute reset time (RFC3339)
  codex_5h_window_minutes?: number // 5h window in minutes (should be ~300)
  codex_7d_used_percent?: number // 7-day window usage percentage
  codex_7d_reset_after_seconds?: number // Seconds until 7d window reset
  codex_7d_reset_at?: string // 7-day window absolute reset time (RFC3339)
  codex_7d_window_minutes?: number // 7d window in minutes (should be ~10080)

  codex_usage_updated_at?: string // Last update timestamp
}

export type OpenAICompactMode = 'auto' | 'force_on' | 'force_off'
export type OpenAIResponsesMode = 'auto' | 'force_responses' | 'force_chat_completions'
export type OpenAIEndpointCapability = 'chat_completions' | 'embeddings' | 'alpha_search'

export interface OpenAICompactState {
  openai_compact_mode?: OpenAICompactMode
  openai_compact_supported?: boolean
  openai_compact_checked_at?: string
  openai_compact_last_status?: number
  openai_compact_last_error?: string
}

export interface OpenAIResponsesState {
  openai_responses_mode?: OpenAIResponsesMode
  openai_responses_supported?: boolean
}

export interface CreateProxyRequest {
  name: string
  protocol: ProxyProtocol
  host: string
  port: number
  username?: string | null
  password?: string | null
  expires_at?: number | null   // unix 秒；null/0 = 永不过期
  fallback_mode?: 'none' | 'proxy' | 'direct'
  backup_proxy_id?: number | null
  expiry_warn_days?: number
}

export interface UpdateProxyRequest {
  name?: string
  protocol?: ProxyProtocol
  host?: string
  port?: number
  username?: string | null
  password?: string | null
  status?: 'active' | 'inactive'
  expires_at?: number | null   // unix 秒；null/0 = 永不过期
  fallback_mode?: 'none' | 'proxy' | 'direct'
  backup_proxy_id?: number | null
  expiry_warn_days?: number
}

export interface AdminDataPayload {
  type?: string
  version?: number
  exported_at: string
  proxies: AdminDataProxy[]
  accounts: AdminDataAccount[]
  // 导出时被排除的 spark 影子账号数量(影子不持凭据、其调度配置不在备份范围)。
  skipped_shadows?: number
}

export interface AdminDataProxy {
  proxy_key: string
  name: string
  protocol: ProxyProtocol
  host: string
  port: number
  username?: string | null
  password?: string | null
  status: 'active' | 'inactive'
}

export interface AdminDataAccount {
  name: string
  notes?: string | null
  platform: AccountPlatform
  type: AccountType
  credentials: Record<string, unknown>
  extra?: Record<string, unknown>
  proxy_key?: string | null
  concurrency: number
  priority: number
  rate_multiplier?: number | null
  expires_at?: number | null
  auto_pause_on_expired?: boolean
}

export interface AdminDataImportError {
  kind: 'proxy' | 'account'
  name?: string
  proxy_key?: string
  message: string
}

export interface AdminDataImportResult {
  proxy_created: number
  proxy_reused: number
  proxy_failed: number
  account_created: number
  account_failed: number
  errors?: AdminDataImportError[]
}
