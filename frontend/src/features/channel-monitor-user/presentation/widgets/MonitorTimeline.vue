<template>
  <div :class="compact ? 'mt-2' : 'mt-4 pt-3 border-t border-gray-100 dark:border-dark-700/60'">
    <div
      v-if="!compact"
      class="flex justify-between text-[10px] font-semibold uppercase tracking-widest text-gray-400 mb-2"
    >
      <span>{{ t('monitorCommon.history60pts', { n: length }) }}</span>
      <span class="tabular-nums">{{ t('monitorCommon.nextUpdateIn', { n: countdownSeconds }) }}</span>
    </div>

    <div
      v-if="maintenance"
      :class="[
        'flex w-full items-center justify-center overflow-hidden rounded-md border border-dashed border-gray-300 dark:border-dark-600 text-[10px] uppercase tracking-widest text-gray-400',
        compact ? 'h-6' : 'h-8',
      ]"
    >
      {{ t('monitorCommon.maintenancePaused') }}
    </div>
    <div
      v-else
      :class="[
        'flex w-full overflow-hidden rounded-md bg-gray-100/70 ring-1 ring-inset ring-gray-200/60 dark:bg-dark-900/50 dark:ring-dark-700/50',
        compact ? 'h-6 px-1 gap-[2px]' : 'h-8 px-1.5 gap-[3px]',
      ]"
    >
      <!-- Tall slots packed edge to edge: the strip stays one dense block while
           every slot reads as a vertical rectangle instead of a flat dash. -->
      <div
        v-for="(bar, idx) in displayBars"
        :key="idx"
        class="flex-1 min-w-0 rounded-[2px] transition-colors duration-300"
        :class="bar.colorClass"
        :title="bar.title"
      ></div>
    </div>

    <div
      v-if="!compact"
      class="mt-1 flex justify-between text-[9px] uppercase tracking-widest text-gray-400"
    >
      <span>{{ t('monitorCommon.past') }}</span>
      <span>{{ t('monitorCommon.now') }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitorTimelinePoint } from '@/features/channel-monitor-user/data/datasources/channelMonitorUserDatasource'
import { useChannelMonitorFormat } from '@/features/channel-monitor-user/presentation/composables/useChannelMonitorFormat'

const props = withDefaults(defineProps<{
  buckets?: MonitorTimelinePoint[]
  countdownSeconds: number
  length?: number
  maintenance?: boolean
  /** Drops the header/footer captions and shrinks the bars for inline rows. */
  compact?: boolean
}>(), {
  buckets: () => [],
  // Fewer, taller slots read better on a card row than a dense 60-bar strip.
  length: 20,
  maintenance: false,
  compact: false,
})

const { t } = useI18n()
const { statusLabel, formatLatencyWithUnit, formatRelativeTime } = useChannelMonitorFormat()

interface Bar {
  colorClass: string
  title: string
}

// Colour is the only encoding: every slot is the same slim rectangle, so a long
// healthy run reads as one calm block instead of a jagged silhouette. Softer
// 400-level fills keep it from looking neon; abnormal slots still stand out
// because they are the only warm hues on screen.
const STATUS_COLOR: Record<string, string> = {
  operational: 'bg-emerald-400 dark:bg-emerald-500',
  degraded: 'bg-amber-400 dark:bg-amber-400',
  failed: 'bg-red-400 dark:bg-red-500',
  error: 'bg-red-400 dark:bg-red-500',
  empty: 'bg-gray-200 dark:bg-dark-700',
}

const displayBars = computed<Bar[]>(() => {
  // Real points come newest-first; convert to oldest-first so the rightmost
  // bar represents "now". Pad the left with empty placeholders to keep the
  // bar count stable at `length`.
  const real = [...(props.buckets ?? [])]
    .slice(0, props.length)
    .reverse()

  const padCount = Math.max(0, props.length - real.length)
  const bars: Bar[] = []

  for (let i = 0; i < padCount; i += 1) {
    bars.push({ colorClass: STATUS_COLOR.empty, title: '' })
  }

  for (const point of real) {
    const status = point.status as keyof typeof STATUS_COLOR
    const colorClass = STATUS_COLOR[status] ?? STATUS_COLOR.empty
    const latency = formatLatencyWithUnit(point.latency_ms)
    const relative = formatRelativeTime(point.checked_at)
    const label = statusLabel(point.status)
    bars.push({
      colorClass,
      title: `${relative} · ${label} · ${latency}`,
    })
  }

  return bars
})
</script>
