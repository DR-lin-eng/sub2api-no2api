import type { Account } from '@/types'

interface AccountIDRow {
  id: number
}

interface AccountListPage {
  items: AccountIDRow[]
  total: number
  pages?: number
}

type AccountPageFetcher = (
  page: number,
  pageSize: number,
  filters: Record<string, unknown>
) => Promise<AccountListPage>

const selectAllPageSize = 1000
const ACCOUNT_UNGROUPED_GROUP_QUERY_VALUE = 'ungrouped'
const ACCOUNT_PRIVACY_MODE_UNSET_QUERY_VALUE = '__unset__'

export interface AccountSelectionFilters {
  platform?: string
  type?: string
  status?: string
  oauth_quota?: string
  group?: string
  privacy_mode?: string
  search?: string
}

const readQuotaNumber = (value: unknown): number | null => {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : null
  }
  return null
}

const quotaResetIsActive = (value: unknown, now: number, unixSeconds = false): boolean => {
  if (value == null || value === '') return true
  if (unixSeconds) {
    const seconds = readQuotaNumber(value)
    return seconds == null || seconds > now / 1000
  }
  if (typeof value !== 'string' && !(value instanceof Date)) return true
  const timestamp = new Date(value).getTime()
  return Number.isNaN(timestamp) || timestamp > now
}

const accountHasExhaustedOAuthQuota = (account: Account, now: number): boolean => {
  if (account.type !== 'oauth' || !account.extra) return false
  const percentWindows = [
    ['codex_5h_used_percent', 'codex_5h_reset_at'],
    ['codex_7d_used_percent', 'codex_7d_reset_at'],
    ['codex_primary_used_percent', 'codex_primary_reset_at'],
    ['codex_secondary_used_percent', 'codex_secondary_reset_at']
  ]
  if (percentWindows.some(([usageKey, resetKey]) =>
    (readQuotaNumber(account.extra?.[usageKey]) ?? -1) >= 100 && quotaResetIsActive(account.extra?.[resetKey], now)
  )) return true
  const ratioWindows = [
    ['session_window_utilization', 'session_window_end'],
    ['passive_usage_7d_utilization', 'passive_usage_7d_reset'],
    ['passive_usage_7d_oi_utilization', 'passive_usage_7d_oi_reset']
  ]
  if (ratioWindows.some(([usageKey, resetKey]) => {
    const resetValue = resetKey === 'session_window_end' ? account.session_window_end : account.extra?.[resetKey]
    return (readQuotaNumber(account.extra?.[usageKey]) ?? -1) >= 1 && quotaResetIsActive(resetValue, now, resetKey !== 'session_window_end')
  })) return true
  const billing = account.extra.grok_billing_snapshot
  if (billing && typeof billing === 'object' && !Array.isArray(billing)) {
    const snapshot = billing as Record<string, unknown>
    const billingWindowActive = quotaResetIsActive(snapshot.period_end, now) && quotaResetIsActive(snapshot.billing_period_end, now)
    return billingWindowActive && ['usage_percent', 'used_percent'].some(key => (readQuotaNumber(snapshot[key]) ?? -1) >= 100)
  }
  return false
}

export function accountMatchesFilters(account: Account, filters: AccountSelectionFilters, now = Date.now()): boolean {
  if (filters.platform && account.platform !== filters.platform) return false
  if (filters.type && account.type !== filters.type) return false
  if (filters.oauth_quota === 'exhausted' && !accountHasExhaustedOAuthQuota(account, now)) return false
  if (filters.status) {
    const rateLimitResetAt = account.rate_limit_reset_at ? new Date(account.rate_limit_reset_at).getTime() : Number.NaN
    const isRateLimited = Number.isFinite(rateLimitResetAt) && rateLimitResetAt > now
    const tempUnschedUntil = account.temp_unschedulable_until ? new Date(account.temp_unschedulable_until).getTime() : Number.NaN
    const isTempUnschedulable = Number.isFinite(tempUnschedUntil) && tempUnschedUntil > now

    if (filters.status === 'active') {
      if (account.status !== 'active' || isRateLimited || isTempUnschedulable || !account.schedulable) return false
    } else if (filters.status === 'rate_limited') {
      if (account.status !== 'active' || !isRateLimited || isTempUnschedulable) return false
    } else if (filters.status === 'temp_unschedulable') {
      if (account.status !== 'active' || !isTempUnschedulable) return false
    } else if (filters.status === 'unschedulable') {
      if (account.status !== 'active' || account.schedulable || isRateLimited || isTempUnschedulable) return false
    } else if (account.status !== filters.status) return false
  }
  if (filters.group) {
    const groupIds = account.group_ids ?? account.groups?.map((group) => group.id) ?? []
    if (filters.group === ACCOUNT_UNGROUPED_GROUP_QUERY_VALUE) {
      if (groupIds.length > 0) return false
    } else if (!groupIds.includes(Number(filters.group))) return false
  }
  const privacyMode = typeof account.extra?.privacy_mode === 'string' ? account.extra.privacy_mode : ''
  if (filters.privacy_mode) {
    if (filters.privacy_mode === ACCOUNT_PRIVACY_MODE_UNSET_QUERY_VALUE) {
      if (privacyMode.trim() !== '') return false
    } else if (privacyMode !== filters.privacy_mode) return false
  }
  const search = String(filters.search || '').trim().toLowerCase()
  return !search || account.name.toLowerCase().includes(search)
}

export async function fetchAllAccountIds(
  fetchPage: AccountPageFetcher,
  filters: Record<string, unknown>
): Promise<number[]> {
  const requestFilters = {
    ...filters,
    lite: '1',
    include_scheduler_score: '0'
  }
  const firstPage = await fetchPage(1, selectAllPageSize, requestFilters)
  const pageCount = Math.max(
    firstPage.pages ?? 0,
    Math.ceil(firstPage.total / selectAllPageSize)
  )
  const ids = firstPage.items.map(account => account.id)

  for (let page = 2; page <= pageCount; page++) {
    const result = await fetchPage(page, selectAllPageSize, requestFilters)
    ids.push(...result.items.map(account => account.id))
  }

  const uniqueIDs = Array.from(new Set(ids))
  if (uniqueIDs.length !== firstPage.total) {
    throw new Error('account list result is incomplete')
  }
  return uniqueIDs
}
