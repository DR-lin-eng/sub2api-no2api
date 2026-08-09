export interface OpsImageGenerationResolutionStats {
  resolution: string
  billing_tier: string
  request_count: number
  image_count: number
  avg_duration_ms?: number | null
  p95_duration_ms?: number | null
  max_duration_ms?: number | null
}

export interface OpsImageGenerationRealtime {
  available: boolean
  scope: 'instance'
  enabled: boolean
  current_concurrent: number
  waiting: number
  limit: number
  max_waiting: number
}

export interface OpsImageGenerationStatsResponse {
  start_time: string
  end_time: string
  platform?: string
  group_id?: number | null
  request_count: number
  image_count: number
  requests_per_minute: number
  avg_duration_ms?: number | null
  p95_duration_ms?: number | null
  max_duration_ms?: number | null
  average_concurrent: number
  peak_concurrent: number
  realtime: OpsImageGenerationRealtime
  by_resolution: OpsImageGenerationResolutionStats[]
}

export interface OpsImageGenerationStatsParams {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
}

export type OpsOpenAITokenStatsTimeRange = '30m' | '1h' | '1d' | '15d' | '30d'

export interface OpsOpenAITokenStatsItem {
  model: string
  request_count: number
  avg_tokens_per_sec?: number | null
  avg_first_token_ms?: number | null
  total_output_tokens: number
  avg_duration_ms: number
  requests_with_first_token: number
}

export interface OpsOpenAITokenStatsResponse {
  time_range: OpsOpenAITokenStatsTimeRange
  start_time: string
  end_time: string
  platform?: string
  group_id?: number | null
  items: OpsOpenAITokenStatsItem[]
  total: number
  page?: number
  page_size?: number
  top_n?: number | null
}

export interface OpsOpenAITokenStatsParams {
  time_range?: OpsOpenAITokenStatsTimeRange
  platform?: string
  group_id?: number | null
  page?: number
  page_size?: number
  top_n?: number
}

export type OpsUserUsageStatsTimeRange = '30m' | '1h' | '24h' | '15d' | '30d'

export interface OpsUserUsageStatsItem {
  user_id: number
  username: string
  email: string
  request_count: number
  input_tokens: number
  output_tokens: number
  cache_tokens: number
  total_tokens: number
  actual_cost: number
  last_request_at: string
}

export interface OpsUserUsageStatsResponse {
  time_range: OpsUserUsageStatsTimeRange
  start_time: string
  end_time: string
  platform?: string
  group_id?: number | null
  items: OpsUserUsageStatsItem[]
  total: number
  page?: number
  page_size?: number
  top_n?: number | null
}

export interface OpsUserUsageStatsParams {
  time_range?: OpsUserUsageStatsTimeRange
  platform?: string
  group_id?: number | null
  page?: number
  page_size?: number
  top_n?: number
}

export interface PlatformConcurrencyInfo {
  platform: string
  current_in_use: number
  max_capacity: number
  load_percentage: number
  waiting_in_queue: number
}

export interface GroupConcurrencyInfo {
  group_id: number
  group_name: string
  platform: string
  current_in_use: number
  max_capacity: number
  load_percentage: number
  waiting_in_queue: number
}

export interface AccountConcurrencyInfo {
  account_id: number
  account_name?: string
  platform: string
  group_id: number
  group_name: string
  current_in_use: number
  max_capacity: number
  load_percentage: number
  waiting_in_queue: number
}

export interface OpsConcurrencyStatsResponse {
  enabled: boolean
  platform: Record<string, PlatformConcurrencyInfo>
  group: Record<string, GroupConcurrencyInfo>
  account: Record<string, AccountConcurrencyInfo>
  timestamp?: string
}

export interface UserConcurrencyInfo {
  user_id: number
  user_email: string
  username: string
  current_in_use: number
  max_capacity: number
  load_percentage: number
  waiting_in_queue: number
}

export interface OpsUserConcurrencyStatsResponse {
  enabled: boolean
  user: Record<string, UserConcurrencyInfo>
  timestamp?: string
}

export interface PlatformAvailability {
  platform: string
  total_accounts: number
  available_count: number
  rate_limit_count: number
  error_count: number
}

export interface GroupAvailability {
  group_id: number
  group_name: string
  platform: string
  total_accounts: number
  available_count: number
  rate_limit_count: number
  error_count: number
}

export interface AccountAvailability {
  account_id: number
  account_name: string
  platform: string
  group_id: number
  group_name: string
  status: string
  is_available: boolean
  is_rate_limited: boolean
  rate_limit_reset_at?: string
  rate_limit_remaining_sec?: number
  is_overloaded: boolean
  overload_until?: string
  overload_remaining_sec?: number
  has_error: boolean
  error_message?: string
}

export interface OpsAccountAvailabilityStatsResponse {
  enabled: boolean
  platform: Record<string, PlatformAvailability>
  group: Record<string, GroupAvailability>
  account: Record<string, AccountAvailability>
  timestamp?: string
}

export interface OpsConcurrencySnapshotResponse {
  enabled: boolean
  concurrency: OpsConcurrencyStatsResponse
  availability: OpsAccountAvailabilityStatsResponse
  timestamp?: string
}

export interface OpsRateSummary {
  current: number
  peak: number
  avg: number
}

export interface OpsRealtimeTrafficSummary {
  window: string
  start_time: string
  end_time: string
  platform: string
  group_id?: number | null
  qps: OpsRateSummary
  tps: OpsRateSummary
}

export interface OpsRealtimeTrafficSummaryResponse {
  enabled: boolean
  summary: OpsRealtimeTrafficSummary | null
  timestamp?: string
}
