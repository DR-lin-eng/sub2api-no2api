<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-3">
          <div class="flex flex-wrap items-center gap-3">
            <SearchInput
              v-model="filterSearch"
              :placeholder="t('keys.searchPlaceholder')"
              class="w-full sm:w-64"
              @search="onFilterChange"
            />
            <Select
              :model-value="filterGroupId"
              class="w-40"
              :options="groupFilterOptions"
              @update:model-value="onGroupFilterChange"
            />
            <Select
              :model-value="filterStatus"
              class="w-40"
              :options="statusFilterOptions"
              @update:model-value="onStatusFilterChange"
            />
          </div>
          <EndpointPopover
            v-if="publicSettings?.api_base_url || (publicSettings?.custom_endpoints?.length ?? 0) > 0"
            :api-base-url="publicSettings?.api_base_url || ''"
            :custom-endpoints="publicSettings?.custom_endpoints || []"
          />
        </div>
      </template>

      <template #actions>
        <div class="flex justify-end gap-3">
          <button
            @click="loadApiKeys"
            :disabled="loading"
            class="btn btn-secondary"
            :title="t('common.refresh')"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
          <div class="relative" ref="columnDropdownRef">
            <button
              @click="showColumnDropdown = !showColumnDropdown"
              class="btn btn-secondary px-2 md:px-3"
              :title="t('keys.columnSettings')"
            >
              <svg class="h-4 w-4 md:mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9 4.5v15m6-15v15m-10.875 0h15.75c.621 0 1.125-.504 1.125-1.125V5.625c0-.621-.504-1.125-1.125-1.125H4.125C3.504 4.5 3 5.004 3 5.625v12.75c0 .621.504 1.125 1.125 1.125z" />
              </svg>
              <span class="hidden md:inline">{{ t('keys.columnSettings') }}</span>
            </button>
            <div
              v-if="showColumnDropdown"
              class="absolute right-0 top-full z-50 mt-1 max-h-80 w-48 overflow-y-auto rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800"
            >
              <button
                v-for="col in toggleableColumns"
                :key="col.key"
                @click="toggleColumn(col.key)"
                class="flex w-full items-center justify-between px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <span>{{ col.label }}</span>
                <Icon
                  v-if="isColumnVisible(col.key)"
                  name="check"
                  size="sm"
                  class="text-primary-500"
                  :stroke-width="2"
                />
              </button>
            </div>
          </div>
          <button @click="showCreateModal = true" class="btn btn-primary" data-tour="keys-create-btn">
            <Icon name="plus" size="md" class="mr-2" />
            {{ t('keys.createKey') }}
          </button>
        </div>
      </template>

      <template #table>
        <KeysTable :context="keysTableContext" />
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <KeyEditorDialog :context="keyEditorDialogContext" />

    <KeyGroupBindingsDialog
      v-model="groupEditorBindings"
      :show="showGroupEditor"
      :api-key-name="groupEditorKey?.name || ''"
      :group-options="groupOptions"
      :saving="groupEditorSaving"
      @close="closeGroupEditor"
      @save="saveKeyGroups"
    />

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('keys.deleteKey')"
      :message="t('keys.deleteConfirmMessage', { name: selectedKey?.name })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="handleDelete"
      @cancel="showDeleteDialog = false"
    />

    <!-- Reset Quota Confirmation Dialog -->
    <ConfirmDialog
      :show="showResetQuotaDialog"
      :title="t('keys.resetQuotaTitle')"
      :message="t('keys.resetQuotaConfirmMessage', { name: selectedKey?.name, used: selectedKey?.quota_used?.toFixed(4) })"
      :confirm-text="t('keys.reset')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="resetQuotaUsed"
      @cancel="showResetQuotaDialog = false"
    />

    <!-- Reset Rate Limit Confirmation Dialog -->
    <ConfirmDialog
      :show="showResetRateLimitDialog"
      :title="t('keys.resetRateLimitTitle')"
      :message="t('keys.resetRateLimitConfirmMessage', { name: selectedKey?.name })"
      :confirm-text="t('keys.reset')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="resetRateLimitUsage"
      @cancel="showResetRateLimitDialog = false"
    />

    <!-- Use Key Modal -->
    <UseKeyModal
      :show="showUseKeyModal"
      :api-key="selectedKey?.key || ''"
      :base-url="publicSettings?.api_base_url || ''"
      :platform="selectedKey?.group?.platform || null"
      :allow-messages-dispatch="selectedKey?.group?.allow_messages_dispatch || false"
      @close="closeUseKeyModal"
    />

    <!-- CCS Client Selection Dialog for Antigravity -->
    <BaseDialog
      :show="showCcsClientSelect"
      :title="t('keys.ccsClientSelect.title')"
      width="narrow"
      @close="closeCcsClientSelect"
    >
      <div class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-400">
          {{ t('keys.ccsClientSelect.description') }}
	        </p>
	        <div class="grid grid-cols-2 gap-3">
	          <button
	            @click="handleCcsClientSelect('claude')"
	            class="flex flex-col items-center gap-2 p-4 rounded-xl border-2 border-gray-200 dark:border-dark-600 hover:border-primary-500 dark:hover:border-primary-500 hover:bg-primary-50 dark:hover:bg-primary-900/20 transition-all"
	          >
	            <Icon name="terminal" size="xl" class="text-gray-600 dark:text-gray-400" />
	            <span class="font-medium text-gray-900 dark:text-white">{{
	              t('keys.ccsClientSelect.claudeCode')
	            }}</span>
	            <span class="text-xs text-gray-500 dark:text-gray-400">{{
	              t('keys.ccsClientSelect.claudeCodeDesc')
	            }}</span>
	          </button>
	          <button
	            @click="handleCcsClientSelect('gemini')"
	            class="flex flex-col items-center gap-2 p-4 rounded-xl border-2 border-gray-200 dark:border-dark-600 hover:border-primary-500 dark:hover:border-primary-500 hover:bg-primary-50 dark:hover:bg-primary-900/20 transition-all"
	          >
	            <Icon name="sparkles" size="xl" class="text-gray-600 dark:text-gray-400" />
	            <span class="font-medium text-gray-900 dark:text-white">{{
	              t('keys.ccsClientSelect.geminiCli')
	            }}</span>
	            <span class="text-xs text-gray-500 dark:text-gray-400">{{
	              t('keys.ccsClientSelect.geminiCliDesc')
	            }}</span>
	          </button>
	        </div>
	      </div>
      <template #footer>
        <div class="flex justify-end">
          <button @click="closeCcsClientSelect" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
        </div>
      </template>
    </BaseDialog>

  </AppLayout>
