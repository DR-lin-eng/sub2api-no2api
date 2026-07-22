<template>
  <div v-if="eligible" class="inline-flex w-fit min-w-[8rem] flex-col items-start gap-1" data-testid="upstream-information">
    <div class="flex h-6 min-w-[7rem] items-center gap-1" data-testid="upstream-rate-row">
      <HelpTooltip class="-ml-1" width-class="w-max max-w-[calc(100vw-2rem)]" data-testid="upstream-billing-details">
        <template #trigger>
        <span
          class="cursor-help border-b border-dotted border-gray-300 text-sm font-medium dark:border-dark-600"
          :class="hasEffectiveRate ? 'font-mono text-gray-800 dark:text-gray-200' : statusClass || 'text-gray-400 dark:text-gray-500'"
          data-testid="upstream-billing-rate"
        >
          {{ primaryValue }}
        </span>
        </template>
        <div class="space-y-1">
        <template v-if="hasEffectiveRate && data">
          <p>{{ t('admin.accounts.upstreamBilling.groupRate', { value: data.group_rate_multiplier }) }}</p>
          <p v-if="data.user_rate_multiplier != null">
            {{ t('admin.accounts.upstreamBilling.userRate', { value: data.user_rate_multiplier }) }}
          </p>
          <p>
            {{
              data.peak_rate_enabled
                ? t('admin.accounts.upstreamBilling.peakRate', {
                    start: data.peak_start,
                    end: data.peak_end,
                    value: data.peak_rate_multiplier,
                    timezone: data.timezone
                  })
                : t('admin.accounts.upstreamBilling.noPeakRate')
            }}
          </p>
          <p>{{ t('admin.accounts.upstreamBilling.effectiveRate', { value: currentEffectiveRate ?? '-' }) }}</p>
          <p>{{ t('admin.accounts.upstreamBilling.updatedAt', { value: formatDate(snapshot?.received_at) }) }}</p>
        </template>
        <template v-else-if="stale && lastDetectedRate != null">
          <p data-testid="upstream-billing-last-rate">
            {{ t('admin.accounts.upstreamBilling.lastDetectedRate', { value: lastDetectedRate }) }}
          </p>
          <p data-testid="upstream-billing-last-time">
            {{ t('admin.accounts.upstreamBilling.lastDetectedAt', { value: formatDate(snapshot?.received_at) }) }}
          </p>
          <p data-testid="upstream-billing-elapsed">
            {{ t('admin.accounts.upstreamBilling.elapsedSince', { value: elapsedSinceLastSuccess }) }}
          </p>
        </template>
        <p v-else>{{ statusLabel || '-' }}</p>
        <p
          v-if="probeEnabled && globalProbeEnabled !== false && nextProbeAt"
          data-testid="upstream-billing-next-probe"
        >
          {{ t('admin.accounts.upstreamBilling.nextProbeAt', { value: formatDate(nextProbeAt) }) }}
        </p>
        <p class="mt-2 border-t border-white/15 pt-2" data-testid="upstream-billing-probe-state">
          {{ t('admin.accounts.upstreamBilling.accountProbeState') }}
          <span :class="probeEnabled ? 'text-emerald-400' : 'text-red-400'">
            {{ probeEnabled ? t('admin.accounts.upstreamBilling.enabled') : t('admin.accounts.upstreamBilling.disabled') }}
          </span>
        </p>
        <p
          v-if="globalProbeEnabled === false"
          class="mt-1"
          data-testid="upstream-billing-global-probe-state"
        >
          {{ t('admin.accounts.upstreamBilling.globalProbeState') }}
          <span class="text-red-400">{{ t('admin.accounts.upstreamBilling.disabled') }}</span>
        </p>
        </div>
      </HelpTooltip>
      <span v-if="hasEffectiveRate && statusLabel" :class="statusClass" class="whitespace-nowrap text-[10px] font-medium">
        {{ statusLabel }}
      </span>
      <button
        type="button"
        class="inline-flex h-6 w-6 flex-shrink-0 items-center justify-center rounded transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-blue-900/30"
        :class="rateActionClass"
        :disabled="probing"
        :aria-label="t('admin.accounts.upstreamBilling.manualProbe')"
        :title="t('admin.accounts.upstreamBilling.manualProbe')"
        data-testid="upstream-billing-probe"
        @click="$emit('probe')"
      >
        <Icon :name="rateActionIcon" size="xs" :class="{ 'animate-spin': probing }" />
      </button>
    </div>
    <div class="flex h-6 min-w-[7rem] items-center gap-1" data-testid="upstream-quota-row" :aria-busy="quotaLoading">
      <HelpTooltip
        v-if="quota || quotaError"
        class="-ml-1"
        width-class="w-max max-w-[calc(100vw-2rem)]"
        data-testid="upstream-quota-details"
      >
        <template #trigger>
          <span
            class="max-w-[6rem] cursor-help truncate border-b border-dotted border-gray-300 text-sm font-medium tabular-nums dark:border-dark-600"
            :class="quotaValueClass"
            data-testid="upstream-quota-value"
          >
            {{ quotaPrimaryValue }}
          </span>
        </template>
        <div class="space-y-1">
          <p v-if="quotaError" class="text-red-300">
            {{ t('admin.accounts.upstreamBilling.quotaError', { message: quotaError }) }}
          </p>
          <template v-else-if="quota">
            <p>{{ t('admin.accounts.upstreamBilling.quotaProvider', { value: quota.provider }) }}</p>
            <p>{{ t('admin.accounts.upstreamBilling.quotaMode', { value: quotaModeLabel }) }}</p>
            <p v-if="quota.subscription?.plan_name">
              {{ t('admin.accounts.upstreamBilling.quotaPlan', { value: quota.subscription.plan_name }) }}
            </p>
            <p v-if="quota.subscription?.unlimited">{{ t('admin.accounts.upstreamBilling.unlimited') }}</p>
            <p v-if="quotaRemaining != null">
              {{ t('admin.accounts.upstreamBilling.quotaRemaining', { value: formatQuotaAmount(quotaRemaining) }) }}
            </p>
            <p v-if="quota.used != null">
              {{ t('admin.accounts.upstreamBilling.quotaUsed', { value: formatQuotaAmount(quota.used) }) }}
            </p>
            <p v-if="quota.total != null">
              {{ t('admin.accounts.upstreamBilling.quotaTotal', { value: formatQuotaAmount(quota.total) }) }}
            </p>
            <p v-for="window in quotaWindows" :key="window.name">
              {{ t('admin.accounts.upstreamBilling.quotaWindow', {
                name: window.name,
                used: formatQuotaAmount(window.used),
                limit: formatQuotaAmount(window.limit),
                remaining: formatQuotaAmount(window.remaining),
                reset: formatDate(window.reset_at ?? undefined)
              }) }}
            </p>
            <p v-if="quotaExpiresAt">
              {{ t('admin.accounts.upstreamBilling.quotaExpiresAt', { value: formatDate(quotaExpiresAt) }) }}
            </p>
            <p>{{ t('admin.accounts.upstreamBilling.quotaObservedAt', { value: formatDate(quotaResult?.observed_at) }) }}</p>
          </template>
          <p v-else>{{ t('admin.accounts.upstreamBilling.notQueried') }}</p>
        </div>
      </HelpTooltip>
      <span
        v-else
        class="max-w-[6rem] truncate text-sm font-medium text-gray-400 dark:text-gray-500"
        data-testid="upstream-quota-value"
      >
        {{ quotaPrimaryValue }}
      </span>
      <button
        type="button"
        class="inline-flex h-6 w-6 flex-shrink-0 items-center justify-center rounded transition-colors hover:bg-emerald-50 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-emerald-900/30"
        :class="quotaActionClass"
        :disabled="quotaLoading"
        :aria-label="quotaActionLabel"
        :title="quotaActionLabel"
        data-testid="upstream-quota-query"
        @click="$emit('query-quota')"
      >
        <Icon :name="quotaActionIcon" size="xs" :class="{ 'animate-spin': quotaLoading }" />
      </button>
    </div>
  </div>
  <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Account, UpstreamBillingProbeSnapshot, UpstreamQuotaQueryResult, UpstreamQuotaWindow } from '@/types'

