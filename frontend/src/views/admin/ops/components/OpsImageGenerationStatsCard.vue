<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/common/EmptyState.vue'
import { opsAPI, type OpsImageGenerationStatsResponse } from '@/api/admin/ops'
import { formatCompactNumber, formatDurationMs, formatExactDurationMs, formatExactNumber } from '../utils/opsFormatters'

interface Props {
  timeRange: '5m' | '30m' | '1h' | '6h' | '24h' | 'custom'
  customStartTime?: string | null
  customEndTime?: string | null
  platformFilter?: string
  groupIdFilter?: number | null
  refreshToken: number
}

const props = withDefaults(defineProps<Props>(), {
  customStartTime: null,
  customEndTime: null,
  platformFilter: '',
  groupIdFilter: null
})

const { t } = useI18n()
const loading = ref(false)
const errorMessage = ref('')
const response = ref<OpsImageGenerationStatsResponse | null>(null)
let requestController: AbortController | null = null
let requestSequence = 0

const rows = computed(() => response.value?.by_resolution ?? [])

function buildParams() {
  const params: Record<string, string | number | undefined> = {
    platform: props.platformFilter || undefined,
    group_id: typeof props.groupIdFilter === 'number' && props.groupIdFilter > 0 ? props.groupIdFilter : undefined
  }
  if (props.timeRange === 'custom') {
    params.start_time = props.customStartTime || undefined
    params.end_time = props.customEndTime || undefined
  } else {
    params.time_range = props.timeRange
  }
  return params
}

function formatResolution(value: string): string {
  if (!value || value === 'unknown') return t('admin.ops.imageGeneration.unknownResolution')
  return value.replace('x', ' × ')
}

function formatTier(value: string): string {
  if (!value || value === 'unknown') return '-'
  return value
}

function formatMetric(value?: number | null, digits = 1): string {
  return formatCompactNumber(value, digits)
}

async function loadData() {
  requestController?.abort()
  requestController = new AbortController()
  requestSequence += 1
  const sequence = requestSequence
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await opsAPI.getImageGenerationStats(buildParams(), { signal: requestController.signal })
    if (sequence !== requestSequence) return
    response.value = result
  } catch (err: any) {
    if (err?.name === 'CanceledError' || err?.code === 'ERR_CANCELED') return
    if (sequence !== requestSequence) return
    console.error('[OpsImageGenerationStatsCard] Failed to load data', err)
    response.value = null
    errorMessage.value = err?.message || t('admin.ops.imageGeneration.failedToLoad')
  } finally {
    if (sequence === requestSequence) loading.value = false
  }
}

watch(
  () => [
    props.timeRange,
    props.customStartTime,
    props.customEndTime,
    props.platformFilter,
    props.groupIdFilter,
    props.refreshToken
  ] as const,
  () => void loadData(),
  { immediate: true }
)

onBeforeUnmount(() => requestController?.abort())
</script>