</template>

<script setup lang="ts">
	import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
	import { useI18n } from 'vue-i18n'
	import { useAppStore } from '@/core/stores/appStore'
	import { useOnboardingStore } from '@/core/stores/onboardingStore'
	import { useClipboard } from '@/common/composables/useClipboard'
import { getPersistedPageSize } from '@/common/composables/usePersistedPageSize'

const { t } = useI18n()
import { keysAPI, authAPI, usageAPI, userGroupsAPI } from '@/api'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import TablePageLayout from '@/common/widgets/layout/TablePageLayout.vue'
	import Pagination from '@/common/widgets/data/Pagination.vue'
	import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
	import ConfirmDialog from '@/common/widgets/feedback/ConfirmDialog.vue'
	import Select from '@/common/widgets/forms/Select.vue'
	import SearchInput from '@/common/widgets/forms/SearchInput.vue'
	import Icon from '@/common/widgets/icons/Icon.vue'
	import UseKeyModal from '@/features/keys/presentation/widgets/UseKeyDialog.vue'
	import EndpointPopover from '@/features/keys/presentation/widgets/EndpointPopover.vue'
import KeyEditorDialog from '@/features/keys/presentation/widgets/KeyEditorDialog.vue'
import KeyGroupBindingsDialog from '@/features/keys/presentation/widgets/KeyGroupBindingsDialog.vue'
import KeysTable from '@/features/keys/presentation/widgets/KeysTable.vue'
	import type { ApiKey, ApiKeyGroupBinding, Group, PublicSettings, UpdateApiKeyRequest } from '@/types'
import type { Column } from '@/common/types/uiTypes'
import type {
  BatchApiKeysUsageResponse,
  BatchApiKeyUsageStats
} from '@/features/usage/data/datasources/usageDatasource'
import type {
  KeyEditorDialogContext,
  KeysTableContext
} from '@/features/keys/presentation/keysPageContext'
import {
  buildCcSwitchImportDeeplink,
  type CcSwitchClientType
} from '@/core/utils/ccswitchImport'

// Helper to format date for datetime-local input
const formatDateTimeLocal = (isoDate: string): string => {
  const date = new Date(isoDate)
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

const appStore = useAppStore()
const onboardingStore = useOnboardingStore()
const { copyToClipboard: clipboardCopy } = useClipboard()

const allColumns = computed<Column[]>(() => [
  { key: 'name', label: t('common.name'), sortable: true },
  { key: 'id', label: t('keys.id'), sortable: true },
  { key: 'key', label: t('keys.apiKey'), sortable: false },
  { key: 'group', label: t('keys.groupRouting'), sortable: false },
  { key: 'current_concurrency', label: t('keys.currentConcurrency'), sortable: true },
  { key: 'usage', label: t('keys.usage'), sortable: false },
  { key: 'rate_limit', label: t('keys.rateLimitColumn'), sortable: false },
  { key: 'expires_at', label: t('keys.expiresAt'), sortable: true },
  { key: 'status', label: t('common.status'), sortable: true },
  { key: 'last_used_at', label: t('keys.lastUsedAt'), sortable: true },
  { key: 'last_used_ip', label: t('keys.lastUsedIP'), sortable: false },
  { key: 'created_at', label: t('keys.created'), sortable: true },
  { key: 'actions', label: t('common.actions'), sortable: false }
])

const ALWAYS_VISIBLE_COLUMNS = new Set(['name', 'actions'])
const DEFAULT_HIDDEN_COLUMNS = ['id', 'rate_limit', 'last_used_at', 'last_used_ip']
const HIDDEN_COLUMNS_KEY = 'api-key-hidden-columns'
const COLUMN_SETTINGS_VERSION_KEY = 'api-key-column-settings-version'
const COLUMN_SETTINGS_VERSION = 3
const VERSION_NEW_HIDDEN_COLUMNS: Record<number, string[]> = {
  2: ['last_used_ip'],
  3: ['id']
}

const toggleableColumns = computed(() =>
  allColumns.value.filter((col) => !ALWAYS_VISIBLE_COLUMNS.has(col.key))
)

const hiddenColumns = reactive<Set<string>>(new Set())

const saveColumnsToStorage = () => {
  try {
    localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns]))
    localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
  } catch (error) {
    console.error('Failed to save API key table columns:', error)
  }
}

