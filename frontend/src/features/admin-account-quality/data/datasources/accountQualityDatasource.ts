import { apiClient } from '@/core/networks/client'
import type { AccountQualityOverview, AccountQualitySettings } from '../dtos/accountQualityDtos'

export async function getOverview(query: Record<string, string | number> = {}): Promise<AccountQualityOverview> {
  const { data } = await apiClient.get<AccountQualityOverview>('/admin/account-quality', { params: query })
  return data
}

export async function updateSettings(settings: AccountQualitySettings): Promise<AccountQualitySettings> {
  const { data } = await apiClient.put<AccountQualitySettings>('/admin/account-quality/settings', settings)
  return data
}

export async function runQualityMonitoring(): Promise<AccountQualityOverview> {
  // Two five-minute stages can run sequentially; keep the browser request
  // alive for the backend's eleven-minute quality-run budget.
  const { data } = await apiClient.post<AccountQualityOverview>('/admin/account-quality/run', undefined, { timeout: 660000 })
  return data
}