<template>
  <section class="card p-4 md:p-5">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-sm font-bold text-gray-900 dark:text-white">
        {{ t('admin.ops.imageGeneration.title') }}
      </h3>
      <span class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.imageGeneration.windowScope') }}
      </span>
    </div>

    <div v-if="errorMessage" class="mb-4 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
      {{ errorMessage }}
    </div>

    <div v-if="loading && !response" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.ops.loadingText') }}
    </div>

    <template v-else-if="response">
      <div class="mb-5 grid grid-cols-2 border-y border-gray-200 py-3 dark:border-dark-700 md:grid-cols-4 xl:grid-cols-8">
        <div class="min-w-0 px-3 py-2">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.imageGeneration.images') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white" :title="formatExactNumber(response.image_count)">
            {{ formatMetric(response.image_count) }}
          </div>
        </div>
        <div class="min-w-0 border-l border-gray-200 px-3 py-2 dark:border-dark-700">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.imageGeneration.requests') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white" :title="formatExactNumber(response.request_count)">
            {{ formatMetric(response.request_count) }}
          </div>
        </div>
        <div class="min-w-0 border-l border-gray-200 px-3 py-2 dark:border-dark-700">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.imageGeneration.avgDuration') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white" :title="formatExactDurationMs(response.avg_duration_ms)">
            {{ formatDurationMs(response.avg_duration_ms) }}
          </div>
        </div>
        <div class="min-w-0 border-l border-gray-200 px-3 py-2 dark:border-dark-700">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.imageGeneration.p95Duration') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white" :title="formatExactDurationMs(response.p95_duration_ms)">
            {{ formatDurationMs(response.p95_duration_ms) }}
          </div>
        </div>
        <div class="min-w-0 px-3 py-2 md:border-l md:border-gray-200 md:dark:border-dark-700">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.imageGeneration.requestsPerMinute') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white" :title="formatExactNumber(response.requests_per_minute)">
            {{ formatMetric(response.requests_per_minute, 2) }}
          </div>
        </div>
        <div class="min-w-0 border-l border-gray-200 px-3 py-2 dark:border-dark-700">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.imageGeneration.windowConcurrency') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">
            {{ formatMetric(response.average_concurrent, 2) }} / {{ formatMetric(response.peak_concurrent) }}
          </div>
        </div>
        <div class="min-w-0 border-l border-gray-200 px-3 py-2 dark:border-dark-700">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.imageGeneration.instanceConcurrency') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">
            {{ response.realtime.available ? `${response.realtime.current_concurrent} / ${response.realtime.limit || t('admin.ops.imageGeneration.unlimited')}` : '-' }}
          </div>
        </div>
        <div class="min-w-0 border-l border-gray-200 px-3 py-2 dark:border-dark-700">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.imageGeneration.instanceQueue') }}</div>
          <div class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">
            {{ response.realtime.available ? response.realtime.waiting : '-' }}
          </div>
        </div>
      </div>

      <EmptyState
        v-if="rows.length === 0"
        :title="t('common.noData')"
        :description="t('admin.ops.imageGeneration.empty')"
      />

      <div v-else class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
        <div class="overflow-x-auto">
          <table class="min-w-full text-left text-xs md:text-sm">
            <thead class="bg-gray-50 text-gray-500 dark:bg-dark-800 dark:text-gray-400">
              <tr class="border-b border-gray-200 dark:border-dark-700">
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.imageGeneration.table.resolution') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.imageGeneration.table.billingTier') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.imageGeneration.table.requests') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.imageGeneration.table.images') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.imageGeneration.table.avgDuration') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.imageGeneration.table.p95Duration') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.imageGeneration.table.maxDuration') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in rows"
                :key="`${row.resolution}:${row.billing_tier}`"
                class="border-b border-gray-100 text-gray-700 last:border-0 dark:border-dark-800 dark:text-gray-200"
              >
                <td class="px-3 py-2 font-medium">{{ formatResolution(row.resolution) }}</td>
                <td class="px-3 py-2">{{ formatTier(row.billing_tier) }}</td>
                <td class="px-3 py-2 tabular-nums" :title="formatExactNumber(row.request_count)">{{ formatMetric(row.request_count) }}</td>
                <td class="px-3 py-2 tabular-nums" :title="formatExactNumber(row.image_count)">{{ formatMetric(row.image_count) }}</td>
                <td class="px-3 py-2 tabular-nums" :title="formatExactDurationMs(row.avg_duration_ms)">{{ formatDurationMs(row.avg_duration_ms) }}</td>
                <td class="px-3 py-2 tabular-nums" :title="formatExactDurationMs(row.p95_duration_ms)">{{ formatDurationMs(row.p95_duration_ms) }}</td>
                <td class="px-3 py-2 tabular-nums" :title="formatExactDurationMs(row.max_duration_ms)">{{ formatDurationMs(row.max_duration_ms) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </section>
</template>
