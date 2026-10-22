<template>
  <AppLayout>
    <MonitorHero
      :overall-status="overallStatus"
      :interval-seconds="DEFAULT_INTERVAL_SECONDS"
      :window="currentWindow"
      :loading="loading"
      :auto-refresh="autoRefresh"
      :show-share-button="showShareButton"
      @update:window="handleWindowChange"
      @refresh="refresh()"
      @copy-share-link="copyShareLink"
    />

    <div v-if="providerOptions.length > 0" class="mb-3 flex items-center gap-2">
      <label class="text-xs font-semibold text-gray-500 dark:text-gray-400" for="provider-filter">
        {{ t('channelStatus.filterProvider') }}
      </label>
      <select id="provider-filter" v-model="selectedProvider" class="input h-8 w-auto py-1 text-xs">
        <option value="all">{{ t('channelStatus.allProviders') }}</option>
        <option v-for="option in providerOptions" :key="option" :value="option">
          {{ providerLabel(option) }}
        </option>
      </select>
    </div>

    <MonitorCardGrid
      :items="filteredItems"
      :window="currentWindow"
      :countdown-seconds="countdown"
      :timeline-points="timelinePoints"
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
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/core/stores/appStore'
import { useAuthStore } from '@/features/auth'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import {
  list as listChannelMonitorViews,
  statusBatch as fetchChannelMonitorDetails,
  type UserMonitorView,
  type UserMonitorDetail,
} from '@/features/channel-monitor-user/data/datasources/channelMonitorUserDatasource'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import MonitorHero, {
  type MonitorWindow,
  type OverallStatus,
} from '@/features/channel-monitor-user/presentation/widgets/MonitorHero.vue'
import MonitorCardGrid from '@/features/channel-monitor-user/presentation/widgets/MonitorCardGrid.vue'
import MonitorDetailDialog from '@/features/channel-monitor-user/presentation/widgets/MonitorDetailDialog.vue'
import { DEFAULT_INTERVAL_SECONDS, STATUS_OPERATIONAL, normalizeChannelMonitorTimelinePoints } from '@/core/constants/channelMonitor'
import { useChannelMonitorFormat } from '@/features/channel-monitor-user'
import { useAutoRefresh } from '@/common/composables/useAutoRefresh'

const { t } = useI18n()
const { providerLabel } = useChannelMonitorFormat()
const appStore = useAppStore()
const authStore = useAuthStore()

const items = ref<UserMonitorView[]>([])
const loading = ref(false)
const currentWindow = ref<MonitorWindow>('7d')
const detailCache = reactive<Record<number, UserMonitorDetail>>({})
const showDetail = ref(false)
const detailTarget = ref<UserMonitorView | null>(null)

let abortController: AbortController | null = null

const autoRefresh = useAutoRefresh({
  storageKey: 'channel-status-auto-refresh',
  intervals: [30, 60, 120] as const,
  defaultInterval: DEFAULT_INTERVAL_SECONDS,
  onRefresh: () => refresh(true),
  shouldPause: () => document.hidden || loading.value,
})
const countdown = autoRefresh.countdown

const overallStatus = computed<OverallStatus>(() => {
  const list = filteredItems.value
  if (list.length === 0) return 'operational'
  for (const it of list) {
    if (it.primary_status === 'failed' || it.primary_status === 'error') return 'degraded'
    if (it.primary_status !== STATUS_OPERATIONAL) return 'degraded'
  }
  return 'operational'
})

const timelinePoints = computed(() =>
  normalizeChannelMonitorTimelinePoints(appStore.cachedPublicSettings?.channel_monitor_timeline_points),
)

const providerOptions = computed(() => [...new Set(items.value.map(item => item.provider))])

const selectedProvider = ref<string>('all')

watch(providerOptions, (options) => {
  if (selectedProvider.value !== 'all' && !options.some(provider => provider === selectedProvider.value)) {
    selectedProvider.value = 'all'
  }
})

const filteredItems = computed(() =>
  selectedProvider.value === 'all'
    ? items.value
    : items.value.filter(item => item.provider === selectedProvider.value),
)

const detailTitle = computed(() => detailTarget.value?.name || t('channelStatus.detailTitle'))

const shareURL = computed(() => `${window.location.origin}/monitor/public`)
const showShareButton = computed(() => {
  return authStore.isAdmin &&
    appStore.cachedPublicSettings?.channel_monitor_enabled !== false &&
    appStore.cachedPublicSettings?.channel_monitor_public_share_enabled === true
})

async function reload(silent = false) {
  if (abortController) abortController.abort()
  const ctrl = new AbortController()
  abortController = ctrl
  if (!silent) loading.value = true
  try {
    const res = await listChannelMonitorViews({ signal: ctrl.signal })
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

async function refresh(silent = false) {
  await reload(silent)
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
    const details = await fetchChannelMonitorDetails(missing)
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

async function copyShareLink() {
  const url = shareURL.value
  try {
    await navigator.clipboard.writeText(url)
    appStore.showSuccess(t('channelStatus.share.copied'))
  } catch {
    const input = document.createElement('input')
    input.value = url
    input.setAttribute('readonly', 'readonly')
    input.style.position = 'fixed'
    input.style.opacity = '0'
    document.body.appendChild(input)
    input.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(input)
    if (ok) {
      appStore.showSuccess(t('channelStatus.share.copied'))
    } else {
      appStore.showError(t('channelStatus.share.copyFailed'))
    }
  }
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
