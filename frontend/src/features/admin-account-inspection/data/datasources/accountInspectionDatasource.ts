import { apiClient } from '@/core/networks/client'
import type {
  AccountInspectionOverview,
  AccountInspectionQuery,
  AccountInspectionSettings,
} from '../dtos/accountInspectionDtos'

export async function getOverview(query: AccountInspectionQuery = {}): Promise<AccountInspectionOverview> {
  const { data } = await apiClient.get<AccountInspectionOverview>('/admin/account-inspection', {
    params: query,
  })
  return data
}

export async function updateSettings(settings: AccountInspectionSettings): Promise<AccountInspectionSettings> {
  const { data } = await apiClient.put<AccountInspectionSettings>('/admin/account-inspection/settings', settings)
  return data
}

export async function runInspection(): Promise<AccountInspectionOverview> {
  const { data } = await apiClient.post<AccountInspectionOverview>('/admin/account-inspection/run')
  return data
}
