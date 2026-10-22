<template>
  <div class="space-y-1">
    <OllamaCloudUsageCell
      v-if="account.ollama_cloud_usage?.eligible"
      :account="account"
    />

    <div
      v-if="todayStats"
      class="mb-0.5 flex items-center"
    >
      <div class="flex items-center gap-1.5 text-[9px] text-gray-500 dark:text-gray-400">
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
          {{ formattedRequests }} req
        </span>
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">
          {{ formattedTokens }}
        </span>
        <span
          class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800"
          :title="t('usage.accountBilled')"
        >
          A ${{ formattedCost }}
        </span>
        <span
          v-if="todayStats.user_cost != null"
          class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800"
          :title="t('usage.userBilled')"
        >
          U ${{ formattedUserCost }}
        </span>
      </div>
    </div>

    <div
      v-else-if="todayStatsLoading"
      class="mb-0.5 flex items-center gap-1"
    >
      <div class="h-3 w-10 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-3 w-8 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-3 w-12 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
    </div>

    <div
      v-if="showConstraintSourceLabels && hasApiKeyQuota"
      class="text-[10px] font-medium text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.accounts.usageWindow.localSource') }}
    </div>

    <UsageProgressBar
      v-if="quotaDailyBar"
      label="1d"
      :utilization="quotaDailyBar.utilization"
      :resets-at="quotaDailyBar.resetsAt"
      color="indigo"
    />
    <UsageProgressBar
      v-if="quotaWeeklyBar"
      label="7d"
      :utilization="quotaWeeklyBar.utilization"
      :resets-at="quotaWeeklyBar.resetsAt"
      color="emerald"
    />
    <UsageProgressBar
      v-if="quotaTotalBar"
      :label="t('admin.accounts.usageWindow.totalLimit')"
      :utilization="quotaTotalBar.utilization"
      color="purple"
    />

    <template v-if="upstreamKeyBars.length">
      <div
        v-if="showConstraintSourceLabels"
        class="text-[10px] font-medium text-gray-500 dark:text-gray-400"
      >
        {{ t('admin.accounts.usageWindow.keySource') }}
      </div>
      <UsageProgressBar
        v-for="bar in upstreamKeyBars"
        :key="bar.key"
        :label="bar.label"
        :utilization="bar.utilization"
        :resets-at="bar.resetsAt"
        :show-now-when-idle="true"
        :color="bar.color"
      />
    </template>

    <template v-if="upstreamSubscription">
      <div
        v-if="showConstraintSourceLabels"
        class="text-[10px] font-medium text-gray-500 dark:text-gray-400"
      >
        {{ t('admin.accounts.usageWindow.subscriptionSource') }}
      </div>
      <div
        v-if="upstreamSubscriptionExpired"
        class="text-[10px] font-medium text-red-600 dark:text-red-400"
        data-testid="upstream-subscription-expired"
      >
        {{ t('admin.accounts.usageWindow.subscriptionExpired') }}
      </div>
      <div
        v-else-if="upstreamSubscription.unlimited"
        class="text-[10px] font-medium text-emerald-600 dark:text-emerald-400"
        data-testid="upstream-subscription-unlimited"
      >
        {{ t('admin.accounts.usageWindow.unlimitedSubscription') }}
      </div>
      <template v-else>
        <UsageProgressBar
          v-for="bar in upstreamSubscriptionBars"
          :key="bar.key"
          :label="bar.label"
          :utilization="bar.utilization"
          :resets-at="bar.resetsAt"
          :show-now-when-idle="true"
          :color="bar.color"
        />
      </template>
    </template>

    <div
      v-if="!todayStats && !todayStatsLoading && !hasApiKeyQuota && !hasUpstreamConstraints && !account.ollama_cloud_usage?.eligible"
      class="text-xs text-gray-400"
    >-</div>
  </div>
</template>

<script setup lang="ts">
import { supportsAccountQuota } from '@/features/admin-accounts/presentation/accountQuota'

import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  Account,
  UpstreamQuotaQueryResult,
  UpstreamQuotaWindow,
  WindowStats
} from '@/types'
import OllamaCloudUsageCell from './OllamaCloudUsageCell.vue'
import UsageProgressBar from './UsageProgressBar.vue'

const props = withDefaults(
  defineProps<{
    account: Account
    todayStats?: WindowStats | null
    todayStatsLoading?: boolean
    upstreamQuotaResult?: UpstreamQuotaQueryResult | null
    now?: number
    formattedRequests: string
    formattedTokens: string
    formattedCost: string
    formattedUserCost: string
  }>(),
  {
    todayStats: null,
    todayStatsLoading: false,
    upstreamQuotaResult: null
  }
)

const { t } = useI18n()

interface QuotaBarInfo {
  utilization: number
  resetsAt: string | null
}

const makeQuotaBar = (
  used: number,
  limit: number,
  startKey?: string
): QuotaBarInfo => {
  const utilization = limit > 0 ? (used / limit) * 100 : 0
  let resetsAt: string | null = null
  if (startKey) {
    const extra = props.account.extra as Record<string, unknown> | undefined
    const isDaily = startKey.includes('daily')
    const mode = isDaily
      ? (extra?.quota_daily_reset_mode as string) || 'rolling'
      : (extra?.quota_weekly_reset_mode as string) || 'rolling'

    if (mode === 'fixed') {
      const resetAtKey = isDaily ? 'quota_daily_reset_at' : 'quota_weekly_reset_at'
      resetsAt = (extra?.[resetAtKey] as string) || null
    } else {
      const startStr = extra?.[startKey] as string | undefined
      if (startStr) {
        const startDate = new Date(startStr)
        const periodMs = isDaily ? 24 * 60 * 60 * 1000 : 7 * 24 * 60 * 60 * 1000
        resetsAt = new Date(startDate.getTime() + periodMs).toISOString()
      }
    }
  }
  return { utilization, resetsAt }
}

