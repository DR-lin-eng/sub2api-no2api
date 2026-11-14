<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import GroupBadge from '@/common/widgets/data/GroupBadge.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import {
  getRoutingSchemeAnalytics,
  type RoutingSchemeAnalyticsDto,
  type RoutingSchemeAnalyticsWindow,
} from '../../data/datasources/routingSchemeQueries'
import type { RoutingScheme } from '../../data/dtos/routingSchemeDtos'

const props = defineProps<{
  show: boolean
  scheme: RoutingScheme | null
}>()

const emit = defineEmits<{
  close: []
}>()

const { t } = useI18n()

const selectedWindow = ref<RoutingSchemeAnalyticsWindow>('24h')
const loading = ref(false)
const failed = ref(false)
const analytics = ref<RoutingSchemeAnalyticsDto | null>(null)

const windows: { value: RoutingSchemeAnalyticsWindow; labelKey: string }[] = [
  { value: '24h', labelKey: 'admin.routingSchemes.analytics.window24h' },
  { value: '7d', labelKey: 'admin.routingSchemes.analytics.window7d' },
  { value: '30d', labelKey: 'admin.routingSchemes.analytics.window30d' },
]

// Switching the window quickly can make a slow response land after a newer
// one; the sequence check keeps only the latest request's result.
let requestSeq = 0
const load = async () => {
  const scheme = props.scheme
  if (!scheme) return
  const seq = ++requestSeq
  loading.value = true
  failed.value = false
  try {
    const result = await getRoutingSchemeAnalytics(scheme.id, selectedWindow.value)
    if (seq !== requestSeq) return
    analytics.value = result
  } catch {
    if (seq !== requestSeq) return
    analytics.value = null
    failed.value = true
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

watch(() => props.show, (show) => {
  if (!show) return
  selectedWindow.value = '24h'
  analytics.value = null
  failed.value = false
  void load()
})

watch(selectedWindow, () => { void load() })

const hasTraffic = computed(() => (analytics.value?.requests ?? 0) > 0)
const formatCost = (value: number) => `$${(value ?? 0).toFixed(4)}`
const formatCount = (value: number) => (value ?? 0).toLocaleString()
const formatPercent = (value: number) => `${Math.round((value ?? 0) * 10) / 10}%`
const formatP95 = (ms: number) => (ms > 0 ? `${Math.round(ms / 100) / 10}s` : '—')
// Success rate is computed by the backend as successes / requests. Provider
// failures are attempt-level and are shown as their own column.
const successRate = (requests: number, rate: number) =>
  requests > 0 ? formatPercent(rate) : '—'
</script>

<template>
  <BaseDialog
    :show="show"
    :title="t('admin.routingSchemes.analytics.title', { name: scheme?.name ?? '' })"
    width="extra-wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">
          {{ t('admin.routingSchemes.analytics.hint') }}
        </p>
        <div class="flex rounded-lg border border-gray-200 p-0.5 dark:border-dark-600">
          <button
            v-for="option in windows"
            :key="option.value"
            type="button"
            class="rounded-md px-3 py-1 text-xs font-medium transition-colors"
            :class="selectedWindow === option.value
              ? 'bg-primary-500 text-white'
              : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
            @click="selectedWindow = option.value"
          >
            {{ t(option.labelKey) }}
          </button>
        </div>
      </div>

      <div v-if="loading" class="py-10 text-center text-sm text-gray-500">
        {{ t('common.loading') }}
      </div>

      <div
        v-else-if="failed"
        class="rounded-lg border border-dashed border-red-200 px-3 py-8 text-center text-sm text-red-500 dark:border-red-900/60 dark:text-red-400"
      >
        {{ t('admin.routingSchemes.analytics.loadFailed') }}
      </div>

      <template v-else-if="analytics">
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div class="rounded-xl border border-gray-200 bg-gray-50/70 p-4 dark:border-dark-700 dark:bg-dark-800/60">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.routingSchemes.analytics.requests') }}</div>
            <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ formatCount(analytics.requests) }}</div>
          </div>
          <div class="rounded-xl border border-gray-200 bg-gray-50/70 p-4 dark:border-dark-700 dark:bg-dark-800/60">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.routingSchemes.analytics.cost') }}</div>
            <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ formatCost(analytics.cost) }}</div>
          </div>
          <div class="rounded-xl border border-gray-200 bg-gray-50/70 p-4 dark:border-dark-700 dark:bg-dark-800/60">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.routingSchemes.analytics.inputTokens') }}</div>
            <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ formatCount(analytics.input_tokens) }}</div>
          </div>
          <div class="rounded-xl border border-gray-200 bg-gray-50/70 p-4 dark:border-dark-700 dark:bg-dark-800/60">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.routingSchemes.analytics.outputTokens') }}</div>
            <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ formatCount(analytics.output_tokens) }}</div>
          </div>
        </div>

        <div
          v-if="!hasTraffic"
          class="rounded-lg border border-dashed border-gray-200 px-3 py-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
        >
          {{ t('admin.routingSchemes.analytics.empty') }}
        </div>

        <div v-else class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
          <table class="w-full min-w-[54rem] text-sm">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr class="text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                <th class="px-3 py-2">{{ t('admin.routingSchemes.analytics.group') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.routingSchemes.analytics.requests') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.routingSchemes.analytics.requestShare') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.routingSchemes.analytics.successRate') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.routingSchemes.analytics.providerFailures') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.routingSchemes.analytics.p95Ttft') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.routingSchemes.analytics.tokens') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.routingSchemes.analytics.cost') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.routingSchemes.analytics.costShare') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 dark:divide-dark-700">
              <tr
                v-for="group in analytics.groups"
                :key="group.group_id"
                class="hover:bg-gray-50 dark:hover:bg-dark-800/60"
              >
                <td class="px-3 py-2">
                  <GroupBadge
                    :name="group.group_name || `#${group.group_id}`"
                    :platform="group.platform"
                    :rate-multiplier="group.rate_multiplier"
                    truncate
                  />
                </td>
                <td class="px-3 py-2 text-right tabular-nums text-gray-700 dark:text-gray-200">
                  {{ formatCount(group.requests) }}
                </td>
                <td class="px-3 py-2">
                  <div class="flex items-center justify-end gap-2">
                    <div class="h-1.5 w-16 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
                      <div class="h-full rounded-full bg-primary-500" :style="{ width: `${Math.min(100, group.request_share)}%` }" />
                    </div>
                    <span class="w-12 text-right tabular-nums text-gray-700 dark:text-gray-200">
                      {{ formatPercent(group.request_share) }}
                    </span>
                  </div>
                </td>
                <td class="px-3 py-2 text-right tabular-nums text-gray-700 dark:text-gray-200">
                  {{ successRate(group.requests, group.success_rate) }}
                </td>
                <td class="px-3 py-2 text-right tabular-nums text-gray-700 dark:text-gray-200">
                  {{ formatCount(group.failures) }}
                </td>
                <td class="px-3 py-2 text-right tabular-nums text-gray-700 dark:text-gray-200">
                  {{ formatP95(group.p95_ttft_ms) }}
                </td>
                <td class="px-3 py-2 text-right tabular-nums text-gray-500 dark:text-gray-400">
                  {{ formatCount(group.input_tokens) }} / {{ formatCount(group.output_tokens) }}
                </td>
                <td class="px-3 py-2 text-right tabular-nums text-gray-700 dark:text-gray-200">
                  {{ formatCost(group.cost) }}
                </td>
                <td class="px-3 py-2 text-right tabular-nums text-gray-700 dark:text-gray-200">
                  {{ formatPercent(group.cost_share) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex items-center justify-between gap-3">
        <span class="inline-flex items-center gap-1 text-xs text-gray-400 dark:text-gray-500">
          <Icon name="chartBar" size="xs" />
          {{ analytics?.since ? t('admin.routingSchemes.analytics.since', { time: new Date(analytics.since).toLocaleString() }) : '' }}
        </span>
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>
