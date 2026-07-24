<template>
  <AppLayout>
    <div class="space-y-6 pb-10" data-test="ingress-risk-view">
      <header class="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
        <div class="min-w-0">
          <h2 class="text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('admin.ingressRisk.title') }}
          </h2>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-dark-400">
            {{ t('admin.ingressRisk.description') }}
          </p>
        </div>
        <div class="flex shrink-0 items-center gap-3">
          <span class="text-xs text-gray-500 dark:text-dark-400">
            {{ lastUpdatedLabel }}
          </span>
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="refreshing"
            data-test="refresh"
            @click="refreshAll"
          >
            <Icon name="refresh" size="sm" :class="refreshing && 'animate-spin'" />
            {{ t('admin.ingressRisk.actions.refresh') }}
          </button>
        </div>
      </header>

      <div
        v-if="healthError"
        class="flex items-start gap-3 rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300"
        role="alert"
      >
        <Icon name="exclamationCircle" size="md" class="mt-0.5 shrink-0" />
        <span>{{ healthError }}</span>
      </div>

      <section
        class="rounded-lg border p-4 sm:p-5"
        :class="healthBandClass"
        data-test="health-band"
      >
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex min-w-0 items-start gap-3">
            <div class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg" :class="healthIconClass">
              <Icon :name="healthIcon" size="md" :stroke-width="2" />
            </div>
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.ingressRisk.health.title') }}
                </h3>
                <span class="rounded-full px-2 py-0.5 text-xs font-semibold" :class="healthBadgeClass">
                  {{ t(`admin.ingressRisk.health.${overallHealth}`) }}
                </span>
              </div>
              <p class="mt-1 text-sm text-gray-600 dark:text-dark-300">
                {{ t(`admin.ingressRisk.health.${overallHealth}Description`) }}
              </p>
              <p v-if="healthLastError" class="mt-1 break-all font-mono text-xs text-red-700 dark:text-red-300">
                {{ healthLastError }}
              </p>
            </div>
          </div>

          <div class="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:min-w-[540px]">
            <div
              v-for="signal in healthSignals"
              :key="signal.key"
              class="rounded-lg border border-black/5 bg-white/60 px-3 py-2.5 dark:border-white/10 dark:bg-dark-900/45"
            >
              <div class="flex items-center gap-1.5 text-xs font-medium text-gray-500 dark:text-dark-400">
                <span class="h-2 w-2 rounded-full" :class="signalDotClass(signal.level)"></span>
                {{ signal.label }}
              </div>
              <div class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">
                {{ signal.value }}
              </div>
            </div>
          </div>
        </div>
      </section>

      <section>
        <div class="mb-3 flex flex-col justify-between gap-1 sm:flex-row sm:items-end">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.ingressRisk.metrics.title') }}
          </h3>
          <p class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('admin.ingressRisk.metrics.cumulativeHint') }}
          </p>
        </div>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <div v-for="metric in metricCards" :key="metric.key" class="card p-4">
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ metric.label }}</p>
                <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white" :data-test="`metric-${metric.key}`">
                  {{ metric.value }}
                </p>
                <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">{{ metric.hint }}</p>
              </div>
              <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg" :class="metric.iconClass">
                <Icon :name="metric.icon" size="md" :stroke-width="2" />
              </div>
            </div>
          </div>
        </div>
      </section>

      <section class="card overflow-hidden">
        <div class="border-b border-gray-200 px-4 py-4 dark:border-dark-700 sm:px-5">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.ingressRisk.runtime.title') }}
          </h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
            {{ t('admin.ingressRisk.runtime.currentHint') }}
          </p>
        </div>
        <div class="grid grid-cols-1 divide-y divide-gray-200 dark:divide-dark-700 sm:grid-cols-2 sm:divide-x sm:divide-y-0 xl:grid-cols-3">
          <div v-for="indicator in runtimeIndicators" :key="indicator.key" class="min-w-0 px-4 py-4 sm:px-5">
            <div class="flex items-center gap-2 text-xs font-medium text-gray-500 dark:text-dark-400">
              <Icon :name="indicator.icon" size="sm" />
              {{ indicator.label }}
            </div>
            <div class="mt-2 truncate text-lg font-semibold text-gray-900 dark:text-white" :title="indicator.value">
              {{ indicator.value }}
            </div>
            <div class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">
              {{ indicator.detail }}
            </div>
          </div>
        </div>
      </section>

      <section class="card p-4 sm:p-5">
        <div class="mb-4 flex items-center gap-2">
          <Icon name="filter" size="sm" class="text-gray-500 dark:text-dark-400" />
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.ingressRisk.filters.title') }}
          </h3>
        </div>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-7">
          <div>
            <label class="input-label">{{ t('admin.ingressRisk.filters.timeRange') }}</label>
            <Select v-model="filters.time_range" :options="timeRangeOptions" :searchable="false" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.ingressRisk.filters.reason') }}</label>
            <Select v-model="filters.reason" :options="reasonOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.ingressRisk.filters.routeFamily') }}</label>
            <Select v-model="filters.route_family" :options="routeOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.ingressRisk.filters.protocol') }}</label>
            <Select v-model="filters.protocol" :options="protocolOptions" :searchable="false" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.ingressRisk.filters.clientIp') }}</label>
            <input
              v-model.trim="filters.client_ip"
              type="text"
              class="input font-mono"
              :placeholder="t('admin.ingressRisk.filters.clientIpPlaceholder')"
              @keyup.enter="search"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.ingressRisk.filters.userId') }}</label>
            <input v-model="filters.user_id" type="number" min="1" class="input" @keyup.enter="search" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.ingressRisk.filters.apiKeyId') }}</label>
            <input v-model="filters.api_key_id" type="number" min="1" class="input" @keyup.enter="search" />
          </div>
        </div>
        <div class="mt-4 flex flex-wrap justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="recordsLoading" @click="resetFilters">
            {{ t('admin.ingressRisk.actions.reset') }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="recordsLoading" data-test="search" @click="search">
            <Icon name="search" size="sm" />
            {{ t('admin.ingressRisk.actions.search') }}
          </button>
        </div>
      </section>

      <section class="card overflow-hidden">
        <div class="flex flex-col justify-between gap-2 border-b border-gray-200 px-4 py-4 dark:border-dark-700 sm:flex-row sm:items-end sm:px-5">
          <div>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.ingressRisk.table.title') }}
            </h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
              {{ t('admin.ingressRisk.table.summary', { total: formatNumber(total), requests: formatNumber(currentPageRequests) }) }}
            </p>
          </div>
          <span class="rounded-md bg-gray-100 px-2 py-1 font-mono text-xs text-gray-600 dark:bg-dark-800 dark:text-dark-300">
            {{ t(`admin.ingressRisk.timeRanges.${filters.time_range}`) }}
          </span>
        </div>

        <div
          v-if="recordsError"
          class="border-b border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300"
          role="alert"
        >
          {{ recordsError }}
        </div>

        <DataTable :columns="columns" :data="records" :loading="recordsLoading" row-key="id" :sticky-first-column="false">
          <template #cell-bucket_start="{ value }">
            <span class="whitespace-nowrap text-gray-600 dark:text-dark-300">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-reject_reason="{ value }">
            <span class="inline-flex rounded-md px-2 py-1 text-xs font-semibold" :class="reasonBadgeClass(value)" :title="value">
              {{ reasonLabel(value) }}
            </span>
          </template>

          <template #cell-route="{ row }">
            <div>
              <div class="font-medium text-gray-900 dark:text-white">{{ routeLabel(row.route_family) }}</div>
              <div class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ protocolLabel(row.protocol) }}</div>
            </div>
          </template>

          <template #cell-client_ip="{ value }">
            <div class="flex items-center gap-2">
              <span class="font-mono text-gray-700 dark:text-dark-200">{{ value }}</span>
              <button
                type="button"
                class="btn-ghost btn-icon h-7 w-7 shrink-0"
                :title="t('admin.ingressRisk.actions.filterIp', { ip: value })"
                :aria-label="t('admin.ingressRisk.actions.filterIp', { ip: value })"
                @click.stop="filterByIp(value)"
              >
                <Icon name="filter" size="xs" />
              </button>
            </div>
          </template>

          <template #cell-request_count="{ value }">
            <span class="font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatNumber(value) }}</span>
          </template>

          <template #cell-seen="{ row }">
            <div class="space-y-0.5 text-xs text-gray-500 dark:text-dark-400">
              <div>{{ t('admin.ingressRisk.table.first', { time: formatDateTime(row.first_seen) }) }}</div>
              <div>{{ t('admin.ingressRisk.table.last', { time: formatDateTime(row.last_seen) }) }}</div>
            </div>
          </template>

          <template #cell-subject="{ row }">
            <div v-if="row.user_id || row.api_key_id" class="space-y-0.5 text-xs">
              <div v-if="row.user_id">{{ t('admin.ingressRisk.table.user', { id: row.user_id }) }}</div>
              <div v-if="row.api_key_id">{{ t('admin.ingressRisk.table.apiKey', { id: row.api_key_id }) }}</div>
            </div>
            <span v-else class="text-gray-400">—</span>
          </template>

          <template #empty>
            <div class="flex flex-col items-center py-8">
              <Icon name="shield" size="xl" class="mb-3 h-10 w-10 text-gray-300 dark:text-dark-600" />
              <p class="text-sm font-medium text-gray-500 dark:text-dark-400">
                {{ t('admin.ingressRisk.table.empty') }}
              </p>
            </div>
          </template>
        </DataTable>

        <Pagination
          v-if="total > 0"
          :total="total"
          :page="page"
          :page-size="pageSize"
          @update:page="onPageChange"
          @update:page-size="onPageSizeChange"
        />
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import {
  ingressRiskAPI,
  type AuthCacheHealth,
  type IngressCollectorHealth,
  type IngressRejection,
  type IngressRejectionQuery,
  type IngressRiskTimeRange,
} from '@/api/admin/ingressRisk'
import { formatDateTime, formatNumber } from '@/utils/format'

