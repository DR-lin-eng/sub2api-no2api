<template>
  <div
    v-if="stats && stats.total_requests > 0"
    class="grid min-w-[9.5rem] grid-cols-2 gap-x-3 gap-y-1 text-xs"
  >
    <div class="flex items-center justify-between gap-2">
      <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.hourlyUsage.ttft') }}</span>
      <span class="font-medium text-gray-800 dark:text-gray-200">{{ formatTTFT(stats.avg_first_token_ms) }}</span>
    </div>
    <div class="flex items-center justify-between gap-2">
      <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.hourlyUsage.successRate') }}</span>
      <span :class="successRateClass">{{ formatSuccessRate(stats.success_rate) }}</span>
    </div>
    <div class="flex items-center justify-between gap-2">
      <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.hourlyUsage.error4xx') }}</span>
      <span class="font-medium text-amber-600 dark:text-amber-400">{{ stats.error_4xx }}</span>
    </div>
    <div class="flex items-center justify-between gap-2">
      <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.hourlyUsage.error5xx') }}</span>
      <span class="font-medium text-red-600 dark:text-red-400">{{ stats.error_5xx }}</span>
    </div>
  </div>
  <span v-else class="text-xs text-gray-400 dark:text-gray-500">-</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountHourlyUsageStats } from '@/types'

const props = defineProps<{
  stats?: AccountHourlyUsageStats | null
}>()

const { t } = useI18n()

const formatTTFT = (value: number | null): string => {
  if (value == null || !Number.isFinite(value)) return '-'
  if (value >= 1000) return `${(value / 1000).toFixed(value >= 10000 ? 1 : 2)}s`
  return `${Math.round(value)}ms`
}

const formatSuccessRate = (value: number): string => {
  if (!Number.isFinite(value)) return '-'
  return `${(Math.min(1, Math.max(0, value)) * 100).toFixed(1)}%`
}

const successRateClass = computed(() => {
  const rate = props.stats?.success_rate ?? 0
  if (rate >= 0.99) return 'font-medium text-emerald-600 dark:text-emerald-400'
  if (rate >= 0.95) return 'font-medium text-amber-600 dark:text-amber-400'
  return 'font-medium text-red-600 dark:text-red-400'
})
</script>
