import type { RequestSchedulingTier, User } from './common'
import type { ApiKey, Group } from './gateway'

// ==================== Usage & Redeem Types ====================

export type RedeemCodeType = 'balance' | 'concurrency' | 'subscription' | 'invitation'
export type UsageRequestType = 'unknown' | 'sync' | 'stream' | 'ws_v2' | 'cyber' | 'live'
export type ImageSizeSource = 'output' | 'input' | 'default' | 'legacy'
export type ImageSizeBreakdown = Record<string, number>

export interface UsageLog {
  id: number
  user_id: number
  api_key_id: number
  account_id: number | null
  request_id: string
  session_id?: string | null
  model: string
  service_tier?: string | null
  reasoning_effort?: string | null
  inbound_endpoint?: string | null

  group_id: number | null
  subscription_id: number | null

  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  cache_creation_5m_tokens: number
  cache_creation_1h_tokens: number

  input_cost: number
  output_cost: number
  cache_creation_cost: number
  cache_read_cost: number
  total_cost: number
  actual_cost: number
  rate_multiplier: number
  long_context_billing_applied: boolean
  billing_type: number

  request_type?: UsageRequestType
  stream: boolean
  openai_ws_mode?: boolean
  duration_ms: number | null
  first_token_ms: number | null

  // 图片生成字段
  image_count: number
  image_size: string | null
  image_input_size: string | null
  image_output_size: string | null
  image_size_source: ImageSizeSource | null
  image_size_breakdown: ImageSizeBreakdown | null
  image_input_tokens: number
  image_input_cost: number
  image_output_tokens: number
  image_output_cost: number

  // 视频生成字段
  video_count?: number
  video_resolution?: string | null
  video_duration_seconds?: number | null

  // User-Agent
  user_agent: string | null
  ip_address?: string | null

  // Cache TTL Override
  cache_ttl_overridden: boolean

  // 计费模式
  billing_mode?: string | null

  created_at: string

  user?: User
  api_key?: ApiKey
  group?: Group
  subscription?: UserSubscription
}

export interface UsageLogAccountSummary {
  id: number
  name: string
}

export interface AdminUsageLog extends UsageLog {
  upstream_endpoint?: string | null
  upstream_model?: string | null
  upstream_response_model?: string | null
  upstream_model_mismatch?: boolean | null
  model_mapping_chain?: string | null

  // 账号计费倍率（仅管理员可见）
  account_rate_multiplier?: number | null
  // 自定义定价规则计算的账号统计费用（nil 时使用 total_cost * multiplier）
  account_stats_cost?: number | null

  // 渠道 ID 和计费等级（仅管理员可见）
  channel_id?: number | null
  billing_tier?: string | null

  // 最小账号信息（仅管理员接口返回）
  account?: UsageLogAccountSummary
}

export interface UsageCleanupFilters {
  start_time: string
  end_time: string
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string | null
  request_type?: UsageRequestType | null
  stream?: boolean | null
  billing_type?: number | null
}

export interface UsageCleanupTask {
  id: number
  status: string
  filters: UsageCleanupFilters
  created_by: number
  deleted_rows: number
  error_message?: string | null
  canceled_by?: number | null
  canceled_at?: string | null
  started_at?: string | null
  finished_at?: string | null
  created_at: string
  updated_at: string
}

export interface RedeemCode {
  id: number
  code: string
  type: RedeemCodeType
  value: number
  status: 'active' | 'used' | 'expired' | 'unused' | 'disabled'
  max_uses: number
  used_count: number
  max_uses_per_user: number
  used_by: number | null
  used_at: string | null
  created_at: string
  expires_at?: string | null
  updated_at?: string
  notes?: string
  group_id?: number | null // 订阅类型专用
  validity_days?: number // 订阅类型专用
  user?: User
  group?: Group // 关联的分组
}

export interface GenerateRedeemCodesRequest {
  count: number
  type: RedeemCodeType
  value: number
  group_id?: number | null // 订阅类型专用
  validity_days?: number // 订阅类型专用
  expires_at?: string | null
  expires_in_days?: number
  max_uses?: number
  max_uses_per_user?: number
}

