<template>
  <section aria-labelledby="cluster-load-summary-title">
    <h2 id="cluster-load-summary-title" class="sr-only">
      {{ t('admin.cluster.loadSummary.title') }}
    </h2>
    <div class="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
      <div
        v-for="metric in metrics"
        :key="metric.label"
        class="min-w-0 rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="flex items-center justify-between gap-2">
          <p class="truncate text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ metric.label }}
          </p>
          <Icon :name="metric.icon" size="sm" :class="metric.iconClass" />
        </div>
        <p class="mt-2 truncate text-2xl font-semibold tabular-nums text-gray-950 dark:text-white">
          {{ metric.value }}
        </p>
        <p class="mt-1 truncate text-[11px] text-gray-400 dark:text-gray-500" :title="metric.detail">
          {{ metric.detail }}
        </p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import type {
  ClusterInstance,
  ClusterSummary,
} from '@/features/admin-cluster/data/datasources/adminClusterDatasource'

const props = defineProps<{
  instances: ClusterInstance[]
  summary?: ClusterSummary
}>()

const { t } = useI18n()

const onlineInstances = computed(() => props.instances.filter((instance) => instance.status === 'online'))
const reportingInstances = computed(() => onlineInstances.value.filter((instance) => instance.load))

function percentageValues(key: 'cpu_usage_percent' | 'memory_usage_percent'): number[] {
  return onlineInstances.value
    .map((instance) => instance.load?.[key])
    .filter((value): value is number => typeof value === 'number' && Number.isFinite(value))
}

function aggregatePercent(key: 'cpu_usage_percent' | 'memory_usage_percent') {
  const values = percentageValues(key)
  if (values.length === 0) return null
  return {
    average: values.reduce((sum, value) => sum + value, 0) / values.length,
    peak: Math.max(...values),
    reporting: values.length,
  }
}

const cpu = computed(() => aggregatePercent('cpu_usage_percent'))
const memory = computed(() => aggregatePercent('memory_usage_percent'))
const inFlightRequests = computed(() => reportingInstances.value.length > 0
  ? reportingInstances.value.reduce((sum, instance) => sum + (instance.load?.in_flight_requests ?? 0), 0)
  : null)

function formatPercent(value: number): string {
  const precision = Number.isInteger(value) ? 0 : 1
  return `${value.toFixed(precision)}%`
}

function percentDetail(value: { peak: number; reporting: number } | null): string {
  if (!value) return t('admin.cluster.loadSummary.noMetrics')
  return t('admin.cluster.loadSummary.peakAndReporting', {
    peak: formatPercent(value.peak),
    count: value.reporting,
  })
}

const metrics = computed(() => [
  {
    label: t('admin.cluster.loadSummary.onlineNodes'),
    value: `${props.summary?.online_nodes ?? 0} / ${props.instances.length}`,
    detail: t('admin.cluster.loadSummary.onlineDetail', {
      stale: props.summary?.stale_nodes ?? 0,
      stopped: props.summary?.stopped_nodes ?? 0,
    }),
    icon: 'server' as const,
    iconClass: 'text-emerald-500',
  },
  {
    label: t('admin.cluster.loadSummary.averageCpu'),
    value: cpu.value ? formatPercent(cpu.value.average) : '-',
    detail: percentDetail(cpu.value),
    icon: 'cpu' as const,
    iconClass: 'text-sky-500',
  },
  {
    label: t('admin.cluster.loadSummary.averageMemory'),
    value: memory.value ? formatPercent(memory.value.average) : '-',
    detail: percentDetail(memory.value),
    icon: 'cube' as const,
    iconClass: 'text-teal-500',
  },
  {
    label: t('admin.cluster.loadSummary.inFlightRequests'),
    value: inFlightRequests.value ?? '-',
    detail: reportingInstances.value.length > 0
      ? t('admin.cluster.loadSummary.reportingNodes', { count: reportingInstances.value.length })
      : t('admin.cluster.loadSummary.noMetrics'),
    icon: 'arrowsUpDown' as const,
    iconClass: 'text-blue-500',
  },
  {
    label: t('admin.cluster.loadSummary.activeTasks'),
    value: props.summary?.active_tasks ?? 0,
    detail: t('admin.cluster.loadSummary.workerNodes', { count: props.summary?.worker_nodes ?? 0 }),
    icon: 'clock' as const,
    iconClass: 'text-amber-500',
  },
  {
    label: t('admin.cluster.loadSummary.unhealthyNodes'),
    value: props.summary?.unhealthy_nodes ?? 0,
    detail: (props.summary?.unhealthy_nodes ?? 0) > 0
      ? t('admin.cluster.loadSummary.needsAttention')
      : t('admin.cluster.loadSummary.allHealthy'),
    icon: 'exclamationTriangle' as const,
    iconClass: (props.summary?.unhealthy_nodes ?? 0) > 0 ? 'text-red-500' : 'text-gray-400',
  },
])
</script>
