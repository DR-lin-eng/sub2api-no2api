<template>
  <div class="mt-4 space-y-4">
    <div class="flex flex-wrap gap-1" role="tablist" :aria-label="t('admin.accounts.upstreamUsage.title')">
      <button v-for="item in tabs" :key="item" type="button" role="tab" :aria-selected="tab === item" class="rounded-lg px-3 py-2 text-xs font-medium" :class="tab === item ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-600 dark:text-gray-300'" @click="tab = item">
        {{ t(`admin.accounts.upstreamUsage.tabs.${item}`) }}
      </button>
    </div>
    <div class="flex flex-wrap items-end gap-3">
      <template v-if="tab === 'daily'">
        <label class="text-xs">{{ t('admin.accounts.upstreamUsage.scope') }}
          <select v-model="scope" class="input mt-1"><option value="daily">{{ t('admin.accounts.upstreamUsage.personal') }}</option><option value="workspace_daily">{{ t('admin.accounts.upstreamUsage.workspace') }}</option></select>
        </label>
        <label class="text-xs">{{ t('admin.accounts.upstreamUsage.startDate') }}<input v-model="startDate" class="input mt-1" type="date" /></label>
        <label class="text-xs">{{ t('admin.accounts.upstreamUsage.endDate') }}<input v-model="endDate" class="input mt-1" type="date" /></label>
      </template>
      <label v-if="tab === 'billing'" class="text-xs">{{ t('admin.accounts.upstreamUsage.resource') }}
        <select v-model="billingKind" class="input mt-1"><option value="remaining_balance">{{ t('admin.accounts.upstreamUsage.remainingBalance') }}</option><option value="monthly">{{ t('admin.accounts.upstreamUsage.monthly') }}</option></select>
      </label>
      <label v-if="tab === 'balance_history'" class="text-xs">{{ t('admin.accounts.upstreamUsage.window') }}
        <select v-model="windowMinutes" class="input mt-1"><option :value="300">{{ t('admin.accounts.upstreamUsage.fiveHour') }}</option><option :value="10080">{{ t('admin.accounts.upstreamUsage.weekly') }}</option></select>
      </label>
      <button v-if="tab !== 'threads'" type="button" class="btn btn-secondary text-xs" :disabled="loading || !query" @click="load()">{{ t('admin.accounts.upstreamUsage.refresh') }}</button>
    </div>
    <div v-if="tab === 'threads'" class="space-y-2">
      <label class="block text-xs">{{ t('admin.accounts.upstreamUsage.threadIDs') }}
        <textarea v-model="threadInput" rows="3" class="input mt-1 w-full font-mono text-xs" :placeholder="t('admin.accounts.upstreamUsage.threadPlaceholder')" />
      </label>
      <p class="text-xs text-gray-500">{{ t('admin.accounts.upstreamUsage.threadHint') }}</p>
      <div class="flex flex-wrap items-center gap-3">
        <label class="text-xs"><input v-model="topThreads" type="checkbox" class="mr-1" />{{ t('admin.accounts.upstreamUsage.topThreads') }}</label>
        <button type="button" class="btn btn-secondary text-xs" :disabled="loading || !threadIDs.length || threadIDs.length > 1000 || topThreads && threadGroups.length > 100" @click="submitThreads">{{ t('admin.accounts.upstreamUsage.query') }}</button>
      </div>
      <p v-if="topThreads" class="text-xs text-gray-500">{{ t('admin.accounts.upstreamUsage.descendantHint') }}</p>
      <label v-if="result?.kind === 'top_threads'" class="block text-xs">{{ t('admin.accounts.upstreamUsage.sortBy') }}
        <select v-model="threadSort" class="input mt-1"><option v-for="metric in topMetrics" :key="metric" :value="metric">{{ fieldLabel(metric) }}</option></select>
      </label>
      <p v-if="threadInvalid" class="text-xs text-red-600">{{ t('admin.accounts.upstreamUsage.invalidThreads') }}</p>
    </div>
    <p v-if="invalidDates" class="text-sm text-red-600">{{ t('admin.accounts.upstreamUsage.invalidDates') }}</p>
    <p v-if="loading" role="status" class="text-sm text-gray-500">{{ t('common.loading') }}</p>
    <p v-if="failed" role="alert" class="text-sm text-red-600">{{ t('admin.accounts.upstreamUsage.failed') }}</p>
    <p v-if="result?.unavailable" class="rounded-lg bg-amber-50 p-3 text-sm text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">{{ t('admin.accounts.upstreamUsage.unavailable', { status: result.upstream_status ?? '—' }) }}</p>
    <template v-if="result?.data && !result.unavailable">
      <div class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-500">
        <span>{{ t('admin.accounts.upstreamUsage.fetchedAt') }}: {{ dateTime(result.fetched_at * 1000) }}</span>
        <span v-if="freshness">{{ t('admin.accounts.upstreamUsage.dataAsOf') }}: {{ dateTime(freshness) }}</span>
        <span>{{ t('admin.accounts.upstreamUsage.interval', { minutes: result.refresh_after_seconds / 60 }) }}</span>
      </div>
      <p v-if="isHistory" class="text-xs text-amber-700 dark:text-amber-300">{{ t('admin.accounts.upstreamUsage.approximate') }}</p>
      <div v-if="tab === 'balance_history' && balanceSamples.length" class="h-56"><Line :data="balanceChart" :options="balanceOptions" /></div>
      <div class="max-h-96 overflow-auto rounded-lg border border-gray-100 dark:border-dark-500">
        <table v-if="tab === 'daily'" class="w-full whitespace-nowrap text-left text-xs">
          <thead class="bg-gray-50 dark:bg-dark-600"><tr><th v-for="column in dailyColumns" :key="column" class="p-2">{{ t(`admin.accounts.upstreamUsage.columns.${column}`) }}</th></tr></thead>
          <tbody><tr v-for="(row, i) in dailyRows" :key="i" class="border-t border-gray-100 dark:border-dark-500"><td class="p-2">{{ row.date }}</td><td class="p-2">{{ row.model }}</td><td v-for="column in dailyNumbers" :key="column" class="p-2 tabular-nums">{{ number(row[column]) }}</td></tr></tbody>
        </table>
        <table v-else-if="tab === 'threads'" class="w-full whitespace-nowrap text-left text-xs">
          <thead class="bg-gray-50 dark:bg-dark-600"><tr><th class="p-2">{{ t('admin.accounts.upstreamUsage.columns.thread') }}</th><th v-for="column in threadColumns" :key="column" class="p-2">{{ fieldLabel(column) }}</th></tr></thead>
          <tbody><tr v-for="(row, i) in threadRows" :key="i" class="border-t border-gray-100 dark:border-dark-500"><td class="max-w-64 truncate p-2" :title="String(row.thread_id)">{{ row.thread_id }}</td><td v-for="column in threadColumns" :key="column" class="p-2 tabular-nums">{{ threadValue(row[column], column) }}</td></tr></tbody>
        </table>
        <table v-else-if="tab === 'plan_history'" class="w-full text-left text-xs">
          <thead class="bg-gray-50 dark:bg-dark-600"><tr><th v-for="column in planColumns" :key="column" class="p-2">{{ fieldLabel(column) }}</th></tr></thead>
          <tbody><tr v-for="(row, i) in planRows" :key="i" class="border-t border-gray-100 dark:border-dark-500"><td v-for="column in planColumns" :key="column" class="p-2">{{ historyValue(row[column], column) }}</td></tr></tbody>
        </table>
        <dl v-else-if="tab === 'billing'" class="grid grid-cols-1 gap-3 p-3 sm:grid-cols-2"><div v-for="field in billingFields" :key="field.key"><dt class="break-words text-gray-500">{{ fieldLabel(field.key) }}</dt><dd class="mt-1 break-words font-medium">{{ scalar(field.value) }}</dd></div></dl>
      </div>
      <p v-if="empty" class="py-3 text-sm text-gray-500">{{ t('admin.accounts.upstreamUsage.empty') }}</p>
      <button v-if="result.truncated && nextCursor" type="button" class="btn btn-secondary text-xs" :disabled="loading" @click="load(nextCursor)">{{ t('admin.accounts.upstreamUsage.loadMore') }}</button>
      <details class="text-xs"><summary class="cursor-pointer text-primary-600">{{ t('admin.accounts.upstreamUsage.fullDetails') }}</summary><pre class="mt-2 max-h-96 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-gray-50 p-3 dark:bg-dark-800">{{ JSON.stringify(result.data, null, 2) }}</pre></details>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend } from 'chart.js'
