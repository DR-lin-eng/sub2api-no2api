<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/common/widgets/feedback/HelpTooltip.vue'
import type { OpsMetricThresholds } from '@/features/admin-ops/data/dtos/opsSettingsDtos'
import type { OpsDashboardOverview } from '@/features/admin-ops/data/dtos/opsDashboardDtos'
import type { OpsRequestDetailsPreset } from '../opsTypeSignals'
import {
  formatCompactNumber,
  formatDurationMs,
  formatExactDurationMs,
  formatExactNumber,
} from '../opsFormatter'

interface Props {
  overview: OpsDashboardOverview
  thresholds?: OpsMetricThresholds | null
  fullscreen?: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (event: 'openRequestDetails', preset?: OpsRequestDetailsPreset): void
  (event: 'openErrorDetails', kind: 'request' | 'upstream'): void
}>()
const { t } = useI18n()
const overview = computed(() => props.overview)

function openDetails(preset?: OpsRequestDetailsPreset) {
  emit('openRequestDetails', preset)
}

function openErrorDetails(kind: 'request' | 'upstream') {
  emit('openErrorDetails', kind)
}

// --- Threshold checking helpers ---
type ThresholdLevel = 'normal' | 'warning' | 'critical'

function getSLAThresholdLevel(slaPercent: number | null): ThresholdLevel {
  if (slaPercent == null) return 'normal'
  const threshold = props.thresholds?.sla_percent_min
  if (threshold == null) return 'normal'

  // SLA is "higher is better":
  // - below threshold => critical
  // - within +0.1% buffer => warning
  const warningBuffer = 0.1

  if (slaPercent < threshold) return 'critical'
  if (slaPercent < threshold + warningBuffer) return 'warning'
  return 'normal'
}

function getTTFTThresholdLevel(ttftMs: number | null): ThresholdLevel {
  if (ttftMs == null) return 'normal'
  const threshold = props.thresholds?.ttft_p99_ms_max
  if (threshold == null) return 'normal'
  if (ttftMs >= threshold) return 'critical'
  if (ttftMs >= threshold * 0.8) return 'warning'
  return 'normal'
}

function getRequestErrorRateThresholdLevel(errorRatePercent: number | null): ThresholdLevel {
  if (errorRatePercent == null) return 'normal'
  const threshold = props.thresholds?.request_error_rate_percent_max
  if (threshold == null) return 'normal'
  if (errorRatePercent >= threshold) return 'critical'
  if (errorRatePercent >= threshold * 0.8) return 'warning'
  return 'normal'
}

function getUpstreamErrorRateThresholdLevel(upstreamErrorRatePercent: number | null): ThresholdLevel {
  if (upstreamErrorRatePercent == null) return 'normal'
  const threshold = props.thresholds?.upstream_error_rate_percent_max
  if (threshold == null) return 'normal'
  if (upstreamErrorRatePercent >= threshold) return 'critical'
  if (upstreamErrorRatePercent >= threshold * 0.8) return 'warning'
  return 'normal'
}

function getThresholdColorClass(level: ThresholdLevel): string {
  switch (level) {
    case 'critical':
      return 'text-red-600 dark:text-red-400'
    case 'warning':
      return 'text-yellow-600 dark:text-yellow-400'
    default:
      return 'text-green-600 dark:text-green-400'
  }
}

const totalRequestsLabel = computed(() => formatCompactNumber(overview.value.request_count_total ?? 0))
const totalTokensLabel = computed(() => formatCompactNumber(overview.value.token_consumed ?? 0))
const qpsAvgLabel = computed(() => formatCompactNumber(overview.value.qps?.avg))
const tpsAvgLabel = computed(() => formatCompactNumber(overview.value.tps?.avg))

const slaPercent = computed(() => {
  const value = overview.value.sla
  if (typeof value !== 'number' || (overview.value.request_count_sla ?? 0) <= 0) return null
  return value * 100
})
const errorRatePercent = computed(() => {
  const value = overview.value.error_rate
  return typeof value === 'number' ? value * 100 : null
})
const upstreamErrorRatePercent = computed(() => {
  const value = overview.value.upstream_error_rate
  return typeof value === 'number' ? value * 100 : null
})

