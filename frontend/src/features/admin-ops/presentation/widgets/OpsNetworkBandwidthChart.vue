<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  CategoryScale,
  Chart as ChartJS,
  Filler,
  Legend,
  LineElement,
  LinearScale,
  PointElement,
  Tooltip
} from 'chart.js'
import { Line } from 'vue-chartjs'
import EmptyState from '@/common/widgets/feedback/EmptyState.vue'
import HelpTooltip from '@/common/widgets/feedback/HelpTooltip.vue'
import type { OpsNetworkBandwidthTrendPoint } from '@/features/admin-ops/data/dtos/opsDashboardDtos'
import type { ChartState } from '../opsTypeSignals'
import { formatBytesPerSecond, formatExactNumber, formatHistoryLabel } from '../opsFormatter'

ChartJS.register(Tooltip, Legend, LineElement, LinearScale, PointElement, CategoryScale, Filler)

interface Props {
  points: OpsNetworkBandwidthTrendPoint[]
  interfaces?: string[] | null
  loading: boolean
  timeRange: string
}

const props = withDefaults(defineProps<Props>(), {
  interfaces: () => []
})
const { t } = useI18n()

const isDarkMode = computed(() => document.documentElement.classList.contains('dark'))
const colors = computed(() => ({
  receive: '#0ea5e9',
  receiveAlpha: '#0ea5e920',
  transmit: '#10b981',
  transmitAlpha: '#10b98118',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  text: isDarkMode.value ? '#9ca3af' : '#6b7280'
}))

function rateValue(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null
}

const hasRates = computed(() => props.points.some((point) => (
  rateValue(point.receive_bytes_per_second) != null || rateValue(point.transmit_bytes_per_second) != null
)))

const currentPoint = computed(() => {
  for (let index = props.points.length - 1; index >= 0; index -= 1) {
    const point = props.points[index]
    if (rateValue(point.receive_bytes_per_second) != null || rateValue(point.transmit_bytes_per_second) != null) {
      return point
    }
  }
  return null
})

const currentReceive = computed(() => rateValue(currentPoint.value?.receive_bytes_per_second))
const currentTransmit = computed(() => rateValue(currentPoint.value?.transmit_bytes_per_second))
const peakCombined = computed<number | null>(() => {
  let peak: number | null = null
  for (const point of props.points) {
    const receive = rateValue(point.receive_bytes_per_second)
    const transmit = rateValue(point.transmit_bytes_per_second)
    if (receive == null && transmit == null) continue
    peak = Math.max(peak ?? 0, (receive ?? 0) + (transmit ?? 0))
  }
  return peak
})

const interfaceLabel = computed(() => {
  const names = (props.interfaces ?? []).map((name) => name.trim()).filter(Boolean)
  return names.length > 0 ? names.join(', ') : ''
})

const chartData = computed(() => {
  if (!hasRates.value) return null
  const c = colors.value
  return {
    labels: props.points.map((point) => formatHistoryLabel(point.bucket_start, props.timeRange)),
    datasets: [
      {
        label: t('admin.ops.networkBandwidth.receive'),
        data: props.points.map((point) => rateValue(point.receive_bytes_per_second)),
        borderColor: c.receive,
        backgroundColor: c.receiveAlpha,
        fill: true,
        tension: 0.3,
        pointRadius: 0,
        pointHitRadius: 10,
        spanGaps: false
      },
      {
        label: t('admin.ops.networkBandwidth.transmit'),
        data: props.points.map((point) => rateValue(point.transmit_bytes_per_second)),
        borderColor: c.transmit,
        backgroundColor: c.transmitAlpha,
        fill: true,
        tension: 0.3,
        pointRadius: 0,
        pointHitRadius: 10,
        spanGaps: false
      }
    ]
  }
})

const state = computed<ChartState>(() => {
  if (chartData.value) return 'ready'
  if (props.loading) return 'loading'
  return 'empty'
})

