<template>
  <div v-if="visible" class="space-y-1">
    <!--
      Unified action row. The parent supplies the primary 查询 action through
      #pre-actions; standalone uses render the same action here.

      Local request/token/cost counters remain separate from rate-limit bars.
      Official App Server buckets are rendered below only after the explicit
      quota query returns them, alongside the reset-credit controls.
    -->
    <div class="flex flex-wrap items-center gap-1.5">
      <slot name="pre-actions" />

      <button
        v-if="!hasPreActions"
        type="button"
        data-testid="openai-primary-query"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :disabled="loading || resetting"
        :title="countButtonTitle"
        @click="handleQuery"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': loading }"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
          />
        </svg>
        {{ t('admin.accounts.usageWindow.activeQuery') }}
      </button>

      <span
        data-testid="openai-reset-credit-count"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 dark:text-blue-400"
        :title="countButtonTitle"
        aria-live="polite"
      >
        {{ t('admin.accounts.openaiQuotaReset.count') }}<span v-if="data"> {{ availableResetCount }}</span>
      </span>

      <button
        type="button"
        data-testid="openai-reset-button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-orange-600 transition-colors hover:bg-orange-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-orange-400 dark:hover:bg-orange-900/30"
        :disabled="resetting || loading || !canReset"
        :title="resetButtonTitle"
        @click="openResetConfirm"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': resetting }"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M20 12a8 8 0 11-2.343-5.657L20 8m0 0V4m0 4h-4"
          />
        </svg>
        {{ t('admin.accounts.openaiQuotaReset.reset') }}
      </button>
    </div>

    <!--
      ChatGPT App Server can return more than one metered bucket (for example
      codex and codex_other).  Keep these official windows next to the query
      controls so the values are visible even when local response-header
      sampling has not observed a window yet.
    -->
    <div
      v-if="rateLimitWindows.length > 0"
      data-testid="openai-rate-limit-buckets"
      class="space-y-1"
    >
      <div class="flex items-center gap-1.5 text-[9px] font-medium text-gray-500 dark:text-gray-400">
        <span>{{ t('admin.accounts.upstreamBilling.quotaModeRateLimits') }}</span>
        <span v-if="isPassiveQuotaData" class="italic text-gray-400 dark:text-gray-500">
          {{ t('admin.accounts.usageWindow.passiveSampled') }}
        </span>
      </div>
      <div
        v-for="window in rateLimitWindows"
        :key="window.key"
        class="flex items-center gap-1"
        :title="`${window.label} · ${formatRateLimitResetTitle(window.resetsAt)}`"
        :data-testid="`openai-rate-limit-${window.key}`"
      >
        <div v-if="window.rawDetails" class="min-w-0 flex-1 whitespace-normal rounded bg-gray-50 px-1.5 py-0.5 text-[10px] text-gray-600 dark:bg-dark-800 dark:text-gray-300">
          <span class="font-medium">{{ window.label }}:</span>
          <span class="ml-1 break-words font-mono">{{ window.rawDetails }}</span>
        </div>
        <template v-else>
          <UsageProgressBar
            :label="window.label"
            :utilization="window.usedPercent"
            :resets-at="window.resetsAt"
            :window-stats="window.windowStats"
            :show-now-when-idle="true"
            :color="window.color"
          />
          <span v-if="window.showDurationDetails !== false" class="shrink-0 text-[9px] tabular-nums text-gray-400 dark:text-gray-500">
            {{ formatWindowDurationDetails(window.windowDurationMins) }}
          </span>
        </template>
      </div>
    </div>

    <div
      v-if="localStatsRows.length > 0"
      data-testid="openai-local-window-stats"
      class="space-y-0.5 whitespace-normal text-[9px] text-gray-500 dark:text-gray-400"
    >
      <div class="font-medium">{{ t('admin.accounts.openaiQuotaReset.localStats') }}</div>
      <div v-for="row in localStatsRows" :key="row.label" class="flex flex-wrap items-center gap-1.5 tabular-nums">
        <span class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-gray-800">{{ row.label }}</span>
        <span>{{ formatLocalRequests(row.stats) }} req</span>
        <span>{{ formatLocalTokens(row.stats) }}</span>
        <span :title="t('usage.accountBilled')">A ${{ formatLocalCost(row.stats.cost) }}</span>
        <span v-if="row.stats.user_cost != null" :title="t('usage.userBilled')">U ${{ formatLocalCost(row.stats.user_cost) }}</span>
      </div>
    </div>

    <div
      v-if="serverTokenUsage"
      data-testid="openai-server-token-usage"
      class="w-full max-w-[390px] space-y-1 whitespace-normal rounded border border-blue-100 bg-blue-50/60 px-1.5 py-1 text-[10px] text-gray-600 dark:border-blue-900/50 dark:bg-blue-950/20 dark:text-gray-300"
    >
      <div class="font-medium text-blue-700 dark:text-blue-300">
        {{ t('admin.accounts.openaiQuotaReset.serverUsage') }}
      </div>
      <div class="flex max-w-full flex-col gap-y-0.5 break-all tabular-nums">
        <span v-if="serverTokenUsage.current_reset_cycle_tokens != null">
          {{ t('admin.accounts.openaiQuotaReset.serverUsageFields.currentResetCycleTokens') }}<template v-if="serverTokenUsage.current_reset_cycle_window_minutes"> ({{ formatWindowDuration(serverTokenUsage.current_reset_cycle_window_minutes) }})</template>: {{ formatServerCount(serverTokenUsage.current_reset_cycle_tokens) }}<template v-if="serverTokenUsage.current_reset_cycle_approximate"> ({{ t('admin.accounts.openaiQuotaReset.serverUsageFields.approximate') }})</template>
        </span>
        <span v-if="serverTokenUsage.summary.lifetime_tokens != null">
          {{ t('admin.accounts.openaiQuotaReset.serverUsageFields.lifetimeTokens') }}: {{ formatServerCount(serverTokenUsage.summary.lifetime_tokens) }}
        </span>
        <span v-if="serverTokenUsage.summary.peak_daily_tokens != null">
          {{ t('admin.accounts.openaiQuotaReset.serverUsageFields.peakDailyTokens') }}: {{ formatServerCount(serverTokenUsage.summary.peak_daily_tokens) }}
        </span>
        <span v-if="serverTokenUsage.summary.longest_running_turn_seconds != null">
          {{ t('admin.accounts.openaiQuotaReset.serverUsageFields.longestRunningTurnSec') }}: {{ serverTokenUsage.summary.longest_running_turn_seconds }}s
        </span>
        <span v-if="serverTokenUsage.summary.current_streak_days != null">
          {{ t('admin.accounts.openaiQuotaReset.serverUsageFields.currentStreakDays') }}: {{ serverTokenUsage.summary.current_streak_days }}d
        </span>
        <span v-if="serverTokenUsage.summary.longest_streak_days != null">
          {{ t('admin.accounts.openaiQuotaReset.serverUsageFields.longestStreakDays') }}: {{ serverTokenUsage.summary.longest_streak_days }}d
        </span>
      </div>
      <div v-if="serverTokenUsage.daily_usage_buckets?.length" class="flex max-w-full flex-col gap-y-0.5 break-all text-gray-500 dark:text-gray-400">
        <span v-for="bucket in serverTokenUsage.daily_usage_buckets" :key="bucket.start_date">
          {{ t('admin.accounts.openaiQuotaReset.serverUsageFields.dailyBucket') }} {{ bucket.start_date }}: {{ formatServerCount(bucket.tokens) }}
        </span>
      </div>
    </div>

    <div v-if="primaryResetCreditExpiry" class="space-y-1">
      <div class="flex flex-wrap items-center gap-1">
        <span
          class="inline-flex max-w-full items-center rounded bg-gray-100 px-1.5 py-0.5 text-[10px] leading-4 text-gray-600 tabular-nums dark:bg-dark-800 dark:text-gray-300"
          :title="t('admin.accounts.openaiQuotaReset.expiresAtFull', { time: formatResetCreditExpiry(primaryResetCreditExpiry, 'full') })"
        >
          {{ t('admin.accounts.openaiQuotaReset.expiresAt', { time: formatResetCreditExpiry(primaryResetCreditExpiry, 'short') }) }}
        </span>
        <button
          v-if="hiddenResetCreditCount > 0"
          type="button"
          data-testid="reset-credit-expiry-toggle"
          class="inline-flex items-center rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium leading-4 text-gray-600 transition-colors hover:bg-gray-200 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700"
          :aria-expanded="showResetCreditDetails"
          :aria-label="resetCreditDetailsToggleLabel"
          :title="resetCreditDetailsTitle"
          @click="toggleResetCreditDetails"
        >
          +{{ hiddenResetCreditCount }}
        </button>
      </div>

      <div
        v-if="showResetCreditDetails && resetCreditExpirations.length > 1"
        data-testid="reset-credit-expiry-details"
        class="inline-grid max-w-full gap-0.5 rounded border border-gray-200 bg-white px-1.5 py-1 text-[10px] leading-4 text-gray-600 shadow-sm dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300"
      >
        <span class="sr-only">{{ t('admin.accounts.openaiQuotaReset.expirationDetails') }}</span>
        <span
          v-for="(expiresAt, index) in resetCreditExpirations"
          :key="`${expiresAt}-${index}`"
          class="flex min-w-0 items-center gap-1 tabular-nums"
          :title="t('admin.accounts.openaiQuotaReset.expiresAtFull', { time: formatResetCreditExpiry(expiresAt, 'full') })"
        >
          <span class="h-1 w-1 shrink-0 rounded-full bg-gray-400 dark:bg-dark-500" />
          <span class="truncate">{{ formatResetCreditExpiry(expiresAt, 'short') }}</span>
        </span>
      </div>
    </div>

    <!-- Error / success feedback -->
    <div
      v-if="error"
      class="text-[10px] text-red-600 dark:text-red-400"
      :title="error"
    >
      {{ truncatedError }}
    </div>
    <div
      v-else-if="resetWarning"
      class="text-[10px] text-amber-600 dark:text-amber-400"
    >
      {{ resetWarning }}
    </div>
    <div
      v-else-if="resetMessage"
      class="text-[10px] text-emerald-600 dark:text-emerald-400"
    >
      {{ resetMessage }}
    </div>

    <ConfirmDialog
      :show="showResetConfirm"
      :title="t('admin.accounts.openaiQuotaReset.confirmTitle')"
      :message="t('admin.accounts.openaiQuotaReset.confirmMessage', { count: availableResetCount })"
      :confirm-text="t('admin.accounts.openaiQuotaReset.reset')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmReset"
      @cancel="showResetConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, useSlots } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account, WindowStats } from '@/types'
