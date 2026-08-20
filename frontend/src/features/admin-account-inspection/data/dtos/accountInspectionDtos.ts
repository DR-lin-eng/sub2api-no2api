export interface AccountInspectionSettings {
  enabled: boolean
  interval_minutes: number
  auto_disable: boolean
  lookback_minutes: number
  min_requests: number
  ttft_threshold_ms: number
  success_rate_threshold: number
  oauth_quota_check_enabled: boolean
  api_key_quota_check_enabled: boolean
  api_key_min_cache_hit_rate: number
  api_key_max_rate_multiplier: number
  api_key_min_remaining_quota: number
}

export interface AccountInspectionSummary {
  inspected: number
  healthy: number
  flagged: number
  disabled: number
  already_disabled: number
  oauth_accounts: number
  api_key_accounts: number
}

export interface AccountInspectionRun {
  run_id?: string
  status: 'idle' | 'running' | 'succeeded' | 'failed' | string
  trigger?: 'manual' | 'scheduled' | string
  started_at?: string | null
  completed_at?: string | null
  next_run_at?: string | null
  summary: AccountInspectionSummary
  error?: string
  results_truncated?: boolean
}

export type AccountInspectionAction = 'none' | 'reported' | 'disabled' | 'already_disabled' | 'error' | string

export interface AccountInspectionResult {
  account_id: number
  name: string
  platform: string
  type: 'oauth' | 'apikey' | 'bedrock' | string
  status: 'healthy' | 'flagged' | 'unknown' | string
  schedulable: boolean
  action: AccountInspectionAction
  reasons: string[]
  total_requests: number
  successful_requests: number
  success_rate?: number | null
  avg_first_token_ms?: number | null
  cache_hit_rate?: number | null
  cache_read_tokens?: number
  cache_creation_tokens?: number
  rate_multiplier?: number | null
  remaining_quota?: number | null
  remaining_quota_dimension?: string
  quota_unlimited?: boolean
  observed_at: string
}

export interface AccountInspectionPage {
  items: AccountInspectionResult[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface AccountInspectionOverview {
  settings: AccountInspectionSettings
  run: AccountInspectionRun
  results: AccountInspectionPage
}

export interface AccountInspectionQuery {
  page?: number
  page_size?: number
  status?: 'all' | 'flagged' | 'healthy' | 'disabled' | string
  type?: 'all' | 'oauth' | 'apikey' | 'bedrock' | string
  search?: string
}
