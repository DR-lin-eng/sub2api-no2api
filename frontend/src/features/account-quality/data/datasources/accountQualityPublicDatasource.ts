import { apiClient } from '@/core/networks/client'

export interface AccountQualityStageDetail {
  status: string
  conversation_id?: string
  response_id?: string
  answer: string
  answer_truncated?: boolean
  reasoning_tokens?: number | null
}

export interface AccountQualityPublicPoint {
  id: string
  status: string
  label?: string
  confidence?: number
  model?: string
  effort?: string
  latency_ms?: number
  started_at: string
  has_preview?: boolean
  preview_format?: string
  details?: { stage1?: AccountQualityStageDetail | null; stage2?: AccountQualityStageDetail | null }
}

export interface AccountQualityPublicSnapshot {
  model: string
  effort: string
  interval_minutes: number
  now: string
  last_run_at?: string
  next_run_at?: string
  total: number
  passed: number
  degraded: number
  uncertain: number
  error: number
  model_version?: string
  points: AccountQualityPublicPoint[]
}

export async function getPublicSnapshot(): Promise<AccountQualityPublicSnapshot> {
  const { data } = await apiClient.get<AccountQualityPublicSnapshot>('/account-quality-share', { params: { limit: 200 } })
  return data
}