type ActionFeedback = 'success' | 'error'

const props = withDefaults(defineProps<{
  account: Account
  now: number
  probing?: boolean
  globalProbeEnabled?: boolean
  quotaResult?: UpstreamQuotaQueryResult | null
  quotaError?: string | null
  quotaLoading?: boolean
  rateFeedback?: ActionFeedback | null
  quotaFeedback?: ActionFeedback | null
}>(), {
  globalProbeEnabled: true,
  quotaResult: null,
  quotaError: null,
  quotaLoading: false,
  rateFeedback: null,
  quotaFeedback: null
})

defineEmits<{
  (event: 'probe'): void
  (event: 'query-quota'): void
}>()

const { t } = useI18n()
const CLOCK_SKEW_TOLERANCE_MS = 5 * 60 * 1000
const eligible = computed(() => props.account.platform === 'openai' && props.account.type === 'apikey')
const snapshot = computed<UpstreamBillingProbeSnapshot | undefined>(() => props.account.extra?.upstream_billing_probe)
const data = computed(() => snapshot.value?.data)
const quota = computed(() => props.quotaResult?.quota ?? null)
const probeEnabled = computed(() => props.account.extra?.upstream_billing_probe_enabled === true)
const nextProbeAt = computed(() => {
  const value = snapshot.value?.next_probe_at
  return typeof value === 'string' && Number.isFinite(Date.parse(value)) ? value : ''
})
const receivedAt = computed(() => typeof snapshot.value?.received_at === 'string' ? Date.parse(snapshot.value.received_at) : Number.NaN)
const freshUntil = computed(() => {
  if (typeof snapshot.value?.fresh_until === 'string') return Date.parse(snapshot.value.fresh_until)
  if (snapshot.value?.status !== 'ok' || typeof snapshot.value.next_probe_at !== 'string') return Number.NaN
  const nextProbeAt = Date.parse(snapshot.value.next_probe_at)
  return Number.isFinite(nextProbeAt) && nextProbeAt > receivedAt.value
    ? receivedAt.value + 2 * (nextProbeAt - receivedAt.value)
    : Number.NaN
})
const validTimestamps = computed(() => {
  if (!Number.isFinite(receivedAt.value) || receivedAt.value > props.now + CLOCK_SKEW_TOLERANCE_MS) return false
  return Number.isFinite(freshUntil.value) && freshUntil.value > receivedAt.value
})
const stale = computed(() => {
  if (!snapshot.value) return false
  if (!Number.isFinite(receivedAt.value)) return snapshot.value.status === 'ok'
  if (!validTimestamps.value) return true
  return props.now > freshUntil.value
})
const parseMinute = (value?: string) => {
  if (typeof value !== 'string') return null
  const match = /^(\d{2}):(\d{2})$/.exec(value)
  if (!match) return null
  const hour = Number(match[1])
  const minute = Number(match[2])
  return hour < 24 && minute < 60 ? hour * 60 + minute : null
}
const minuteInTimeZone = (timestamp: number, timeZone?: string) => {
  if (!timeZone) return null
  try {
    const parts = new Intl.DateTimeFormat('en-GB', {
      timeZone,
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23'
    }).formatToParts(new Date(timestamp))
    const hour = Number(parts.find(part => part.type === 'hour')?.value)
    const minute = Number(parts.find(part => part.type === 'minute')?.value)
    return Number.isInteger(hour) && Number.isInteger(minute) ? hour * 60 + minute : null
  } catch {
    return null
  }
}
const currentEffectiveRate = computed(() => {
  const billing = data.value
  if (!billing) return null
  if (billing.billing_scope !== 'token') return null
  const base = billing.resolved_rate_multiplier
  if (typeof base !== 'number' || !Number.isFinite(base) || base < 0) return null
  if (typeof billing.peak_rate_enabled !== 'boolean') return null
  if (!billing.peak_rate_enabled) return base
  const start = parseMinute(billing.peak_start)
  const end = parseMinute(billing.peak_end)
  const minute = minuteInTimeZone(props.now, billing.timezone)
  const peak = billing.peak_rate_multiplier
  if (start == null || end == null || minute == null || start >= end || typeof peak !== 'number' || !Number.isFinite(peak) || peak < 0) return null
  const value = minute >= start && minute < end ? base * peak : base
  return Number.isFinite(value) ? value : null
})
const lastDetectedRate = computed(() => {
  const value = data.value?.effective_rate_multiplier
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? Number(value.toPrecision(12))
    : null
})
const elapsedSinceLastSuccess = computed(() => {
  if (!Number.isFinite(receivedAt.value)) return '-'
  const elapsedMinutes = Math.max(0, Math.floor((props.now - receivedAt.value) / 60_000))
  if (elapsedMinutes < 1) return t('admin.accounts.upstreamBilling.justNow')
  if (elapsedMinutes < 60) return t('admin.accounts.upstreamBilling.minutesAgo', { count: elapsedMinutes })
  const elapsedHours = Math.floor(elapsedMinutes / 60)
  if (elapsedHours < 24) return t('admin.accounts.upstreamBilling.hoursAgo', { count: elapsedHours })
  return t('admin.accounts.upstreamBilling.daysAgo', { count: Math.floor(elapsedHours / 24) })
})
const effectiveRate = computed(() => {
  if (!validTimestamps.value || stale.value || !['ok', 'failed'].includes(snapshot.value?.status ?? '')) return '-'
  const value = currentEffectiveRate.value
  return value == null ? '-' : `${Number(value.toPrecision(12))}x`
})
const statusLabel = computed(() => {
  if (!snapshot.value) return t('admin.accounts.upstreamBilling.notProbed')
  if (snapshot.value.status === 'unsupported') return t('admin.accounts.upstreamBilling.unsupported')
  if (stale.value) return t('admin.accounts.upstreamBilling.stale')
  if (snapshot.value.status === 'failed') return t('admin.accounts.upstreamBilling.failed')
  return ''
})
const statusClass = computed(() => {
  if (!snapshot.value) return 'text-gray-400 dark:text-gray-500'
  if (snapshot.value.status === 'unsupported') return 'text-gray-500 dark:text-gray-400'
  if (stale.value) return 'text-amber-600 dark:text-amber-400'
  if (snapshot.value.status === 'failed') return 'text-red-600 dark:text-red-400'
  return ''
})
const hasEffectiveRate = computed(() => effectiveRate.value !== '-')
const primaryValue = computed(() => hasEffectiveRate.value ? effectiveRate.value : statusLabel.value || '-')
const isFiniteNumber = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value)
const quotaWindows = computed<UpstreamQuotaWindow[]>(() => [
  ...(quota.value?.windows ?? []),
  ...(quota.value?.subscription?.windows ?? [])
])
const quotaRemaining = computed(() => {
  if (quota.value?.subscription?.unlimited) return null
  if (isFiniteNumber(quota.value?.subscription?.remaining)) return quota.value.subscription.remaining
  if (isFiniteNumber(quota.value?.remaining)) return quota.value.remaining
  const remaining = quotaWindows.value
    .map(window => window.remaining)
    .filter(isFiniteNumber)
  return remaining.length > 0 ? Math.min(...remaining) : null
})
const quotaExpiresAt = computed(() => quota.value?.subscription?.expires_at ?? quota.value?.expires_at ?? '')
const quotaModeLabel = computed(() => {
  switch (quota.value?.mode) {
    case 'balance': return t('admin.accounts.upstreamBilling.quotaModeBalance')
    case 'quota': return t('admin.accounts.upstreamBilling.quotaModeQuota')
    case 'subscription': return t('admin.accounts.upstreamBilling.quotaModeSubscription')
    case 'rate_limits': return t('admin.accounts.upstreamBilling.quotaModeRateLimits')
    default: return '-'
  }
})
const quotaNumberFormatter = new Intl.NumberFormat(undefined, { maximumFractionDigits: 8 })
const formatQuotaAmount = (value: unknown) => {
  if (!isFiniteNumber(value)) return '-'
  const formatted = quotaNumberFormatter.format(Math.abs(value))
  const sign = value < 0 ? '-' : ''
  if (quota.value?.unit === 'USD') return `${sign}$${formatted}`
  if (quota.value?.unit === 'CNY') return `${sign}\u00A5${formatted}`
  return quota.value?.unit === 'TOKENS' ? `${sign}${formatted} TOKENS` : `${sign}${formatted}`
}
const quotaPrimaryValue = computed(() => {
  if (props.quotaLoading) return t('admin.accounts.upstreamBilling.queryingQuota')
  if (props.quotaError && !quota.value) return t('admin.accounts.upstreamBilling.quotaFailed')
  if (quota.value?.subscription?.unlimited) return t('admin.accounts.upstreamBilling.unlimited')
  if (quotaRemaining.value != null) return formatQuotaAmount(quotaRemaining.value)
  if (quota.value) return quotaModeLabel.value
  return t('admin.accounts.upstreamBilling.notQueried')
})
const quotaValueClass = computed(() => {
  if (props.quotaError) return 'text-red-600 dark:text-red-400'
  if (props.quotaLoading) return 'text-gray-500 dark:text-gray-400'
  if (quotaRemaining.value != null && quotaRemaining.value <= 0) return 'font-mono text-red-600 dark:text-red-400'
  if (quota.value) return 'font-mono text-emerald-600 dark:text-emerald-400'
  return 'text-gray-400 dark:text-gray-500'
})
const actionClass = (loading: boolean, feedback: ActionFeedback | null, idleClass: string) => {
  if (loading) return 'text-blue-600 dark:text-blue-400'
  if (feedback === 'success') return 'text-emerald-600 dark:text-emerald-400'
  if (feedback === 'error') return 'text-red-600 dark:text-red-400'
  return idleClass
}
const rateActionClass = computed(() => actionClass(
  Boolean(props.probing),
  props.rateFeedback,
  'text-blue-600 dark:text-blue-400'
))
const quotaActionClass = computed(() => actionClass(
  props.quotaLoading,
  props.quotaFeedback,
  'text-emerald-600 dark:text-emerald-400'
))
const rateActionIcon = computed(() => {
  if (props.probing) return 'refresh'
  if (props.rateFeedback === 'success') return 'check'
  if (props.rateFeedback === 'error') return 'x'
  return 'refresh'
})
const quotaActionIcon = computed(() => {
  if (props.quotaLoading) return 'refresh'
  if (props.quotaFeedback === 'success') return 'check'
  if (props.quotaFeedback === 'error') return 'x'
  return props.quotaResult ? 'refresh' : 'search'
})
const quotaActionLabel = computed(() => t(
  props.quotaResult
    ? 'admin.accounts.upstreamBilling.refreshQuota'
    : 'admin.accounts.upstreamBilling.queryQuota'
))
const formatDate = (value?: string) => value
  ? new Date(value).toLocaleString(undefined, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    })
  : '-'
</script>
