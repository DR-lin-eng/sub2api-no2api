<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/common/widgets/feedback/HelpTooltip.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { OpsDashboardOverview } from '@/features/admin-ops/data/dtos/opsDashboardDtos'
import type { OpsRealtimeTrafficSummary } from '@/features/admin-ops/data/dtos/opsMetricsDtos'
import type { OpsRealtimeWindow } from '../composables/useOpsRealtimeTraffic'
import {
  formatCompactNumber,
  formatDurationMs,
  formatExactNumber,
} from '../opsFormatter'

interface Props {
  overview: OpsDashboardOverview
  fullscreen?: boolean
  realtimeWindow: OpsRealtimeWindow
  availableRealtimeWindows: readonly OpsRealtimeWindow[]
  realtimeTrafficSummary: OpsRealtimeTrafficSummary | null
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (event: 'update:realtimeWindow', value: OpsRealtimeWindow): void
}>()
const { t } = useI18n()
const overview = computed(() => props.overview)

const displayRealTimeQps = computed(() => {
  const value = props.realtimeTrafficSummary?.qps?.current
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
})
const displayRealTimeTps = computed(() => {
  const value = props.realtimeTrafficSummary?.tps?.current
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
})
const displayRealTimeQpsLabel = computed(() => formatCompactNumber(displayRealTimeQps.value))
const displayRealTimeTpsLabel = computed(() => formatCompactNumber(displayRealTimeTps.value))
const realtimeQpsPeakLabel = computed(() => formatCompactNumber(props.realtimeTrafficSummary?.qps?.peak))
const realtimeTpsPeakLabel = computed(() => formatCompactNumber(props.realtimeTrafficSummary?.tps?.peak))
const realtimeQpsAvgLabel = computed(() => formatCompactNumber(props.realtimeTrafficSummary?.qps?.avg))
const realtimeTpsAvgLabel = computed(() => formatCompactNumber(props.realtimeTrafficSummary?.tps?.avg))

// --- Health Score & Diagnosis (primary) ---

const isSystemIdle = computed(() => {
  const ov = overview.value
  if (!ov) return true
  const qps = ov.qps?.current
  const errorRate = ov.error_rate ?? 0
  return (qps ?? 0) === 0 && errorRate === 0
})