const options = computed(() => {
  const c = colors.value
  return {
    responsive: true,
    maintainAspectRatio: false,
    interaction: { intersect: false, mode: 'index' as const },
    plugins: {
      legend: {
        position: 'top' as const,
        align: 'end' as const,
        labels: { color: c.text, usePointStyle: true, boxWidth: 6, font: { size: 10 } }
      },
      tooltip: {
        backgroundColor: isDarkMode.value ? '#1f2937' : '#ffffff',
        titleColor: isDarkMode.value ? '#f3f4f6' : '#111827',
        bodyColor: isDarkMode.value ? '#d1d5db' : '#4b5563',
        borderColor: c.grid,
        borderWidth: 1,
        padding: 10,
        callbacks: {
          label: (context: any) => {
            const value = rateValue(context?.parsed?.y)
            return `${context.dataset.label}: ${formatBytesPerSecond(value)}`
          }
        }
      }
    },
    scales: {
      x: {
        type: 'category' as const,
        grid: { display: false },
        ticks: {
          color: c.text,
          font: { size: 10 },
          maxTicksLimit: 10,
          autoSkip: true,
          autoSkipPadding: 10
        }
      },
      y: {
        type: 'linear' as const,
        beginAtZero: true,
        grid: { color: c.grid, borderDash: [4, 4] },
        ticks: {
          color: c.text,
          font: { size: 10 },
          callback: (value: string | number) => formatBytesPerSecond(Number(value))
        }
      }
    }
  }
})

function exactRateTitle(value: number | null): string {
  return value == null ? '-' : `${formatExactNumber(value, 1)} B/s`
}
</script>

<template>
  <div data-testid="network-bandwidth-chart" class="flex h-full min-w-0 flex-col rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
    <div class="mb-3 flex shrink-0 flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
      <div class="min-w-0">
        <h3 class="flex items-center gap-2 text-sm font-bold text-gray-900 dark:text-white">
          <svg class="h-4 w-4 shrink-0 text-cyan-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7l5-5 5 5M12 2v14m5 1l-5 5-5-5m5 5V8" />
          </svg>
          {{ t('admin.ops.networkBandwidth.title') }}
          <HelpTooltip :content="t('admin.ops.tooltips.networkBandwidth')" />
        </h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.networkBandwidth.publicScope') }}
          <span v-if="interfaceLabel" class="ml-1 font-mono text-gray-600 dark:text-gray-300">
            {{ t('admin.ops.networkBandwidth.interfaces', { names: interfaceLabel }) }}
          </span>
        </p>
      </div>
    </div>

    <div class="mb-4 grid shrink-0 grid-cols-1 border-y border-gray-100 py-3 dark:border-dark-700 sm:grid-cols-3">
      <div class="py-1 sm:border-r sm:border-gray-100 sm:px-4 sm:first:pl-0 dark:sm:border-dark-700">
        <div class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.networkBandwidth.currentReceive') }}</div>
        <div class="mt-1 font-mono text-base font-bold text-sky-600 dark:text-sky-400" :title="exactRateTitle(currentReceive)">
          {{ formatBytesPerSecond(currentReceive) }}
        </div>
      </div>
      <div class="py-1 sm:border-r sm:border-gray-100 sm:px-4 dark:sm:border-dark-700">
        <div class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.networkBandwidth.currentTransmit') }}</div>
        <div class="mt-1 font-mono text-base font-bold text-emerald-600 dark:text-emerald-400" :title="exactRateTitle(currentTransmit)">
          {{ formatBytesPerSecond(currentTransmit) }}
        </div>
      </div>
      <div class="py-1 sm:px-4 sm:last:pr-0">
        <div class="text-[10px] font-bold uppercase text-gray-400">{{ t('admin.ops.networkBandwidth.peakCombined') }}</div>
        <div class="mt-1 font-mono text-base font-bold text-gray-900 dark:text-white" :title="exactRateTitle(peakCombined)">
          {{ formatBytesPerSecond(peakCombined) }}
        </div>
      </div>
    </div>

    <div class="min-h-0 min-w-0 flex-1">
      <Line v-if="state === 'ready' && chartData" :data="chartData" :options="options" />
      <div v-else class="flex h-full items-center justify-center">
        <div v-if="state === 'loading'" class="animate-pulse text-sm text-gray-400">{{ t('common.loading') }}</div>
        <EmptyState v-else :title="t('common.noData')" :description="t('admin.ops.networkBandwidth.empty')" />
      </div>
    </div>
  </div>
</template>
