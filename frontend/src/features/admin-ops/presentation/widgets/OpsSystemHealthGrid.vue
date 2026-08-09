<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import HelpTooltip from '@/common/widgets/feedback/HelpTooltip.vue'
import type { OpsDashboardOverview } from '@/features/admin-ops/data/dtos/opsDashboardDtos'
import { formatBytes } from '@/core/utils/format'
import {
  formatCompactNumber,
  formatDurationMs,
  formatExactDurationMs,
  formatExactNumber,
} from '../opsFormatter'

interface Props {
  overview: OpsDashboardOverview
  fullscreen?: boolean
}

const props = defineProps<Props>()
const { t } = useI18n()
const overview = computed(() => props.overview)
const systemMetrics = computed(() => overview.value.system_metrics ?? null)

// --- System health (secondary) ---

function formatTimeShort(ts?: string | null): string {
  if (!ts) return '-'
  const d = new Date(ts)
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleTimeString()
}

const cpuPercentValue = computed<number | null>(() => {
  const v = systemMetrics.value?.cpu_usage_percent
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const cpuPercentClass = computed(() => {
  const v = cpuPercentValue.value
  if (v == null) return 'text-gray-900 dark:text-white'
  if (v >= 95) return 'text-rose-600 dark:text-rose-400'
  if (v >= 80) return 'text-yellow-600 dark:text-yellow-400'
  return 'text-emerald-600 dark:text-emerald-400'
})

const memPercentValue = computed<number | null>(() => {
  const v = systemMetrics.value?.memory_usage_percent
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const memPercentClass = computed(() => {
  const v = memPercentValue.value
  if (v == null) return 'text-gray-900 dark:text-white'
  if (v >= 95) return 'text-rose-600 dark:text-rose-400'
  if (v >= 85) return 'text-yellow-600 dark:text-yellow-400'
  return 'text-emerald-600 dark:text-emerald-400'
})

const dbConnActiveValue = computed<number | null>(() => {
  const v = systemMetrics.value?.db_conn_active
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const dbConnIdleValue = computed<number | null>(() => {
  const v = systemMetrics.value?.db_conn_idle
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const dbConnWaitingValue = computed<number | null>(() => {
  const v = systemMetrics.value?.db_conn_waiting
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const dbConnOpenValue = computed<number | null>(() => {
  if (dbConnActiveValue.value == null || dbConnIdleValue.value == null) return null
  return dbConnActiveValue.value + dbConnIdleValue.value
})

const dbMaxOpenConnsValue = computed<number | null>(() => {
  const v = systemMetrics.value?.db_max_open_conns
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const dbUsagePercent = computed<number | null>(() => {
  if (dbConnOpenValue.value == null || dbMaxOpenConnsValue.value == null || dbMaxOpenConnsValue.value <= 0) return null
  return Math.min(100, Math.max(0, (dbConnOpenValue.value / dbMaxOpenConnsValue.value) * 100))
})

const dbMiddleLabel = computed(() => {
  if (systemMetrics.value?.db_ok === false) return 'FAIL'
  if (dbUsagePercent.value != null) return `${dbUsagePercent.value.toFixed(0)}%`
  if (systemMetrics.value?.db_ok === true) return t('admin.ops.ok')
  return t('admin.ops.noData')
})

const dbMiddleClass = computed(() => {
  if (systemMetrics.value?.db_ok === false) return 'text-rose-600 dark:text-rose-400'
  if (dbUsagePercent.value != null) {
    if (dbUsagePercent.value >= 90) return 'text-rose-600 dark:text-rose-400'
    if (dbUsagePercent.value >= 70) return 'text-yellow-600 dark:text-yellow-400'
    return 'text-emerald-600 dark:text-emerald-400'
  }
  if (systemMetrics.value?.db_ok === true) return 'text-emerald-600 dark:text-emerald-400'
  return 'text-gray-900 dark:text-white'
})

const redisConnTotalValue = computed<number | null>(() => {
  const v = systemMetrics.value?.redis_conn_total
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const redisConnIdleValue = computed<number | null>(() => {
  const v = systemMetrics.value?.redis_conn_idle
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const redisConnActiveValue = computed<number | null>(() => {
  if (redisConnTotalValue.value == null || redisConnIdleValue.value == null) return null
  return Math.max(redisConnTotalValue.value - redisConnIdleValue.value, 0)
})

const redisPoolSizeValue = computed<number | null>(() => {
  const v = systemMetrics.value?.redis_pool_size
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const redisUsagePercent = computed<number | null>(() => {
  if (
    redisConnActiveValue.value == null ||
    redisPoolSizeValue.value == null ||
    redisPoolSizeValue.value <= 0
  )
    return null
  return Math.min(100, Math.max(0, (redisConnActiveValue.value / redisPoolSizeValue.value) * 100))
})

const redisHealthValue = computed<boolean | null>(() => {
  const v = systemMetrics.value?.redis_ok
  return typeof v === 'boolean' ? v : null
})

const redisMiddleLabel = computed(() => {
  if (redisHealthValue.value == null) return t('admin.ops.noData')
  if (!redisHealthValue.value) return 'FAIL'
  if (redisUsagePercent.value != null) return `${redisUsagePercent.value.toFixed(0)}%`
  return t('admin.ops.ok')
})

const redisMiddleClass = computed(() => {
  if (redisHealthValue.value == null) return 'text-gray-900 dark:text-white'
  if (!redisHealthValue.value) return 'text-rose-600 dark:text-rose-400'
  if (redisUsagePercent.value != null) {
    if (redisUsagePercent.value >= 90) return 'text-rose-600 dark:text-rose-400'
    if (redisUsagePercent.value >= 70) return 'text-yellow-600 dark:text-yellow-400'
    return 'text-emerald-600 dark:text-emerald-400'
  }
  return 'text-emerald-600 dark:text-emerald-400'
})

const goroutineCountValue = computed<number | null>(() => {
  const v = systemMetrics.value?.goroutine_count
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const goroutinesWarnThreshold = 30_000
const goroutinesCriticalThreshold = 50_000

const goroutineStatus = computed<'ok' | 'warning' | 'critical' | 'unknown'>(() => {
  const n = goroutineCountValue.value
  if (n == null) return 'unknown'
  if (n >= goroutinesCriticalThreshold) return 'critical'
  if (n >= goroutinesWarnThreshold) return 'warning'
  return 'ok'
})

const goroutineStatusLabel = computed(() => {
  switch (goroutineStatus.value) {
    case 'ok':
      return t('admin.ops.ok')
    case 'warning':
      return t('common.warning')
    case 'critical':
      return t('common.critical')
    default:
      return t('admin.ops.noData')
  }
})

const goroutineStatusClass = computed(() => {
  switch (goroutineStatus.value) {
    case 'ok':
      return 'text-emerald-600 dark:text-emerald-400'
    case 'warning':
      return 'text-yellow-600 dark:text-yellow-400'
    case 'critical':
      return 'text-rose-600 dark:text-rose-400'
    default:
      return 'text-gray-900 dark:text-white'
  }
})

const jobHeartbeats = computed(() => overview.value?.job_heartbeats ?? [])

const jobsStatus = computed<'ok' | 'warn' | 'unknown'>(() => {
  const list = jobHeartbeats.value
  if (!list.length) return 'unknown'
  for (const hb of list) {
    if (!hb) continue
    if (hb.last_error_at && (!hb.last_success_at || hb.last_error_at > hb.last_success_at)) return 'warn'
  }
  return 'ok'
})

const jobsWarnCount = computed(() => {
  let warn = 0
  for (const hb of jobHeartbeats.value) {
    if (!hb) continue
    if (hb.last_error_at && (!hb.last_success_at || hb.last_error_at > hb.last_success_at)) warn++
  }
  return warn
})

const jobsStatusLabel = computed(() => {
  switch (jobsStatus.value) {
    case 'ok':
      return t('admin.ops.ok')
    case 'warn':
      return t('common.warning')
    default:
      return t('admin.ops.noData')
  }
})

const jobsStatusClass = computed(() => {
  switch (jobsStatus.value) {
    case 'ok':
      return 'text-emerald-600 dark:text-emerald-400'
    case 'warn':
      return 'text-yellow-600 dark:text-yellow-400'
    default:
      return 'text-gray-900 dark:text-white'
  }
})

const showJobsDetails = ref(false)

function openJobsDetails() {
  showJobsDetails.value = true
}
</script>

<template>
<div v-if="overview" class="mt-2 border-t border-gray-100 pt-4 dark:border-dark-700">
  <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
    <!-- CPU -->
    <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
      <div class="flex items-center gap-1">
        <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">CPU</div>
        <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.cpu')" />
      </div>
      <div class="mt-1 text-lg font-black" :class="cpuPercentClass">
        {{ cpuPercentValue == null ? '-' : `${cpuPercentValue.toFixed(1)}%` }}
      </div>
      <div v-if="!props.fullscreen" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
        {{ t('common.warning') }} 80% · {{ t('common.critical') }} 95%
      </div>
    </div>

    <!-- MEM -->
    <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
      <div class="flex items-center gap-1">
        <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.memory') }}</div>
        <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.memory')" />
      </div>
      <div class="mt-1 text-lg font-black" :class="memPercentClass">
        {{ memPercentValue == null ? '-' : `${memPercentValue.toFixed(1)}%` }}
      </div>
      <div v-if="!props.fullscreen" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
        {{
          systemMetrics?.memory_used_mb == null || systemMetrics?.memory_total_mb == null
            ? '-'
            : `${formatBytes(systemMetrics.memory_used_mb * 1024 * 1024, 1)} / ${formatBytes(systemMetrics.memory_total_mb * 1024 * 1024, 1)}`
        }}
      </div>
    </div>

    <!-- DB -->
    <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
      <div class="flex items-center gap-1">
        <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.db') }}</div>
        <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.db')" />
      </div>
      <div class="mt-1 text-lg font-black" :class="dbMiddleClass">
        {{ dbMiddleLabel }}
      </div>
      <div v-if="!props.fullscreen" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.conns') }} {{ formatCompactNumber(dbConnOpenValue) }} / {{ formatCompactNumber(dbMaxOpenConnsValue) }}
        · {{ t('admin.ops.active') }} {{ formatCompactNumber(dbConnActiveValue) }}
        · {{ t('admin.ops.idle') }} {{ formatCompactNumber(dbConnIdleValue) }}
        <span v-if="dbConnWaitingValue != null"> · {{ t('admin.ops.waiting') }} {{ formatCompactNumber(dbConnWaitingValue) }} </span>
      </div>
    </div>

    <!-- Redis -->
    <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
      <div class="flex items-center gap-1">
        <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">Redis</div>
        <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.redis')" />
      </div>
      <div class="mt-1 text-lg font-black" :class="redisMiddleClass">
        {{ redisMiddleLabel }}
      </div>
      <div v-if="!props.fullscreen" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.conns') }} {{ formatCompactNumber(redisConnTotalValue) }} / {{ formatCompactNumber(redisPoolSizeValue) }}
        <span v-if="redisConnActiveValue != null"> · {{ t('admin.ops.active') }} {{ formatCompactNumber(redisConnActiveValue) }} </span>
        <span v-if="redisConnIdleValue != null"> · {{ t('admin.ops.idle') }} {{ formatCompactNumber(redisConnIdleValue) }} </span>
      </div>
    </div>

    <!-- Goroutines -->
    <div data-test="goroutine-card" class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
      <div class="flex items-center gap-1">
        <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.goroutines') }}</div>
        <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.goroutines')" />
      </div>
      <div data-test="goroutine-status" class="mt-1 text-lg font-black" :class="goroutineStatusClass">
        {{ goroutineStatusLabel }}
      </div>
      <div v-if="!props.fullscreen" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.current') }} <span class="font-mono">{{ formatCompactNumber(goroutineCountValue) }}</span>
        · {{ t('common.warning') }} <span class="font-mono">{{ formatCompactNumber(goroutinesWarnThreshold) }}</span>
        · {{ t('common.critical') }} <span class="font-mono">{{ formatCompactNumber(goroutinesCriticalThreshold) }}</span>
        <span v-if="systemMetrics?.concurrency_queue_depth != null">
          · {{ t('admin.ops.queue') }} <span class="font-mono">{{ formatCompactNumber(systemMetrics.concurrency_queue_depth) }}</span>
        </span>
      </div>
    </div>

    <!-- Jobs -->
    <div class="rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center gap-1">
          <div class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.jobs') }}</div>
          <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.jobs')" />
        </div>
        <button v-if="!props.fullscreen" class="text-[10px] font-bold text-blue-500 hover:underline" type="button" @click="openJobsDetails">
          {{ t('admin.ops.requestDetails.details') }}
        </button>
      </div>

      <div class="mt-1 text-lg font-black" :class="jobsStatusClass">
        {{ jobsStatusLabel }}
      </div>

      <div v-if="!props.fullscreen" class="mt-1 text-[10px] text-gray-500 dark:text-gray-400">
        {{ t('common.total') }}
        <span class="font-mono tabular-nums" :title="formatExactNumber(jobHeartbeats.length)">{{ formatCompactNumber(jobHeartbeats.length) }}</span>
        · {{ t('common.warning') }}
        <span class="font-mono tabular-nums" :title="formatExactNumber(jobsWarnCount)">{{ formatCompactNumber(jobsWarnCount) }}</span>
      </div>
    </div>
  </div>
</div>

<BaseDialog :show="showJobsDetails" :title="t('admin.ops.jobs')" width="wide" @close="showJobsDetails = false">
  <div v-if="!jobHeartbeats.length" class="text-sm text-gray-500 dark:text-gray-400">
    {{ t('admin.ops.noData') }}
  </div>
  <div v-else class="space-y-3">
    <div
      v-for="hb in jobHeartbeats"
      :key="hb.job_name"
      class="rounded-xl border border-gray-100 bg-white p-4 dark:border-dark-700 dark:bg-dark-900"
    >
      <div class="flex items-center justify-between gap-3">
        <div class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ hb.job_name }}</div>
        <div class="flex items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
          <span
            v-if="hb.last_duration_ms != null"
            class="font-mono tabular-nums"
            :title="formatExactDurationMs(hb.last_duration_ms)"
          >{{ formatDurationMs(hb.last_duration_ms) }}</span>
          <span>{{ formatTimeShort(hb.updated_at) }}</span>
        </div>
      </div>

      <div class="mt-2 grid grid-cols-1 gap-2 text-xs text-gray-600 dark:text-gray-300 sm:grid-cols-2">
        <div>
          {{ t('admin.ops.lastSuccess') }} <span class="font-mono">{{ formatTimeShort(hb.last_success_at) }}</span>
        </div>
        <div>
          {{ t('admin.ops.lastError') }} <span class="font-mono">{{ formatTimeShort(hb.last_error_at) }}</span>
        </div>
        <div>
          {{ t('admin.ops.result') }} <span class="font-mono">{{ hb.last_result || '-' }}</span>
        </div>
      </div>

      <div
        v-if="hb.last_error"
        class="mt-3 rounded-lg bg-rose-50 p-2 text-xs text-rose-700 dark:bg-rose-900/20 dark:text-rose-300"
      >
        {{ hb.last_error }}
      </div>
    </div>
  </div>
</BaseDialog>
</template>
