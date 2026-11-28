<template>
  <div class="card">
    <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.records') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.recordsHint') }}</p>
        </div>
        <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="loading" @click="emit('refresh')">
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          {{ t('admin.riskControl.refresh') }}
        </button>
      </div>

      <div class="flex flex-col gap-2 rounded-lg border border-gray-100 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-900/30 sm:flex-row sm:items-center sm:justify-between">
        <div class="flex min-w-0 items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
          <Icon name="filter" size="sm" class="flex-shrink-0 text-gray-400" />
          <span class="font-medium">{{ t('admin.riskControl.modelFilter') }}</span>
          <span class="truncate text-gray-500 dark:text-gray-400">{{ modelFilterSummary }}</span>
        </div>
        <div v-if="modelFilterPreviewModels.length > 0" class="flex flex-wrap gap-1.5">
          <span
            v-for="model in modelFilterPreviewModels"
            :key="model"
            class="inline-flex max-w-[180px] items-center truncate rounded-md bg-white px-2 py-1 font-mono text-xs text-gray-600 shadow-sm dark:bg-dark-800 dark:text-gray-300"
          >
            {{ model }}
          </span>
          <span v-if="hiddenModelFilterModelCount > 0" class="inline-flex rounded-md bg-white px-2 py-1 text-xs text-gray-500 shadow-sm dark:bg-dark-800 dark:text-gray-400">
            +{{ hiddenModelFilterModelCount }}
          </span>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-6">
        <Select v-model="resultFilter" :options="resultOptions" @change="emit('reload')" />
        <Select v-model="groupFilter" :options="groupFilterOptions" @change="emit('reload')" />
        <Select v-model="endpointFilter" :options="endpointOptions" @change="emit('reload')" />
        <input v-model.trim="searchFilter" type="search" class="input" :placeholder="t('admin.riskControl.filters.search')" @keyup.enter="emit('reload')" />
        <input v-model="fromFilter" type="datetime-local" class="input" :title="t('admin.riskControl.filters.from')" @change="emit('reload')" />
        <input v-model="toFilter" type="datetime-local" class="input" :title="t('admin.riskControl.filters.to')" @change="emit('reload')" />
      </div>
    </div>

    <div class="overflow-x-auto">
      <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
        <thead class="bg-gray-50 dark:bg-dark-800">
          <tr>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.time') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.group') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.user') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.apiKey') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.endpoint') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.result') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.highest') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.actionMeta') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.latency') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.input') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
          <tr v-if="loading">
            <td colspan="10" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</td>
          </tr>
          <tr v-else-if="logs.length === 0">
            <td colspan="10" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.emptyLogs') }}</td>
          </tr>
          <template v-else>
            <tr v-for="row in logs" :key="row.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
              <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(row.created_at) }}</td>
              <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ row.group_name || '-' }}</td>
              <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                <div>{{ row.user_email || '-' }}</div>
                <div v-if="row.user_id" class="text-xs text-gray-400">UID {{ row.user_id }}</div>
              </td>
              <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ row.api_key_name || '-' }}</td>
              <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                <div>{{ row.endpoint || '-' }}</div>
                <div class="text-xs text-gray-400">{{ row.provider || '-' }} / {{ row.model || '-' }}</div>
              </td>
              <td class="whitespace-nowrap px-5 py-4">
                <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="resultBadgeClass(row)">
                  {{ resultLabel(row) }}
                </span>
              </td>
              <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                <div>{{ row.highest_category || '-' }}</div>
                <div class="text-xs text-gray-400">{{ percent(row.highest_score) }}</div>
                <div v-if="row.matched_keyword" class="mt-0.5 text-xs font-medium text-red-600 dark:text-red-300" :title="t('admin.riskControl.matchedKeyword') + ': ' + row.matched_keyword">
                  {{ t('admin.riskControl.matchedKeyword') }}: {{ row.matched_keyword }}
                </div>
              </td>
              <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                <div>{{ violationCountText(row) }}</div>
                <div class="text-xs text-gray-400">
                  {{ row.email_sent ? t('admin.riskControl.emailSent') : t('admin.riskControl.emailNotSent') }}
                  <span v-if="row.auto_banned"> / {{ t('admin.riskControl.autoBanned') }}</span>
                </div>
                <button
                  v-if="canUnbanRow(row)"
                  type="button"
                  class="mt-2 inline-flex items-center gap-1 rounded-md border border-emerald-200 bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 transition-colors hover:bg-emerald-100 disabled:cursor-not-allowed disabled:opacity-60 dark:border-emerald-900/60 dark:bg-emerald-900/20 dark:text-emerald-300 dark:hover:bg-emerald-900/30"
                  :disabled="unbanningUserId === row.user_id"
                  @click="emit('unban', row)"
                >
                  <Icon name="checkCircle" size="xs" :class="unbanningUserId === row.user_id ? 'animate-spin' : ''" />
                  {{ unbanningUserId === row.user_id ? t('common.processing') : t('admin.riskControl.unbanUser') }}
                </button>
              </td>
              <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                <div>{{ latencyText(row.upstream_latency_ms) }}</div>
                <div v-if="row.queue_delay_ms !== null && row.queue_delay_ms !== undefined" class="text-xs text-gray-400">
                  {{ t('admin.riskControl.queueDelay', { ms: row.queue_delay_ms }) }}
                </div>
              </td>
              <td class="w-[320px] max-w-sm px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                <button
                  type="button"
                  class="group flex w-full min-w-0 items-center gap-2 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                  :title="inputSummaryText(row)"
                  @click="inputDetailRow = row"
                >
                  <span class="min-w-0 flex-1 truncate">{{ inputSummaryText(row) }}</span>
                  <Icon name="eye" size="xs" class="flex-shrink-0 text-gray-300 transition-colors group-hover:text-primary-500 dark:text-gray-500" />
                </button>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>

    <Pagination
      v-if="pagination.total > 0"
      :page="pagination.page"
      :total="pagination.total"
      :page-size="pagination.page_size"
      @update:page="emit('pageChange', $event)"
      @update:pageSize="emit('pageSizeChange', $event)"
    />
  </div>

  <BaseDialog
    :show="inputDetailRow !== null"
    :title="t('admin.riskControl.inputDetailTitle')"
    width="wide"
    @close="inputDetailRow = null"
  >
    <div v-if="inputDetailRow" class="space-y-5">
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.time') }}</p>
          <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ formatDateTime(inputDetailRow.created_at) }}</p>
        </div>
        <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.user') }}</p>
          <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ inputDetailRow.user_email || '-' }}</p>
        </div>
        <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.result') }}</p>
          <span class="mt-1 inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="resultBadgeClass(inputDetailRow)">
            {{ resultLabel(inputDetailRow) }}
          </span>
        </div>
        <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.highest') }}</p>
          <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">
            {{ inputDetailRow.highest_category || '-' }} / {{ percent(inputDetailRow.highest_score) }}
          </p>
        </div>
        <div v-if="inputDetailRow.matched_keyword" class="rounded-lg border border-red-100 bg-red-50 p-4 dark:border-red-900/60 dark:bg-red-900/20">
          <p class="text-xs font-medium text-red-500 dark:text-red-300">{{ t('admin.riskControl.matchedKeyword') }}</p>
          <p class="mt-1 truncate text-sm font-semibold text-red-700 dark:text-red-200" :title="inputDetailRow.matched_keyword">{{ inputDetailRow.matched_keyword }}</p>
        </div>
      </div>

      <div class="rounded-xl border border-gray-100 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.inputDetailContent') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ inputDetailRow.endpoint || '-' }} · {{ inputDetailRow.provider || '-' }} / {{ inputDetailRow.model || '-' }}
            </p>
          </div>
          <span v-if="inputDetailRow.group_name" class="inline-flex rounded-md bg-sky-50 px-2.5 py-1 text-xs font-medium text-sky-700 dark:bg-sky-900/20 dark:text-sky-300">
            {{ inputDetailRow.group_name }}
          </span>
        </div>
        <pre class="mt-4 max-h-[420px] overflow-auto whitespace-pre-wrap break-words rounded-lg bg-gray-950 p-4 text-sm leading-6 text-gray-100 shadow-inner dark:bg-black/50">{{ inputDetailText }}</pre>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button type="button" class="btn btn-secondary" @click="inputDetailRow = null">{{ t('common.close') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import Pagination from '@/common/widgets/data/Pagination.vue'
import Select from '@/common/widgets/forms/Select.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { ContentModerationLog } from '@/features/admin-risk-control/data/datasources/adminRiskControlDatasource'
import type { SelectOption } from '@/types'
import { formatDateTime as formatDateTimeValue } from '@/core/utils/format'

type LogFilters = {
  result: string
  group_id: number
  endpoint: string
  search: string
  from: string
  to: string
}

type LogPagination = {
  page: number
  page_size: number
  total: number
  pages: number
}

const props = defineProps<{
  logs: ContentModerationLog[]
  loading: boolean
  filters: LogFilters
  resultOptions: SelectOption[]
  groupFilterOptions: SelectOption[]
  endpointOptions: SelectOption[]
  modelFilterSummary: string
  modelFilterPreviewModels: string[]
  hiddenModelFilterModelCount: number
  pagination: LogPagination
  unbanningUserId: number | null
}>()

const emit = defineEmits<{
  refresh: []
  reload: []
  pageChange: [page: number]
  pageSizeChange: [pageSize: number]
  unban: [row: ContentModerationLog]
  updateFilter: [key: keyof LogFilters, value: string | number]
}>()

const { t } = useI18n()
const resultFilter = computed({
  get: () => props.filters.result,
  set: (value: string) => emit('updateFilter', 'result', value),
})
const groupFilter = computed({
  get: () => props.filters.group_id,
  set: (value: number) => emit('updateFilter', 'group_id', value),
})
const endpointFilter = computed({
  get: () => props.filters.endpoint,
  set: (value: string) => emit('updateFilter', 'endpoint', value),
})
const searchFilter = computed({
  get: () => props.filters.search,
  set: (value: string) => emit('updateFilter', 'search', value),
})
const fromFilter = computed({
  get: () => props.filters.from,
  set: (value: string) => emit('updateFilter', 'from', value),
})
const toFilter = computed({
  get: () => props.filters.to,
  set: (value: string) => emit('updateFilter', 'to', value),
})
const inputDetailRow = ref<ContentModerationLog | null>(null)
const inputDetailText = computed(
  () => inputDetailRow.value?.input_excerpt || inputDetailRow.value?.error || '-',
)

function canUnbanRow(row: ContentModerationLog): boolean {
  return Boolean(row.auto_banned && row.user_id && row.user_status === 'disabled')
}

function inputSummaryText(row: ContentModerationLog): string {
  return row.input_excerpt || row.error || '-'
}

function resultLabel(row: ContentModerationLog): string {
  if (row.action === 'image_safety') return t('admin.riskControl.action.imageSafety')
  if (row.action === 'cyber_policy') return t('admin.riskControl.action.cyberPolicy')
  if (row.action === 'keyword_block') return t('admin.riskControl.action.keywordBlock')
  if (row.action === 'block') return t('admin.riskControl.action.block')
  if (row.action === 'error' || row.error) return t('admin.riskControl.action.error')
  if (row.flagged) return t('admin.riskControl.result.hit')
  return t('admin.riskControl.result.pass')
}

function resultBadgeClass(row: ContentModerationLog): string {
  if (row.action === 'block' || row.action === 'keyword_block' || row.action === 'cyber_policy' || row.action === 'image_safety') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (row.action === 'error' || row.error) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  if (row.flagged) return 'bg-pink-100 text-pink-700 dark:bg-pink-900/30 dark:text-pink-300'
  return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
}

function percent(value: number): string {
  if (!Number.isFinite(value)) return '-'
  return `${(value * 100).toFixed(1)}%`
}

function latencyText(value: number | null): string {
  if (value === null || value === undefined) return '-'
  return `${value} ms`
}

function violationCountText(row: ContentModerationLog): string {
  if (!row.flagged) return '-'
  if (row.violation_count === 0) return t('admin.riskControl.violationNotCounted')
  return t('admin.riskControl.violationCount', { count: row.violation_count || 1 })
}

function formatDateTime(value: string): string {
  return formatDateTimeValue(value) || '-'
}
</script>
