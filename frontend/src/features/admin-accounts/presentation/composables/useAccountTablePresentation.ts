import { computed, type Reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/features/auth/presentation/stores/authStore'
import { formatDateTime } from '@/core/utils/format'
import { proxyExpiryBadgeClass, proxyExpiryLabelKey } from '@/core/utils/proxyExpiry'
import { sanitizeUrl } from '@/core/utils/url'
import type { Account, AccountSchedulerGroupScore, Proxy as AccountProxy } from '@/types'
import { resolveAccountPlanType } from '../grokPlanResolver'

type OpenAICompactBadgeState = 'active' | 'blocked' | 'auto'

export function useAccountTablePresentation(hiddenColumns: Reactive<Set<string>>) {
  const { t } = useI18n()
  const authStore = useAuthStore()

  const getAccountPlanType = (row: Account): string | undefined =>
    resolveAccountPlanType(row)

  const getOpenAIAuthMode = (row: any): string | undefined => {
    if (!row || row.platform !== 'openai' || row.type !== 'oauth') return undefined
    const authMode = row.credentials?.auth_mode
    return typeof authMode === 'string' && authMode.trim() ? authMode : undefined
  }

  const getAntigravityTierFromRow = (row: any): string | null => {
    if (row.platform !== 'antigravity') return null
    const extra = row.extra as Record<string, unknown> | undefined
    if (!extra) return null
    const lca = extra.load_code_assist as Record<string, unknown> | undefined
    if (!lca) return null
    const paid = lca.paidTier as Record<string, unknown> | undefined
    if (paid && typeof paid.id === 'string') return paid.id
    const current = lca.currentTier as Record<string, unknown> | undefined
    if (current && typeof current.id === 'string') return current.id
    return null
  }

  const getAntigravityTierLabel = (row: any): string | null => {
    const tier = getAntigravityTierFromRow(row)
    switch (tier) {
      case 'free-tier': return t('admin.accounts.tier.free')
      case 'g1-pro-tier': return t('admin.accounts.tier.pro')
      case 'g1-ultra-tier': return t('admin.accounts.tier.ultra')
      default: return null
    }
  }

  const accountDisplayEmail = (row: any): string =>
    row.extra?.email_address || row.extra?.email || row.credentials?.email || row.parent_email || ''

  const accountHomepageUrl = (row: Account): string => {
    if (row.type !== 'apikey' || typeof row.credentials?.base_url !== 'string') return ''
    const baseUrl = sanitizeUrl(row.credentials.base_url)
    return baseUrl ? new URL(baseUrl).origin : ''
  }

  const getOpenAICompactState = (row: any): OpenAICompactBadgeState | null => {
    if (row.platform !== 'openai' || (row.type !== 'oauth' && row.type !== 'apikey')) return null
    const extra = row.extra as Record<string, unknown> | undefined
    const mode = typeof extra?.openai_compact_mode === 'string' ? extra.openai_compact_mode : 'auto'
    if (mode === 'force_on') return 'active'
    if (mode === 'force_off') return 'blocked'
    if (typeof extra?.openai_compact_supported === 'boolean') {
      return extra.openai_compact_supported ? 'active' : 'blocked'
    }
    return 'auto'
  }

  const getOpenAICompactMeta = (row: any): { label: string; className: string; dotClass: string } | null => {
    const state = getOpenAICompactState(row)
    if (!state) return null
    switch (state) {
      case 'active':
        return {
          label: t('admin.accounts.openai.compactSupported'),
          className: 'text-emerald-600 dark:text-emerald-300',
          dotClass: 'bg-emerald-500 shadow-[0_0_0_2px_rgba(16,185,129,0.14)]'
        }
      case 'blocked':
        return {
          label: t('admin.accounts.openai.compactUnsupported'),
          className: 'text-rose-600 dark:text-rose-300',
          dotClass: 'bg-rose-500 shadow-[0_0_0_2px_rgba(244,63,94,0.14)]'
        }
      case 'auto':
        return {
          label: t('admin.accounts.openai.compactAuto'),
          className: 'text-slate-500 dark:text-slate-400',
          dotClass: 'bg-slate-300 dark:bg-slate-500'
        }
    }
  }

  const getOpenAICompactTitle = (row: any): string => {
    const extra = row.extra as Record<string, unknown> | undefined
    const checkedAt = typeof extra?.openai_compact_checked_at === 'string' ? extra.openai_compact_checked_at : ''
    const label = getOpenAICompactMeta(row)?.label || ''
    if (!checkedAt) return label
    return `${label} | ${t('admin.accounts.openai.compactLastChecked')}: ${formatDateTime(new Date(checkedAt))}`
  }

  const getAntigravityTierClass = (row: any): string => {
    const tier = getAntigravityTierFromRow(row)
    switch (tier) {
      case 'free-tier': return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
      case 'g1-pro-tier': return 'bg-blue-100 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
      case 'g1-ultra-tier': return 'bg-purple-100 text-purple-600 dark:bg-purple-900/40 dark:text-purple-300'
      default: return ''
    }
  }

  const autoRefreshIntervalLabel = (seconds: number) => {
    if (seconds === 5) return t('admin.accounts.refreshInterval5s')
    if (seconds === 10) return t('admin.accounts.refreshInterval10s')
    if (seconds === 15) return t('admin.accounts.refreshInterval15s')
    if (seconds === 30) return t('admin.accounts.refreshInterval30s')
    return `${seconds}s`
  }

  const formatSchedulerScore = (value: unknown): string => {
    const number = Number(value)
    if (!Number.isFinite(number)) return '-'
    return number.toFixed(6).replace(/\.?0+$/, '')
  }

  const formatStickySchedulerScore = (score: AccountSchedulerGroupScore): string => {
    if (!score) return '-'
    if (score.sticky_score_infinity) return '+∞'
    return formatSchedulerScore(score.sticky_score)
  }

  const getSchedulerScoreRows = (account: Account): AccountSchedulerGroupScore[] => {
    const groupRows = Array.isArray(account.scheduler_scores)
      ? account.scheduler_scores.filter(score => score.group_id != null)
      : []
    if (groupRows.length) return groupRows
    if (account.scheduler_score) return [{ group_id: null, ...account.scheduler_score }]
    return []
  }

  const formatSchedulerScoreGroup = (score: AccountSchedulerGroupScore): string => {
    if ('group_name' in score && score.group_name) return score.group_name
    if ('group_id' in score && score.group_id != null) return `#${score.group_id}`
    return t('admin.accounts.schedulerScore.ungrouped')
  }

  const formatExpiresAt = (value: number | null) => {
    if (!value) return '-'
    return formatDateTime(
      new Date(value * 1000),
      {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false
      },
      'sv-SE'
    )
  }

  const isExpired = (value: number | null) => Boolean(value && value * 1000 <= Date.now())
  const proxyExpiryBadge = (proxy: AccountProxy): string =>
    proxyExpiryBadgeClass(proxy.expires_at, proxy.status)
  const proxyExpiryText = (proxy: AccountProxy): string => {
    const { key, params } = proxyExpiryLabelKey(proxy.expires_at, proxy.status)
    return params ? t(key, params) : t(key)
  }

  const allColumns = computed(() => {
    const columns = [
      { key: 'select', label: '', sortable: false },
      { key: 'name', label: t('admin.accounts.columns.name'), sortable: true },
      { key: 'id', label: t('admin.accounts.columns.id'), sortable: true },
      { key: 'platform_type', label: t('admin.accounts.columns.platformType'), sortable: false },
      { key: 'capacity', label: t('admin.accounts.columns.capacity'), sortable: false },
      { key: 'status', label: t('admin.accounts.columns.status'), sortable: true },
      {
        key: 'schedulable',
        label: t('admin.accounts.columns.schedulable'),
        sortable: true,
        class: 'w-px whitespace-nowrap text-center'
      },
      { key: 'today_stats', label: t('admin.accounts.columns.todayStats'), sortable: false },
      { key: 'hourly_usage', label: t('admin.accounts.columns.hourlyUsage'), sortable: false }
    ]
    if (!authStore.isSimpleMode) {
      columns.push({ key: 'groups', label: t('admin.accounts.columns.groups'), sortable: false })
    }
    columns.push(
      { key: 'usage', label: t('admin.accounts.columns.usageWindows'), sortable: false },
      { key: 'proxy', label: t('admin.accounts.columns.proxy'), sortable: false },
      { key: 'priority', label: t('admin.accounts.columns.priority'), sortable: true },
      { key: 'scheduler_score', label: t('admin.accounts.columns.schedulerScore'), sortable: false },
      { key: 'rate_multiplier', label: t('admin.accounts.columns.billingRateMultiplier'), sortable: true },
      { key: 'upstream_billing_rate', label: t('admin.accounts.columns.upstreamBillingRate'), sortable: true },
      { key: 'last_used_at', label: t('admin.accounts.columns.lastUsed'), sortable: true },
      { key: 'created_at', label: t('admin.accounts.columns.createdAt'), sortable: true },
      { key: 'expires_at', label: t('admin.accounts.columns.expiresAt'), sortable: true },
      { key: 'notes', label: t('admin.accounts.columns.notes'), sortable: false },
      { key: 'actions', label: t('admin.accounts.columns.actions'), sortable: false }
    )
    return columns
  })

  const toggleableColumns = computed(() =>
    allColumns.value.filter(column => column.key !== 'select' && column.key !== 'name' && column.key !== 'actions')
  )

  const cols = computed(() =>
    allColumns.value.filter(column =>
      column.key === 'select' || column.key === 'name' || column.key === 'actions' || !hiddenColumns.has(column.key)
    )
  )

  return {
    accountDisplayEmail,
    accountHomepageUrl,
    getAccountPlanType,
    getOpenAIAuthMode,
    getAntigravityTierLabel,
    getAntigravityTierClass,
    getOpenAICompactMeta,
    getOpenAICompactTitle,
    autoRefreshIntervalLabel,
    getSchedulerScoreRows,
    formatSchedulerScoreGroup,
    formatSchedulerScore,
    formatStickySchedulerScore,
    formatExpiresAt,
    isExpired,
    proxyExpiryBadge,
    proxyExpiryText,
    toggleableColumns,
    cols
  }
}
