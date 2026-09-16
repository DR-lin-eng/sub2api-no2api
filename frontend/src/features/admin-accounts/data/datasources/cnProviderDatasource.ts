import { apiClient } from '@/core/networks/client'

export interface CNQuotaTier {
  window: '5h' | 'weekly' | 'monthly'
  used_percent: number
  reset_at?: string
}

export interface CNProviderQuotaProbeResult {
  provider: string
  source?: string
  success: boolean
  credential_valid: boolean
  tiers?: CNQuotaTier[]
  plan_level?: string
  status_code?: number
  fetched_at: number
  persisted: boolean
  error?: string
}

export interface CNProviderBalanceEntry {
  currency: string
  balance: number
}

export interface CNProviderBalanceResult {
  provider: string
  success: boolean
  balance: number
  currency?: string
  balances?: CNProviderBalanceEntry[]
  available: boolean
  status_code?: number
  fetched_at: number
  persisted: boolean
  error?: string
}

export async function queryCNProviderQuota(id: number): Promise<CNProviderQuotaProbeResult> {
  const { data } = await apiClient.post<CNProviderQuotaProbeResult>(`/admin/accounts/${id}/cn-provider/quota`)
  return data
}

export async function queryCNProviderBalance(id: number): Promise<CNProviderBalanceResult> {
  const { data } = await apiClient.post<CNProviderBalanceResult>(`/admin/accounts/${id}/cn-provider/balance`)
  return data
}
