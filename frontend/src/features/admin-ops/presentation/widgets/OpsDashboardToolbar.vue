<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/common/widgets/forms/Select.vue'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import { getAll as getAllGroups } from '@/features/admin-groups/data/datasources/adminGroupQueries'

interface Props {
  platform: string
  groupId: number | null
  timeRange: string
  queryMode: string
  loading: boolean
  lastUpdated: Date | null
  autoRefreshEnabled?: boolean
  autoRefreshCountdown?: number
  fullscreen?: boolean
  customStartTime?: string | null
  customEndTime?: string | null
}

interface Emits {
  (event: 'update:platform', value: string): void
  (event: 'update:group', value: number | null): void
  (event: 'update:timeRange', value: string): void
  (event: 'update:queryMode', value: string): void
  (event: 'update:customTimeRange', startTime: string, endTime: string): void
  (event: 'refresh'): void
  (event: 'openSettings'): void
  (event: 'openAlertRules'): void
  (event: 'enterFullscreen'): void
  (event: 'exitFullscreen'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const { t } = useI18n()

// --- Filters ---

const showCustomTimeRangeDialog = ref(false)
const customStartTimeInput = ref('')
const customEndTimeInput = ref('')

function formatCustomTimeRangeLabel(startTime: string, endTime: string): string {
  const start = new Date(startTime)
  const end = new Date(endTime)
  const formatDate = (d: Date) => {
    const month = String(d.getMonth() + 1).padStart(2, '0')
    const day = String(d.getDate()).padStart(2, '0')
    const hour = String(d.getHours()).padStart(2, '0')
    const minute = String(d.getMinutes()).padStart(2, '0')
    return `${month}-${day} ${hour}:${minute}`
  }
  return `${formatDate(start)} ~ ${formatDate(end)}`
}

const groups = ref<Array<{ id: number; name: string; platform: string }>>([])

const platformOptions = computed(() => [
  { value: '', label: t('common.all') },
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' },
  { value: 'grok', label: 'Grok' }
])

const timeRangeOptions = computed(() => [
  { value: '5m', label: t('admin.ops.timeRange.5m') },
  { value: '30m', label: t('admin.ops.timeRange.30m') },
  { value: '1h', label: t('admin.ops.timeRange.1h') },
  { value: '6h', label: t('admin.ops.timeRange.6h') },
  { value: '24h', label: t('admin.ops.timeRange.24h') },
  {
    value: 'custom',
    label: props.timeRange === 'custom' && props.customStartTime && props.customEndTime
      ? `${t('admin.ops.timeRange.custom')} (${formatCustomTimeRangeLabel(props.customStartTime, props.customEndTime)})`
      : t('admin.ops.timeRange.custom')
  }
])

const queryModeOptions = computed(() => [
  { value: 'auto', label: t('admin.ops.queryMode.auto') },
  { value: 'raw', label: t('admin.ops.queryMode.raw') },
  { value: 'preagg', label: t('admin.ops.queryMode.preagg') }
])

const groupOptions = computed(() => {
  const filtered = props.platform ? groups.value.filter((g) => g.platform === props.platform) : groups.value
  return [{ value: null, label: t('common.all') }, ...filtered.map((g) => ({ value: g.id, label: g.name }))]
})

watch(
  () => props.platform,
  (newPlatform) => {
    if (!newPlatform) return
    const currentGroup = groups.value.find((g) => g.id === props.groupId)
    if (currentGroup && currentGroup.platform !== newPlatform) {
      emit('update:group', null)
    }
  }
)

onMounted(async () => {
  try {
    const list = await getAllGroups()
    groups.value = list.map((g) => ({ id: g.id, name: g.name, platform: g.platform }))
  } catch (e) {
    console.error('[OpsDashboardHeader] Failed to load groups', e)
    groups.value = []
  }
})

function handlePlatformChange(val: string | number | boolean | null) {
  emit('update:platform', String(val || ''))
}

function handleGroupChange(val: string | number | boolean | null) {
  if (val === null || val === '' || typeof val === 'boolean') {
    emit('update:group', null)
    return
  }
  const id = typeof val === 'number' ? val : Number.parseInt(String(val), 10)
  emit('update:group', Number.isFinite(id) && id > 0 ? id : null)
}

function handleTimeRangeChange(val: string | number | boolean | null) {
  const newValue = String(val || '1h')
  if (newValue === 'custom') {
    // 初始化为最近1小时
    const now = new Date()
    const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000)
    customStartTimeInput.value = oneHourAgo.toISOString().slice(0, 16)
    customEndTimeInput.value = now.toISOString().slice(0, 16)
    showCustomTimeRangeDialog.value = true
  } else {
    emit('update:timeRange', newValue)
  }
}

function handleCustomTimeRangeConfirm() {
  if (!customStartTimeInput.value || !customEndTimeInput.value) return
  const startTime = new Date(customStartTimeInput.value).toISOString()
  const endTime = new Date(customEndTimeInput.value).toISOString()
  // Emit custom time range first so the parent can build correct API params
  // when it reacts to timeRange switching to "custom".
  emit('update:customTimeRange', startTime, endTime)
  emit('update:timeRange', 'custom')
  showCustomTimeRangeDialog.value = false
}

function handleCustomTimeRangeCancel() {
  showCustomTimeRangeDialog.value = false
  // 如果当前不是 custom，不需要做任何事
  // 如果当前是 custom，保持不变
}

function handleQueryModeChange(val: string | number | boolean | null) {
  emit('update:queryMode', String(val || 'auto'))
}

function handleToolbarRefresh() {
  emit('refresh')
}
</script>

<template>
  <div>
  <!-- Top Toolbar -->
      <div class="flex flex-wrap items-center justify-between gap-4 border-b border-gray-100 pb-4 dark:border-dark-700">
        <div>
          <h1 class="flex items-center gap-2 text-xl font-black text-gray-900 dark:text-white">
            <svg class="h-6 w-6 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 01-2 2h-2a2 2 0 01-2-2z"
              />
            </svg>
            {{ t('admin.ops.title') }}
          </h1>