export interface BatchUpdateRedeemCodeFields {
  status?: 'unused' | 'disabled'
  expires_at?: string | null
  notes?: string
  group_id?: number | null
}

export interface BatchUpdateRedeemCodesRequest {
  ids: number[]
  fields: BatchUpdateRedeemCodeFields
}

export interface RedeemCodeRequest {
  code: string
}

// ==================== Dashboard & Statistics ====================

export interface DashboardStats {
  // 用户统计
  total_users: number
  today_new_users: number // 今日新增用户数
  active_users: number // 今日有请求的用户数
  hourly_active_users: number // 当前小时活跃用户数（UTC）
  stats_updated_at: string // 统计更新时间（UTC RFC3339）
  stats_stale: boolean // 统计是否过期

  // API Key 统计
  total_api_keys: number
  active_api_keys: number // 状态为 active 的 API Key 数

  // 账户统计
  total_accounts: number
  normal_accounts: number // 正常账户数
  error_accounts: number // 异常账户数
  ratelimit_accounts: number // 限流账户数
  overload_accounts: number // 过载账户数

  // 累计 Token 使用统计
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
  total_cache_creation_tokens: number
  total_cache_read_tokens: number
  total_tokens: number
  total_cost: number // 累计标准计费
  total_actual_cost: number // 累计实际扣除
  total_account_cost: number // 累计账号成本

  // 今日 Token 使用统计
  today_requests: number
  today_input_tokens: number
  today_output_tokens: number
  today_cache_creation_tokens: number
  today_cache_read_tokens: number
  today_tokens: number
  today_cost: number // 今日标准计费
  today_actual_cost: number // 今日实际扣除
  today_account_cost: number // 今日账号成本

  // 系统运行统计
  average_duration_ms: number // 平均响应时间
  uptime: number // 系统运行时间(秒)

  // 性能指标
  rpm: number // 近5分钟平均每分钟请求数
  tpm: number // 近5分钟平均每分钟Token数
}

export interface UsageStatsResponse {
  period?: string
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
  total_cache_tokens: number
  total_cache_read_tokens: number
  total_cache_creation_tokens: number
  total_tokens: number
  total_cost: number // 标准计费
  total_actual_cost: number // 实际扣除
  average_duration_ms: number
  models?: Record<string, number>
  endpoints?: EndpointStat[]
  upstream_endpoints?: EndpointStat[]
  endpoint_paths?: EndpointStat[]
}

// ==================== Trend & Chart Types ====================

export interface TrendDataPoint {
  date: string
  requests: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  total_tokens: number
  cost: number // 标准计费
  actual_cost: number // 实际扣除
}

export interface ModelStat {
  model: string
  requests: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  total_tokens: number
  cost: number // 标准计费
  actual_cost: number // 实际扣除
  account_cost?: number // 账号成本（仅管理员接口返回）
}

export interface EndpointStat {
  endpoint: string
  requests: number
  total_tokens: number
  cost: number
  actual_cost: number
}

export interface GroupStat {
  group_id: number
  group_name: string
  requests: number
  total_tokens: number
  cost: number // 标准计费
  actual_cost: number // 实际扣除
  account_cost?: number // 账号成本（仅管理员接口返回）
}

export interface UserBreakdownItem {
  user_id: number
  email: string
  requests: number
  input_tokens: number
  output_tokens: number
  cache_tokens: number
  total_tokens: number
  cost: number
  actual_cost: number
  account_cost: number
}

export interface UserUsageTrendPoint {
  date: string
  user_id: number
  email: string
  username: string
  requests: number
  tokens: number
  cost: number // 标准计费
  actual_cost: number // 实际扣除
}

export interface UserSpendingRankingItem {
  user_id: number
  email: string
  username?: string
  actual_cost: number
  requests: number
  tokens: number
}

export interface UserSpendingRankingResponse {
  ranking: UserSpendingRankingItem[]
  total_actual_cost: number
  total_requests: number
  total_tokens: number
  start_date: string
  end_date: string
}

export interface ApiKeyUsageTrendPoint {
  date: string
  api_key_id: number
  key_name: string
  requests: number
  tokens: number
}

// ==================== Admin User Management ====================

