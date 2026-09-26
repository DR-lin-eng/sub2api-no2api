<template>
  <div class="min-h-screen bg-gradient-to-br from-gray-50 via-gray-50 to-gray-100 dark:from-dark-950 dark:via-dark-950 dark:to-dark-900">
    <!-- 单列通栏大容器 -->
    <main class="mx-auto max-w-[1600px] px-4 py-5 sm:px-6 lg:px-10">
      <!-- 子集 1: 页面头部信息区 -->
      <section class="mb-5 pb-5 border-b border-gray-200/60 dark:border-dark-800/60">
        <div class="flex items-start justify-between gap-4 flex-wrap">
          <div class="flex-1 min-w-0 space-y-1.5">
            <h1 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-gray-50">
              {{ t('channelStatus.title') }}
            </h1>
            <p class="text-[13px] leading-relaxed text-gray-600 dark:text-gray-400">
              {{ t('channelStatus.description') }}
            </p>
          </div>
          
          <!-- 整体状态指示器 -->
          <div class="flex items-center gap-3">
            <span
              class="inline-flex items-center px-4 py-2 rounded-xl text-sm font-bold tracking-wide uppercase shadow-sm"
              :class="overallStatusBadgeClass"
            >
              <span
                class="w-2 h-2 rounded-full mr-2 animate-pulse"
                :class="overallStatusDotClass"
              ></span>
              {{ overallStatusLabel }}
            </span>
          </div>
        </div>
      </section>

      <!-- 子集 2: 控制面板区 -->
      <section class="mb-6 p-4 rounded-xl bg-white/80 dark:bg-dark-900/60 backdrop-blur-sm border border-gray-200/60 dark:border-dark-800/50 shadow-sm">
        <MonitorHero
          :overall-status="overallStatus"
          :interval-seconds="DEFAULT_INTERVAL_SECONDS"
          :window="currentWindow"
          :loading="loading"
          :auto-refresh="autoRefresh"
          @update:window="handleWindowChange"
          @refresh="manualReload"
        />
      </section>

      <!-- 子集 3: 模型监控数据区（单列布局） -->
      <section class="space-y-4">
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-50">
            {{ t('channelStatus.providerStatus') || '服务提供商状态' }}
          </h2>
          <span class="text-xs text-gray-500 dark:text-gray-400 font-medium">
            {{ items.length }} {{ t('channelStatus.totalProviders') || '个监控项' }}
          </span>
        </div>
        
        <MonitorCardGrid
          :items="items"
          :window="currentWindow"
          :countdown-seconds="countdown"
          :loading="loading"
          :detail-cache="detailCache"
          @card-click="openDetail"
        />
      </section>

      <MonitorDetailDialog
        :show="showDetail"
        :monitor-id="detailTarget?.id ?? null"
        :title="detailTitle"
        :initial-detail="detailTarget ? detailCache[detailTarget.id] : null"
        :fetch-detail="fetchSharedChannelMonitorDetail"
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
  statusShared as fetchSharedChannelMonitorDetail,
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

const overallStatusLabel = computed(() => {
  return overallStatus.value === 'operational' 
    ? t('channelStatus.allOperational') || '全部正常'
    : t('channelStatus.someIssues') || '存在异常'
})

const overallStatusBadgeClass = computed(() => {
  return overallStatus.value === 'operational'
    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-500/30'
    : 'bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-300 border border-amber-200 dark:border-amber-500/30'
})

const overallStatusDotClass = computed(() => {
  return overallStatus.value === 'operational'
    ? 'bg-emerald-500 dark:bg-emerald-400'
    : 'bg-amber-500 dark:bg-amber-400'
})

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
      autoRefresh.resetCountdown()
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
