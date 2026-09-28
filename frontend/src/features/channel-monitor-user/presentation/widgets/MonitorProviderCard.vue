<template>
  <section
    class="group/card rounded-xl bg-gradient-to-br from-white via-white to-gray-50/40 dark:from-dark-800 dark:via-dark-800 dark:to-dark-900 backdrop-blur-sm border border-gray-200/70 dark:border-dark-700/60 shadow-sm hover:shadow-md dark:shadow-dark-900/30 transition-all duration-300 overflow-hidden"
  >
    <!-- Header: provider identity + summary - 可点击折叠 -->
    <header 
      class="relative flex items-center gap-4 px-6 py-4 border-b border-gray-100/80 dark:border-dark-700/60 bg-gradient-to-r from-gray-50/30 via-transparent to-transparent dark:from-dark-700/20 cursor-pointer hover:bg-gray-50/50 dark:hover:bg-dark-700/30 transition-colors"
      @click="isCollapsed = !isCollapsed"
    >
      <span
          class="w-12 h-12 rounded-xl ring-1 ring-black/[0.04] dark:ring-white/[0.08] shadow-sm grid place-items-center flex-shrink-0 transition-transform group-hover/card:scale-105 duration-300"
          :class="[providerGradient(provider), providerTintClass]"
        >
        <ProviderIcon :provider="provider" :size="24" />
      </span>
      
      <div class="flex-1 min-w-0">
        <div class="flex items-center gap-2.5 min-w-0">
          <span class="text-base font-bold tracking-tight truncate text-gray-900 dark:text-white">
            {{ providerLabel(provider) }}
          </span>
          <span
            v-if="modeLabelText"
            class="px-2 py-0.5 rounded-md text-[10px] font-bold uppercase tracking-wider flex-shrink-0 bg-gray-100/90 text-gray-700 dark:bg-dark-700/80 dark:text-gray-300 border border-gray-200/60 dark:border-dark-600/60"
          >
            {{ modeLabelText }}
          </span>
        </div>
        <div class="mt-1 text-xs truncate text-gray-600 dark:text-gray-300 font-medium">
          {{ summaryLabel }}
        </div>
      </div>
      
      <span
        class="px-3.5 py-1.5 rounded-full text-xs font-bold uppercase tracking-wider flex-shrink-0 shadow-sm transition-all"
        :class="statusBadgeClass(overallStatus)"
      >
        {{ statusLabel(overallStatus) }}
      </span>
      
      <!-- 折叠指示器 -->
      <button
          class="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-gray-100/80 dark:hover:bg-dark-700/60 transition-all flex-shrink-0"
          @click.stop="isCollapsed = !isCollapsed"
        >
        <Icon
          name="chevronDown"
          size="sm"
          class="text-gray-500 dark:text-gray-400 transition-transform duration-300"
          :class="{ 'rotate-180': isCollapsed }"
        />
      </button>
    </header>

    <!-- 模型列表 - 优化信息密度的网格布局，支持折叠 -->
    <div 
      v-show="!isCollapsed"
      class="px-6 py-4 transition-all duration-300"
    >
      <div class="grid gap-3 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-3">
        <button
          v-for="row in sortedItems"
          :key="row.id"
          type="button"
          class="group/item relative text-left p-4 rounded-lg bg-gray-50/60 dark:bg-dark-900/40 border border-gray-200/50 dark:border-dark-700/50 hover:border-gray-300 dark:hover:border-dark-600 hover:bg-white dark:hover:bg-dark-800/70 transition-all duration-200 hover:shadow-sm active:scale-[0.98]"
          @click="emit('cardClick', row)"
        >
          <!-- 模型信息头部 -->
          <div class="flex items-start justify-between gap-2.5 mb-3">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 min-w-0 mb-1">
                <span class="text-sm font-bold truncate text-gray-900 dark:text-white leading-tight">
                  {{ row.group_name || row.name || t('channelStatus.unnamedGroup') }}
                </span>
              </div>
              <span class="inline-block font-mono text-[10px] truncate text-gray-600 dark:text-gray-400 bg-gray-100/70 dark:bg-dark-800/60 px-2 py-0.5 rounded border border-gray-200/50 dark:border-dark-700/40">
                {{ row.primary_model }}
              </span>
            </div>
            
            <span
              class="px-2 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wide flex-shrink-0 shadow-xs transition-transform group-hover/item:scale-105"
              :class="statusBadgeClass(row.primary_status)"
            >
              {{ statusLabel(row.primary_status) }}
            </span>
          </div>

          <!-- 指标数据 -->
          <div class="flex items-center gap-4 mb-3 text-xs">
              <div class="flex items-center gap-1.5 font-mono tabular-nums font-semibold text-gray-700 dark:text-gray-300">
              <span class="w-1.5 h-1.5 rounded-full bg-blue-500 dark:bg-blue-400 shadow-sm"></span>
              <span>{{ formatLatencyWithUnit(row.primary_latency_ms) }}</span>
            </div>
            <div class="flex items-center gap-1.5 tabular-nums font-bold" :style="availabilityColor(row)">
              <span class="w-1.5 h-1.5 rounded-full shadow-sm" :style="availabilityColor(row)"></span>
              <span>{{ formatPercent(resolveAvailability(row)) }}</span>
            </div>
          </div>

          <!-- 时间线 -->
          <MonitorTimeline
            compact
            :buckets="row.timeline"
            :countdown-seconds="countdownSeconds"
          />

          <!-- Hover 指示器 -->
          <div class="absolute top-3 right-3 opacity-0 group-hover/item:opacity-100 transition-opacity">
            <Icon
              name="chevronRight"
              size="xs"
              class="text-gray-400 dark:text-gray-400"
            />
          </div>
        </button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
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
import Icon from '@/common/widgets/icons/Icon.vue'

