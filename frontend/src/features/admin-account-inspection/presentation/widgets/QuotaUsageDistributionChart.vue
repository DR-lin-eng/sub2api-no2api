<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { BarElement, CategoryScale, Chart as ChartJS, LinearScale, Tooltip } from 'chart.js'
import type { ChartData, ChartOptions } from 'chart.js'
import { Bar } from 'vue-chartjs'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { AccountInspectionQuotaDistribution } from '../../data/dtos/accountInspectionDtos'

ChartJS.register(BarElement, CategoryScale, LinearScale, Tooltip)

const props = defineProps<{
  distribution?: AccountInspectionQuotaDistribution | null
  loading: boolean
}>()

const { t } = useI18n()

const emptyDistribution: AccountInspectionQuotaDistribution = {
  average_used_percent: null,
  measured_accounts: 0,
  unknown_accounts: 0,
  buckets: [],
}

const quotaDistribution = computed(() => props.distribution ?? emptyDistribution)
const hasData = computed(() => quotaDistribution.value.measured_accounts > 0)
const averageLabel = computed(() => {
  const average = quotaDistribution.value.average_used_percent
  return average == null ? '-' : `${average.toFixed(1)}%`
})

const bucketColors: Record<string, string> = {
  '0_20': '#22c55e',
  '20_40': '#14b8a6',
  '40_70': '#3b82f6',
  '70_90': '#f59e0b',
  '90_100': '#f97316',
  over_100: '#ef4444',
}

const chartData = computed<ChartData<'bar'>>(() => ({
  labels: quotaDistribution.value.buckets.map((bucket) => t(`admin.accountInspection.quotaUsage.buckets.${bucket.key}`)),
  datasets: [{
    label: t('admin.accountInspection.quotaUsage.measured'),
    data: quotaDistribution.value.buckets.map((bucket) => bucket.count),
    backgroundColor: quotaDistribution.value.buckets.map((bucket) => bucketColors[bucket.key] ?? '#6b7280'),
    borderRadius: 4,
    borderSkipped: false,
    maxBarThickness: 72,
  }],
}))

const chartOptions = computed<ChartOptions<'bar'>>(() => {
  const dark = document.documentElement.classList.contains('dark')
  const textColor = dark ? '#d1d5db' : '#4b5563'
  const gridColor = dark ? '#374151' : '#e5e7eb'
  return {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: { display: false },
      tooltip: {
        callbacks: {
          label: (context) => t('admin.accountInspection.quotaUsage.accounts', { count: context.parsed.y ?? 0 }),
        },
      },
    },
    scales: {
      x: {
        grid: { display: false },
        ticks: { color: textColor },
      },
      y: {
        beginAtZero: true,
        grid: { color: gridColor },
        ticks: { color: textColor, precision: 0 },
      },
    },
  }
})
</script>

<template>
  <section aria-labelledby="quota-usage-title" class="border-y border-gray-200 py-5 dark:border-dark-700">
    <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
      <div class="min-w-0">
        <div class="flex items-center gap-2">
          <Icon name="chartBar" size="sm" class="text-primary-600 dark:text-primary-400" />
          <h2 id="quota-usage-title" class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.accountInspection.quotaUsage.title') }}
          </h2>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accountInspection.quotaUsage.caption') }}
        </p>
      </div>

      <dl class="grid w-full grid-cols-3 gap-4 text-right sm:w-auto sm:min-w-[420px]">
        <div>
          <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accountInspection.quotaUsage.average') }}</dt>
          <dd class="mt-1 text-base font-semibold tabular-nums text-gray-900 dark:text-white">{{ averageLabel }}</dd>
        </div>
        <div>
          <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accountInspection.quotaUsage.measured') }}</dt>
          <dd class="mt-1 text-base font-semibold tabular-nums text-gray-900 dark:text-white">{{ quotaDistribution.measured_accounts }}</dd>
        </div>
        <div>
          <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accountInspection.quotaUsage.unknown') }}</dt>
          <dd class="mt-1 text-base font-semibold tabular-nums text-gray-900 dark:text-white">{{ quotaDistribution.unknown_accounts }}</dd>
        </div>
      </dl>
    </div>

    <div class="mt-5 h-64">
      <div v-if="loading" class="h-full animate-pulse rounded bg-gray-100 dark:bg-dark-800" />
      <Bar v-else-if="hasData" :data="chartData" :options="chartOptions" />
      <div v-else class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.accountInspection.quotaUsage.empty') }}
      </div>
    </div>

    <p class="mt-3 text-xs leading-5 text-gray-500 dark:text-gray-400">
      {{ t('admin.accountInspection.quotaUsage.calculation') }}
    </p>
  </section>
</template>