const loadSavedColumns = () => {
  hiddenColumns.clear()
  try {
    const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY)
    if (saved) {
      const parsed = JSON.parse(saved) as string[]
      const validColumnKeys = new Set(allColumns.value.map((col) => col.key))
      parsed
        .filter((key) =>
          typeof key === 'string' &&
          validColumnKeys.has(key) &&
          !ALWAYS_VISIBLE_COLUMNS.has(key)
        )
        .forEach((key) => hiddenColumns.add(key))
      const storedVersion = Number(localStorage.getItem(COLUMN_SETTINGS_VERSION_KEY) ?? '1')
      if (storedVersion < COLUMN_SETTINGS_VERSION) {
        for (let v = storedVersion + 1; v <= COLUMN_SETTINGS_VERSION; v++) {
          for (const key of VERSION_NEW_HIDDEN_COLUMNS[v] ?? []) {
            if (validColumnKeys.has(key) && !ALWAYS_VISIBLE_COLUMNS.has(key)) {
              hiddenColumns.add(key)
            }
          }
        }
        saveColumnsToStorage()
      } else {
        localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
      }
    } else {
      DEFAULT_HIDDEN_COLUMNS.forEach((key) => hiddenColumns.add(key))
      localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
    }
  } catch (error) {
    console.error('Failed to load API key table columns:', error)
    DEFAULT_HIDDEN_COLUMNS.forEach((key) => hiddenColumns.add(key))
  }
}

const toggleColumn = (key: string) => {
  if (ALWAYS_VISIBLE_COLUMNS.has(key)) return
  if (hiddenColumns.has(key)) {
    hiddenColumns.delete(key)
  } else {
    hiddenColumns.add(key)
  }
  saveColumnsToStorage()
}

const isColumnVisible = (key: string) => !hiddenColumns.has(key)

const columns = computed<Column[]>(() =>
  allColumns.value.filter((col) => ALWAYS_VISIBLE_COLUMNS.has(col.key) || !hiddenColumns.has(col.key))
)

const apiKeys = ref<ApiKey[]>([])
const groups = ref<Group[]>([])
const loading = ref(false)
const submitting = ref(false)
const now = ref(new Date())
let resetTimer: ReturnType<typeof setInterval> | null = null
let usageRefreshTimer: ReturnType<typeof setTimeout> | null = null
let usageRefreshAbortController: AbortController | null = null
let lastFullUsageRefreshAt = 0
const usageStats = ref<Record<string, BatchApiKeyUsageStats>>({})
const usageStatsLoadingKeyIds = ref<Set<number>>(new Set())
const usageStatsErrorKeyIds = ref<Set<number>>(new Set())
const pendingUsageAvailable = ref(true)
const userGroupRates = ref<Record<number, number>>({})

const isUsageStatsLoading = (apiKeyId: number) => usageStatsLoadingKeyIds.value.has(apiKeyId)
const hasUsageStatsError = (apiKeyId: number) => usageStatsErrorKeyIds.value.has(apiKeyId)
const pendingUsage = (apiKeyId: number) => Number(usageStats.value[apiKeyId]?.pending_actual_cost ?? 0)
const usageCost = (apiKeyId: number, field: 'today_actual_cost' | 'total_actual_cost') =>
  Number(usageStats.value[apiKeyId]?.[field] ?? 0)
const quotaUsedWithPending = (apiKey: ApiKey) => {
  const settled = Number(apiKey.quota_used ?? 0)
  return settled + (pendingUsageAvailable.value ? pendingUsage(apiKey.id) : 0)
}

