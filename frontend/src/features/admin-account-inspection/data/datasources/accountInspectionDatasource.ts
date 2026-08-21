import { apiClient } from '@/core/networks/client'
import type {
  AccountInspectionOverview,
  AccountInspectionQuery,
  AccountInspectionResult,
  AccountInspectionSettings,
} from '../dtos/accountInspectionDtos'

function normalizeAccountInspectionResult(result: AccountInspectionResult): AccountInspectionResult {
  return {
    ...result,
    // Older snapshots omit `reasons` for healthy accounts. Keep the UI DTO stable
    // while mixed-version backends or previously persisted snapshots exist.
    reasons: Array.isArray(result.reasons)
      ? result.reasons.filter((reason): reason is string => typeof reason === 'string')
      : [],
  }
}

function normalizeAccountInspectionOverview(data: AccountInspectionOverview): AccountInspectionOverview {
  if (!data?.results || !Array.isArray(data.results.items)) return data

  return {
    ...data,
    results: {
      ...data.results,
      items: data.results.items.map(normalizeAccountInspectionResult),
    },
  }
}

export async function getOverview(query: AccountInspectionQuery = {}): Promise<AccountInspectionOverview> {
  const { data } = await apiClient.get<AccountInspectionOverview>('/admin/account-inspection', {
    params: query,
  })
  return normalizeAccountInspectionOverview(data)
}

export async function updateSettings(settings: AccountInspectionSettings): Promise<AccountInspectionSettings> {
  const { data } = await apiClient.put<AccountInspectionSettings>('/admin/account-inspection/settings', settings)
  return data
}

export async function runInspection(): Promise<AccountInspectionOverview> {
  const { data } = await apiClient.post<AccountInspectionOverview>('/admin/account-inspection/run')
  return normalizeAccountInspectionOverview(data)
}