          <div v-if="!props.fullscreen" class="mt-1 flex items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
            <span class="flex items-center gap-1.5" :title="props.loading ? t('admin.ops.loadingText') : t('admin.ops.ready')">
              <span class="relative flex h-2 w-2">
                <span class="relative inline-flex h-2 w-2 rounded-full" :class="props.loading ? 'bg-gray-400' : 'bg-green-500'"></span>
              </span>
              {{ props.loading ? t('admin.ops.loadingText') : t('admin.ops.ready') }}
            </span>

            <span>·</span>
            <span>{{ t('common.refresh') }}: {{ props.lastUpdated ? props.lastUpdated.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).replace(/\//g, '-') : t('common.unknown') }}</span>

            <template v-if="props.autoRefreshEnabled && props.autoRefreshCountdown !== undefined">
              <span>·</span>
              <span>{{ t('admin.ops.autoRefreshRemaining', { seconds: props.autoRefreshCountdown }) }}</span>
            </template>
          </div>
        </div>

        <div class="flex flex-wrap items-center gap-3">
          <template v-if="!props.fullscreen">
            <Select
              :model-value="platform"
              :options="platformOptions"
              class="w-full sm:w-[140px]"
              @update:model-value="handlePlatformChange"
            />

            <Select
              :model-value="groupId"
              :options="groupOptions"
              class="w-full sm:w-[160px]"
              @update:model-value="handleGroupChange"
            />

            <div class="mx-1 hidden h-4 w-[1px] bg-gray-200 dark:bg-dark-700 sm:block"></div>

            <Select
              :model-value="timeRange"
              :options="timeRangeOptions"
              class="relative w-full sm:w-[150px]"
              @update:model-value="handleTimeRangeChange"
            />
          </template>

          <Select
            v-if="false"
            :model-value="queryMode"
            :options="queryModeOptions"
            class="relative w-full sm:w-[170px]"
            @update:model-value="handleQueryModeChange"
          />

          <button
            v-if="!props.fullscreen"
            type="button"
            class="flex h-8 w-8 items-center justify-center rounded-lg bg-gray-100 text-gray-500 transition-colors hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600"
            :disabled="loading"
            :title="t('common.refresh')"
            @click="handleToolbarRefresh"
          >
            <svg class="h-4 w-4" :class="{ 'animate-spin': loading }" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
              />
            </svg>
          </button>

          <div v-if="!props.fullscreen" class="mx-1 hidden h-4 w-[1px] bg-gray-200 dark:bg-dark-700 sm:block"></div>

          <!-- Alert Rules Button (hidden in fullscreen) -->
          <button
            v-if="!props.fullscreen"
            type="button"
            class="flex h-8 items-center gap-1.5 rounded-lg bg-blue-100 px-3 text-xs font-bold text-blue-700 transition-colors hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-400 dark:hover:bg-blue-900/50"
            :title="t('admin.ops.alertRules.title')"
            @click="emit('openAlertRules')"
          >
            <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
            </svg>
            <span class="hidden sm:inline">{{ t('admin.ops.alertRules.manage') }}</span>
          </button>

          <!-- Settings Button (hidden in fullscreen) -->
          <button
            v-if="!props.fullscreen"
            type="button"
            class="flex h-8 items-center gap-1.5 rounded-lg bg-gray-100 px-3 text-xs font-bold text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
            :title="t('admin.ops.settings.title')"
            @click="emit('openSettings')"
          >
            <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
            </svg>
            <span class="hidden sm:inline">{{ t('common.settings') }}</span>
          </button>

          <!-- Enter Fullscreen Button (hidden in fullscreen mode) -->
          <button
            v-if="!props.fullscreen"
            type="button"
            class="flex h-8 w-8 items-center justify-center rounded-lg bg-gray-100 text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
            :title="t('admin.ops.fullscreen.enter')"
            @click="emit('enterFullscreen')"
          >
            <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 8V4m0 0h4M4 4l5 5m11-1V4m0 0h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4" />
            </svg>
          </button>
        </div>
      </div>

  <!-- Custom Time Range Dialog -->
      <BaseDialog :show="showCustomTimeRangeDialog" :title="t('admin.ops.timeRange.custom')" width="narrow" @close="handleCustomTimeRangeCancel">
        <div class="space-y-4">
          <div>
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              {{ t('admin.ops.customTimeRange.startTime') }}
            </label>
            <input
              v-model="customStartTimeInput"
              type="datetime-local"
              class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-dark-600 dark:bg-dark-800 dark:text-white"
            />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              {{ t('admin.ops.customTimeRange.endTime') }}
            </label>
            <input
              v-model="customEndTimeInput"
              type="datetime-local"
              class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-dark-600 dark:bg-dark-800 dark:text-white"
            />
          </div>
          <div class="flex justify-end gap-3 pt-2">
            <button
              type="button"
              class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
              @click="handleCustomTimeRangeCancel"
            >
              {{ t('common.cancel') }}
            </button>
            <button
              type="button"
              class="rounded-lg bg-blue-500 px-4 py-2 text-sm font-medium text-white hover:bg-blue-600"
              @click="handleCustomTimeRangeConfirm"
            >
              {{ t('common.confirm') }}
            </button>
          </div>
        </div>
      </BaseDialog>
  </div>
</template>
