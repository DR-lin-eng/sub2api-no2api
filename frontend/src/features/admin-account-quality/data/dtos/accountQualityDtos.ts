export interface AccountQualitySettings {
  enabled: boolean
  interval_minutes: number
  timeout_seconds: number
  model: string
  effort: string
  prompt?: string
  stage1_enabled: boolean
  stage1_prompt: string
  stage1_answer: string
  stage2_enabled: boolean
  stage2_prompt: string
  failure_threshold: number
  recovery_threshold: number
  degraded_group_id: number | null
  source_group_id: number | null
  max_concurrent: number
  min_confidence: number
  min_reasoning_tokens: number
  public_enabled: boolean
}

export interface AccountQualityResult {
  account_id: number
  name: string
  platform: string
  type: string
  quality_status?: string
  quality_stage1_status?: string
  quality_stage2_status?: string
  quality_reasoning_tokens?: number | null
  quality_consecutive_failures?: number
  quality_consecutive_passes?: number
  quality_action?: string
  quality_error?: string
  quality_latency_ms?: number
  quality_label?: string
  quality_confidence?: number
  observed_at: string
}

export interface AccountQualitySummary {
  inspected: number
  passed: number
  degraded: number
  uncertain: number
  errors: number
  switched: number
  reasoning_token_distribution: AccountQualityReasoningTokenDistribution
}

export interface AccountQualityReasoningTokenBucket {
  key: string
  min: number
  max?: number | null
  count: number
}

export interface AccountQualityReasoningTokenDistribution {
  average_tokens?: number | null
  measured_accounts: number
  unknown_accounts: number
  buckets: AccountQualityReasoningTokenBucket[]
}

export interface AccountQualityRun {
  run_id?: string
  status: string
  trigger?: string
  started_at?: string | null
  completed_at?: string | null
  next_run_at?: string | null
  last_run_at?: string | null
  summary: AccountQualitySummary
  results?: AccountQualityResult[]
  error?: string
  progress: { total: number; completed: number; current_account_id?: number; current_account?: string; current_stage?: string }
}

export interface AccountQualityOverview {
  settings: AccountQualitySettings
  run: AccountQualityRun
  results: {
    items: AccountQualityResult[]
    total: number
    page: number
    page_size: number
    pages: number
  }
}