const hasApiKeyQuota = computed(() => {
  if (!supportsAccountQuota(props.account.type)) return false
  return (
    (props.account.quota_daily_limit ?? 0) > 0 ||
    (props.account.quota_weekly_limit ?? 0) > 0 ||
    (props.account.quota_limit ?? 0) > 0
  )
})

const quotaDailyBar = computed((): QuotaBarInfo | null => {
  const limit = props.account.quota_daily_limit ?? 0
  if (limit <= 0) return null
  return makeQuotaBar(props.account.quota_daily_used ?? 0, limit, 'quota_daily_start')
})

const quotaWeeklyBar = computed((): QuotaBarInfo | null => {
  const limit = props.account.quota_weekly_limit ?? 0
  if (limit <= 0) return null
  return makeQuotaBar(props.account.quota_weekly_used ?? 0, limit, 'quota_weekly_start')
})

const quotaTotalBar = computed((): QuotaBarInfo | null => {
  const limit = props.account.quota_limit ?? 0
  if (limit <= 0) return null
  return makeQuotaBar(props.account.quota_used ?? 0, limit)
})

type UpstreamBarColor = 'indigo' | 'emerald' | 'purple' | 'amber'
type UpstreamUsageBar = {
  key: string
  label: string
  utilization: number
  resetsAt: string | null
  color: UpstreamBarColor
}

const upstreamQuota = computed(() => {
  const result = props.upstreamQuotaResult
  if (!result || result.account_id !== props.account.id || result.quota?.provider !== 'sub2api') {
    return null
  }
  return result.quota
})

const upstreamSubscription = computed(() => upstreamQuota.value?.subscription ?? null)
const upstreamSubscriptionExpired = computed(() => {
  const expiresAt = upstreamSubscription.value?.expires_at
  const timestamp = expiresAt ? Date.parse(expiresAt) : Number.NaN
  return Number.isFinite(timestamp) && timestamp <= (props.now ?? Date.now())
})

const makeUpstreamUsageBar = (
  window: UpstreamQuotaWindow,
  label: string,
  color: UpstreamBarColor,
  keyPrefix: string
): UpstreamUsageBar | null => {
  const used = window.used
  const limit = window.limit
  if (
    typeof used !== 'number' ||
    !Number.isFinite(used) ||
    typeof limit !== 'number' ||
    !Number.isFinite(limit) ||
    limit <= 0
  ) {
    return null
  }
  return {
    key: `${keyPrefix}:${window.name}`,
    label,
    utilization: (used / limit) * 100,
    resetsAt: typeof window.reset_at === 'string' ? window.reset_at : null,
    color
  }
}

const upstreamKeyBars = computed<UpstreamUsageBar[]>(() => {
  const quota = upstreamQuota.value
  if (!quota || quota.mode === 'subscription' || quota.mode === 'balance') return []
  const labels: Record<string, { label: string; color: UpstreamBarColor }> = {
    '5h': { label: '5h', color: 'indigo' },
    '1d': { label: '1d', color: 'purple' },
    '7d': { label: '7d', color: 'emerald' }
  }
  const bars = (quota.windows ?? []).flatMap(window => {
    const display = labels[window.name]
    if (!display) return []
    const bar = makeUpstreamUsageBar(window, display.label, display.color, 'key')
    return bar ? [bar] : []
  })
  if (typeof quota.used === 'number' && typeof quota.total === 'number') {
    const totalBar = makeUpstreamUsageBar(
      { name: 'total', used: quota.used, limit: quota.total },
      t('admin.accounts.usageWindow.totalLimit'),
      'purple',
      'key'
    )
    if (totalBar) bars.push(totalBar)
  }
  return bars
})

const upstreamSubscriptionBars = computed<UpstreamUsageBar[]>(() => {
  const subscription = upstreamSubscription.value
  if (!subscription || subscription.unlimited || upstreamSubscriptionExpired.value) return []
  const labels: Record<string, { label: string; color: UpstreamBarColor }> = {
    daily: { label: '1d', color: 'indigo' },
    weekly: { label: '7d', color: 'emerald' },
    monthly: { label: '30d', color: 'purple' }
  }
  return (subscription.windows ?? []).flatMap(window => {
    const display = labels[window.name]
    if (!display) return []
    const bar = makeUpstreamUsageBar(window, display.label, display.color, 'subscription')
    return bar ? [bar] : []
  })
})

const hasUpstreamConstraints = computed(
  () => upstreamKeyBars.value.length > 0 || upstreamSubscription.value != null
)
const constraintSourceCount = computed(() =>
  Number(hasApiKeyQuota.value) +
  Number(upstreamKeyBars.value.length > 0) +
  Number(upstreamSubscription.value != null)
)
const showConstraintSourceLabels = computed(() => constraintSourceCount.value > 1)
</script>
