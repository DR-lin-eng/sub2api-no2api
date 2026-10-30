const platforms = new Set(['anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'multimodal'])

export const ACCOUNT_FILTER_STORAGE_KEY = 'account-filter-values'
export type AccountFilterState = { platform: string; status: string }
const statuses = new Set(['active', 'inactive', 'error', 'rate_limited', 'temp_unschedulable', 'unschedulable'])

export const loadInitialAccountFilterState = (
  storage: Pick<Storage, 'getItem'> = localStorage,
): AccountFilterState => {
  const fallback = { platform: '', status: '' }
  try {
    const raw = storage.getItem(ACCOUNT_FILTER_STORAGE_KEY)
    if (!raw) return fallback
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return fallback
    const values = parsed as Record<string, unknown>
    return {
      platform: typeof values.platform === 'string' && platforms.has(values.platform) ? values.platform : '',
      status: typeof values.status === 'string' && statuses.has(values.status) ? values.status : '',
    }
  } catch { return fallback }
}

export const saveAccountFilterState = (
  values: AccountFilterState,
  storage: Pick<Storage, 'setItem'> = localStorage,
): void => {
  try { storage.setItem(ACCOUNT_FILTER_STORAGE_KEY, JSON.stringify({ platform: values.platform, status: values.status })) }
  catch { /* Filtering remains available when storage is unavailable. */ }
}