const durationP99Ms = computed(() => overview.value.duration?.p99_ms ?? null)
const durationP95Ms = computed(() => overview.value.duration?.p95_ms ?? null)
const durationP90Ms = computed(() => overview.value.duration?.p90_ms ?? null)
const durationP50Ms = computed(() => overview.value.duration?.p50_ms ?? null)
const durationAvgMs = computed(() => overview.value.duration?.avg_ms ?? null)
const durationMaxMs = computed(() => overview.value.duration?.max_ms ?? null)

const ttftP99Ms = computed(() => overview.value.ttft?.p99_ms ?? null)
const ttftP95Ms = computed(() => overview.value.ttft?.p95_ms ?? null)
const ttftP90Ms = computed(() => overview.value.ttft?.p90_ms ?? null)
const ttftP50Ms = computed(() => overview.value.ttft?.p50_ms ?? null)
const ttftAvgMs = computed(() => overview.value.ttft?.avg_ms ?? null)
const ttftMaxMs = computed(() => overview.value.ttft?.max_ms ?? null)
</script>

<template>
<div class="grid h-full grid-cols-1 content-center gap-4 sm:grid-cols-2 lg:col-span-7 lg:grid-cols-3">
    <!-- Card 1: Requests -->
    <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900" style="order: 1;">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-1">
          <span class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.requestsTitle') }}</span>
          <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.totalRequests')" />
        </div>
        <button
          v-if="!props.fullscreen"
          class="text-[10px] font-bold text-blue-500 hover:underline"
          type="button"
          @click="openDetails({ title: t('admin.ops.requestDetails.title') })"
        >
          {{ t('admin.ops.requestDetails.details') }}
        </button>
      </div>
      <div class="mt-2 space-y-2 text-xs">
        <div class="flex justify-between">
          <span class="text-gray-500">{{ t('admin.ops.requests') }}:</span>
          <span class="tabular-nums font-bold text-gray-900 dark:text-white" :title="formatExactNumber(overview.request_count_total)">{{ totalRequestsLabel }}</span>
        </div>
        <div class="flex justify-between">
          <span class="text-gray-500">{{ t('admin.ops.tokens') }}:</span>
          <span class="tabular-nums font-bold text-gray-900 dark:text-white" :title="formatExactNumber(overview.token_consumed)">{{ totalTokensLabel }}</span>
        </div>
        <div class="flex justify-between">
          <span class="text-gray-500">{{ t('admin.ops.avgQps') }}:</span>
          <span class="tabular-nums font-bold text-gray-900 dark:text-white" :title="formatExactNumber(overview.qps?.avg)">{{ qpsAvgLabel }}</span>
        </div>
        <div class="flex justify-between">
          <span class="text-gray-500">{{ t('admin.ops.avgTps') }}:</span>
          <span class="tabular-nums font-bold text-gray-900 dark:text-white" :title="formatExactNumber(overview.tps?.avg)">{{ tpsAvgLabel }}</span>
        </div>
      </div>
    </div>

    <!-- Card 2: SLA -->
    <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900" style="order: 2;">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2">
          <span class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.sla') }}</span>
          <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.sla')" />
          <span class="h-1.5 w-1.5 rounded-full" :class="getSLAThresholdLevel(slaPercent) === 'critical' ? 'bg-red-500' : getSLAThresholdLevel(slaPercent) === 'warning' ? 'bg-yellow-500' : 'bg-green-500'"></span>
        </div>
        <button
          v-if="!props.fullscreen"
          class="text-[10px] font-bold text-blue-500 hover:underline"
          type="button"
          @click="openDetails({ title: t('admin.ops.requestDetails.title'), kind: 'error' })"
        >
          {{ t('admin.ops.requestDetails.details') }}
        </button>
      </div>
      <div class="mt-2 text-3xl font-black" :class="getThresholdColorClass(getSLAThresholdLevel(slaPercent))">
        {{ slaPercent == null ? '-' : `${slaPercent.toFixed(3)}%` }}
      </div>
      <div class="mt-3 h-2 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
        <div class="h-full transition-all" :class="getSLAThresholdLevel(slaPercent) === 'critical' ? 'bg-red-500' : getSLAThresholdLevel(slaPercent) === 'warning' ? 'bg-yellow-500' : 'bg-green-500'" :style="{ width: `${Math.max((slaPercent ?? 0) - 90, 0) * 10}%` }"></div>
      </div>
      <div class="mt-3 text-xs">
        <div class="flex justify-between">
          <span class="text-gray-500">{{ t('admin.ops.exceptions') }}:</span>
          <span class="tabular-nums font-bold text-gray-900 dark:text-white">{{ formatCompactNumber((overview.request_count_sla ?? 0) - (overview.success_count ?? 0)) }}</span>
        </div>
      </div>
    </div>

    <!-- Card 4: Request Duration -->
    <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900" style="order: 4;">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-1">
          <span class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.latencyDuration') }}</span>
          <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.latency')" />
        </div>
        <button
          v-if="!props.fullscreen"
          class="text-[10px] font-bold text-blue-500 hover:underline"
          type="button"
          @click="openDetails({ title: t('admin.ops.latencyDuration'), sort: 'duration_desc' })"
        >
          {{ t('admin.ops.requestDetails.details') }}
        </button>
      </div>
      <div class="mt-2 flex items-baseline gap-2">
        <div class="min-w-0 tabular-nums text-3xl font-black text-gray-900 dark:text-white" :title="formatExactDurationMs(durationP99Ms)">
          {{ formatDurationMs(durationP99Ms) }}
        </div>
        <span class="text-xs font-bold text-gray-400">P99</span>
      </div>
      <div class="mt-3 grid grid-cols-1 gap-x-3 gap-y-1 text-xs 2xl:grid-cols-2">
        <div class="flex items-baseline gap-1 whitespace-nowrap">
          <span class="text-gray-500">P95:</span>
          <span class="tabular-nums font-bold text-gray-900 dark:text-white" :title="formatExactDurationMs(durationP95Ms)">{{ formatDurationMs(durationP95Ms) }}</span>
        </div>
        <div class="flex items-baseline gap-1 whitespace-nowrap">
          <span class="text-gray-500">P90:</span>
          <span class="tabular-nums font-bold text-gray-900 dark:text-white" :title="formatExactDurationMs(durationP90Ms)">{{ formatDurationMs(durationP90Ms) }}</span>
        </div>
        <div class="flex items-baseline gap-1 whitespace-nowrap">
          <span class="text-gray-500">P50:</span>
          <span class="tabular-nums font-bold text-gray-900 dark:text-white" :title="formatExactDurationMs(durationP50Ms)">{{ formatDurationMs(durationP50Ms) }}</span>
        </div>
        <div class="flex items-baseline gap-1 whitespace-nowrap">
          <span class="text-gray-500">Avg:</span>
          <span class="tabular-nums font-bold text-gray-900 dark:text-white" :title="formatExactDurationMs(durationAvgMs)">{{ formatDurationMs(durationAvgMs) }}</span>
        </div>
        <div class="flex items-baseline gap-1 whitespace-nowrap">
          <span class="text-gray-500">Max:</span>
          <span class="tabular-nums font-bold text-gray-900 dark:text-white" :title="formatExactDurationMs(durationMaxMs)">{{ formatDurationMs(durationMaxMs) }}</span>
        </div>
      </div>
    </div>

    <!-- Card 5: TTFT -->
    <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900" style="order: 5;">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-1">
          <span class="text-[10px] font-bold uppercase text-gray-400">TTFT</span>
          <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.ttft')" />
        </div>
        <button
          v-if="!props.fullscreen"
          class="text-[10px] font-bold text-blue-500 hover:underline"
          type="button"
          @click="openDetails({ title: t('admin.ops.ttftLabel'), kind: 'success', sort: 'ttft_desc', ttft_only: true })"
        >
          {{ t('admin.ops.requestDetails.details') }}
        </button>
      </div>
      <div class="mt-2 flex items-baseline gap-2">
        <div
          class="min-w-0 tabular-nums text-3xl font-black"
          :class="getThresholdColorClass(getTTFTThresholdLevel(ttftP99Ms))"
          :title="formatExactDurationMs(ttftP99Ms)"
        >
          {{ formatDurationMs(ttftP99Ms) }}
        </div>
        <span class="text-xs font-bold text-gray-400">P99</span>
      </div>
      <div class="mt-3 grid grid-cols-1 gap-x-3 gap-y-1 text-xs 2xl:grid-cols-2">
        <div class="flex items-baseline gap-1 whitespace-nowrap">
          <span class="text-gray-500">P95:</span>
          <span class="tabular-nums font-bold" :class="getThresholdColorClass(getTTFTThresholdLevel(ttftP95Ms))" :title="formatExactDurationMs(ttftP95Ms)">{{ formatDurationMs(ttftP95Ms) }}</span>
        </div>
        <div class="flex items-baseline gap-1 whitespace-nowrap">
          <span class="text-gray-500">P90:</span>
          <span class="tabular-nums font-bold" :class="getThresholdColorClass(getTTFTThresholdLevel(ttftP90Ms))" :title="formatExactDurationMs(ttftP90Ms)">{{ formatDurationMs(ttftP90Ms) }}</span>
        </div>
        <div class="flex items-baseline gap-1 whitespace-nowrap">
          <span class="text-gray-500">P50:</span>
          <span class="tabular-nums font-bold" :class="getThresholdColorClass(getTTFTThresholdLevel(ttftP50Ms))" :title="formatExactDurationMs(ttftP50Ms)">{{ formatDurationMs(ttftP50Ms) }}</span>
        </div>
        <div class="flex items-baseline gap-1 whitespace-nowrap">
          <span class="text-gray-500">Avg:</span>
          <span class="tabular-nums font-bold" :class="getThresholdColorClass(getTTFTThresholdLevel(ttftAvgMs))" :title="formatExactDurationMs(ttftAvgMs)">{{ formatDurationMs(ttftAvgMs) }}</span>
        </div>
        <div class="flex items-baseline gap-1 whitespace-nowrap">
          <span class="text-gray-500">Max:</span>
          <span class="tabular-nums font-bold" :class="getThresholdColorClass(getTTFTThresholdLevel(ttftMaxMs))" :title="formatExactDurationMs(ttftMaxMs)">{{ formatDurationMs(ttftMaxMs) }}</span>
        </div>
      </div>
    </div>

    <!-- Card 3: Request Errors -->
    <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900" style="order: 3;">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-1">
          <span class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.requestErrors') }}</span>
          <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.errors')" />
        </div>
        <button v-if="!props.fullscreen" class="text-[10px] font-bold text-blue-500 hover:underline" type="button" @click="openErrorDetails('request')">
          {{ t('admin.ops.requestDetails.details') }}
        </button>
      </div>
      <div class="mt-2 text-3xl font-black" :class="getThresholdColorClass(getRequestErrorRateThresholdLevel(errorRatePercent))">
        {{ errorRatePercent == null ? '-' : `${errorRatePercent.toFixed(2)}%` }}
      </div>
      <div class="mt-3 space-y-1 text-xs">
        <div class="flex justify-between">
          <span class="text-gray-500">{{ t('admin.ops.errorCount') }}:</span>
          <span class="tabular-nums font-bold text-gray-900 dark:text-white">{{ formatCompactNumber(overview.error_count_sla ?? 0) }}</span>
        </div>
        <div class="flex justify-between">
          <span class="text-gray-500">{{ t('admin.ops.businessLimited') }}:</span>
          <span class="tabular-nums font-bold text-gray-900 dark:text-white">{{ formatCompactNumber(overview.business_limited_count ?? 0) }}</span>
        </div>
      </div>
    </div>

    <!-- Card 6: Upstream Errors -->
    <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-900" style="order: 6;">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-1">
          <span class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.upstreamErrors') }}</span>
          <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.upstreamErrors')" />
        </div>
        <button v-if="!props.fullscreen" class="text-[10px] font-bold text-blue-500 hover:underline" type="button" @click="openErrorDetails('upstream')">
          {{ t('admin.ops.requestDetails.details') }}
        </button>
      </div>
      <div class="mt-2 text-3xl font-black" :class="getThresholdColorClass(getUpstreamErrorRateThresholdLevel(upstreamErrorRatePercent))">
        {{ upstreamErrorRatePercent == null ? '-' : `${upstreamErrorRatePercent.toFixed(2)}%` }}
      </div>
      <div class="mt-3 space-y-1 text-xs">
        <div class="flex justify-between">
          <span class="text-gray-500">{{ t('admin.ops.errorCountExcl429529') }}:</span>
          <span class="tabular-nums font-bold text-gray-900 dark:text-white">{{ formatCompactNumber(overview.upstream_error_count_excl_429_529 ?? 0) }}</span>
        </div>
        <div class="flex justify-between">
          <span class="text-gray-500">429/529:</span>
          <span class="tabular-nums font-bold text-gray-900 dark:text-white">{{ formatCompactNumber((overview.upstream_429_count ?? 0) + (overview.upstream_529_count ?? 0)) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