const pagination = ref({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})
const sortState = ref({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

// Filter state
const filterSearch = ref('')
const filterStatus = ref('')
const filterGroupId = ref<string | number>('')

const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const showResetQuotaDialog = ref(false)
const showResetRateLimitDialog = ref(false)
const showUseKeyModal = ref(false)
const showCcsClientSelect = ref(false)
const showColumnDropdown = ref(false)
const pendingCcsRow = ref<ApiKey | null>(null)
const selectedKey = ref<ApiKey | null>(null)
const copiedKeyId = ref<number | null>(null)
const showGroupEditor = ref(false)
const groupEditorKey = ref<ApiKey | null>(null)
const groupEditorBindings = ref<ApiKeyGroupBinding[]>([])
const groupEditorSaving = ref(false)
const publicSettings = ref<PublicSettings | null>(null)
const columnDropdownRef = ref<HTMLElement | null>(null)
let abortController: AbortController | null = null

const ACTIVE_PENDING_REFRESH_MS = 5000
const IDLE_PENDING_REFRESH_MS = 60000
const FULL_USAGE_REFRESH_MS = 60000
const PENDING_USAGE_EPSILON = 0.0000000001
const USAGE_STATS_BATCH_SIZE = 5
const USAGE_STATS_REQUEST_CONCURRENCY = 2

const formData = ref({
  name: '',
  group_id: null as number | null,
  group_bindings: [] as ApiKeyGroupBinding[],
  status: 'active' as 'active' | 'inactive',
  use_custom_key: false,
  custom_key: '',
  enable_ip_restriction: false,
  ip_whitelist: '',
  ip_blacklist: '',
  // Quota settings (empty = unlimited)
  enable_quota: false,
  quota: null as number | null,
  concurrency_limit: 0,
  // Rate limit settings
  enable_rate_limit: false,
  rate_limit_5h: null as number | null,
  rate_limit_1d: null as number | null,
  rate_limit_7d: null as number | null,
  enable_expiration: false,
  expiration_preset: '30' as '7' | '30' | '90' | 'custom',
  expiration_date: ''
})

// 自定义Key验证
const customKeyError = computed(() => {
  if (!formData.value.use_custom_key || !formData.value.custom_key) {
    return ''
  }
  const key = formData.value.custom_key
  if (key.length < 16) {
    return t('keys.customKeyTooShort')
  }
  // 检查字符：只允许字母、数字、下划线、连字符
  if (!/^[a-zA-Z0-9_-]+$/.test(key)) {
    return t('keys.customKeyInvalidChars')
  }
  return ''
})

const statusOptions = computed(() => [
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') }
])

const shouldSubmitEditStatus = (key: ApiKey, status: 'active' | 'inactive') => {
  if (key.status === 'quota_exhausted' || key.status === 'expired') {
    return status === 'active'
  }
  return true
}

// Filter dropdown options
const groupFilterOptions = computed(() => [
  { value: '', label: t('keys.allGroups') },
  { value: 0, label: t('keys.noGroup') },
  ...groups.value.map((g) => ({ value: g.id, label: g.name }))
])

const statusFilterOptions = computed(() => [
  { value: '', label: t('keys.allStatus') },
  { value: 'active', label: t('keys.status.active') },
  { value: 'inactive', label: t('keys.status.inactive') },
  { value: 'quota_exhausted', label: t('keys.status.quota_exhausted') },
  { value: 'expired', label: t('keys.status.expired') }
])

const onFilterChange = () => {
  pagination.value.page = 1
  loadApiKeys()
}

const onGroupFilterChange = (value: string | number | boolean | null) => {
  filterGroupId.value = value as string | number
  onFilterChange()
}

const onStatusFilterChange = (value: string | number | boolean | null) => {
  filterStatus.value = value as string
  onFilterChange()
}

// Convert groups to Select options format with rate multiplier and subscription type
const groupOptions = computed(() =>
  groups.value.map((group) => ({
    value: group.id,
    label: group.name,
    description: group.description,
    rate: group.rate_multiplier,
    userRate: userGroupRates.value[group.id] ?? null,
    peakRateEnabled: group.peak_rate_enabled,
    peakStart: group.peak_start,
    peakEnd: group.peak_end,
    peakRateMultiplier: group.peak_rate_multiplier,
    subscriptionType: group.subscription_type,
    platform: group.platform
  }))
)

const copyToClipboard = async (text: string, keyId: number) => {
  const success = await clipboardCopy(text, t('keys.copied'))
  if (success) {
    copiedKeyId.value = keyId
    setTimeout(() => {
      copiedKeyId.value = null
    }, 800)
  }
}

const isAbortError = (error: unknown) => {
  if (!error || typeof error !== 'object') return false
  const { name, code } = error as { name?: string; code?: string }
  return name === 'AbortError' || code === 'ERR_CANCELED'
}

const updateUsageKeySet = (current: Set<number>, apiKeyIds: number[], enabled: boolean) => {
  const next = new Set(current)
  for (const apiKeyId of apiKeyIds) {
    if (enabled) {
      next.add(apiKeyId)
    } else {
      next.delete(apiKeyId)
    }
  }
  return next
}

const loadUsageStatsInBatches = async (apiKeyIds: number[], signal: AbortSignal) => {
  const batches: number[][] = []
  for (let start = 0; start < apiKeyIds.length; start += USAGE_STATS_BATCH_SIZE) {
    batches.push(apiKeyIds.slice(start, start + USAGE_STATS_BATCH_SIZE))
  }

  let nextBatchIndex = 0
  let receivedResponse = false
  let allPendingUsageAvailable = true
  const mergeUsageResponse = (
    apiKeyIdsForResponse: number[],
    response: BatchApiKeysUsageResponse
  ) => {
    receivedResponse = true
    usageStats.value = { ...usageStats.value, ...response.stats }
    allPendingUsageAvailable =
      allPendingUsageAvailable && response.pending_usage_available !== false
    usageStatsErrorKeyIds.value = updateUsageKeySet(
      usageStatsErrorKeyIds.value,
      apiKeyIdsForResponse,
      false
    )
  }

  const loadSingleKeyUsage = async (apiKeyId: number) => {
    try {
      const response = await usageAPI.getDashboardApiKeysUsage([apiKeyId], { signal })
      if (signal.aborted) return
      mergeUsageResponse([apiKeyId], response)
    } catch (error) {
      if (signal.aborted || isAbortError(error)) return
      if (!usageStats.value[apiKeyId]) {
        usageStatsErrorKeyIds.value = updateUsageKeySet(
          usageStatsErrorKeyIds.value,
          [apiKeyId],
          true
        )
      }
      console.error(`Failed to load API key ${apiKeyId} usage:`, error)
    } finally {
      if (!signal.aborted) {
        usageStatsLoadingKeyIds.value = updateUsageKeySet(
          usageStatsLoadingKeyIds.value,
          [apiKeyId],
          false
        )
      }
    }
  }

  const worker = async () => {
    while (!signal.aborted) {
      const batchIndex = nextBatchIndex
      nextBatchIndex += 1
      if (batchIndex >= batches.length) return

      const batch = batches[batchIndex]
      try {
        const response = await usageAPI.getDashboardApiKeysUsage(batch, { signal })
        if (signal.aborted) return
        mergeUsageResponse(batch, response)
      } catch (error) {
        if (signal.aborted || isAbortError(error)) return
        console.error('Failed to load API key usage batch:', error)
        for (const apiKeyId of batch) {
          if (signal.aborted) return
          await loadSingleKeyUsage(apiKeyId)
        }
      } finally {
        if (!signal.aborted) {
          usageStatsLoadingKeyIds.value = updateUsageKeySet(
            usageStatsLoadingKeyIds.value,
            batch,
            false
          )
        }
      }
    }
  }

  const workerCount = Math.min(USAGE_STATS_REQUEST_CONCURRENCY, batches.length)
  await Promise.all(Array.from({ length: workerCount }, () => worker()))
  if (!signal.aborted && receivedResponse) {
    pendingUsageAvailable.value = allPendingUsageAvailable
  }
}

const hasPendingUsage = () =>
  pendingUsageAvailable.value &&
  apiKeys.value.some((apiKey) => pendingUsage(apiKey.id) > PENDING_USAGE_EPSILON)

const stopUsageRefresh = () => {
  if (usageRefreshTimer) {
    clearTimeout(usageRefreshTimer)
    usageRefreshTimer = null
  }
  usageRefreshAbortController?.abort()
  usageRefreshAbortController = null
}

const scheduleUsageRefresh = (delay?: number) => {
  if (document.hidden || apiKeys.value.length === 0) return
  if (usageRefreshTimer) clearTimeout(usageRefreshTimer)
  usageRefreshTimer = setTimeout(
    refreshVisibleUsage,
    delay ?? (hasPendingUsage() ? ACTIVE_PENDING_REFRESH_MS : IDLE_PENDING_REFRESH_MS)
  )
}

const applyPendingUsage = (costs: Record<string, number>) => {
  const next = { ...usageStats.value }
  for (const apiKey of apiKeys.value) {
    const current = next[apiKey.id]
    if (!current) continue
    next[apiKey.id] = {
      ...current,
      pending_actual_cost: Number(costs[apiKey.id] ?? 0)
    }
  }
  usageStats.value = next
}

const refreshVisibleUsage = async () => {
  usageRefreshTimer = null
  if (document.hidden || apiKeys.value.length === 0) return

  const controller = new AbortController()
  usageRefreshAbortController = controller
  const keyIds = apiKeys.value.map((apiKey) => apiKey.id)
  const hadPending = hasPendingUsage()
  try {
    const shouldRefreshFullStats = Date.now() - lastFullUsageRefreshAt >= FULL_USAGE_REFRESH_MS
    if (shouldRefreshFullStats) {
      await loadUsageStatsInBatches(keyIds, controller.signal)
      if (controller.signal.aborted) return
      lastFullUsageRefreshAt = Date.now()
    } else {
      const response = await usageAPI.getDashboardApiKeysPendingUsage(keyIds, { signal: controller.signal })
      if (controller.signal.aborted) return
      pendingUsageAvailable.value = response.pending_usage_available !== false
      if (pendingUsageAvailable.value) {
        applyPendingUsage(response.pending_actual_costs)
      }

      if (hadPending && !hasPendingUsage()) {
        await loadUsageStatsInBatches(keyIds, controller.signal)
        if (controller.signal.aborted) return
        lastFullUsageRefreshAt = Date.now()
      }
    }
  } catch (error) {
    if (!isAbortError(error)) {
      console.error('Failed to refresh API key usage:', error)
    }
  } finally {
    if (usageRefreshAbortController === controller) {
      usageRefreshAbortController = null
      scheduleUsageRefresh()
    }
  }
}

const handleUsageVisibilityChange = () => {
  if (document.hidden) {
    stopUsageRefresh()
    return
  }
  scheduleUsageRefresh(0)
}

const loadApiKeys = async () => {
  stopUsageRefresh()
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  const { signal } = controller
  loading.value = true
  usageStats.value = {}
  usageStatsLoadingKeyIds.value = new Set()
  usageStatsErrorKeyIds.value = new Set()
  pendingUsageAvailable.value = true
  try {
    // Build filters
    const filters: {
      search?: string
      status?: string
      group_id?: number | string
      sort_by?: string
      sort_order?: 'asc' | 'desc'
    } = {}
    if (filterSearch.value) filters.search = filterSearch.value
    if (filterStatus.value) filters.status = filterStatus.value
    if (filterGroupId.value !== '') filters.group_id = filterGroupId.value
    filters.sort_by = sortState.value.sort_by
    filters.sort_order = sortState.value.sort_order

    const response = await keysAPI.list(pagination.value.page, pagination.value.page_size, filters, {
      signal
    })
    if (signal.aborted) return
    const keyIds = response.items.map((apiKey) => apiKey.id)
    usageStatsLoadingKeyIds.value = new Set(keyIds)
    apiKeys.value = response.items
    pagination.value.total = response.total
    pagination.value.pages = response.pages

    // Usage aggregation can be much slower than the key list. Render the keys as soon as
    // the list request finishes and keep the usage column on its own loading state.
    if (abortController === controller) {
      loading.value = false
    }

    if (keyIds.length > 0) {
      await loadUsageStatsInBatches(keyIds, signal)
      if (signal.aborted) return
      lastFullUsageRefreshAt = Date.now()
    }
  } catch (error) {
    if (isAbortError(error)) {
      return
    }
    appStore.showError(t('keys.failedToLoad'))
  } finally {
    if (abortController === controller) {
      loading.value = false
      scheduleUsageRefresh()
    }
  }
}

const loadGroups = async () => {
  try {
    groups.value = await userGroupsAPI.getAvailable()
  } catch (error) {
    console.error('Failed to load groups:', error)
  }
}

const loadUserGroupRates = async () => {
  try {
    userGroupRates.value = await userGroupsAPI.getUserGroupRates()
  } catch (error) {
    console.error('Failed to load user group rates:', error)
  }
}

const loadPublicSettings = async () => {
  try {
    publicSettings.value = await authAPI.getPublicSettings()
  } catch (error) {
    console.error('Failed to load public settings:', error)
  }
}

const openUseKeyModal = (key: ApiKey) => {
  selectedKey.value = key
  showUseKeyModal.value = true
}

const closeUseKeyModal = () => {
  showUseKeyModal.value = false
  selectedKey.value = null
}

const handlePageChange = (page: number) => {
  pagination.value.page = page
  loadApiKeys()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.value.page_size = pageSize
  pagination.value.page = 1
  loadApiKeys()
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.value.sort_by = key
  sortState.value.sort_order = order
  pagination.value.page = 1
  loadApiKeys()
}

const editKey = (key: ApiKey) => {
  selectedKey.value = key
  const hasIPRestriction = (key.ip_whitelist?.length > 0) || (key.ip_blacklist?.length > 0)
  const hasExpiration = !!key.expires_at
  const groupBindings = key.group_bindings?.length
    ? key.group_bindings.map(binding => ({ ...binding }))
    : key.group_id
      ? [{ group_id: key.group_id, max_rate_multiplier: null }]
      : []
  formData.value = {
    name: key.name,
    group_id: groupBindings[0]?.group_id ?? null,
    group_bindings: groupBindings,
    status: key.status === 'quota_exhausted' || key.status === 'expired' ? 'inactive' : key.status,
    use_custom_key: false,
    custom_key: '',
    enable_ip_restriction: hasIPRestriction,
    ip_whitelist: (key.ip_whitelist || []).join('\n'),
    ip_blacklist: (key.ip_blacklist || []).join('\n'),
    enable_quota: key.quota > 0,
    quota: key.quota > 0 ? key.quota : null,
    concurrency_limit: key.concurrency_limit ?? 0,
    enable_rate_limit: (key.rate_limit_5h > 0) || (key.rate_limit_1d > 0) || (key.rate_limit_7d > 0),
    rate_limit_5h: key.rate_limit_5h || null,
    rate_limit_1d: key.rate_limit_1d || null,
    rate_limit_7d: key.rate_limit_7d || null,
    enable_expiration: hasExpiration,
    expiration_preset: 'custom',
    expiration_date: key.expires_at ? formatDateTimeLocal(key.expires_at) : ''
  }
  showEditModal.value = true
}

const toggleKeyStatus = async (key: ApiKey) => {
  const newStatus = key.status === 'active' ? 'inactive' : 'active'
  try {
    await keysAPI.toggleStatus(key.id, newStatus)
    appStore.showSuccess(
      newStatus === 'active' ? t('keys.keyEnabledSuccess') : t('keys.keyDisabledSuccess')
    )
    loadApiKeys()
  } catch {
    appStore.showError(t('keys.failedToUpdateStatus'))
  }
}

const bindingsForKey = (key: ApiKey): ApiKeyGroupBinding[] => key.group_bindings?.length
  ? key.group_bindings.map(binding => ({ ...binding }))
  : key.group_id
    ? [{ group_id: key.group_id, max_rate_multiplier: null }]
    : []

const manageKeyGroups = (key: ApiKey) => {
  groupEditorKey.value = key
  groupEditorBindings.value = bindingsForKey(key)
  showGroupEditor.value = true
}

const closeGroupEditor = () => {
  if (groupEditorSaving.value) return
  showGroupEditor.value = false
  groupEditorKey.value = null
  groupEditorBindings.value = []
}

const saveKeyGroups = async () => {
  const key = groupEditorKey.value
  if (!key || groupEditorSaving.value) return

  const bindings = groupEditorBindings.value.map(binding => ({ ...binding }))
  if (bindings.length === 0) {
    appStore.showError(t('keys.groupRequired'))
    return
  }
  if (bindings.some(binding => binding.max_rate_multiplier !== null && (
    !Number.isFinite(binding.max_rate_multiplier) || binding.max_rate_multiplier <= 0
  ))) {
    appStore.showError(t('keys.groupBindings.invalidRateProtection'))
    return
  }

  groupEditorSaving.value = true
  try {
    await keysAPI.update(key.id, {
      group_id: bindings[0]?.group_id ?? null,
      group_bindings: bindings
    })
    appStore.showSuccess(t('keys.groupBindings.updatedSuccess'))
    showGroupEditor.value = false
    groupEditorKey.value = null
    groupEditorBindings.value = []
    await loadApiKeys()
  } catch {
    appStore.showError(t('keys.groupBindings.updateFailed'))
  } finally {
    groupEditorSaving.value = false
  }
}

const closeColumnDropdown = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) {
    showColumnDropdown.value = false
  }
}

