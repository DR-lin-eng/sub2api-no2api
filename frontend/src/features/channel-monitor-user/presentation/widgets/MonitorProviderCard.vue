<template>
  <section
    class="rounded-2xl bg-white/70 backdrop-blur-xl border border-gray-200/80 shadow-card dark:bg-dark-800/60 dark:border-dark-700/70 overflow-hidden flex flex-col"
  >
    <!-- Header: provider identity + roll-up summary -->
    <header class="flex items-center gap-3 p-4 border-b border-gray-100 dark:border-dark-700/60">
      <span
        class="w-9 h-9 rounded-xl ring-1 ring-black/5 dark:ring-white/10 grid place-items-center flex-shrink-0"
        :class="[providerGradient(provider), providerTintClass]"
      >
        <ProviderIcon :provider="provider" :size="20" />
      </span>
      <div class="flex-1 min-w-0">
        <div class="flex items-center gap-2 min-w-0">
          <span class="text-sm font-semibold truncate text-gray-900 dark:text-gray-100">
            {{ providerLabel(provider) }}
          </span>
          <span
            v-if="modeLabelText"
            class="px-1.5 py-0.5 rounded-md text-[10px] font-medium flex-shrink-0 bg-gray-100 text-gray-600 dark:bg-dark-700/80 dark:text-gray-300"
          >
            {{ modeLabelText }}
          </span>
        </div>
        <div class="mt-0.5 text-[11px] truncate text-gray-500 dark:text-gray-400">
          {{ summaryLabel }}
        </div>
      </div>
      <span
        class="px-2.5 py-1 rounded-full text-xs font-semibold flex-shrink-0"
        :class="statusBadgeClass(overallStatus)"
      >
        {{ statusLabel(overallStatus) }}
      </span>
    </header>

    <!-- One compact row per monitored group -->
    <ul class="divide-y divide-gray-100 dark:divide-dark-700/60 flex-1">
      <li v-for="row in sortedItems" :key="row.id">
        <button
          type="button"
          class="group w-full text-left px-4 py-3 hover:bg-gray-50/80 dark:hover:bg-dark-700/40 transition-colors"
          @click="emit('cardClick', row)"
        >
          <div class="flex items-center gap-3">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 min-w-0">
                <span class="text-sm font-medium truncate text-gray-900 dark:text-gray-100">
                  {{ row.group_name || row.name || t('channelStatus.unnamedGroup') }}
                </span>
                <span class="font-mono text-[11px] truncate text-gray-500 dark:text-gray-400">
                  {{ row.primary_model }}
                </span>
              </div>
              <div class="mt-1 flex items-center gap-3 text-[11px] text-gray-500 dark:text-gray-400">
                <span class="font-mono tabular-nums">
                  {{ formatLatencyWithUnit(row.primary_latency_ms) }}
                </span>
                <span class="tabular-nums" :style="availabilityColor(row)">
                  {{ formatPercent(resolveAvailability(row)) }}
                </span>
              </div>
            </div>
            <span
              class="px-2 py-0.5 rounded-full text-[11px] font-semibold flex-shrink-0"
              :class="statusBadgeClass(row.primary_status)"
            >
              {{ statusLabel(row.primary_status) }}
            </span>
            <Icon
              name="chevronRight"
              size="xs"
              class="flex-shrink-0 text-gray-300 dark:text-dark-600 group-hover:text-gray-400"
            />
          </div>

          <MonitorTimeline
            compact
            :buckets="row.timeline"
            :countdown-seconds="countdownSeconds"
          />
        </button>
      </li>
    </ul>

  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  UserMonitorView,
  UserMonitorDetail,
  Provider,
  MonitorMode,
  MonitorStatus,
} from '@/features/channel-monitor-user/data/datasources/channelMonitorUserDatasource'
import {
  useChannelMonitorFormat,
  providerGradient,
  hslForPct,
} from '@/features/channel-monitor-user/presentation/composables/useChannelMonitorFormat'
import ProviderIcon from './ProviderIcon.vue'
import MonitorTimeline from './MonitorTimeline.vue'
/** Lower rank surfaces first, so failures never end up behind the fold. */
const STATUS_RANK: Record<string, number> = {
  failed: 0,
  error: 0,
  degraded: 1,
  unknown: 2,
  operational: 3,
}

const PROVIDER_TINT: Record<string, string> = {
  openai: 'text-emerald-600 dark:text-emerald-300',
  anthropic: 'text-orange-600 dark:text-orange-300',
  gemini: 'text-sky-600 dark:text-sky-300',
  grok: 'text-zinc-700 dark:text-zinc-200',
  kimi: 'text-pink-600 dark:text-pink-300',
  zhipu: 'text-indigo-600 dark:text-indigo-300',
  deepseek: 'text-cyan-600 dark:text-cyan-300',
  minimax: 'text-rose-600 dark:text-rose-300',
  opencode_go: 'text-amber-600 dark:text-amber-300',
}

const props = defineProps<{
  items: UserMonitorView[]
  mode?: MonitorMode
  window: '7d' | '15d' | '30d'
  countdownSeconds: number
  detailCache: Record<number, UserMonitorDetail>
}>()

const emit = defineEmits<{
  (e: 'cardClick', item: UserMonitorView): void
}>()

const { t } = useI18n()
const { statusLabel, statusBadgeClass, modeLabel, providerLabel, formatLatencyWithUnit, formatPercent } =
  useChannelMonitorFormat()

/** Legacy monitors without a mode keep the header unlabelled. */
const modeLabelText = computed(() =>
  props.mode === 'active' || props.mode === 'passive' ? modeLabel(props.mode) : '',
)

const provider = computed<Provider>(() => props.items[0]?.provider ?? ('' as Provider))

const providerTintClass = computed(
  () => PROVIDER_TINT[provider.value] ?? 'text-gray-500 dark:text-gray-300',
)

const overallStatus = computed<MonitorStatus>(() => {
  const statuses = props.items.map(item => item.primary_status)
  if (statuses.some(s => s === 'failed' || s === 'error')) return 'failed'
  if (statuses.some(s => s === 'degraded')) return 'degraded'
  // No probe result yet reads as unknown, not as a partial outage.
  if (statuses.some(s => s !== 'operational')) return 'unknown'
  return 'operational'
})

const abnormalCount = computed(
  () => props.items.filter(item => item.primary_status !== 'operational').length,
)

const summaryLabel = computed(() => {
  const parts = [t('channelStatus.groupCount', { n: props.items.length })]
  if (abnormalCount.value > 0) {
    parts.push(t('channelStatus.abnormalCount', { n: abnormalCount.value }))
  }
  return parts.join(' · ')
})

const sortedItems = computed(() =>
  [...props.items].sort(
    (a, b) =>
      (STATUS_RANK[a.primary_status] ?? 2) - (STATUS_RANK[b.primary_status] ?? 2),
  ),
)

function resolveAvailability(item: UserMonitorView): number | null {
  if (props.window === '7d') return item.availability_7d ?? null
  const detail = props.detailCache[item.id]
  if (!detail) return null
  const primary = detail.models.find(m => m.model === item.primary_model)
  if (!primary) return null
  return props.window === '15d' ? primary.availability_15d ?? null : primary.availability_30d ?? null
}

function availabilityColor(item: UserMonitorView) {
  const colour = hslForPct(resolveAvailability(item))
  return colour ? { color: colour } : { color: 'rgb(156 163 175)' }
}
</script>