const healthScoreValue = computed<number | null>(() => {
  const v = overview.value?.health_score
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const healthScoreColor = computed(() => {
  if (isSystemIdle.value) return '#9ca3af' // gray-400
  const score = healthScoreValue.value
  if (score == null) return '#9ca3af'
  if (score >= 90) return '#10b981' // green
  if (score >= 60) return '#f59e0b' // yellow
  return '#ef4444' // red
})

const healthScoreClass = computed(() => {
  if (isSystemIdle.value) return 'text-gray-400'
  const score = healthScoreValue.value
  if (score == null) return 'text-gray-400'
  if (score >= 90) return 'text-green-500'
  if (score >= 60) return 'text-yellow-500'
  return 'text-red-500'
})

const circleSize = computed(() => props.fullscreen ? 140 : 100)
const strokeWidth = computed(() => props.fullscreen ? 10 : 8)
const radius = computed(() => (circleSize.value - strokeWidth.value) / 2)
const circumference = computed(() => 2 * Math.PI * radius.value)
const dashOffset = computed(() => {
  if (isSystemIdle.value) return 0
  if (healthScoreValue.value == null) return 0
  const score = Math.max(0, Math.min(100, healthScoreValue.value))
  return circumference.value - (score / 100) * circumference.value
})

interface DiagnosisItem {
  type: 'critical' | 'warning' | 'info'
  message: string
  impact: string
  action?: string
}

const diagnosisReport = computed<DiagnosisItem[]>(() => {
  const ov = overview.value
  if (!ov) return []

  const report: DiagnosisItem[] = []

  if (isSystemIdle.value) {
    report.push({
      type: 'info',
      message: t('admin.ops.diagnosis.idle'),
      impact: t('admin.ops.diagnosis.idleImpact')
    })
    return report
  }

  // Resource diagnostics (highest priority)
  const sm = ov.system_metrics
  if (sm) {
    if (sm.db_ok === false) {
      report.push({
        type: 'critical',
        message: t('admin.ops.diagnosis.dbDown'),
        impact: t('admin.ops.diagnosis.dbDownImpact'),
        action: t('admin.ops.diagnosis.dbDownAction')
      })
    }
    if (sm.redis_ok === false) {
      report.push({
        type: 'warning',
        message: t('admin.ops.diagnosis.redisDown'),
        impact: t('admin.ops.diagnosis.redisDownImpact'),
        action: t('admin.ops.diagnosis.redisDownAction')
      })
    }

    const cpuPct = sm.cpu_usage_percent ?? 0
    if (cpuPct > 90) {
      report.push({
        type: 'critical',
        message: t('admin.ops.diagnosis.cpuCritical', { usage: cpuPct.toFixed(1) }),
        impact: t('admin.ops.diagnosis.cpuCriticalImpact'),
        action: t('admin.ops.diagnosis.cpuCriticalAction')
      })
    } else if (cpuPct > 80) {
      report.push({
        type: 'warning',
        message: t('admin.ops.diagnosis.cpuHigh', { usage: cpuPct.toFixed(1) }),
        impact: t('admin.ops.diagnosis.cpuHighImpact'),
        action: t('admin.ops.diagnosis.cpuHighAction')
      })
    }

    const memPct = sm.memory_usage_percent ?? 0
    if (memPct > 90) {
      report.push({
        type: 'critical',
        message: t('admin.ops.diagnosis.memoryCritical', { usage: memPct.toFixed(1) }),
        impact: t('admin.ops.diagnosis.memoryCriticalImpact'),
        action: t('admin.ops.diagnosis.memoryCriticalAction')
      })
    } else if (memPct > 85) {
      report.push({
        type: 'warning',
        message: t('admin.ops.diagnosis.memoryHigh', { usage: memPct.toFixed(1) }),
        impact: t('admin.ops.diagnosis.memoryHighImpact'),
        action: t('admin.ops.diagnosis.memoryHighAction')
      })
    }
  }

  const ttftP99 = ov.ttft?.p99_ms ?? 0
  if (ttftP99 > 500) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.ttftHigh', { ttft: formatDurationMs(ttftP99) }),
      impact: t('admin.ops.diagnosis.ttftHighImpact'),
      action: t('admin.ops.diagnosis.ttftHighAction')
    })
  }

  // Error rate diagnostics (adjusted thresholds)
  const upstreamRatePct = (ov.upstream_error_rate ?? 0) * 100
  if (upstreamRatePct > 5) {
    report.push({
      type: 'critical',
      message: t('admin.ops.diagnosis.upstreamCritical', { rate: upstreamRatePct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.upstreamCriticalImpact'),
      action: t('admin.ops.diagnosis.upstreamCriticalAction')
    })
  } else if (upstreamRatePct > 2) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.upstreamHigh', { rate: upstreamRatePct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.upstreamHighImpact'),
      action: t('admin.ops.diagnosis.upstreamHighAction')
    })
  }

  const errorPct = (ov.error_rate ?? 0) * 100
  if (errorPct > 3) {
    report.push({
      type: 'critical',
      message: t('admin.ops.diagnosis.errorHigh', { rate: errorPct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.errorHighImpact'),
      action: t('admin.ops.diagnosis.errorHighAction')
    })
  } else if (errorPct > 0.5) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.errorElevated', { rate: errorPct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.errorElevatedImpact'),
      action: t('admin.ops.diagnosis.errorElevatedAction')
    })
  }

  // SLA diagnostics
  const slaPct = (ov.sla ?? 0) * 100
  if (slaPct < 90) {
    report.push({
      type: 'critical',
      message: t('admin.ops.diagnosis.slaCritical', { sla: slaPct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.slaCriticalImpact'),
      action: t('admin.ops.diagnosis.slaCriticalAction')
    })
  } else if (slaPct < 98) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.slaLow', { sla: slaPct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.slaLowImpact'),
      action: t('admin.ops.diagnosis.slaLowAction')
    })
  }

  // Health score diagnostics (lowest priority)
  if (healthScoreValue.value != null) {
    if (healthScoreValue.value < 60) {
      report.push({
        type: 'critical',
        message: t('admin.ops.diagnosis.healthCritical', { score: healthScoreValue.value }),
        impact: t('admin.ops.diagnosis.healthCriticalImpact'),
        action: t('admin.ops.diagnosis.healthCriticalAction')
      })
    } else if (healthScoreValue.value < 90) {
      report.push({
        type: 'warning',
        message: t('admin.ops.diagnosis.healthLow', { score: healthScoreValue.value }),
        impact: t('admin.ops.diagnosis.healthLowImpact'),
        action: t('admin.ops.diagnosis.healthLowAction')
      })
    }
  }

  if (report.length === 0) {
    report.push({
      type: 'info',
      message: t('admin.ops.diagnosis.healthy'),
      impact: t('admin.ops.diagnosis.healthyImpact')
    })
  }

  return report
})
</script>