const confirmDelete = (key: ApiKey) => {
  selectedKey.value = key
  showDeleteDialog.value = true
}

const handleSubmit = async () => {
  const groupBindings = formData.value.group_bindings.map(binding => ({ ...binding }))
  const groupID = groupBindings[0]?.group_id ?? null

  if (groupBindings.length === 0) {
    appStore.showError(t('keys.groupRequired'))
    return
  }
  if (groupBindings.some(binding => binding.max_rate_multiplier !== null && (
    !Number.isFinite(binding.max_rate_multiplier) || binding.max_rate_multiplier <= 0
  ))) {
    appStore.showError(t('keys.groupBindings.invalidRateProtection'))
    return
  }

  // Validate custom key if enabled
  if (!showEditModal.value && formData.value.use_custom_key) {
    if (!formData.value.custom_key) {
      appStore.showError(t('keys.customKeyRequired'))
      return
    }
    if (customKeyError.value) {
      appStore.showError(customKeyError.value)
      return
    }
  }

  // Parse IP lists only if IP restriction is enabled
  const parseIPList = (text: string): string[] =>
    text.split('\n').map(ip => ip.trim()).filter(ip => ip.length > 0)
  const ipWhitelist = formData.value.enable_ip_restriction ? parseIPList(formData.value.ip_whitelist) : []
  const ipBlacklist = formData.value.enable_ip_restriction ? parseIPList(formData.value.ip_blacklist) : []

  // Calculate quota value (null/empty/0 = unlimited, stored as 0)
  const quota = formData.value.quota && formData.value.quota > 0 ? formData.value.quota : 0

  // Calculate expiration
  let expiresInDays: number | undefined
  let expiresAt: string | null | undefined
  if (formData.value.enable_expiration && formData.value.expiration_date) {
    if (!showEditModal.value) {
      // Create mode: calculate days from date
      const expDate = new Date(formData.value.expiration_date)
      const now = new Date()
      const diffDays = Math.ceil((expDate.getTime() - now.getTime()) / (1000 * 60 * 60 * 24))
      expiresInDays = diffDays > 0 ? diffDays : 1
    } else {
      // Edit mode: use custom date directly
      expiresAt = new Date(formData.value.expiration_date).toISOString()
    }
  } else if (showEditModal.value) {
    // Edit mode: if expiration disabled or date cleared, send empty string to clear
    expiresAt = ''
  }

  // Calculate rate limit values (send 0 when toggle is off)
  const rateLimitData = formData.value.enable_rate_limit ? {
    rate_limit_5h: formData.value.rate_limit_5h && formData.value.rate_limit_5h > 0 ? formData.value.rate_limit_5h : 0,
    rate_limit_1d: formData.value.rate_limit_1d && formData.value.rate_limit_1d > 0 ? formData.value.rate_limit_1d : 0,
    rate_limit_7d: formData.value.rate_limit_7d && formData.value.rate_limit_7d > 0 ? formData.value.rate_limit_7d : 0,
  } : { rate_limit_5h: 0, rate_limit_1d: 0, rate_limit_7d: 0 }

  submitting.value = true
  try {
    if (showEditModal.value && selectedKey.value) {
      const updates: UpdateApiKeyRequest = {
        name: formData.value.name,
        group_id: groupID,
        group_bindings: groupBindings,
        ip_whitelist: ipWhitelist,
        ip_blacklist: ipBlacklist,
        quota: quota,
        concurrency_limit: Math.max(0, Math.trunc(Number(formData.value.concurrency_limit) || 0)),
        expires_at: expiresAt,
        rate_limit_5h: rateLimitData.rate_limit_5h,
        rate_limit_1d: rateLimitData.rate_limit_1d,
        rate_limit_7d: rateLimitData.rate_limit_7d,
      }
      if (shouldSubmitEditStatus(selectedKey.value, formData.value.status)) {
        updates.status = formData.value.status
      }
      await keysAPI.update(selectedKey.value.id, updates)
      appStore.showSuccess(t('keys.keyUpdatedSuccess'))
    } else {
      const customKey = formData.value.use_custom_key ? formData.value.custom_key : undefined
      await keysAPI.create(
        formData.value.name,
        groupID,
        customKey,
        ipWhitelist,
        ipBlacklist,
        quota,
        expiresInDays,
        rateLimitData,
        groupBindings
      )
      appStore.showSuccess(t('keys.keyCreatedSuccess'))
      // Only advance tour if active, on submit step, and creation succeeded
      if (onboardingStore.isCurrentStep('[data-tour="key-form-submit"]')) {
        onboardingStore.nextStep(500)
      }
    }
    closeModals()
    loadApiKeys()
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToSave')
    appStore.showError(errorMsg)
    // Don't advance tour on error
  } finally {
    submitting.value = false
  }
}