/**
 * 计算模型的综合健康度评分
 * 评分逻辑：整体状态健康度 + 最新状态权重
 * 返回值越高，排序越靠前（健康的模型优先展示）
 */
function calculateHealthScore(item: UserMonitorView): number {
  const timeline = item.timeline || []
  const totalPoints = timeline.length
  
  // 1. 计算整体健康度：统计所有绿色（operational）指标的占比
  const operationalCount = timeline.filter(
    point => point.status === 'operational'
  ).length
  const overallHealthRatio = totalPoints > 0 ? operationalCount / totalPoints : 0
  
  // 2. 最新状态评分（primary_status）
  const latestStatusScore = getStatusScore(item.primary_status)
  
  // 3. 近期趋势评分：最近10个点的健康度（权重更高）
  const recentPoints = timeline.slice(-10)
  const recentOperationalCount = recentPoints.filter(
    point => point.status === 'operational'
  ).length
  const recentHealthRatio = recentPoints.length > 0 
    ? recentOperationalCount / recentPoints.length 
    : 0
  
  // 综合评分公式：
  // 整体健康度（40%） + 近期趋势（30%） + 最新状态（30%）
  const score = (
    overallHealthRatio * 40 +
    recentHealthRatio * 30 +
    latestStatusScore * 30
  )
  
  return score
}

/**
 * 状态评分映射
 * operational(正常) = 1.0 | degraded(降级) = 0.5 | 其他异常 = 0.0
 */
function getStatusScore(status: MonitorStatus): number {
  switch (status) {
    case 'operational':
      return 1.0
    case 'degraded':
      return 0.5
    case 'unknown':
      return 0.3
    case 'failed':
    case 'error':
    default:
      return 0.0
  }
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

// 折叠状态
const isCollapsed = ref(false)

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

/**
 * 智能排序：综合健康度评分排序
 * 1. 全绿（健康）模型优先展示在顶部，按绿色指标完成度排序
 * 2. 爆红（异常）模型排在末尾
 * 3. 中间状态按"整体健康度+最新状态"综合评分排序
 */
const sortedItems = computed(() => {
  return [...props.items].sort((a, b) => {
    const scoreA = calculateHealthScore(a)
    const scoreB = calculateHealthScore(b)
    
    // 健康度评分高的排在前面（降序）
    return scoreB - scoreA
  })
})

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