import { Line } from 'vue-chartjs'
import type { Account } from '@/types'
import type { OpenAIUsageQuery } from '../../data/datasources/openAIUsageDatasource'
import { useOpenAIUsageDetails } from '../composables/useOpenAIUsageDetails'
import { dailyModelUsage, sampleUsageBalance, threadUsageRows, usageNumber, usageRows, usageScalarFields } from '../openAIUsageDetailsDisplay'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend)
const props = defineProps<{ account: Account }>()
const { t, te } = useI18n()
const tabs = ['daily', 'billing', 'threads', 'plan_history', 'balance_history'] as const
const tab = ref<typeof tabs[number]>('daily')
const scope = ref<'daily' | 'workspace_daily'>('daily')
const billingKind = ref<'remaining_balance' | 'monthly'>('remaining_balance')
const windowMinutes = ref<300 | 10080>(300)
const localDate = (date: Date) => `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
const endDate = ref(localDate(new Date()))
const startDate = ref(localDate(new Date(Date.now() - 29 * 86400000)))
const threadInput = ref('')
const topThreads = ref(false)
const threadInvalid = ref(false)
const submittedThreads = ref<OpenAIUsageQuery | null>(null)
const threadIDs = computed(() => threadInput.value.trim().split(/[\s,，]+/).filter(Boolean))
const threadGroups = computed(() => threadInput.value.trim().split(/\r?\n/).map(line => line.trim().split(/[\s,，]+/).filter(Boolean)).filter(ids => ids.length))
const topMetrics = ['five_hour_limit_percent', 'weekly_limit_percent', 'balance_usage_credits']
const threadSort = ref('five_hour_limit_percent')
const invalidDates = computed(() => tab.value === 'daily' && (!startDate.value || !endDate.value || startDate.value > endDate.value || Date.parse(endDate.value) - Date.parse(startDate.value) > 90 * 86400000))
const query = computed<OpenAIUsageQuery | null>(() => {
  if (tab.value === 'threads') return submittedThreads.value
  if (tab.value === 'daily') return invalidDates.value ? null : { kind: scope.value, start_date: startDate.value, end_date: endDate.value }
  if (tab.value === 'billing') return { kind: billingKind.value }
  if (tab.value === 'balance_history') return { kind: 'balance_history', window_minutes: windowMinutes.value }
  return { kind: 'plan_history' }
})
const { result, loading, failed, load } = useOpenAIUsageDetails(
  () => props.account.id,
  () => query.value,
  () => [props.account.credentials, props.account.proxy_id, props.account.parent_account_id]
)
const submitThreads = () => {
  threadInvalid.value = new Set(threadIDs.value).size !== threadIDs.value.length || threadIDs.value.some(id => id.length > 200)
  if (threadInvalid.value) return
  const next: OpenAIUsageQuery = topThreads.value
    ? { kind: 'top_threads', threads: threadGroups.value.map(([thread_id, ...descendant_thread_ids]) => ({ thread_id, created_at: null, descendant_thread_ids })) }
    : { kind: 'threads', thread_ids: threadIDs.value }
  if (JSON.stringify(next) === JSON.stringify(submittedThreads.value)) void load()
  else submittedThreads.value = next
}
const dailyRows = computed(() => dailyModelUsage(result.value?.data ?? null))
const dailyNumbers = ['uncached', 'cached', 'output', 'total', 'credits'] as const
const dailyColumns = ['date', 'model', ...dailyNumbers]
const threadRows = computed(() => {
  const rows = threadUsageRows(result.value?.data ?? null)
  return result.value?.kind === 'top_threads' ? rows.sort((a, b) => (usageNumber(b[threadSort.value]) ?? -Infinity) - (usageNumber(a[threadSort.value]) ?? -Infinity)) : rows
})
const threadColumns = computed(() => result.value?.kind === 'top_threads'
  ? topMetrics
  : ['net_new_input_tokens', 'cached_input_tokens', 'output_tokens', 'estimated_usage_credits_micros', 'native_usage_usd_micros'])
const planRows = computed(() => usageRows(result.value?.data?.periods))
const planColumns = ['window_minutes', 'starts_at', 'ends_at', 'used_basis_points', 'accounting_complete']
const billingFields = computed(() => usageScalarFields(result.value?.data))
const balancePoints = computed(() => usageRows(result.value?.data?.points))
const balanceSamples = computed(() => sampleUsageBalance(balancePoints.value))
const balanceChart = computed(() => ({ labels: balanceSamples.value.map(point => dateTime(point.timestamp)), datasets: [{ label: t('admin.accounts.upstreamUsage.remainingPercent'), data: balanceSamples.value.map(point => point.remaining), borderColor: '#6366f1', backgroundColor: '#6366f1', pointRadius: 1, stepped: true as const }] }))
const balanceOptions = { responsive: true, maintainAspectRatio: false, scales: { y: { min: 0, max: 100, ticks: { callback: (value: string | number) => `${value}%` } } } }
const freshness = computed(() => result.value?.data?.data_as_of ?? result.value?.data?.data_freshness_ts)
const nextCursor = computed(() => typeof result.value?.data?.next_cursor === 'string' ? result.value.data.next_cursor : undefined)
const isHistory = computed(() => tab.value === 'plan_history' || tab.value === 'balance_history')
const empty = computed(() => tab.value === 'daily' ? !dailyRows.value.length : tab.value === 'threads' ? !threadRows.value.length : tab.value === 'plan_history' ? !planRows.value.length : tab.value === 'balance_history' ? !balancePoints.value.length : !billingFields.value.length)
const number = (value: unknown) => usageNumber(value)?.toLocaleString(undefined, { maximumFractionDigits: 6 }) ?? '—'
const scalar = (value: unknown): string => value == null ? '—' : typeof value === 'boolean' ? t(value ? 'common.yes' : 'common.no') : String(value)
const dateTime = (value: unknown) => {
  const date = new Date(typeof value === 'number' ? value > 0 && value < 1e12 ? value * 1000 : value : String(value))
  return Number.isNaN(date.getTime()) ? scalar(value) : date.toLocaleString()
}
const fieldLabel = (field: string) => {
  const key = `admin.accounts.upstreamUsage.fields.${field.replace(/\./g, '_')}`
  return te(key) ? t(key) : field.replace(/_/g, ' ')
}
const threadValue = (value: unknown, field: string) => {
  const parsed = usageNumber(value)
  if (parsed === null) return '—'
  if (field.endsWith('_micros')) return `${field === 'native_usage_usd_micros' ? '$' : ''}${number(parsed / 1e6)}`
  return `${number(parsed)}${field.endsWith('_percent') ? '%' : ''}`
}
const historyValue = (value: unknown, field: string) => value == null ? '—' : field.endsWith('_at') ? dateTime(value) : field === 'used_basis_points' ? `${number((usageNumber(value) ?? 0) / 100)}%` : scalar(value)
</script>
