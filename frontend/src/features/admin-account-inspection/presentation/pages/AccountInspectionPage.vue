<template>
  <AppLayout>
    <main class="mx-auto max-w-[1600px] space-y-5 pb-12">
      <header class="flex flex-wrap items-start justify-between gap-4 border-b border-gray-200 pb-4 dark:border-dark-700">
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <Icon name="clipboard" size="lg" class="text-primary-600 dark:text-primary-400" />
            <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.accountInspection.title') }}</h1>
            <span class="rounded-md px-2 py-0.5 text-xs font-medium" :class="runStatusClass">
              {{ runStatusLabel }}
            </span>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accountInspection.subtitle') }}</p>
          <p class="mt-2 text-xs text-gray-400 dark:text-gray-500">
            <span>{{ t('admin.accountInspection.lastRun') }} {{ run?.completed_at ? formatDateTime(run.completed_at) : '-' }}</span>
            <span class="mx-2">·</span>
            <span>{{ t('admin.accountInspection.nextRun') }} {{ run?.next_run_at && settings.enabled ? formatDateTime(run.next_run_at) : '-' }}</span>
          </p>
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="btn btn-secondary inline-flex items-center gap-2"
            :disabled="loading || running"
            :title="t('admin.accountInspection.refresh')"
            @click="loadOverview"
          >
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
            <span>{{ t('admin.accountInspection.refresh') }}</span>
          </button>
          <button
            type="button"
            class="btn btn-primary inline-flex items-center gap-2"
            :disabled="running || loading"
            @click="runNow"
          >
            <Icon name="play" size="sm" :class="{ 'animate-pulse': running }" />
            <span>{{ running ? t('admin.accountInspection.running') : t('admin.accountInspection.runNow') }}</span>
          </button>
        </div>
      </header>

      <div v-if="errorMessage" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
        {{ errorMessage }}
      </div>

      <section aria-labelledby="inspection-settings-title" class="border-y border-gray-200 py-5 dark:border-dark-700">
        <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 id="inspection-settings-title" class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accountInspection.settings.title') }}</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accountInspection.settings.caption') }}</p>
          </div>
          <button type="button" class="btn btn-primary inline-flex items-center gap-2" :disabled="saving || loading" @click="saveSettings">
            <Icon name="check" size="sm" />
            <span>{{ saving ? t('admin.accountInspection.saving') : t('admin.accountInspection.save') }}</span>
          </button>
        </div>

        <div class="grid gap-x-6 gap-y-4 md:grid-cols-2 xl:grid-cols-4">
          <label class="flex items-center justify-between gap-3 rounded-md bg-gray-50 px-3 py-2.5 dark:bg-dark-800/70">
            <span class="text-sm text-gray-700 dark:text-gray-200">{{ t('admin.accountInspection.settings.enabled') }}</span>
            <Toggle :model-value="settings.enabled" @update:model-value="settings.enabled = $event" />
          </label>
          <label class="flex items-center justify-between gap-3 rounded-md bg-gray-50 px-3 py-2.5 dark:bg-dark-800/70">
            <span class="text-sm text-gray-700 dark:text-gray-200">{{ t('admin.accountInspection.settings.autoDisable') }}</span>
            <Toggle :model-value="settings.auto_disable" @update:model-value="settings.auto_disable = $event" />
          </label>
          <label class="field-group">
            <span class="field-label">{{ t('admin.accountInspection.settings.interval') }}</span>
            <select v-model.number="settings.interval_minutes" class="input">
              <option v-for="option in intervalOptions" :key="option" :value="option">{{ t('admin.accountInspection.settings.minutes', { value: option }) }}</option>
            </select>
          </label>
          <label class="field-group">
            <span class="field-label">{{ t('admin.accountInspection.settings.lookback') }}</span>
            <select v-model.number="settings.lookback_minutes" class="input">
              <option v-for="option in lookbackOptions" :key="option" :value="option">{{ t('admin.accountInspection.settings.minutes', { value: option }) }}</option>
            </select>
          </label>
          <label class="field-group">
            <span class="field-label">{{ t('admin.accountInspection.settings.ttft') }}</span>
            <input v-model.number="settings.ttft_threshold_ms" class="input" type="number" min="0" step="100" />
          </label>
          <label class="field-group">
            <span class="field-label">{{ t('admin.accountInspection.settings.successRate') }}</span>
            <input :value="percentValue(settings.success_rate_threshold)" class="input" type="number" min="0" max="100" step="1" @input="setPercent('success_rate_threshold', $event)" />
          </label>
          <label class="field-group">
            <span class="field-label">{{ t('admin.accountInspection.settings.minRequests') }}</span>
            <input v-model.number="settings.min_requests" class="input" type="number" min="1" step="1" />
          </label>
          <label class="flex items-center justify-between gap-3 rounded-md bg-gray-50 px-3 py-2.5 dark:bg-dark-800/70">
            <span class="text-sm text-gray-700 dark:text-gray-200">{{ t('admin.accountInspection.settings.oauthQuota') }}</span>
            <Toggle :model-value="settings.oauth_quota_check_enabled" @update:model-value="settings.oauth_quota_check_enabled = $event" />
          </label>
          <label class="flex items-center justify-between gap-3 rounded-md bg-gray-50 px-3 py-2.5 dark:bg-dark-800/70">
            <span class="text-sm text-gray-700 dark:text-gray-200">{{ t('admin.accountInspection.settings.apiKeyQuota') }}</span>
            <Toggle :model-value="settings.api_key_quota_check_enabled" @update:model-value="settings.api_key_quota_check_enabled = $event" />
          </label>
          <label class="field-group">
            <span class="field-label">{{ t('admin.accountInspection.settings.cacheMinimum') }}</span>
            <input :value="percentValue(settings.api_key_min_cache_hit_rate)" class="input" type="number" min="0" max="100" step="1" @input="setPercent('api_key_min_cache_hit_rate', $event)" />
          </label>
          <label class="field-group">
            <span class="field-label">{{ t('admin.accountInspection.settings.multiplierMaximum') }}</span>
            <input v-model.number="settings.api_key_max_rate_multiplier" class="input" type="number" min="0" step="0.05" />
          </label>
          <label class="field-group">
            <span class="field-label">{{ t('admin.accountInspection.settings.remainingMinimum') }}</span>
            <input v-model.number="settings.api_key_min_remaining_quota" class="input" type="number" min="0" step="0.01" />
          </label>
        </div>
      </section>

      <section aria-label="Inspection summary" class="grid grid-cols-2 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800 sm:grid-cols-3 xl:grid-cols-6">
        <div v-for="item in summaryItems" :key="item.key" class="border-b border-r border-gray-200 px-4 py-3 last:border-r-0 dark:border-dark-700 xl:border-b-0">
          <dt class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</dt>
          <dd class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">{{ item.value }}</dd>
        </div>
      </section>

      <QuotaUsageDistributionChart :distribution="summary.quota_usage_distribution" :loading="loading" />

      <section aria-labelledby="inspection-results-title" class="space-y-3">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 id="inspection-results-title" class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accountInspection.results.title') }}</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ resultRangeLabel }}</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <div class="relative">
              <Icon name="search" size="sm" class="pointer-events-none absolute left-2.5 top-2.5 text-gray-400" />
              <input v-model="search" class="input w-52 pl-8" :placeholder="t('admin.accountInspection.results.search')" @keyup.enter="reloadResults" />
            </div>
            <select v-model="statusFilter" class="input w-32" @change="reloadResults">
              <option value="all">{{ t('admin.accountInspection.results.all') }}</option>
              <option value="flagged">{{ t('admin.accountInspection.results.flagged') }}</option>
              <option value="healthy">{{ t('admin.accountInspection.results.healthy') }}</option>
              <option value="disabled">{{ t('admin.accountInspection.results.disabled') }}</option>
            </select>
            <select v-model="typeFilter" class="input w-32" @change="reloadResults">
              <option value="all">{{ t('admin.accountInspection.results.allTypes') }}</option>
              <option value="oauth">OAuth</option>
              <option value="apikey">API Key</option>
              <option value="bedrock">Bedrock</option>
            </select>
            <button type="button" class="btn btn-ghost inline-flex h-9 w-9 items-center justify-center p-0" :title="t('admin.accountInspection.refresh')" @click="reloadResults">
              <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
            </button>
          </div>
        </div>

        <div class="overflow-x-auto rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <table class="min-w-[1180px] w-full table-fixed text-left text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/60 dark:text-gray-400">
              <tr>
                <th class="w-64 px-4 py-3 font-medium">{{ t('admin.accountInspection.results.account') }}</th>
                <th class="w-28 px-4 py-3 font-medium">{{ t('admin.accountInspection.results.state') }}</th>
                <th class="w-36 px-4 py-3 font-medium">{{ t('admin.accountInspection.results.ttft') }}</th>
                <th class="w-32 px-4 py-3 font-medium">{{ t('admin.accountInspection.results.successRate') }}</th>
                <th class="w-32 px-4 py-3 font-medium">{{ t('admin.accountInspection.results.cache') }}</th>
                <th class="w-28 px-4 py-3 font-medium">{{ t('admin.accountInspection.results.multiplier') }}</th>
                <th class="w-36 px-4 py-3 font-medium">{{ t('admin.accountInspection.results.remaining') }}</th>
                <th class="px-4 py-3 font-medium">{{ t('admin.accountInspection.results.reason') }}</th>
                <th class="w-28 px-4 py-3 font-medium">{{ t('admin.accountInspection.results.action') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="row in results" :key="row.account_id" class="align-top">
                <td class="px-4 py-3">
                  <p class="truncate font-medium text-gray-900 dark:text-white" :title="row.name">{{ row.name }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">#{{ row.account_id }} · {{ row.platform }} · {{ typeLabel(row.type) }}</p>
                </td>
                <td class="px-4 py-3"><span class="rounded-md px-2 py-1 text-xs font-medium" :class="stateClass(row)">{{ stateLabel(row) }}</span></td>
                <td class="px-4 py-3 tabular-nums text-gray-700 dark:text-gray-200">{{ formatDuration(row.avg_first_token_ms) }}<span class="mt-1 block text-xs text-gray-400">{{ row.total_requests }} {{ t('admin.accountInspection.results.requests') }}</span></td>
                <td class="px-4 py-3 tabular-nums text-gray-700 dark:text-gray-200">{{ formatPercent(row.success_rate) }}</td>
                <td class="px-4 py-3 tabular-nums text-gray-700 dark:text-gray-200">{{ row.type === 'oauth' ? '-' : formatPercent(row.cache_hit_rate) }}</td>
                <td class="px-4 py-3 tabular-nums text-gray-700 dark:text-gray-200">{{ row.rate_multiplier == null ? '-' : row.rate_multiplier.toFixed(2) }}</td>
                <td class="px-4 py-3 tabular-nums text-gray-700 dark:text-gray-200">{{ formatRemaining(row) }}</td>
                <td class="px-4 py-3"><div v-if="row.reasons?.length" class="flex flex-wrap gap-1"><span v-for="reason in row.reasons ?? []" :key="reason" class="rounded bg-red-50 px-1.5 py-0.5 text-xs text-red-700 dark:bg-red-950/40 dark:text-red-300">{{ reasonLabel(reason) }}</span></div><span v-else class="text-gray-400">-</span></td>
                <td class="px-4 py-3 text-xs" :class="actionClass(row.action)">{{ actionLabel(row.action) }}</td>
              </tr>
              <tr v-if="!loading && results.length === 0"><td colspan="9" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accountInspection.results.noResults') }}</td></tr>
            </tbody>
          </table>
          <Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="changePage" @update:pageSize="changePageSize" />
        </div>
      </section>
    </main>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import Pagination from '@/common/widgets/data/Pagination.vue'
import { useAppStore } from '@/core/stores/appStore'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import { formatDateTime } from '@/core/utils/format'
import { getOverview, runInspection, updateSettings } from '../../data/datasources/accountInspectionDatasource'
import type { AccountInspectionOverview, AccountInspectionResult, AccountInspectionSettings } from '../../data/dtos/accountInspectionDtos'
import QuotaUsageDistributionChart from '../widgets/QuotaUsageDistributionChart.vue'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
const running = ref(false)
const errorMessage = ref('')
const search = ref('')
const statusFilter = ref('all')
const typeFilter = ref('all')
const overview = ref<AccountInspectionOverview | null>(null)
const settings = reactive<AccountInspectionSettings>({
  enabled: false,
  interval_minutes: 60,
  auto_disable: true,
  lookback_minutes: 60,
  min_requests: 1,
  ttft_threshold_ms: 30000,
  success_rate_threshold: 0.6,
  oauth_quota_check_enabled: true,
  api_key_quota_check_enabled: true,
  api_key_min_cache_hit_rate: 0,
  api_key_max_rate_multiplier: 0,
  api_key_min_remaining_quota: 0,
})
const pagination = reactive({ page: 1, page_size: 50, total: 0 })
const intervalOptions = [5, 15, 30, 60, 120, 360, 720, 1440]
const lookbackOptions = [60, 120, 360, 720, 1440]
let refreshTimer: ReturnType<typeof setInterval> | null = null

const run = computed(() => overview.value?.run)
const results = computed(() => overview.value?.results?.items ?? [])
const runStatusLabel = computed(() => {
  const status = run.value?.status
  if (status === 'running') return t('admin.accountInspection.status.running')
  if (status === 'succeeded') return t('admin.accountInspection.status.succeeded')
  if (status === 'failed') return t('admin.accountInspection.status.failed')
  return t('admin.accountInspection.status.idle')
})
const runStatusClass = computed(() => {
  const status = run.value?.status
  if (status === 'running') return 'bg-blue-50 text-blue-700 dark:bg-blue-950/40 dark:text-blue-300'
  if (status === 'failed') return 'bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300'
  if (status === 'succeeded') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
})
const summary = computed(() => run.value?.summary ?? {
  inspected: 0,
  healthy: 0,
  flagged: 0,
  disabled: 0,
  already_disabled: 0,
  oauth_accounts: 0,
  api_key_accounts: 0,
  quota_usage_distribution: {
    average_used_percent: null,
    measured_accounts: 0,
    unknown_accounts: 0,
    buckets: [],
  },
})
const summaryItems = computed(() => [
  { key: 'inspected', label: t('admin.accountInspection.summary.inspected'), value: summary.value.inspected },
  { key: 'flagged', label: t('admin.accountInspection.summary.flagged'), value: summary.value.flagged },
  { key: 'disabled', label: t('admin.accountInspection.summary.disabled'), value: summary.value.disabled },
  { key: 'healthy', label: t('admin.accountInspection.summary.healthy'), value: summary.value.healthy },
  { key: 'oauth', label: t('admin.accountInspection.summary.oauth'), value: summary.value.oauth_accounts },
  { key: 'apikey', label: t('admin.accountInspection.summary.apiKey'), value: summary.value.api_key_accounts },
])
const resultRangeLabel = computed(() => {
  const total = pagination.total
  if (!total) return t('admin.accountInspection.results.noResults')
  const from = (pagination.page - 1) * pagination.page_size + 1
  const to = Math.min(total, pagination.page * pagination.page_size)
  return t('admin.accountInspection.results.range', { from, to, total })
})

async function loadOverview(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await getOverview({ page: pagination.page, page_size: pagination.page_size, status: statusFilter.value, type: typeFilter.value, search: search.value.trim() })
    overview.value = data
    Object.assign(settings, data.settings)
    pagination.total = data.results.total
    running.value = data.run.status === 'running'
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error, t('admin.accountInspection.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function reloadResults(): Promise<void> {
  pagination.page = 1
  await loadOverview()
}

async function saveSettings(): Promise<void> {
  if (saving.value) return
  saving.value = true
  errorMessage.value = ''
  try {
    const saved = await updateSettings({ ...settings })
    Object.assign(settings, saved)
    appStore.showSuccess(t('admin.accountInspection.saved'))
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error, t('admin.accountInspection.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function runNow(): Promise<void> {
  if (running.value) return
  if (settings.auto_disable && !window.confirm(t('admin.accountInspection.confirmAutoDisable'))) return
  running.value = true
  errorMessage.value = ''
  try {
    const data = await runInspection()
    overview.value = data
    Object.assign(settings, data.settings)
    pagination.total = data.results.total
    appStore.showSuccess(t('admin.accountInspection.runCompleted', { count: data.run.summary.flagged }))
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error, t('admin.accountInspection.runFailed'))
  } finally {
    running.value = false
    await loadOverview()
  }
}

function changePage(page: number): void {
  pagination.page = page
  void loadOverview()
}

function changePageSize(value: number): void {
  pagination.page_size = value
  pagination.page = 1
  void loadOverview()
}

function percentValue(value: number): number {
  return Math.round(value * 1000) / 10
}

function setPercent(key: 'success_rate_threshold' | 'api_key_min_cache_hit_rate', event: Event): void {
  const value = Number((event.target as HTMLInputElement).value)
  settings[key] = Number.isFinite(value) ? Math.max(0, Math.min(100, value)) / 100 : 0
}

function formatPercent(value?: number | null): string {
  return value == null ? '-' : `${(value * 100).toFixed(1)}%`
}

function formatDuration(value?: number | null): string {
  if (value == null) return '-'
  if (value >= 1000) return `${(value / 1000).toFixed(1)}s`
  return `${Math.round(value)}ms`
}

function formatRemaining(row: AccountInspectionResult): string {
  if (row.type === 'oauth') return '-'
  if (row.quota_unlimited) return t('admin.accountInspection.results.unlimited')
  if (row.remaining_quota == null) return '-'
  return `$${row.remaining_quota.toFixed(2)}${row.remaining_quota_dimension ? ` · ${row.remaining_quota_dimension}` : ''}`
}

function typeLabel(type: string): string {
  if (type === 'oauth') return 'OAuth'
  if (type === 'bedrock') return 'Bedrock'
  return 'API Key'
}

function stateLabel(row: AccountInspectionResult): string {
  if (row.status === 'unknown') return t('admin.accountInspection.results.unknown')
  if (!row.reasons?.length) return t('admin.accountInspection.results.healthy')
  return t('admin.accountInspection.results.flagged')
}

function stateClass(row: AccountInspectionResult): string {
  if (row.status === 'unknown') return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  if (row.reasons?.length) return 'bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300'
  return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
}

function actionLabel(action: string): string {
  if (action === 'disabled') return t('admin.accountInspection.results.actionDisabled')
  if (action === 'already_disabled') return t('admin.accountInspection.results.actionAlreadyDisabled')
  if (action === 'reported') return t('admin.accountInspection.results.actionReported')
  if (action === 'error') return t('admin.accountInspection.results.actionError')
  return '-'
}

function actionClass(action: string): string {
  if (action === 'disabled') return 'text-emerald-600 dark:text-emerald-400'
  if (action === 'error') return 'text-red-600 dark:text-red-400'
  if (action === 'reported' || action === 'already_disabled') return 'text-amber-600 dark:text-amber-400'
  return 'text-gray-400'
}

function reasonLabel(reason: string): string {
  if (reason.startsWith('oauth_quota_exhausted')) return t('admin.accountInspection.reasons.oauthQuota')
  if (reason === 'first_token_over_threshold') return t('admin.accountInspection.reasons.ttft')
  if (reason === 'success_rate_below_threshold') return t('admin.accountInspection.reasons.successRate')
  if (reason === 'cache_hit_rate_below_threshold') return t('admin.accountInspection.reasons.cache')
  if (reason === 'rate_multiplier_over_threshold') return t('admin.accountInspection.reasons.multiplier')
  if (reason === 'remaining_quota_below_threshold') return t('admin.accountInspection.reasons.remaining')
  if (reason === 'metrics_unavailable') return t('admin.accountInspection.reasons.metricsUnavailable')
  return reason
}

onMounted(() => {
  void loadOverview()
  refreshTimer = setInterval(() => {
    if (run.value?.status === 'running') void loadOverview()
  }, 5000)
})

onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<style scoped>
.field-group {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.field-label {
  color: rgb(107 114 128);
  font-size: 0.75rem;
  line-height: 1rem;
}

@media (prefers-color-scheme: dark) {
  .field-label {
    color: rgb(156 163 175);
  }
}
</style>