import {
  refreshOpenAIQuota,
  resetOpenAIQuota
} from '@/features/admin-accounts/data/datasources/adminAccountsDatasource'
import type {
  OpenAIAppServerRateLimitBucket,
  OpenAIQuotaRefreshResult,
  OpenAIQuotaUsage,
  OpenAIQuotaResetResult
} from '@/features/admin-accounts/data/dtos/openAIQuotaDtos'
import ConfirmDialog from '@/common/widgets/feedback/ConfirmDialog.vue'
import { formatCompactNumber } from '@/core/utils/format'
import UsageProgressBar from './UsageProgressBar.vue'

// Account rows are replaced by the admin auto-refresh list. Keep the last
// explicit App Server result outside the row component so a reactive row
// replacement does not erase the queried percentage within a few seconds.
const openAIQuotaSnapshotCache = new Map<number, OpenAIQuotaUsage>()

const props = defineProps<{
  account: Account
  localWindowStats?: {
    five_hour?: WindowStats | null
    seven_day?: WindowStats | null
  } | null
  /** Refresh local request/token/cost counters with the upstream snapshot. */
  queryLocalUsage?: () => Promise<void>
  externalQuotaResult?: OpenAIQuotaRefreshResult | null
}>()

const emit = defineEmits<{
  'account-updated': [account: Account]
  'quota-updated': [usage: OpenAIQuotaUsage]
}>()