/**
 * 处理删除 API Key 的操作
 * 优化：错误处理改进，优先显示后端返回的具体错误消息（如权限不足等），
 * 若后端未返回消息则显示默认的国际化文本
 */
const handleDelete = async () => {
  if (!selectedKey.value) return

  try {
    await keysAPI.delete(selectedKey.value.id)
    appStore.showSuccess(t('keys.keyDeletedSuccess'))
    showDeleteDialog.value = false
    loadApiKeys()
  } catch (error: any) {
    // 优先使用后端返回的错误消息，提供更具体的错误信息给用户
    const errorMsg = error?.message || t('keys.failedToDelete')
    appStore.showError(errorMsg)
  }
}

const closeModals = () => {
  showCreateModal.value = false
  showEditModal.value = false
  selectedKey.value = null
  formData.value = {
    name: '',
    group_id: null,
    group_bindings: [],
    status: 'active',
    use_custom_key: false,
    custom_key: '',
    enable_ip_restriction: false,
    ip_whitelist: '',
    ip_blacklist: '',
    enable_quota: false,
    quota: null,
    concurrency_limit: 0,
    enable_rate_limit: false,
    rate_limit_5h: null,
    rate_limit_1d: null,
    rate_limit_7d: null,
    enable_expiration: false,
    expiration_preset: '30',
    expiration_date: ''
  }
}

