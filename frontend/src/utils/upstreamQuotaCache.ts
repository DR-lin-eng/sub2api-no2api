import type { Account, UpstreamQuotaQueryResult } from '@/types'

const CACHE_PREFIX = 'sub2api:admin:upstream-quota:v2'

type StoredUpstreamQuotaCacheEntry = {
  identity: string
  result: UpstreamQuotaQueryResult
}

const isOptionalFiniteNumber = (value: unknown): boolean => value == null ||
  (typeof value === 'number' && Number.isFinite(value))

const isOptionalDate = (value: unknown): boolean => value == null ||
  (typeof value === 'string' && Number.isFinite(Date.parse(value)))

const hasInvalidWindows = (windows: unknown): boolean => windows != null && (
  !Array.isArray(windows) || windows.some(window => {
    if (!window || typeof window !== 'object') return true
    const value = window as Record<string, unknown>
    return typeof value.name !== 'string' ||
      !isOptionalFiniteNumber(value.used) ||
      !isOptionalFiniteNumber(value.limit) ||
      !isOptionalFiniteNumber(value.remaining) ||
      !isOptionalDate(value.reset_at)
  })
)

const isStoredEntry = (value: unknown, accountID: number): value is StoredUpstreamQuotaCacheEntry => {
  if (!value || typeof value !== 'object') return false
  const entry = value as { identity?: unknown; result?: unknown }
  if (typeof entry.identity !== 'string' || !entry.result || typeof entry.result !== 'object') return false

  const result = entry.result as { account_id?: unknown; observed_at?: unknown; quota?: unknown }
  if (result.account_id !== accountID ||
    typeof result.observed_at !== 'string' ||
    !Number.isFinite(Date.parse(result.observed_at)) ||
    !result.quota ||
    typeof result.quota !== 'object') return false

  const quota = result.quota as Record<string, unknown>
  if ((quota.provider !== 'sub2api' && quota.provider !== 'new_api') ||
    (quota.mode !== 'balance' &&
      quota.mode !== 'quota' &&
      quota.mode !== 'subscription' &&
      quota.mode !== 'rate_limits')) return false
  if (quota.unit != null && quota.unit !== 'USD' && quota.unit !== 'CNY' && quota.unit !== 'TOKENS') return false
  if (!isOptionalFiniteNumber(quota.remaining) ||
    !isOptionalFiniteNumber(quota.used) ||
    !isOptionalFiniteNumber(quota.total) ||
    !isOptionalDate(quota.expires_at) ||
    hasInvalidWindows(quota.windows)) return false

  if (quota.subscription != null) {
    if (typeof quota.subscription !== 'object') return false
    const subscription = quota.subscription as Record<string, unknown>
    if (typeof subscription.plan_name !== 'string' ||
      subscription.plan_name.trim() === '' ||
      typeof subscription.expires_at !== 'string' ||
      !Number.isFinite(Date.parse(subscription.expires_at)) ||
      !isOptionalFiniteNumber(subscription.remaining) ||
      (subscription.unlimited != null && typeof subscription.unlimited !== 'boolean') ||
      hasInvalidWindows(subscription.windows)) return false
  }

  return true
}

const cachePrefix = (adminID: number): string | null => Number.isSafeInteger(adminID) && adminID > 0
  ? `${CACHE_PREFIX}:${adminID}:`
  : null

const cacheKey = (adminID: number, accountID: number): string | null => {
  const prefix = cachePrefix(adminID)
  return prefix && Number.isSafeInteger(accountID) && accountID > 0 ? `${prefix}${accountID}` : null
}

export const upstreamQuotaCacheIdentity = (account: Account): string => JSON.stringify({
  platform: account.platform,
  type: account.type,
  base_url: typeof account.credentials?.base_url === 'string' ? account.credentials.base_url : '',
  has_api_key: account.credentials_status?.has_api_key === true,
  proxy_id: account.proxy_id,
  proxy_updated_at: account.proxy?.updated_at ?? null,
  enable_tls_fingerprint: account.extra?.enable_tls_fingerprint ?? account.enable_tls_fingerprint ?? null,
  tls_fingerprint_profile_id: account.extra?.tls_fingerprint_profile_id ?? account.tls_fingerprint_profile_id ?? null
})

export const readUpstreamQuotaCache = (
  storage: Storage,
  adminID: number,
  account: Account
): UpstreamQuotaQueryResult | null => {
  const key = cacheKey(adminID, account.id)
  if (!key) return null
  try {
    const raw = storage.getItem(key)
    if (!raw) return null
    const entry: unknown = JSON.parse(raw)
    if (!isStoredEntry(entry, account.id) || entry.identity !== upstreamQuotaCacheIdentity(account)) {
      storage.removeItem(key)
      return null
    }
    return entry.result
  } catch {
    try { storage.removeItem(key) } catch { /* Storage may be disabled. */ }
    return null
  }
}

export const persistUpstreamQuotaCache = (
  storage: Storage,
  adminID: number,
  account: Account,
  result: UpstreamQuotaQueryResult
): void => {
  const key = cacheKey(adminID, account.id)
  if (!key || !result.quota || !isStoredEntry({ identity: upstreamQuotaCacheIdentity(account), result }, account.id)) return
  try {
    storage.setItem(key, JSON.stringify({ identity: upstreamQuotaCacheIdentity(account), result }))
  } catch {
    // A storage failure must not hide the successfully queried in-memory result.
  }
}

export const removeUpstreamQuotaCache = (
  storage: Storage,
  adminID: number,
  accountID?: number
): void => {
  const prefix = cachePrefix(adminID)
  if (!prefix) return
  try {
    if (accountID != null) {
      const key = cacheKey(adminID, accountID)
      if (key) storage.removeItem(key)
      return
    }
    for (let index = storage.length - 1; index >= 0; index -= 1) {
      const key = storage.key(index)
      if (key?.startsWith(prefix)) storage.removeItem(key)
    }
  } catch {
    // Storage may be disabled by the browser.
  }
}
