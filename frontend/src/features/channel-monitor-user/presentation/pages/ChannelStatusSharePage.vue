<template>
  <div class="min-h-screen bg-gray-50 dark:bg-dark-950">
    <main class="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
      <section class="mb-6 space-y-2">
        <h1 class="text-2xl font-semibold tracking-tight text-gray-900 dark:text-gray-50">
          {{ t('channelStatus.title') }}
        </h1>
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('channelStatus.description') }}
        </p>
      </section>

      <MonitorHero
        :overall-status="overallStatus"
        :interval-seconds="DEFAULT_INTERVAL_SECONDS"
        :window="currentWindow"
        :loading="loading"
        :auto-refresh="autoRefresh"
        @update:window="handleWindowChange"
        @refresh="manualReload"
      />

      <MonitorCardGrid
        :items="items"
        :window="currentWindow"
        :countdown-seconds="countdown"
        :loading="loading"
        :detail-cache="detailCache"
        @card-click="openDetail"
      />

      <MonitorDetailDialog
        :show="showDetail"
        :monitor-id="detailTarget?.id ?? null"
        :title="detailTitle"
        :initial-detail="detailTarget ? detailCache[detailTarget.id] : null"
        @close="closeDetail"
      />
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/core/stores/appStore'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import {
  listShared as listSharedChannelMonitorViews,
  statusBatchShared as fetchSharedChannelMonitorDetails,
  type UserMonitorView,
  type UserMonitorDetail,
} from '@/features/channel-monitor-user/data/datasources/channelMonitorUserDatasource'
import MonitorHero, {
  type MonitorWindow,
  type OverallStatus,
} from '@/features/channel-monitor-user/presentation/widgets/MonitorHero.vue'
import MonitorCardGrid from '@/features/channel-monitor-user/presentation/widgets/MonitorCardGrid.vue'
import MonitorDetailDialog from '@/features/channel-monitor-user/presentation/widgets/MonitorDetailDialog.vue'
import { DEFAULT_INTERVAL_SECONDS, STATUS_OPERATIONAL } from '@/core/constants/channelMonitor'
import { useAutoRefresh } from '@/common/composables/useAutoRefresh'

const { t } = useI18n()
const appStore = useAppStore()

const items = ref<UserMonitorView[]>([])
const loading = ref(false)
const currentWindow = ref<MonitorWindow>('7d')
const detailCache = reactive<Record<number, UserMonitorDetail>>({})
const showDetail = ref(false)
const detailTarget = ref<UserMonitorView | null>(null)

let abortController: AbortController | null = null

const autoRefresh = useAutoRefresh({
  storageKey: 'channel-status-share-auto-refresh',
  intervals: [30, 60, 120] as const,
  defaultInterval: DEFAULT_INTERVAL_SECONDS,
  onRefresh: () => reload(true),
  shouldPause: () => document.hidden || loading.value,
})
const countdown = autoRefresh.countdown

const overallStatus = computed<OverallStatus>(() => {
  if (items.value.length === 0) return 'operational'
  for (const it of items.value) {
    if (it.primary_status === 'failed' || it.primary_status === 'error') return 'degraded'
    if (it.primary_status !== STATUS_OPERATIONAL) return 'degraded'
  }
  return 'operational'
})

const detailTitle = computed(() => detailTarget.value?.name || t('channelStatus.detailTitle'))

async function reload(silent = false) {
  if (abortController) abortController.abort()
  const ctrl = new AbortController()
  abortController = ctrl
  if (!silent) loading.value = true
  try {
    const res = await listSharedChannelMonitorViews({ signal: ctrl.signal })
    if (ctrl.signal.aborted || abortController !== ctrl) return
    items.value = res.items || []
  } catch (err: unknown) {
    const e = err as { name?: string; code?: string }
    if (e?.name === 'AbortError' || e?.code === 'ERR_CANCELED') return
    appStore.showError(extractApiErrorMessage(err, t('channelStatus.loadError')))
  } finally {
    if (abortController === ctrl) {
      if (!silent) loading.value = false
      countdown.value = DEFAULT_INTERVAL_SECONDS
      abortController = null
    }
  }
}

async function manualReload() {
  await reload(false)
  if (currentWindow.value !== '7d') {
    await loadDetails(items.value.map(it => it.id), true)
  }
}

async function ensureDetailsForWindow() {
  if (currentWindow.value === '7d') return
  await loadDetails(items.value.map(it => it.id))
}

async function loadDetails(ids: number[], force = false) {
  const missing = force ? ids : ids.filter(id => !detailCache[id])
  if (missing.length === 0) return
  try {
    const details = await fetchSharedChannelMonitorDetails(missing)
    for (const detail of details) detailCache[detail.id] = detail
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('channelStatus.detailLoadError')))
  }
}

async function handleWindowChange(value: MonitorWindow) {
  currentWindow.value = value
  await ensureDetailsForWindow()
}

function openDetail(row: UserMonitorView) {
  detailTarget.value = row
  showDetail.value = true
}

function closeDetail() {
  showDetail.value = false
  detailTarget.value = null
}

watch(items, () => {
  void ensureDetailsForWindow()
})

watch(
  () => appStore.cachedPublicSettings?.channel_monitor_enabled,
  (enabled) => {
    if (enabled === false) autoRefresh.stop()
    else if (autoRefresh.enabled.value) autoRefresh.start()
  },
)

onMounted(() => {
  void reload(false)
  if (appStore.cachedPublicSettings?.channel_monitor_enabled !== false) {
    autoRefresh.setEnabled(true)
  }
})

onBeforeUnmount(() => {
  if (abortController) abortController.abort()
})
</script>