<template>
<div :class="['rounded-2xl bg-gray-50 dark:bg-dark-900 lg:col-span-5', props.fullscreen ? 'p-6' : 'p-4']">
    <div class="grid h-full grid-cols-1 gap-6 md:grid-cols-[200px_1fr] md:items-center">
      <!-- 1) Health Score -->
      <div
        class="group relative flex cursor-pointer flex-col items-center justify-center rounded-xl py-2 transition-all hover:bg-white/60 dark:hover:bg-dark-800/60 md:border-r md:border-gray-200 md:pr-6 dark:md:border-dark-700"
      >
        <!-- Diagnosis Popover (hover) -->
        <div
          class="pointer-events-none absolute left-1/2 top-full z-50 mt-2 w-72 -translate-x-1/2 opacity-0 transition-opacity duration-200 group-hover:pointer-events-auto group-hover:opacity-100 md:left-full md:top-0 md:ml-2 md:mt-0 md:translate-x-0"
        >
          <div class="rounded-xl bg-white p-4 shadow-xl ring-1 ring-black/5 dark:bg-dark-800 dark:ring-white/10">
            <h4 class="mb-3 border-b border-gray-100 pb-2 text-sm font-bold text-gray-900 dark:border-dark-700 dark:text-white flex items-center gap-2">
              <Icon name="brain" size="sm" class="text-blue-500" />
              {{ t('admin.ops.diagnosis.title') }}
            </h4>

            <div class="space-y-3">
              <div v-for="(item, idx) in diagnosisReport" :key="idx" class="flex gap-3">
                <div class="mt-0.5 shrink-0">
                  <svg v-if="item.type === 'critical'" class="h-4 w-4 text-red-500" fill="currentColor" viewBox="0 0 20 20">
                    <path
                      fill-rule="evenodd"
                      d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
                      clip-rule="evenodd"
                    />
                  </svg>
                  <svg v-else-if="item.type === 'warning'" class="h-4 w-4 text-yellow-500" fill="currentColor" viewBox="0 0 20 20">
                    <path
                      fill-rule="evenodd"
                      d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z"
                      clip-rule="evenodd"
                    />
                  </svg>
                  <svg v-else class="h-4 w-4 text-blue-500" fill="currentColor" viewBox="0 0 20 20">
                    <path
                      fill-rule="evenodd"
                      d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-8-3a1 1 0 100 2 1 1 0 000-2zm-1 3a1 1 0 012 0v4a1 1 0 11-2 0v-4z"
                      clip-rule="evenodd"
                    />
                  </svg>
                </div>
                <div class="flex-1">
                  <div class="text-xs font-semibold text-gray-900 dark:text-white">{{ item.message }}</div>
                  <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">{{ item.impact }}</div>
                  <div v-if="item.action" class="mt-1 text-[11px] text-blue-600 dark:text-blue-400 flex items-center gap-1">
                    <Icon name="lightbulb" size="xs" />
                    {{ item.action }}
                  </div>
                </div>
              </div>
            </div>

            <div class="mt-3 border-t border-gray-100 pt-2 text-[10px] text-gray-400 dark:border-dark-700">
              {{ t('admin.ops.diagnosis.footer') }}
            </div>
          </div>
        </div>

        <div class="relative flex items-center justify-center">
          <svg :width="circleSize" :height="circleSize" class="-rotate-90 transform">
            <circle
              :cx="circleSize / 2"
              :cy="circleSize / 2"
              :r="radius"
              :stroke-width="strokeWidth"
              fill="transparent"
              class="text-gray-200 dark:text-dark-700"
              stroke="currentColor"
            />
            <circle
              :cx="circleSize / 2"
              :cy="circleSize / 2"
              :r="radius"
              :stroke-width="strokeWidth"
              fill="transparent"
              :stroke="healthScoreColor"
              stroke-linecap="round"
              :stroke-dasharray="circumference"
              :stroke-dashoffset="dashOffset"
              class="transition-all duration-1000 ease-out"
            />
          </svg>

          <div class="absolute flex flex-col items-center">
            <span :class="[props.fullscreen ? 'text-5xl' : 'text-3xl', 'font-black', healthScoreClass]">
              {{ isSystemIdle ? t('admin.ops.idleStatus') : (overview.health_score ?? '--') }}
            </span>
            <span :class="[props.fullscreen ? 'text-xs' : 'text-[10px]', 'font-bold uppercase tracking-wider text-gray-400']">{{ t('admin.ops.health') }}</span>
          </div>
        </div>

        <div class="mt-4 text-center" v-if="!props.fullscreen">
          <div class="flex items-center justify-center gap-1 text-xs font-medium text-gray-500">
            {{ t('admin.ops.healthCondition') }}
            <HelpTooltip :content="t('admin.ops.healthHelp')" />
          </div>
          <div class="mt-1 text-xs font-bold" :class="healthScoreClass">
            {{
              isSystemIdle
                ? t('admin.ops.idleStatus')
                : typeof overview.health_score === 'number' && overview.health_score >= 90
                  ? t('admin.ops.healthyStatus')
                  : t('admin.ops.riskyStatus')
            }}
          </div>
        </div>
      </div>

      <!-- 2) Realtime Traffic -->
      <div class="flex h-full flex-col justify-center py-2">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div class="flex items-center gap-2">
            <div class="relative flex h-3 w-3 shrink-0">
              <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-blue-400 opacity-75"></span>
              <span class="relative inline-flex h-3 w-3 rounded-full bg-blue-500"></span>
            </div>
            <h3 class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.realtime.title') }}</h3>
            <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.qps')" />
          </div>

          <!-- Time Window Selector -->
          <div class="flex flex-wrap gap-1">
            <button
              v-for="window in availableRealtimeWindows"
              :key="window"
              type="button"
              class="rounded px-1.5 py-0.5 text-[9px] font-bold transition-colors sm:px-2 sm:text-[10px]"
              :class="realtimeWindow === window
                ? 'bg-blue-500 text-white'
                : 'bg-gray-200 text-gray-600 hover:bg-gray-300 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600'"
              @click="emit('update:realtimeWindow', window)"
            >
              {{ window }}
            </button>
          </div>
        </div>

        <div :class="props.fullscreen ? 'space-y-4' : 'space-y-3'">
          <!-- Row 1: Current -->
          <div>
            <div :class="[props.fullscreen ? 'text-xs' : 'text-[10px]', 'font-bold uppercase text-gray-400']">{{ t('admin.ops.current') }}</div>
            <div class="mt-1 flex flex-wrap items-baseline gap-x-4 gap-y-2">
              <div class="flex items-baseline gap-1.5">
                <span
                  :class="[props.fullscreen ? 'text-4xl' : 'text-xl sm:text-2xl', 'min-w-0 tabular-nums font-black text-gray-900 dark:text-white']"
                  :title="`${formatExactNumber(displayRealTimeQps)} QPS`"
                >{{ displayRealTimeQpsLabel }}</span>
                <span :class="[props.fullscreen ? 'text-sm' : 'text-xs', 'font-bold text-gray-500']">QPS</span>
              </div>
              <div class="flex items-baseline gap-1.5">
                <span
                  :class="[props.fullscreen ? 'text-4xl' : 'text-xl sm:text-2xl', 'min-w-0 tabular-nums font-black text-gray-900 dark:text-white']"
                  :title="`${formatExactNumber(displayRealTimeTps)} TPS`"
                >{{ displayRealTimeTpsLabel }}</span>
                <span :class="[props.fullscreen ? 'text-sm' : 'text-xs', 'font-bold text-gray-500']">{{ t('admin.ops.tps') }}</span>
              </div>
            </div>
          </div>

          <!-- Row 2: Peak + Average -->
          <div class="grid grid-cols-2 gap-3">
            <!-- Peak -->
            <div>
              <div :class="[props.fullscreen ? 'text-xs' : 'text-[10px]', 'font-bold uppercase text-gray-400']">{{ t('admin.ops.peak') }}</div>
              <div :class="[props.fullscreen ? 'text-base' : 'text-sm', 'mt-1 space-y-0.5 font-medium text-gray-600 dark:text-gray-400']">
                <div class="flex items-baseline gap-1.5">
                  <span class="tabular-nums font-black text-gray-900 dark:text-white" :title="formatExactNumber(realtimeTrafficSummary?.qps?.peak)">{{ realtimeQpsPeakLabel }}</span>
                  <span class="text-xs">QPS</span>
                </div>
                <div class="flex items-baseline gap-1.5">
                  <span class="tabular-nums font-black text-gray-900 dark:text-white" :title="formatExactNumber(realtimeTrafficSummary?.tps?.peak)">{{ realtimeTpsPeakLabel }}</span>
                  <span class="text-xs">{{ t('admin.ops.tps') }}</span>
                </div>
              </div>
            </div>

            <!-- Average -->
            <div>
              <div :class="[props.fullscreen ? 'text-xs' : 'text-[10px]', 'font-bold uppercase text-gray-400']">{{ t('admin.ops.average') }}</div>
              <div :class="[props.fullscreen ? 'text-base' : 'text-sm', 'mt-1 space-y-0.5 font-medium text-gray-600 dark:text-gray-400']">
                <div class="flex items-baseline gap-1.5">
                  <span class="tabular-nums font-black text-gray-900 dark:text-white" :title="formatExactNumber(realtimeTrafficSummary?.qps?.avg)">{{ realtimeQpsAvgLabel }}</span>
                  <span class="text-xs">QPS</span>
                </div>
                <div class="flex items-baseline gap-1.5">
                  <span class="tabular-nums font-black text-gray-900 dark:text-white" :title="formatExactNumber(realtimeTrafficSummary?.tps?.avg)">{{ realtimeTpsAvgLabel }}</span>
                  <span class="text-xs">{{ t('admin.ops.tps') }}</span>
                </div>
              </div>
            </div>
          </div>

          <!-- Animated Pulse Line (Heart Beat Animation) -->
          <div class="h-8 w-full overflow-hidden opacity-50">
            <svg class="h-full w-full" viewBox="0 0 280 32" preserveAspectRatio="none">
              <path
                d="M0 16 Q 20 16, 40 16 T 80 16 T 120 10 T 160 22 T 200 16 T 240 16 T 280 16"
                fill="none"
                stroke="#3b82f6"
                stroke-width="2"
                vector-effect="non-scaling-stroke"
              >
                <animate
                  attributeName="d"
                  dur="2s"
                  repeatCount="indefinite"
                  values="M0 16 Q 20 16, 40 16 T 80 16 T 120 10 T 160 22 T 200 16 T 240 16 T 280 16;
                          M0 16 Q 20 16, 40 16 T 80 16 T 120 16 T 160 16 T 200 10 T 240 22 T 280 16;
                          M0 16 Q 20 16, 40 16 T 80 16 T 120 16 T 160 16 T 200 16 T 240 16 T 280 16"
                  keyTimes="0;0.5;1"
                />
              </path>
            </svg>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