export interface UpdateUserRequest {
  email?: string
  password?: string
  username?: string
  notes?: string
  role?: 'admin' | 'user'
  balance?: number
  concurrency?: number
  rpm_limit?: number
  scheduling_tier?: RequestSchedulingTier
  status?: 'active' | 'disabled'
  allowed_groups?: number[] | null
  // 用户专属分组倍率配置 (group_id -> rate_multiplier | null)
  // null 表示删除该分组的专属倍率
  group_rates?: Record<number, number | null>
}

export interface ChangePasswordRequest {
  old_password: string
  new_password: string
}

// ==================== User Subscription Types ====================

export interface UserSubscription {
  id: number
  user_id: number
  group_id: number
  status: 'active' | 'expired' | 'revoked' | 'suspended'
  starts_at: string
  daily_usage_usd: number
  weekly_usage_usd: number
  monthly_usage_usd: number
  daily_window_start: string | null
  weekly_window_start: string | null
  monthly_window_start: string | null
  created_at: string
  updated_at: string
  revoked_at?: string | null
  expires_at: string | null
  user?: User
  group?: Group
}

export interface SubscriptionProgress {
  subscription_id: number
  daily: {
    used: number
    limit: number | null
    percentage: number
    reset_in_seconds: number | null
  } | null
  weekly: {
    used: number
    limit: number | null
    percentage: number
    reset_in_seconds: number | null
  } | null
  monthly: {
    used: number
    limit: number | null
    percentage: number
    reset_in_seconds: number | null
  } | null
  expires_at: string | null
  days_remaining: number | null
}

export interface AssignSubscriptionRequest {
  user_id: number
  group_id: number
  validity_days?: number
}

export interface BulkAssignSubscriptionRequest {
  user_ids: number[]
  group_id: number
  validity_days?: number
}

export interface ExtendSubscriptionRequest {
  days: number
}

// ==================== Query Parameters ====================

export interface UserErrorRequest {
  id: number
  created_at: string
  model: string
  inbound_endpoint: string
  status_code: number
  category: string
  platform: string
  message: string
  key_name: string
  key_deleted: boolean
  client_ip?: string
  group_name?: string
  request_type?: number
  stream?: boolean
  user_agent?: string
}

export interface UserErrorRequestDetail extends UserErrorRequest {
  error_body: string
  upstream_status_code?: number
}

