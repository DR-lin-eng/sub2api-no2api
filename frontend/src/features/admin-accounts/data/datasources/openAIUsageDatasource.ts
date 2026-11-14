import apiClient, { buildApiUrl } from '@/core/networks/client'
import { getAccessToken } from '@/core/networks/tokenStore'
import type { OpenAIQuotaUsage } from '../dtos/openAIQuotaDtos'

export type OpenAIUsageKind = 'daily' | 'workspace_daily' | 'remaining_balance' | 'monthly' | 'threads' | 'top_threads' | 'plan_history' | 'balance_history'

export interface OpenAIUsageQuery {
  kind: OpenAIUsageKind
  start_date?: string
  end_date?: string
  window_minutes?: 300 | 10080
  cursor?: string
  thread_ids?: string[]
  threads?: { thread_id: string; created_at: string | null; descendant_thread_ids?: string[] }[]
}

export interface OpenAIUsageDetails {
  kind: OpenAIUsageKind
  data: Record<string, unknown> | null
  unavailable: boolean
  upstream_status?: number
  fetched_at: number
  refresh_after_seconds: number
  truncated?: boolean
}

export interface OpenAIUsagePushEvent {
  type: 'quota' | 'fallback'
  transport: 'websocket' | 'polling'
  quota?: OpenAIQuotaUsage
}

export async function queryOpenAIUsage(id: number, query: OpenAIUsageQuery, signal?: AbortSignal): Promise<OpenAIUsageDetails> {
  const { data } = await apiClient.post<OpenAIUsageDetails>(`/admin/openai/accounts/${id}/usage/query`, query, { signal, timeout: 35_000 })
  return data
}

export function openOpenAIUsageSocket(id: number): WebSocket | null {
  const token = getAccessToken()
  if (!token || typeof WebSocket === 'undefined') return null
  const url = new URL(buildApiUrl(`/admin/openai/accounts/${id}/usage/ws`), window.location.origin)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return new WebSocket(url.toString(), ['sub2api-admin', `jwt.${token}`])
}
