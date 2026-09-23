import { apiClient } from '@/core/networks/client'
import type { Account, PaginatedResponse } from '@/types'
import type { CodexTurnStateObservability } from '@/features/admin-settings/data/dtos/adminSettingsDtos'

/**
 * Read-only runtime diagnostics for the current process. The response is
 * deliberately redacted by the backend and never contains opaque state data.
 */
export async function getStateDiagnostics(): Promise<CodexTurnStateObservability> {
  const { data } = await apiClient.get<CodexTurnStateObservability>('/admin/state-diagnostics')
  return data
}
/**
 * The state observer is account-scoped, so the page also needs the complete
 * OpenAI OAuth pool to show accounts that have not produced an observation yet.
 */
export async function listStatePoolAccounts(signal?: AbortSignal): Promise<Account[]> {
  const { data } = await apiClient.get<PaginatedResponse<Account>>('/admin/accounts', {
    params: {
      page: 1,
      page_size: 1000,
      platform: 'openai',
      type: 'oauth',
      sort_by: 'name',
      sort_order: 'asc',
      lite: '1',
    },
    signal,
  })
  return data.items ?? []
}