const { t } = useI18n()
const slots = useSlots()
const hasPreActions = computed(() => Boolean(slots['pre-actions']))

// Visible only for OpenAI OAuth accounts.
const visible = computed(() => props.account.platform === 'openai' && props.account.type === 'oauth')

const loading = ref(false)
const resetting = ref(false)
const error = ref<string | null>(null)
const data = ref<OpenAIQuotaUsage | null>(null)
const cachedData = ref<OpenAIQuotaUsage | null>(null)
const resetMessage = ref<string | null>(null)
const resetWarning = ref<string | null>(null)
const showResetConfirm = ref(false)
const showResetCreditDetails = ref(false)

const quotaNumber = (value: unknown): number | null => {
  if (value == null || (typeof value === 'string' && value.trim() === '')) return null
  const parsed = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

const quotaResetUnixSeconds = (value: unknown): number | undefined => {
  if (typeof value !== 'string' || value.trim() === '') return undefined
  const milliseconds = Date.parse(value)
  return Number.isNaN(milliseconds) ? undefined : Math.floor(milliseconds / 1000)
}

const readCachedResetCredits = (account: Account): OpenAIQuotaUsage | null => {
  const cached = account.extra?.codex_reset_credit_snapshot
  if (!cached || typeof cached !== 'object' || Array.isArray(cached)) return null

  const { available_count: count, credits: rawCredits } = cached as {
    available_count?: unknown
    credits?: unknown
  }
  if (typeof count !== 'number' || !Number.isFinite(count)) return null

  const now = Date.now()
  const credits: { expires_at?: string }[] = []
  if (Array.isArray(rawCredits)) {
    for (const credit of rawCredits) {
      if (!credit || typeof credit !== 'object') continue
      const expiresAt = (credit as { expires_at?: unknown }).expires_at
      if (typeof expiresAt !== 'string' || expiresAt.trim() === '') continue
      const expiryTime = new Date(expiresAt).getTime()
      if (!Number.isNaN(expiryTime) && expiryTime <= now) continue
      credits.push({ expires_at: expiresAt })
    }
  }

  const availableCount = Math.min(Math.max(Math.trunc(count), 0), credits.length)
  if (count > 0 && availableCount <= 0) return null
  return {
    fetched_at: 0,
    rate_limit_reset_credits: {
      available_count: availableCount,
      credits: credits.slice(0, availableCount)
    }
  }
}

const cachedQuotaSnapshot = (account: Account): OpenAIQuotaUsage | null =>
  openAIQuotaSnapshotCache.get(account.id) ?? null

const readPersistedQuotaSnapshot = (account: Account): OpenAIQuotaUsage | null => {
  const raw = account.extra?.codex_rate_limit_snapshot
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null

  const snapshot = raw as Record<string, unknown>
  const rateLimits = snapshot.rate_limits_by_limit_id ?? snapshot.rateLimitsByLimitId
  const rateLimit = snapshot.rate_limit ?? snapshot.rateLimit
  const hasBuckets = rateLimits && typeof rateLimits === 'object' && !Array.isArray(rateLimits) && Object.keys(rateLimits).length > 0
  const hasLegacyBucket = rateLimit && typeof rateLimit === 'object' && !Array.isArray(rateLimit)
  if (!hasBuckets && !hasLegacyBucket) return null

  const fetchedAt = Number(snapshot.fetched_at ?? snapshot.fetchedAt ?? 0)
  return {
    source: 'active',
    fetched_at: Number.isFinite(fetchedAt) ? fetchedAt : 0,
    rate_limits_by_limit_id: hasBuckets
      ? rateLimits as OpenAIQuotaUsage['rate_limits_by_limit_id']
      : undefined,
    rate_limit: hasLegacyBucket ? rateLimit as OpenAIQuotaUsage['rate_limit'] : undefined
  }
}

const readPassiveQuotaSnapshot = (account: Account): OpenAIQuotaUsage | null => {
  const extra = account.extra
  if (!extra) return null

  const fiveHourUsed = quotaNumber(extra.codex_5h_used_percent)
  const sevenDayUsed = quotaNumber(extra.codex_7d_used_percent)
  if (fiveHourUsed == null && sevenDayUsed == null) return null

  const fiveHourMinutes = quotaNumber(extra.codex_5h_window_minutes) ?? 300
  const sevenDayMinutes = quotaNumber(extra.codex_7d_window_minutes) ?? 10080
  const fiveHourReset = quotaResetUnixSeconds(extra.codex_5h_reset_at)
  const sevenDayReset = quotaResetUnixSeconds(extra.codex_7d_reset_at)
  const nowSeconds = Math.floor(Date.now() / 1000)
  const primary = sevenDayUsed == null ? undefined : {
    used_percent: sevenDayReset != null && sevenDayReset <= nowSeconds ? 0 : sevenDayUsed,
    window_duration_mins: sevenDayMinutes,
    resets_at: sevenDayReset
  }
  const secondary = fiveHourUsed == null ? undefined : {
    used_percent: fiveHourReset != null && fiveHourReset <= nowSeconds ? 0 : fiveHourUsed,
    window_duration_mins: fiveHourMinutes,
    resets_at: fiveHourReset
  }
  return {
    source: 'passive',
    fetched_at: typeof extra.codex_usage_updated_at === 'string'
      ? quotaResetUnixSeconds(extra.codex_usage_updated_at) ?? 0
      : 0,
    rate_limits_by_limit_id: {
      codex: {
        limit_id: 'codex',
        limit_name: 'codex',
        primary,
        secondary
      }
    }
  }
}

const quotaDataForAccount = (account: Account): OpenAIQuotaUsage | null => {
  const snapshot = cachedQuotaSnapshot(account) ?? readPersistedQuotaSnapshot(account) ?? readPassiveQuotaSnapshot(account)
  const credits = readCachedResetCredits(account)
  if (!snapshot) return credits
  return {
    ...snapshot,
    rate_limit_reset_credits: credits?.rate_limit_reset_credits
  }
}

const initialQuotaData = quotaDataForAccount(props.account)
cachedData.value = initialQuotaData
data.value = initialQuotaData

// 影子账号的额度查询会 resolve 到母账号,但影子本身不支持重置(后端返回 409);
// 重置必须在母账号上进行。前端据此禁用影子的重置入口(外审 F6)。
const isShadow = computed(() => props.account.parent_account_id != null)

const availableResetCount = computed(() => data.value?.rate_limit_reset_credits?.available_count ?? 0)
const isPassiveQuotaData = computed(() => data.value?.source === 'passive')
const resetCreditExpirations = computed(() =>
  ((data.value ?? cachedData.value)?.rate_limit_reset_credits?.credits ?? [])
    .map((credit) => credit.expires_at?.trim() ?? '')
    .filter((expiresAt) => expiresAt.length > 0)
    .sort(compareResetCreditExpiry)
)
const primaryResetCreditExpiry = computed(() => resetCreditExpirations.value[0] ?? '')
const hiddenResetCreditCount = computed(() => Math.max(resetCreditExpirations.value.length - 1, 0))
const canReset = computed(() => availableResetCount.value > 0 && !isShadow.value)

type RateLimitColor = 'indigo' | 'emerald' | 'purple' | 'amber'

interface RateLimitDisplayWindow {
  key: string
  label: string
  usedPercent: number
  windowDurationMins: number
  resetsAt: string | null
  color: RateLimitColor
  windowStats?: WindowStats | null
  showDurationDetails?: boolean
  rawDetails?: string
}

const finiteNumber = (value: unknown): number | null => {
  const number = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(number) ? number : null
}

const unixSecondsToISO = (value: unknown): string | null => {
  const number = finiteNumber(value)
  if (number == null || number <= 0) return null
  // App Server documents Unix seconds; accepting milliseconds costs nothing
  // and makes the display tolerant of a few proxy implementations.
  const milliseconds = number > 1e12 ? number : number * 1000
  const date = new Date(milliseconds)
  return Number.isNaN(date.getTime()) ? null : date.toISOString()
}

const formatWindowDuration = (minutes: number): string => {
  if (minutes >= 10080 && minutes % 10080 === 0) return `${minutes / 1440}d`
  if (minutes >= 1440 && minutes % 1440 === 0) return `${minutes / 1440}d`
  if (minutes >= 60 && minutes % 60 === 0) return `${minutes / 60}h`
  return `${minutes}m`
}

const formatWindowDurationDetails = (minutes: number): string => {
  return formatWindowDuration(minutes)
}

const formatRateLimitWindowLabel = (
  limitID: string,
  limitName: string,
  windowDurationMins: number
): string => {
  // The documented `codex` bucket is the normal 5h/7d account window.
  // Map this known field directly to the human window label instead of
  // rendering the redundant "codex 7d" form.
  const normalizedID = limitID.trim().toLowerCase()
  const normalizedName = limitName.trim().toLowerCase()
  if (normalizedID === 'codex' || normalizedName === 'codex') {
    return formatWindowDuration(windowDurationMins)
  }
  return `${limitName} ${formatWindowDuration(windowDurationMins)}`
}

const formatServerCount = (value: number): string => formatCompactNumber(value)

const formatLocalRequests = (stats: WindowStats): string => formatCompactNumber(stats.requests, { allowBillions: false })
const formatLocalTokens = (stats: WindowStats): string => formatCompactNumber(stats.tokens)
const formatLocalCost = (value: number): string => value.toFixed(2)

const localStatsForWindow = (windowDurationMins: number): WindowStats | null => {
  // Local usage-log windows are specifically 5h/7d. Do not attach a 5h
  // counter to a future short bucket such as the documented 15m window.
  if (windowDurationMins >= 240 && windowDurationMins <= 360) {
    return props.localWindowStats?.five_hour ?? null
  }
  if (windowDurationMins >= 10080) {
    return props.localWindowStats?.seven_day ?? null
  }
  return null
}

const formatRawValue = (value: unknown): string => {
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (value == null) return 'null'
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

const formatRawBucketDetails = (key: string, bucket: OpenAIAppServerRateLimitBucket): string => {
  if (bucket.raw_value !== undefined) return formatRawValue(bucket.raw_value)
  const fields = bucket.raw_fields ?? Object.fromEntries(
    Object.entries(bucket).filter(([field]) => !field.startsWith('raw_'))
  )
  if (fields && Object.keys(fields).length > 0) {
    return Object.entries(fields)
      .map(([field, value]) => `${field}: ${formatRawValue(value)}`)
      .join(', ')
  }
  return `${key}: unparsed bucket`
}

const serverTokenUsage = computed(() => data.value?.server_token_usage ?? null)

const rateLimitWindows = computed<RateLimitDisplayWindow[]>(() => {
  const buckets = data.value?.rate_limits_by_limit_id ?? data.value?.rateLimitsByLimitId

  const colors: RateLimitColor[] = ['indigo', 'emerald', 'purple', 'amber']
  const entries = buckets && typeof buckets === 'object' && !Array.isArray(buckets)
    ? Object.entries(buckets)
      .filter(([, bucket]) => bucket != null)
      .sort(([left], [right]) => left.localeCompare(right))
    : []

  const windows: RateLimitDisplayWindow[] = []
  entries.forEach(([key, rawBucket], bucketIndex) => {
    const windowsBeforeBucket = windows.length
    const bucket = rawBucket && typeof rawBucket === 'object'
      ? rawBucket as OpenAIAppServerRateLimitBucket
      : { limit_id: key, raw_value: rawBucket } as OpenAIAppServerRateLimitBucket
    const limitID = String(bucket.limit_id ?? bucket.limitId ?? key).trim() || key
    const limitName = String(bucket.limit_name ?? bucket.limitName ?? limitID).trim() || limitID
    const isKnownCodexBucket = limitID.toLowerCase() === 'codex' || limitName.toLowerCase() === 'codex'
    const bucketWindows: Array<{
      kind: 'primary' | 'secondary'
      usedPercent: unknown
      windowDurationMins: unknown
      resetsAt: unknown
    }> = []

    if (bucket.primary && typeof bucket.primary === 'object') {
      bucketWindows.push({
        kind: 'primary',
        usedPercent: bucket.primary.used_percent ?? bucket.primary.usedPercent,
        windowDurationMins: bucket.primary.window_duration_mins ?? bucket.primary.windowDurationMins,
        resetsAt: bucket.primary.resets_at ?? bucket.primary.resetsAt
      })
    }
    if (bucket.secondary && typeof bucket.secondary === 'object') {
      bucketWindows.push({
        kind: 'secondary',
        usedPercent: bucket.secondary.used_percent ?? bucket.secondary.usedPercent,
        windowDurationMins: bucket.secondary.window_duration_mins ?? bucket.secondary.windowDurationMins,
        resetsAt: bucket.secondary.resets_at ?? bucket.secondary.resetsAt
      })
    }
    // Some early ChatGPT responses flattened one window directly on the
    // bucket. Only use it when no nested window is present.
    if (bucketWindows.length === 0 && (
      bucket.used_percent != null ||
      bucket.usedPercent != null ||
      bucket.window_duration_mins != null ||
      bucket.windowDurationMins != null ||
      bucket.resets_at != null ||
      bucket.resetsAt != null
    )) {
      bucketWindows.push({
        kind: 'primary',
        usedPercent: bucket.used_percent ?? bucket.usedPercent,
        windowDurationMins: bucket.window_duration_mins ?? bucket.windowDurationMins,
        resetsAt: bucket.resets_at ?? bucket.resetsAt
      })
    }

    bucketWindows.forEach((window, windowIndex) => {
      const usedPercent = finiteNumber(window.usedPercent)
      const rawWindowDurationMins = finiteNumber(window.windowDurationMins)
      const windowDurationMins = rawWindowDurationMins == null
        ? 0
        : Math.max(0, Math.round(rawWindowDurationMins))
      if (usedPercent == null || windowDurationMins <= 0) return
      windows.push({
        key: `${key}-${window.kind}-${windowIndex}`,
        label: formatRateLimitWindowLabel(limitID, limitName, windowDurationMins),
        usedPercent,
        windowDurationMins,
        resetsAt: unixSecondsToISO(window.resetsAt),
        color: colors[(bucketIndex + windowIndex) % colors.length],
        windowStats: localStatsForWindow(windowDurationMins),
        showDurationDetails: !isKnownCodexBucket
      })
    })

    if (windows.length === windowsBeforeBucket) {
      windows.push({
        key: `${key}-raw`,
        label: limitName,
        usedPercent: 0,
        windowDurationMins: 0,
        resetsAt: null,
        color: colors[bucketIndex % colors.length],
        rawDetails: formatRawBucketDetails(key, bucket)
      })
    }
  })

  // Legacy /wham/usage responses expose only `rate_limit.primary_window` and
  // `secondary_window`. Use them only when the keyed view did not yield a
  // usable window, avoiding a duplicate mirror of the codex bucket.
  if (windows.length === 0) {
    const legacy = data.value?.rate_limit ?? data.value?.rateLimit
    if (legacy) {
      const legacyWindows = [
        { kind: 'primary', window: legacy.primary_window ?? legacy.primary },
        { kind: 'secondary', window: legacy.secondary_window ?? legacy.secondary }
      ] as const
      const legacyLabel = String(
        legacy.limit_name ?? legacy.limitName ?? legacy.limit_id ?? legacy.limitId ?? 'codex'
      ).trim() || 'codex'
      legacyWindows.forEach(({ kind, window }, index) => {
        if (!window) return
        const usedPercent = finiteNumber(window.used_percent ?? window.usedPercent)
        const durationMinutes = window.window_duration_mins ?? window.windowDurationMins
        const rawDuration = finiteNumber(
          window.limit_window_seconds ?? (durationMinutes != null ? durationMinutes * 60 : null)
        )
        const windowDurationMins = rawDuration == null ? 0 : Math.max(0, Math.round(rawDuration / 60))
        if (usedPercent == null || windowDurationMins <= 0) return
        windows.push({
          key: `legacy-${kind}-${index}`,
          label: formatRateLimitWindowLabel(
            String(legacy.limit_id ?? legacy.limitId ?? 'codex'),
            legacyLabel,
            windowDurationMins
          ),
          usedPercent,
          windowDurationMins,
          resetsAt: unixSecondsToISO(window.reset_at ?? window.resets_at ?? window.resetsAt),
          color: colors[index % colors.length],
          windowStats: localStatsForWindow(windowDurationMins),
          showDurationDetails: false
        })
      })
    }
  }
  return windows
})

const localStatsRows = computed(() => {
  const candidates = [
    { label: '5h', stats: props.localWindowStats?.five_hour ?? null },
    { label: '7d', stats: props.localWindowStats?.seven_day ?? null }
  ]
  const consumed = new Set(
    rateLimitWindows.value
      .map((window) => window.windowStats)
      .filter((stats): stats is WindowStats => stats != null)
  )
  return candidates.filter((row): row is { label: string; stats: WindowStats } =>
    row.stats != null && !consumed.has(row.stats)
  )
})

const formatRateLimitResetTitle = (resetsAt: string | null): string => {
  if (!resetsAt) return t('admin.accounts.openaiQuotaReset.expiresAt', { time: '-' })
  const date = new Date(resetsAt)
  if (Number.isNaN(date.getTime())) return resetsAt
  return t('admin.accounts.openaiQuotaReset.expiresAtFull', {
    time: new Intl.DateTimeFormat(undefined, {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    }).format(date)
  })
}

const resetCreditDetailsTitle = computed(() =>
  resetCreditExpirations.value
    .map((expiresAt) => formatResetCreditExpiry(expiresAt, 'full'))
    .join('\n')
)

const resetCreditDetailsToggleLabel = computed(() => {
  if (showResetCreditDetails.value) {
    return t('admin.accounts.openaiQuotaReset.collapseExpirations')
  }
  return t('admin.accounts.openaiQuotaReset.expandExpirations', { count: hiddenResetCreditCount.value })
})

const resetButtonTitle = computed(() => {
  if (isShadow.value) return t('admin.accounts.openaiQuotaReset.resetTooltipShadow')
  if (!data.value) return t('admin.accounts.openaiQuotaReset.resetTooltipNeedQuery')
  if (!canReset.value) return t('admin.accounts.openaiQuotaReset.resetTooltipNoCredits')
  return t('admin.accounts.openaiQuotaReset.resetTooltipReady')
})

// "次数" button doubles as the upstream-query trigger and the count display.
// Tooltip differs between "click to load" (no data yet) and "click to refresh".
const countButtonTitle = computed(() => {
  if (!data.value) return t('admin.accounts.openaiQuotaReset.countTooltipLoad')
  return t('admin.accounts.openaiQuotaReset.countTooltipRefresh')
})

const truncatedError = computed(() => {
  if (!error.value) return ''
  return error.value.length > 80 ? `${error.value.slice(0, 80)}…` : error.value
})

const getResetCreditExpiryTime = (value: string): number => {
  const time = new Date(value).getTime()
  return Number.isNaN(time) ? Number.POSITIVE_INFINITY : time
}

const compareResetCreditExpiry = (a: string, b: string): number => {
  const diff = getResetCreditExpiryTime(a) - getResetCreditExpiryTime(b)
  if (diff !== 0) return diff
  return a.localeCompare(b)
}

const formatResetCreditExpiry = (value: string, style: 'short' | 'full'): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value

  const options: Intl.DateTimeFormatOptions = {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }
  if (style === 'full') {
    options.year = 'numeric'
  }

  return new Intl.DateTimeFormat(undefined, options).format(date)
}

const extractErrorMessage = (e: unknown): string => {
  // The project's axios response interceptor (api/client.ts) flattens server
  // errors into { status, code, message, reason, ... } and re-rejects them, so
  // the message lives at the top level rather than under .response.data. Fall
  // back to the raw axios shape for the cancellation/network branches that
  // bypass the flattening, and finally to the generic i18n string.
  const err = e as {
    message?: string
    reason?: string
    response?: { data?: { message?: string; error?: string } }
  }
  return (
    err?.message ||
    err?.reason ||
    err?.response?.data?.message ||
    err?.response?.data?.error ||
    t('common.error')
  )
}

const toggleResetCreditDetails = () => {
  if (hiddenResetCreditCount.value <= 0) return
  showResetCreditDetails.value = !showResetCreditDetails.value
}

const handleQuery = async () => {
  if (loading.value) return
  loading.value = true
  error.value = null
  resetMessage.value = null
  resetWarning.value = null
  showResetCreditDetails.value = false
  try {
    // Start both reads together. The primary 查询 action therefore refreshes
    // local counters and the authoritative App Server snapshot as one gesture.
    const quotaPromise = refreshOpenAIQuota(props.account.id)
    const localPromise = props.queryLocalUsage?.() ?? Promise.resolve()
    const result = await quotaPromise
    await localPromise
    const activeResult = { ...result, source: 'active' as const }
    openAIQuotaSnapshotCache.set(props.account.id, activeResult)
    data.value = activeResult
    emit('quota-updated', activeResult)
    if (result.account_auto_enabled && result.account) {
      emit('account-updated', result.account)
    }
    if (result.cache_persisted) {
      cachedData.value = activeResult
    } else {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.refreshCachePersistFailed')
    }
  } catch (e) {
    error.value = extractErrorMessage(e)
  } finally {
    loading.value = false
  }
}

const clearQuotaSnapshot = () => {
  openAIQuotaSnapshotCache.delete(props.account.id)
  const fallback = quotaDataForAccount(props.account)
  cachedData.value = fallback
  data.value = fallback
}

defineExpose({ query: handleQuery, clear: clearQuotaSnapshot })

const openResetConfirm = () => {
  if (resetting.value || loading.value) return
  if (!canReset.value) {
    error.value = t('admin.accounts.openaiQuotaReset.noCreditsAvailable')
    return
  }
  showResetConfirm.value = true
}

const confirmReset = async () => {
  showResetConfirm.value = false
  if (resetting.value) return
  if (!canReset.value) {
    error.value = t('admin.accounts.openaiQuotaReset.noCreditsAvailable')
    return
  }
  resetting.value = true
  error.value = null
  resetMessage.value = null
  resetWarning.value = null
  try {
    const result: OpenAIQuotaResetResult = await resetOpenAIQuota(props.account.id)
    showResetCreditDetails.value = false
    if (result.cache_refreshed && result.quota) {
      openAIQuotaSnapshotCache.set(props.account.id, result.quota)
      data.value = result.quota
      cachedData.value = result.quota
      emit('quota-updated', result.quota)
    } else {
      openAIQuotaSnapshotCache.delete(props.account.id)
      data.value = null
      cachedData.value = null
      emit('quota-updated', { fetched_at: 0 })
    }
    if (result.account) emit('account-updated', result.account)

    if (result.warning_code === 'reset_credit_cache_refresh_failed') {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.resetCacheRefreshFailed')
    } else if (result.warning_code === 'account_state_recovery_failed') {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.resetAccountRecoveryFailed')
    } else if (result.warning_code === 'account_state_refresh_failed') {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.resetAccountRefreshFailed')
    } else {
      resetMessage.value = t('admin.accounts.openaiQuotaReset.resetSuccess', {
        windows: result.windows_reset
      })
    }
  } catch (e) {
    error.value = extractErrorMessage(e)
  } finally {
    resetting.value = false
  }
}

watch(
  () => [
    props.account.id,
    props.account.extra?.codex_reset_credit_snapshot,
    props.account.extra?.codex_rate_limit_snapshot,
    props.account.extra?.codex_usage_updated_at,
    props.account.extra?.codex_5h_used_percent,
    props.account.extra?.codex_5h_window_minutes,
    props.account.extra?.codex_5h_reset_at,
    props.account.extra?.codex_7d_used_percent,
    props.account.extra?.codex_7d_window_minutes,
    props.account.extra?.codex_7d_reset_at
  ] as const,
  () => {
    // Account row may be reused across paginated lists; reset local state.
    const fallback = quotaDataForAccount(props.account)
    cachedData.value = fallback
    data.value = fallback
    error.value = null
    resetMessage.value = null
    resetWarning.value = null
    loading.value = false
    resetting.value = false
    showResetConfirm.value = false
    showResetCreditDetails.value = false
  }
)

watch(
  () => props.externalQuotaResult,
  (result) => {
    if (!result) return
    const activeResult = { ...result, source: 'active' as const }
    openAIQuotaSnapshotCache.set(props.account.id, activeResult)
    data.value = activeResult
    cachedData.value = activeResult
    error.value = null
    emit('quota-updated', activeResult)
  },
  { immediate: true }
)

watch(
  resetCreditExpirations,
  () => {
    if (hiddenResetCreditCount.value <= 0) {
      showResetCreditDetails.value = false
    }
  }
)
</script>
