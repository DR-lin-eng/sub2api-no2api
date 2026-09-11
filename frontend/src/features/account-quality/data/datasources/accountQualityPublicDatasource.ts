import { apiClient } from '@/core/networks/client'

export interface AccountQualityPublicPoint {
  id: string
  status: string
  label?: string
  confidence?: number
  model?: string
  effort?: string
  latency_ms?: number
  started_at: string
}

export interface AccountQualityPublicSnapshot {
  model: string
  effort: string
  interval_minutes: number
  now: string
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
