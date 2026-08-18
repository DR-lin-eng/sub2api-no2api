import { computed, ref, watch, type Ref } from 'vue'
import { getRealtimeTrafficSummary } from '@/features/admin-ops/data/datasources/opsMetricsQueries'
import type { OpsRealtimeTrafficSummary } from '@/features/admin-ops/data/dtos/opsMetricsDtos'
import { useAdminSettingsStore } from '@/features/admin-settings/adminSettingsStore'

export type OpsRealtimeWindow = '1min' | '5min' | '30min' | '1h'

interface OpsRealtimeTrafficOptions {
  timeRange: Readonly<Ref<string>>
  platform: Readonly<Ref<string>>
  groupId: Readonly<Ref<number | null>>
  autoRefreshEnabled: Readonly<Ref<boolean | undefined>>
  autoRefreshCountdown: Readonly<Ref<number | undefined>>
  loading: Readonly<Ref<boolean>>
}

const REALTIME_WINDOW_MINUTES: Record<OpsRealtimeWindow, number> = {
  '1min': 1,
  '5min': 5,
  '30min': 30,
  '1h': 60,
}

const TOOLBAR_RANGE_MINUTES: Record<string, number> = {
  '5m': 5,
  '30m': 30,
  '1h': 60,
  '6h': 6 * 60,
  '24h': 24 * 60,
}

export function useOpsRealtimeTraffic(options: OpsRealtimeTrafficOptions) {
  const adminSettingsStore = useAdminSettingsStore()
  const realtimeWindow = ref<OpsRealtimeWindow>('1min')
  const realtimeTrafficSummary = ref<OpsRealtimeTrafficSummary | null>(null)
  const realtimeTrafficLoading = ref(false)

  const availableRealtimeWindows = computed(() => {
    const toolbarMinutes = TOOLBAR_RANGE_MINUTES[options.timeRange.value] ?? 60
    return (['1min', '5min', '30min', '1h'] as const).filter(
      (window) => REALTIME_WINDOW_MINUTES[window] <= toolbarMinutes,
    )
  })

  function makeZeroRealtimeTrafficSummary(): OpsRealtimeTrafficSummary {
    const now = new Date().toISOString()
    return {
      window: realtimeWindow.value,
      start_time: now,
      end_time: now,
      platform: options.platform.value,
      group_id: options.groupId.value,
      qps: { current: 0, peak: 0, avg: 0 },
      tps: { current: 0, peak: 0, avg: 0 },
    }
  }

  async function refreshRealtime(): Promise<void> {
    if (realtimeTrafficLoading.value) return
    if (!adminSettingsStore.opsRealtimeMonitoringEnabled) {
      realtimeTrafficSummary.value = makeZeroRealtimeTrafficSummary()
      return
    }

    realtimeTrafficLoading.value = true
    try {
      const response = await getRealtimeTrafficSummary(
        realtimeWindow.value,
        options.platform.value,
        options.groupId.value,
      )
      if (response?.enabled === false) {
        adminSettingsStore.setOpsRealtimeMonitoringEnabledLocal(false)
      }
      realtimeTrafficSummary.value = response?.summary ?? null
    } catch (error) {
      console.error('[OpsDashboardHeader] Failed to load realtime traffic summary', error)
      realtimeTrafficSummary.value = null
    } finally {
      realtimeTrafficLoading.value = false
    }
  }

  watch(options.timeRange, () => {
    // The realtime window must stay inside the toolbar range.
    realtimeWindow.value = '1min'
    void refreshRealtime()
  })

  watch(
    () => [realtimeWindow.value, options.platform.value, options.groupId.value] as const,
    () => {
      void refreshRealtime()
    },
    { immediate: true },
  )

  watch(
    () => adminSettingsStore.opsRealtimeMonitoringEnabled,
    (enabled) => {
      if (!enabled) {
        realtimeTrafficSummary.value = makeZeroRealtimeTrafficSummary()
      } else {
        void refreshRealtime()
      }
    },
    { immediate: true },
  )

  watch(
    () =>
      [
        options.autoRefreshEnabled.value,
        options.autoRefreshCountdown.value,
        options.loading.value,
      ] as const,
    ([enabled, countdown, loading]) => {
      if (enabled && !loading && countdown === 0) {
        void refreshRealtime()
      }
    },
  )

  return {
    availableRealtimeWindows,
    realtimeTrafficSummary,
    realtimeWindow,
    refreshRealtime,
  }
}