const { t } = useI18n()

type HealthLevel = 'healthy' | 'warning' | 'critical' | 'unknown'
type DisplayIcon = 'key' | 'ban' | 'shield' | 'globe' | 'database' | 'server' | 'sync' | 'clock'

const REASONS = [
  'query_api_key_deprecated', 'api_key_required', 'invalid_api_key', 'invalid_auth_rate_limited',
  'api_key_auth_overloaded', 'api_key_disabled', 'ip_restricted', 'user_inactive', 'group_deleted',
  'group_disabled', 'group_not_allowed', 'group_unassigned', 'other',
] as const
const ROUTES = [
  'antigravity', 'gemini', 'codex', 'messages', 'responses', 'chat_completions',
  'images', 'videos', 'embeddings', 'models', 'other',
] as const
const PROTOCOLS = ['google', 'anthropic', 'openai', 'gateway', 'other'] as const
const TIME_RANGES: IngressRiskTimeRange[] = ['5m', '30m', '1h', '6h', '24h', '7d', '30d']

const filters = reactive({
  time_range: '1h' as IngressRiskTimeRange,
  reason: '',
  route_family: '',
  protocol: '',
  client_ip: '',
  user_id: '',
  api_key_id: '',
})

const records = ref<IngressRejection[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(25)
const recordsLoading = ref(false)
const healthLoading = ref(false)
const recordsError = ref('')
const healthError = ref('')
const collectorHealth = ref<IngressCollectorHealth | null>(null)
const authHealth = ref<AuthCacheHealth | null>(null)
const lastUpdated = ref<Date | null>(null)

const refreshing = computed(() => recordsLoading.value || healthLoading.value)
const currentPageRequests = computed(() => records.value.reduce((sum, row) => sum + row.request_count, 0))
const lastUpdatedLabel = computed(() => lastUpdated.value
  ? t('admin.ingressRisk.updatedAt', { time: formatDateTime(lastUpdated.value) })
  : t('admin.ingressRisk.neverUpdated'))

const timeRangeOptions = computed(() => TIME_RANGES.map((value) => ({
  value,
  label: t(`admin.ingressRisk.timeRanges.${value}`),
})))
const reasonOptions = computed(() => [
  { value: '', label: t('admin.ingressRisk.filters.all') },
  ...REASONS.map((value) => ({ value, label: reasonLabel(value) })),
])
const routeOptions = computed(() => [
  { value: '', label: t('admin.ingressRisk.filters.all') },
  ...ROUTES.map((value) => ({ value, label: routeLabel(value) })),
])
const protocolOptions = computed(() => [
  { value: '', label: t('admin.ingressRisk.filters.all') },
  ...PROTOCOLS.map((value) => ({ value, label: protocolLabel(value) })),
])

const columns = computed<Column[]>(() => [
  { key: 'bucket_start', label: t('admin.ingressRisk.table.bucket') },
  { key: 'reject_reason', label: t('admin.ingressRisk.table.reason') },
  { key: 'route', label: t('admin.ingressRisk.table.route') },
  { key: 'client_ip', label: t('admin.ingressRisk.table.clientIp') },
  { key: 'request_count', label: t('admin.ingressRisk.table.requests'), class: 'text-right' },
  { key: 'seen', label: t('admin.ingressRisk.table.seen') },
  { key: 'subject', label: t('admin.ingressRisk.table.subject') },
])

const overallHealth = computed<HealthLevel>(() => {
  const collector = collectorHealth.value
  const auth = authHealth.value
  if (!collector || !auth) return 'unknown'
  if (!collector.accepting || !auth.subscriber.connected || !auth.outbox.running) return 'critical'
  if (
    collector.flush_failure_count > 0 || collector.dropped_count > 0 || collector.overflowed_count > 0 ||
    collector.pending_batches > 0 || auth.lookup.rejected > 0 || auth.outbox.failures > 0 ||
    auth.outbox.pending > 0 || auth.invalid_abuse.overflowed > 0 || auth.invalid_abuse.global_blocked > 0
  ) return 'warning'
  return 'healthy'
})

const healthSignals = computed(() => {
  const collector = collectorHealth.value
  const auth = authHealth.value
  return [
    {
      key: 'collector',
      label: t('admin.ingressRisk.health.collector'),
      value: collector ? t(`admin.ingressRisk.health.${collector.accepting ? 'running' : 'stopped'}`) : '—',
      level: !collector ? 'unknown' : !collector.accepting ? 'critical' : collector.flush_failure_count > 0 || collector.dropped_count > 0 || collector.overflowed_count > 0 ? 'warning' : 'healthy',
    },
    {
      key: 'subscriber',
      label: t('admin.ingressRisk.health.subscriber'),
      value: auth ? t(`admin.ingressRisk.health.${auth.subscriber.connected ? 'connected' : 'disconnected'}`) : '—',
      level: !auth ? 'unknown' : auth.subscriber.connected ? auth.subscriber.failures > 0 ? 'warning' : 'healthy' : 'critical',
    },
    {
      key: 'lookup',
      label: t('admin.ingressRisk.health.lookup'),
      value: auth ? t('admin.ingressRisk.health.rejected', { count: formatNumber(auth.lookup.rejected) }) : '—',
      level: !auth ? 'unknown' : auth.lookup.rejected > 0 ? 'warning' : 'healthy',
    },
    {
      key: 'outbox',
      label: t('admin.ingressRisk.health.outbox'),
      value: auth ? t('admin.ingressRisk.health.pending', { count: formatNumber(auth.outbox.pending) }) : '—',
      level: !auth ? 'unknown' : !auth.outbox.running ? 'critical' : auth.outbox.pending > 0 || auth.outbox.failures > 0 ? 'warning' : 'healthy',
    },
  ] as Array<{ key: string; label: string; value: string; level: HealthLevel }>
})

const healthLastError = computed(() => collectorHealth.value?.last_error || authHealth.value?.outbox.last_error || authHealth.value?.outbox.stats_error || '')
const healthIcon = computed(() => overallHealth.value === 'healthy' ? 'checkCircle' : overallHealth.value === 'unknown' ? 'clock' : 'exclamationTriangle')
const healthBandClass = computed(() => ({
  healthy: 'border-emerald-200 bg-emerald-50/70 dark:border-emerald-900/60 dark:bg-emerald-950/20',
  warning: 'border-amber-200 bg-amber-50/70 dark:border-amber-900/60 dark:bg-amber-950/20',
  critical: 'border-red-200 bg-red-50/70 dark:border-red-900/60 dark:bg-red-950/20',
  unknown: 'border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-900/60',
})[overallHealth.value])
const healthIconClass = computed(() => ({
  healthy: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  warning: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300',
  critical: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300',
  unknown: 'bg-gray-200 text-gray-600 dark:bg-dark-700 dark:text-dark-300',
})[overallHealth.value])
const healthBadgeClass = computed(() => ({
  healthy: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  warning: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300',
  critical: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300',
  unknown: 'bg-gray-200 text-gray-600 dark:bg-dark-700 dark:text-dark-300',
})[overallHealth.value])

const metricCards = computed<Array<{ key: string; label: string; value: string; hint: string; icon: DisplayIcon; iconClass: string }>>(() => {
  const abuse = authHealth.value?.invalid_abuse
  const value = (count?: number) => abuse ? formatNumber(count ?? 0) : '—'
  return [
    { key: 'recorded', label: t('admin.ingressRisk.metrics.recorded'), value: value(abuse?.recorded), hint: t('admin.ingressRisk.metrics.recordedHint'), icon: 'key', iconClass: 'bg-blue-100 text-blue-700 dark:bg-blue-900/35 dark:text-blue-300' },
    { key: 'rejected', label: t('admin.ingressRisk.metrics.rejected'), value: value(abuse?.rejected), hint: t('admin.ingressRisk.metrics.rejectedHint'), icon: 'ban', iconClass: 'bg-red-100 text-red-700 dark:bg-red-900/35 dark:text-red-300' },
    { key: 'blocks', label: t('admin.ingressRisk.metrics.blocks'), value: value(abuse?.blocks), hint: t('admin.ingressRisk.metrics.blocksHint'), icon: 'shield', iconClass: 'bg-amber-100 text-amber-700 dark:bg-amber-900/35 dark:text-amber-300' },
    { key: 'global', label: t('admin.ingressRisk.metrics.globalBlocked'), value: value(abuse?.global_blocked), hint: t('admin.ingressRisk.metrics.globalBlockedHint'), icon: 'globe', iconClass: 'bg-violet-100 text-violet-700 dark:bg-violet-900/35 dark:text-violet-300' },
  ]
})

const runtimeIndicators = computed<Array<{ key: string; label: string; value: string; detail: string; icon: DisplayIcon }>>(() => {
  const collector = collectorHealth.value
  const auth = authHealth.value
  return [
    {
      key: 'tracked', icon: 'shield', label: t('admin.ingressRisk.runtime.tracked'),
      value: auth ? t('admin.ingressRisk.runtime.currentCapacity', { current: formatNumber(auth.invalid_abuse.tracked), capacity: formatNumber(auth.invalid_abuse.capacity) }) : '—',
      detail: auth ? t(`admin.ingressRisk.runtime.${auth.invalid_abuse.enabled ? 'enabled' : 'disabled'}`) : t('admin.ingressRisk.metrics.unavailable'),
    },
    {
      key: 'lookup', icon: 'key', label: t('admin.ingressRisk.runtime.lookup'),
      value: auth ? t('admin.ingressRisk.runtime.currentCapacity', { current: formatNumber(auth.lookup.in_flight), capacity: formatNumber(auth.lookup.capacity) }) : '—',
      detail: auth ? t('admin.ingressRisk.runtime.lookupTotals', { total: formatNumber(auth.lookup.total), rejected: formatNumber(auth.lookup.rejected) }) : t('admin.ingressRisk.metrics.unavailable'),
    },
    {
      key: 'collector', icon: 'database', label: t('admin.ingressRisk.runtime.collector'),
      value: collector ? t('admin.ingressRisk.runtime.currentCapacity', { current: formatNumber(collector.cardinality), capacity: formatNumber(collector.capacity) }) : '—',
      detail: collector ? t('admin.ingressRisk.runtime.collectorPending', { rows: formatNumber(collector.pending_rows), batches: formatNumber(collector.pending_batches) }) : t('admin.ingressRisk.metrics.unavailable'),
    },
    {
      key: 'delivery', icon: 'server', label: t('admin.ingressRisk.runtime.delivery'),
      value: collector ? formatNumber(collector.flushed_request_count) : '—',
      detail: collector ? t('admin.ingressRisk.runtime.deliveryTotals', { dropped: formatNumber(collector.dropped_count), overflowed: formatNumber(collector.overflowed_count), failed: formatNumber(collector.flush_failure_count) }) : t('admin.ingressRisk.metrics.unavailable'),
    },
    {
      key: 'subscriber', icon: 'sync', label: t('admin.ingressRisk.runtime.subscriber'),
      value: auth ? t(`admin.ingressRisk.health.${auth.subscriber.connected ? 'connected' : 'disconnected'}`) : '—',
      detail: auth ? t('admin.ingressRisk.runtime.subscriberFailures', { count: formatNumber(auth.subscriber.failures) }) : t('admin.ingressRisk.metrics.unavailable'),
    },
    {
      key: 'outbox', icon: 'clock', label: t('admin.ingressRisk.runtime.outbox'),
      value: auth ? formatNumber(auth.outbox.pending) : '—',
      detail: auth ? t('admin.ingressRisk.runtime.outboxTotals', { processed: formatNumber(auth.outbox.processed), failures: formatNumber(auth.outbox.failures) }) : t('admin.ingressRisk.metrics.unavailable'),
    },
  ]
})

function signalDotClass(level: HealthLevel) {
  return {
    healthy: 'bg-emerald-500', warning: 'bg-amber-500', critical: 'bg-red-500', unknown: 'bg-gray-400',
  }[level]
}

function reasonLabel(reason: string) {
  return t(`admin.ingressRisk.reasons.${reason}`)
}

function routeLabel(route: string) {
  return t(`admin.ingressRisk.routes.${route}`)
}

function protocolLabel(protocol: string) {
  return t(`admin.ingressRisk.protocols.${protocol}`)
}

function reasonBadgeClass(reason: string) {
  if (['invalid_api_key', 'invalid_auth_rate_limited', 'api_key_auth_overloaded'].includes(reason)) {
    return 'bg-red-100 text-red-700 dark:bg-red-900/35 dark:text-red-300'
  }
  if (['query_api_key_deprecated', 'api_key_required', 'api_key_disabled', 'ip_restricted'].includes(reason)) {
    return 'bg-amber-100 text-amber-700 dark:bg-amber-900/35 dark:text-amber-300'
  }
  return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-dark-200'
}

function positiveNumber(value: string): number | undefined {
  const parsed = Number.parseInt(value, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined
}

function buildQuery(): IngressRejectionQuery {
  return {
    time_range: filters.time_range,
    reason: filters.reason || undefined,
    route_family: filters.route_family || undefined,
    protocol: filters.protocol || undefined,
    client_ip: filters.client_ip || undefined,
    user_id: positiveNumber(filters.user_id),
    api_key_id: positiveNumber(filters.api_key_id),
    page: page.value,
    page_size: pageSize.value,
  }
}

function errorMessage(error: unknown, fallback: string) {
  if (typeof error === 'object' && error && 'message' in error && typeof error.message === 'string') {
    return error.message
  }
  return fallback
}

async function loadRecords() {
  recordsLoading.value = true
  recordsError.value = ''
  try {
    const result = await ingressRiskAPI.listIngressRejections(buildQuery())
    records.value = result.items ?? []
    total.value = result.total ?? 0
    lastUpdated.value = new Date()
  } catch (error) {
    recordsError.value = errorMessage(error, t('admin.ingressRisk.errors.records'))
  } finally {
    recordsLoading.value = false
  }
}

async function loadHealth() {
  healthLoading.value = true
  healthError.value = ''
  const [collectorResult, authResult] = await Promise.allSettled([
    ingressRiskAPI.getIngressCollectorHealth(),
    ingressRiskAPI.getAuthCacheHealth(),
  ])
  if (collectorResult.status === 'fulfilled') collectorHealth.value = collectorResult.value
  if (authResult.status === 'fulfilled') authHealth.value = authResult.value
  if (collectorResult.status === 'rejected' || authResult.status === 'rejected') {
    const failure = collectorResult.status === 'rejected' ? collectorResult.reason : authResult.status === 'rejected' ? authResult.reason : null
    healthError.value = errorMessage(failure, t('admin.ingressRisk.errors.health'))
  } else {
    lastUpdated.value = new Date()
  }
  healthLoading.value = false
}

async function refreshAll() {
  await Promise.all([loadRecords(), loadHealth()])
}

function search() {
  page.value = 1
  void loadRecords()
}

function resetFilters() {
  filters.time_range = '1h'
  filters.reason = ''
  filters.route_family = ''
  filters.protocol = ''
  filters.client_ip = ''
  filters.user_id = ''
  filters.api_key_id = ''
  page.value = 1
  void loadRecords()
}

function filterByIp(ip: string) {
  filters.client_ip = ip
  page.value = 1
  void loadRecords()
}

function onPageChange(nextPage: number) {
  page.value = nextPage
  void loadRecords()
}

function onPageSizeChange(nextPageSize: number) {
  pageSize.value = nextPageSize
  page.value = 1
  void loadRecords()
}

onMounted(() => {
  void refreshAll()
})
</script>