// Show reset quota confirmation dialog
const confirmResetQuota = () => {
  showResetQuotaDialog.value = true
}

// Set expiration date based on quick select days
const setExpirationDays = (days: number) => {
  formData.value.expiration_preset = days.toString() as '7' | '30' | '90'
  const expDate = new Date()
  expDate.setDate(expDate.getDate() + days)
  formData.value.expiration_date = formatDateTimeLocal(expDate.toISOString())
}

// Reset quota used for an API key
const resetQuotaUsed = async () => {
  if (!selectedKey.value) return
  showResetQuotaDialog.value = false
  try {
    await keysAPI.update(selectedKey.value.id, { reset_quota: true })
    appStore.showSuccess(t('keys.quotaResetSuccess'))
    // Update local state
    if (selectedKey.value) {
      selectedKey.value.quota_used = 0
    }
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToResetQuota')
    appStore.showError(errorMsg)
  }
}

// Show reset rate limit confirmation dialog (from edit modal)
const confirmResetRateLimit = () => {
  showResetRateLimitDialog.value = true
}

// Show reset rate limit confirmation dialog (from table row)
const confirmResetRateLimitFromTable = (row: ApiKey) => {
  selectedKey.value = row
  showResetRateLimitDialog.value = true
}

// Reset rate limit usage for an API key
const resetRateLimitUsage = async () => {
  if (!selectedKey.value) return
  showResetRateLimitDialog.value = false
  try {
    await keysAPI.update(selectedKey.value.id, { reset_rate_limit_usage: true })
    appStore.showSuccess(t('keys.rateLimitResetSuccess'))
    // Refresh key data
    await loadApiKeys()
    // Update the editing key with fresh data
    const refreshedKey = apiKeys.value.find(k => k.id === selectedKey.value!.id)
    if (refreshedKey) {
      selectedKey.value = refreshedKey
    }
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToResetRateLimit')
    appStore.showError(errorMsg)
  }
}

