import type { Account } from '@/types'

export interface OpenAIRateLimitWindow {
  used_percent: number
  limit_window_seconds: number
  reset_after_seconds: number
  reset_at: number
  resets_at?: number
  usedPercent?: number
  window_duration_mins?: number
  windowDurationMins?: number
  resetsAt?: number
}

export interface OpenAIAppServerRateLimitWindow {
  used_percent?: number
  limit_window_seconds?: number
  reset_after_seconds?: number
  reset_at?: number
  resets_at?: number
  window_duration_mins?: number
  usedPercent?: number
  windowDurationMins?: number
  resetsAt?: number
}

export interface OpenAIRateLimit {
  limit_id?: string
  limit_name?: string | null
  limitId?: string
  limitName?: string | null
  allowed: boolean
  limit_reached: boolean
  primary_window?: OpenAIRateLimitWindow | null
  secondary_window?: OpenAIRateLimitWindow | null
  primary?: OpenAIAppServerRateLimitWindow | null
  secondary?: OpenAIAppServerRateLimitWindow | null
}

export interface OpenAIAdditionalRateLimit {
  limit_name: string
  metered_feature: string
  rate_limit?: OpenAIRateLimit | null
}

export interface OpenAIAppServerRateLimitBucket {
  limit_id?: string
  limit_name?: string | null
  limitId?: string
  limitName?: string | null
  used_percent?: number
  window_duration_mins?: number
  resets_at?: number
  usedPercent?: number
  windowDurationMins?: number
  resetsAt?: number
  primary?: OpenAIAppServerRateLimitWindow | null
  secondary?: OpenAIAppServerRateLimitWindow | null
  rate_limit_reached_type?: string | null
  rateLimitReachedType?: string | null
  raw_fields?: Record<string, unknown>
  raw_value?: unknown
}

export type OpenAIRateLimitBucket = OpenAIAppServerRateLimitBucket
export type OpenAIRateLimitBucketWindow = OpenAIAppServerRateLimitWindow

export interface OpenAIRateLimitResetCreditDetail {
  expires_at?: string
}

export interface OpenAIRateLimitResetCredits {
  available_count: number
  credits?: OpenAIRateLimitResetCreditDetail[]
}

export interface OpenAITokenUsageSummary {
  lifetime_tokens?: number | null
  peak_daily_tokens?: number | null
  longest_running_turn_seconds?: number | null
  current_streak_days?: number | null
  longest_streak_days?: number | null
}

export interface OpenAITokenUsageDailyBucket {
  start_date: string
  tokens: number
}

export interface OpenAIServerTokenUsage {
  summary: OpenAITokenUsageSummary
  daily_usage_buckets?: OpenAITokenUsageDailyBucket[] | null
  current_reset_cycle_tokens?: number | null
  current_reset_cycle_window_minutes?: number
  current_reset_cycle_limit_id?: string
  current_reset_cycle_approximate?: boolean
}

export interface OpenAIQuotaUsage {
  source?: 'passive' | 'active'
  user_id?: string
  account_id?: string
  email?: string
  plan_type?: string
  rate_limit?: OpenAIRateLimit | null
  rateLimit?: OpenAIRateLimit | null
  additional_rate_limits?: OpenAIAdditionalRateLimit[]
  rate_limits_by_limit_id?: Record<string, OpenAIAppServerRateLimitBucket | unknown>
  rateLimitsByLimitId?: Record<string, OpenAIAppServerRateLimitBucket | unknown>
  rate_limit_reset_credits?: OpenAIRateLimitResetCredits | null
  server_token_usage?: OpenAIServerTokenUsage | null
  fetched_at: number
}

export interface OpenAIQuotaResetCredit {
  id?: string
  reset_type?: string
  status?: string
  granted_at?: string
  expires_at?: string
  redeem_started_at?: string
  redeemed_at?: string
}

export interface OpenAIQuotaResetResult {
  code: string
  credit?: OpenAIQuotaResetCredit | null
  windows_reset: number
  quota?: OpenAIQuotaUsage | null
  account?: Account | null
  cache_refreshed: boolean
  account_state_recovered: boolean
  warning_code?:
    | 'reset_credit_cache_refresh_failed'
    | 'account_state_recovery_failed'
    | 'account_state_refresh_failed'
}

export interface OpenAIQuotaRefreshResult extends OpenAIQuotaUsage {
  cache_persisted: boolean
  rate_limit_snapshot_persisted?: boolean
}
