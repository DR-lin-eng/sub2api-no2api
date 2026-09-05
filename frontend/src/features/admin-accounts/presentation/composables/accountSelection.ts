import type { Account } from '@/types'
import { ACCOUNT_OAUTH_QUOTA_FILTER } from '@/features/admin-accounts/data/dtos/accountQuotaFilters'

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

const quotaWindowIsActive = (
  extra: Record<string, unknown>,
  resetAtKey: string,
  resetAfterKey: string,
  now: number
): boolean => {
  if (extra[resetAtKey] != null && extra[resetAtKey] !== '') {
    return quotaResetIsActive(extra[resetAtKey], now)
  }
  const resetAfter = readQuotaNumber(extra[resetAfterKey])
  if (resetAfter != null) {
    const updatedAt = typeof extra.codex_usage_updated_at === 'string'
      ? Date.parse(extra.codex_usage_updated_at)
      : Number.NaN
    if (Number.isFinite(updatedAt)) return updatedAt + resetAfter * 1000 > now
  }
  return true
}

interface OpenAIQuotaSnapshotWindow {
  used: unknown
  durationMinutes: unknown
  resetAt: unknown
}

const snapshotObject = (value: unknown): Record<string, unknown> | null => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  return value as Record<string, unknown>
}

const openAIQuotaSnapshotWindows = (account: Account): OpenAIQuotaSnapshotWindow[] => {
  const extra = snapshotObject(account.extra?.codex_rate_limit_snapshot)
  if (!extra) return []
  const windows: OpenAIQuotaSnapshotWindow[] = []
  const buckets = snapshotObject(extra.rate_limits_by_limit_id ?? extra.rateLimitsByLimitId)
  if (buckets) {
    Object.values(buckets).forEach((rawBucket) => {
      const bucket = snapshotObject(rawBucket)
      if (!bucket) return
      const nested = ['primary', 'secondary']
        .map(key => snapshotObject(bucket[key]))
        .filter((window): window is Record<string, unknown> => window != null)
      if (nested.length > 0) {
        nested.forEach(window => windows.push({
          used: window.used_percent ?? window.usedPercent,
          durationMinutes: window.window_duration_mins ?? window.windowDurationMins,
          resetAt: window.resets_at ?? window.resetsAt
        }))
        return
      }
      if (bucket.used_percent != null || bucket.usedPercent != null) {
        windows.push({
          used: bucket.used_percent ?? bucket.usedPercent,
          durationMinutes: bucket.window_duration_mins ?? bucket.windowDurationMins,
          resetAt: bucket.resets_at ?? bucket.resetsAt
        })
      }
    })
  }
  const legacy = snapshotObject(extra.rate_limit ?? extra.rateLimit)
  if (legacy) {
    const legacyWindowKeys = ['primary_window', 'secondary_window', 'primary', 'secondary']
    legacyWindowKeys.forEach(key => {
      const window = snapshotObject(legacy[key])
      if (!window) return
      windows.push({
        used: window.used_percent ?? window.usedPercent,
        durationMinutes: window.window_duration_mins ?? window.windowDurationMins ?? (
          readQuotaNumber(window.limit_window_seconds) != null
            ? readQuotaNumber(window.limit_window_seconds)! / 60
            : undefined
        ),
        resetAt: window.reset_at ?? window.resets_at ?? window.resetsAt
      })
    })
  }
  return windows
}

const openAIQuotaWindowMatches = (
  account: Account,
  window: '5h' | '7d',
  now: number,
  exhausted: boolean
): boolean => {
  if (account.platform !== 'openai' || account.type !== 'oauth' || !account.extra) return false
  const extra = account.extra as Record<string, unknown>
  const canonical = window === '5h'
    ? { used: 'codex_5h_used_percent', resetAt: 'codex_5h_reset_at', resetAfter: 'codex_5h_reset_after_seconds' }
    : { used: 'codex_7d_used_percent', resetAt: 'codex_7d_reset_at', resetAfter: 'codex_7d_reset_after_seconds' }
  const canonicalUsed = readQuotaNumber(extra[canonical.used])
  if (canonicalUsed != null && (exhausted ? canonicalUsed >= 100 : canonicalUsed < 100) &&
    quotaWindowIsActive(extra, canonical.resetAt, canonical.resetAfter, now)) return true

  const legacyWindows = [
    { name: 'primary', defaultWindow: '7d' },
    { name: 'secondary', defaultWindow: '5h' }
  ] as const
  if (legacyWindows.some(candidate => {
    const used = readQuotaNumber(extra[`codex_${candidate.name}_used_percent`])
    const durationKey = `codex_${candidate.name}_window_minutes`
    const duration = readQuotaNumber(extra[durationKey])
    const matchesWindow = duration == null
      ? !Object.prototype.hasOwnProperty.call(extra, durationKey) && candidate.defaultWindow === window
      : window === '5h' ? duration >= 240 && duration <= 360 : duration >= 10000 && duration <= 10160
    return used != null && matchesWindow && (exhausted ? used >= 100 : used < 100) && quotaWindowIsActive(
      extra,
      `codex_${candidate.name}_reset_at`,
      `codex_${candidate.name}_reset_after_seconds`,
      now
    )
  })) return true

  return openAIQuotaSnapshotWindows(account).some(snapshot => {
    const duration = readQuotaNumber(snapshot.durationMinutes)
    const used = readQuotaNumber(snapshot.used)
    if (duration == null || used == null) return false
    const inWindow = window === '5h' ? duration >= 240 && duration <= 360 : duration >= 10000 && duration <= 10160
    return inWindow && (exhausted ? used >= 100 : used < 100) && quotaResetIsActive(snapshot.resetAt, now, true)
  })
}