const importToCcswitch = (row: ApiKey) => {
  const platform = row.group?.platform || 'anthropic'

  // For antigravity platform, show client selection dialog
  if (platform === 'antigravity') {
    pendingCcsRow.value = row
    showCcsClientSelect.value = true
    return
  }

  // For other platforms, execute directly
  executeCcsImport(row, platform === 'gemini' ? 'gemini' : 'claude')
}

const executeCcsImport = (row: ApiKey, clientType: CcSwitchClientType) => {
  const baseUrl = publicSettings.value?.api_base_url || window.location.origin
  const platform = row.group?.platform || 'anthropic'

  const usageScript = `({
    request: {
      url: "{{baseUrl}}/v1/usage",
      method: "GET",
      headers: { "Authorization": "Bearer {{apiKey}}" }
    },
    extractor: function(response) {
      const remaining = response?.remaining ?? response?.quota?.remaining ?? response?.balance;
      const unit = response?.unit ?? response?.quota?.unit ?? "USD";
      return {
        isValid: response?.is_active ?? response?.isValid ?? true,
        remaining,
        unit
      };
    }
  })`
  const providerName = (publicSettings.value?.site_name || 'sub2api').trim() || 'sub2api'
  const deeplink = buildCcSwitchImportDeeplink({
    baseUrl,
    platform,
    clientType,
    providerName,
    apiKey: row.key,
    usageScript
  })

  try {
    window.open(deeplink, '_self')

    // Check if the protocol handler worked by detecting if we're still focused
    setTimeout(() => {
      if (document.hasFocus()) {
        // Still focused means the protocol handler likely failed
        appStore.showError(t('keys.ccSwitchNotInstalled'))
      }
    }, 100)
  } catch {
    appStore.showError(t('keys.ccSwitchNotInstalled'))
  }
}

const handleCcsClientSelect = (clientType: CcSwitchClientType) => {
  if (pendingCcsRow.value) {
    executeCcsImport(pendingCcsRow.value, clientType)
  }
  showCcsClientSelect.value = false
  pendingCcsRow.value = null
}

const closeCcsClientSelect = () => {
  showCcsClientSelect.value = false
  pendingCcsRow.value = null
}

function formatResetTime(resetAt: string | null): string {
  if (!resetAt) return ''
  const diff = new Date(resetAt).getTime() - now.value.getTime()
  if (diff <= 0) return t('keys.resetNow')
  const days = Math.floor(diff / 86400000)
  const hours = Math.floor((diff % 86400000) / 3600000)
  const mins = Math.floor((diff % 3600000) / 60000)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

const keysTableContext: KeysTableContext = {
  columns,
  apiKeys,
  loading,
  copiedKeyId,
  copyToClipboard,
  groupOptions,
  manageKeyGroups,
  isUsageStatsLoading,
  hasUsageStatsError,
  pendingUsageAvailable,
  usageCost,
  pendingUsage,
  quotaUsedWithPending,
  confirmResetRateLimitFromTable,
  formatResetTime,
  publicSettings,
  openUseKeyModal,
  importToCcswitch,
  toggleKeyStatus,
  editKey,
  confirmDelete,
  showCreateModal,
  handleSort
}

const keyEditorDialogContext: KeyEditorDialogContext = {
  showCreateModal,
  showEditModal,
  selectedKey,
  formData,
  groupOptions,
  customKeyError,
  statusOptions,
  submitting,
  closeModals,
  handleSubmit,
  confirmResetQuota,
  confirmResetRateLimit,
  setExpirationDays
}

onMounted(() => {
  loadSavedColumns()
  loadApiKeys()
  loadGroups()
  loadUserGroupRates()
  loadPublicSettings()
  document.addEventListener('click', closeColumnDropdown)
  document.addEventListener('visibilitychange', handleUsageVisibilityChange)
  resetTimer = setInterval(() => { now.value = new Date() }, 60000)
})

onUnmounted(() => {
  document.removeEventListener('click', closeColumnDropdown)
  document.removeEventListener('visibilitychange', handleUsageVisibilityChange)
  stopUsageRefresh()
  abortController?.abort()
  if (resetTimer) clearInterval(resetTimer)
})
</script>
