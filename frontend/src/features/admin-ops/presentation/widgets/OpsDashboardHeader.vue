<script setup lang="ts">
import { toRefs } from 'vue'
import type { OpsMetricThresholds } from '@/features/admin-ops/data/datasources/adminOpsDatasource'
import type { OpsDashboardOverview } from '@/features/admin-ops/data/dtos/opsDashboardDtos'
import type { OpsRequestDetailsPreset } from '../opsTypeSignals'
import { useOpsRealtimeTraffic } from '../composables/useOpsRealtimeTraffic'
import OpsDashboardToolbar from './OpsDashboardToolbar.vue'
import OpsHealthOverview from './OpsHealthOverview.vue'
import OpsMetricsGrid from './OpsMetricsGrid.vue'
import OpsSystemHealthGrid from './OpsSystemHealthGrid.vue'

interface Props {
  overview?: OpsDashboardOverview | null
  platform: string
  groupId: number | null
  timeRange: string
  queryMode: string
  loading: boolean
  lastUpdated: Date | null
  thresholds?: OpsMetricThresholds | null
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
  (event: 'openRequestDetails', preset?: OpsRequestDetailsPreset): void
  (event: 'openErrorDetails', kind: 'request' | 'upstream'): void
  (event: 'openSettings'): void
  (event: 'openAlertRules'): void
  (event: 'enterFullscreen'): void
  (event: 'exitFullscreen'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const {
  autoRefreshCountdown,
  autoRefreshEnabled,
  groupId,
  loading,
  platform,
  timeRange,
} = toRefs(props)

const {
  availableRealtimeWindows,
  realtimeTrafficSummary,
  realtimeWindow,
  refreshRealtime,
} = useOpsRealtimeTraffic({
  timeRange,
  platform,
  groupId,
  autoRefreshEnabled,
  autoRefreshCountdown,
  loading,
})

function handleToolbarRefresh(): void {
  void refreshRealtime()
  emit('refresh')
}
</script>

<template>
  <div
    :class="[
      'flex flex-col gap-4 rounded-3xl bg-white shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700',
      props.fullscreen ? 'p-8' : 'p-6',
    ]"
  >
    <OpsDashboardToolbar
      :platform="props.platform"
      :group-id="props.groupId"
      :time-range="props.timeRange"
      :query-mode="props.queryMode"
      :loading="props.loading"
      :last-updated="props.lastUpdated"
      :auto-refresh-enabled="props.autoRefreshEnabled"
      :auto-refresh-countdown="props.autoRefreshCountdown"
      :fullscreen="props.fullscreen"
      :custom-start-time="props.customStartTime"
      :custom-end-time="props.customEndTime"
      @update:platform="emit('update:platform', $event)"
      @update:group="emit('update:group', $event)"
      @update:time-range="emit('update:timeRange', $event)"
      @update:query-mode="emit('update:queryMode', $event)"
      @update:custom-time-range="(startTime, endTime) => emit('update:customTimeRange', startTime, endTime)"
      @refresh="handleToolbarRefresh"
      @open-settings="emit('openSettings')"
      @open-alert-rules="emit('openAlertRules')"
      @enter-fullscreen="emit('enterFullscreen')"
      @exit-fullscreen="emit('exitFullscreen')"
    />

    <div v-if="props.overview" class="grid grid-cols-1 gap-6 lg:grid-cols-12">
      <OpsHealthOverview
        v-model:realtime-window="realtimeWindow"
        :overview="props.overview"
        :fullscreen="props.fullscreen"
        :available-realtime-windows="availableRealtimeWindows"
        :realtime-traffic-summary="realtimeTrafficSummary"
      />
      <OpsMetricsGrid
        :overview="props.overview"
        :thresholds="props.thresholds"
        :fullscreen="props.fullscreen"
        @open-request-details="emit('openRequestDetails', $event)"
        @open-error-details="emit('openErrorDetails', $event)"
      />
    </div>

    <OpsSystemHealthGrid
      v-if="props.overview"
      :overview="props.overview"
      :fullscreen="props.fullscreen"
    />
  </div>
</template>
