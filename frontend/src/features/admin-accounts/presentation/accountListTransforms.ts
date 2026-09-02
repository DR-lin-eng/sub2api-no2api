import type { Account } from '@/types'
import type { AccountSortState } from './accountSortState'

export function buildAccountQueryFiltersFromState(
  params: Record<string, unknown>,
  sortState: AccountSortState,
) {
  const stringValue = (value: unknown) => typeof value === 'string' ? value : ''
  return {
    platform: stringValue(params.platform),
    type: stringValue(params.type),
    status: stringValue(params.status),
    oauth_quota: stringValue(params.oauth_quota),
    group: stringValue(params.group),
    privacy_mode: stringValue(params.privacy_mode),
    search: stringValue(params.search),
    sort_by: sortState.sort_by,
    sort_order: sortState.sort_order,
  }
}

export function mergeRuntimeFields(oldAccount: Account, updatedAccount: Account): Account {
  return {
    ...updatedAccount,
    current_concurrency: updatedAccount.current_concurrency ?? oldAccount.current_concurrency,
    current_window_cost: updatedAccount.current_window_cost ?? oldAccount.current_window_cost,
    active_sessions: updatedAccount.active_sessions ?? oldAccount.active_sessions,
    cpa_capacity: updatedAccount.cpa_capacity ?? oldAccount.cpa_capacity,
  }
}