export interface UserErrorListParams {
  page?: number
  page_size?: number
  start_date?: string
  end_date?: string
  timezone?: string
  model?: string
  status_code?: number
  category?: string
  api_key_id?: number
  // 服务端排序,列白名单见后端 opsErrorLogsOrderBy(created_at/model/status_code)
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export interface UsageQueryParams {
  page?: number
  page_size?: number
  api_key_id?: number
  user_id?: number
  account_id?: number
  group_id?: number
  model?: string
  request_type?: UsageRequestType
  stream?: boolean
  billing_type?: number | null
  billing_mode?: string | null
  upstream_model_mismatch?: boolean | null
  start_date?: string
  end_date?: string
  timezone?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

// ==================== Account Usage Statistics ====================

export interface AccountUsageHistory {
  date: string
  label: string
  requests: number
  tokens: number
  cost: number
  actual_cost: number // Account cost (account multiplier)
  user_cost: number // User/API key billed cost (group multiplier)
}

export interface AccountUsageSummary {
  days: number
  actual_days_used: number
  total_cost: number // Account cost (account multiplier)
  total_user_cost: number
  total_standard_cost: number
  total_requests: number
  total_tokens: number
  avg_daily_cost: number // Account cost
  avg_daily_user_cost: number
  avg_daily_requests: number
  avg_daily_tokens: number
  avg_duration_ms: number
  avg_first_token_ms: number | null
  today: {
    date: string
    cost: number
    user_cost: number
    requests: number
    tokens: number
  } | null
  highest_cost_day: {
    date: string
    label: string
    cost: number
    user_cost: number
    requests: number
  } | null
  highest_request_day: {
    date: string
    label: string
    requests: number
    cost: number
    user_cost: number
  } | null
}

export interface AccountUsageStatsResponse {
  history: AccountUsageHistory[]
  summary: AccountUsageSummary
  models: ModelStat[]
  endpoints: EndpointStat[]
  upstream_endpoints: EndpointStat[]
}

// ==================== User Attribute Types ====================

export type UserAttributeType = 'text' | 'textarea' | 'number' | 'email' | 'url' | 'date' | 'select' | 'multi_select'

export interface UserAttributeOption {
  value: string
  label: string
  [key: string]: unknown
}

export interface UserAttributeValidation {
  min_length?: number
  max_length?: number
  min?: number
  max?: number
  pattern?: string
  message?: string
}

export interface UserAttributeDefinition {
  id: number
  key: string
  name: string
  description: string
  type: UserAttributeType
  options: UserAttributeOption[]
  required: boolean
  validation: UserAttributeValidation
  placeholder: string
  display_order: number
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface UserAttributeValue {
  id: number
  user_id: number
  attribute_id: number
  value: string
  created_at: string
  updated_at: string
}

export interface CreateUserAttributeRequest {
  key: string
  name: string
  description?: string
  type: UserAttributeType
  options?: UserAttributeOption[]
  required?: boolean
  validation?: UserAttributeValidation
  placeholder?: string
  display_order?: number
  enabled?: boolean
}

export interface UpdateUserAttributeRequest {
  key?: string
  name?: string
  description?: string
  type?: UserAttributeType
  options?: UserAttributeOption[]
  required?: boolean
  validation?: UserAttributeValidation
  placeholder?: string
  display_order?: number
  enabled?: boolean
}

export interface UserAttributeValuesMap {
  [attributeId: number]: string
}

// ==================== Promo Code Types ====================

export interface PromoCode {
  id: number
  code: string
  bonus_amount: number
  max_uses: number
  used_count: number
  status: 'active' | 'disabled'
  expires_at: string | null
  notes: string | null
  created_at: string
  updated_at: string
}

export interface PromoCodeUsage {
  id: number
  promo_code_id: number
  user_id: number
  bonus_amount: number
  used_at: string
  user?: User
}

export interface CreatePromoCodeRequest {
  code?: string
  bonus_amount: number
  max_uses?: number
  expires_at?: number | null
  notes?: string
}

export interface UpdatePromoCodeRequest {
  code?: string
  bonus_amount?: number
  max_uses?: number
  status?: 'active' | 'disabled'
  expires_at?: number | null
  notes?: string
}

// ==================== TOTP (2FA) Types ====================

export interface TotpStatus {
  enabled: boolean
  enabled_at: number | null  // Unix timestamp in seconds
  feature_enabled: boolean
}

export interface TotpSetupRequest {
  email_code?: string
  password?: string
}

export interface TotpSetupResponse {
  secret: string
  qr_code_url: string
  setup_token: string
  countdown: number
}

export interface TotpEnableRequest {
  totp_code: string
  setup_token: string
}

export interface TotpEnableResponse {
  success: boolean
}

export interface TotpDisableRequest {
  email_code?: string
  password?: string
}

export interface TotpVerificationMethod {
  method: 'email' | 'password'
}

export interface TotpLoginResponse {
  requires_2fa: boolean
  temp_token?: string
  user_email_masked?: string
}

export interface TotpLogin2FARequest {
  temp_token: string
  totp_code: string
}

// ==================== Scheduled Test Types ====================

export interface ScheduledTestPlan {
  id: number
  account_id: number
  model_id: string
  cron_expression: string
  enabled: boolean
  max_results: number
  auto_recover: boolean
  last_run_at: string | null
  next_run_at: string | null
  created_at: string
  updated_at: string
}

export interface ScheduledTestResult {
  id: number
  plan_id: number
  status: string
  response_text: string
  error_message: string
  latency_ms: number
  started_at: string
  finished_at: string
  created_at: string
}

export interface CreateScheduledTestPlanRequest {
  account_id: number
  model_id: string
  cron_expression: string
  enabled?: boolean
  max_results?: number
  auto_recover?: boolean
}

export interface UpdateScheduledTestPlanRequest {
  model_id?: string
  cron_expression?: string
  enabled?: boolean
  max_results?: number
  auto_recover?: boolean
}