const accountHasOpenAIQuotaReset = (account: Account, now: number): boolean => {
  if (account.platform !== 'openai' || account.type !== 'oauth' || !account.extra) return false
  const extra = account.extra as Record<string, unknown>
  const resetCredits = snapshotObject(extra.codex_reset_credit_snapshot)
  const count = readQuotaNumber(resetCredits?.available_count ?? resetCredits?.availableCount) ?? 0
  if (count <= 0 || !Array.isArray(resetCredits?.credits)) return false
  return resetCredits.credits.some(rawCredit => {
    const credit = snapshotObject(rawCredit)
    const expiresAt = credit?.expires_at ?? credit?.expiresAt
    if (typeof expiresAt !== 'string' || expiresAt.trim() === '') return false
    const expiry = Date.parse(expiresAt)
    return Number.isNaN(expiry) || expiry > now
  })
}

const accountHasKnownOAuthQuota = (account: Account): boolean => {
  if (account.type !== 'oauth' || !account.extra) return false
  const extra = account.extra as Record<string, unknown>
  const knownKeys = [
    'codex_5h_used_percent', 'codex_7d_used_percent',
    'codex_primary_used_percent', 'codex_secondary_used_percent',
    'session_window_utilization', 'passive_usage_7d_utilization',
    'passive_usage_7d_oi_utilization'
  ]
  if (knownKeys.some(key => readQuotaNumber(extra[key]) != null)) return true
  const billing = snapshotObject(extra.grok_billing_snapshot)
  if (billing && ['usage_percent', 'used_percent'].some(key => readQuotaNumber(billing[key]) != null)) return true
  return openAIQuotaSnapshotWindows(account).some(window => readQuotaNumber(window.used) != null)
}

const accountHasExhaustedOAuthQuota = (account: Account, now: number): boolean => {
  if (account.type !== 'oauth' || !account.extra) return false
  const extra = account.extra as Record<string, unknown>
  const percentWindows = [
    ['codex_5h_used_percent', 'codex_5h_reset_at', 'codex_5h_reset_after_seconds'],
    ['codex_7d_used_percent', 'codex_7d_reset_at', 'codex_7d_reset_after_seconds'],
    ['codex_primary_used_percent', 'codex_primary_reset_at', 'codex_primary_reset_after_seconds'],
    ['codex_secondary_used_percent', 'codex_secondary_reset_at', 'codex_secondary_reset_after_seconds']
  ]
  if (percentWindows.some(([usageKey, resetKey, resetAfterKey]) =>
    (readQuotaNumber(extra[usageKey]) ?? -1) >= 100 && quotaWindowIsActive(extra, resetKey, resetAfterKey, now)
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
    if (billingWindowActive && ['usage_percent', 'used_percent'].some(key => (readQuotaNumber(snapshot[key]) ?? -1) >= 100)) return true
  }
  return openAIQuotaSnapshotWindows(account).some(window =>
    (readQuotaNumber(window.used) ?? -1) >= 100 && quotaResetIsActive(window.resetAt, now, true)
  )
}

export function accountMatchesFilters(account: Account, filters: AccountSelectionFilters, now = Date.now()): boolean {
  if (filters.platform && account.platform !== filters.platform) return false
  if (filters.type && account.type !== filters.type) return false
  const quotaFilter = filters.oauth_quota ?? ''
  if (quotaFilter === ACCOUNT_OAUTH_QUOTA_FILTER.exhausted && !accountHasExhaustedOAuthQuota(account, now)) return false
  if (quotaFilter === ACCOUNT_OAUTH_QUOTA_FILTER.hasQuota &&
    (!accountHasKnownOAuthQuota(account) || accountHasExhaustedOAuthQuota(account, now))) return false
  if (quotaFilter === ACCOUNT_OAUTH_QUOTA_FILTER.withReset && !accountHasOpenAIQuotaReset(account, now)) return false
  if (quotaFilter === ACCOUNT_OAUTH_QUOTA_FILTER.fiveHourExhausted && !openAIQuotaWindowMatches(account, '5h', now, true)) return false
  if (quotaFilter === ACCOUNT_OAUTH_QUOTA_FILTER.sevenDayExhausted && !openAIQuotaWindowMatches(account, '7d', now, true)) return false
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
