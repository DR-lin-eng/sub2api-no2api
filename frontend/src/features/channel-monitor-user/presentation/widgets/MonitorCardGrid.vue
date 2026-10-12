<template>
  <div>
    <!-- Loading 骨架屏 - 单列布局 -->
    <div
      v-if="loading && items.length === 0"
      class="space-y-3"
    >
      <div
        v-for="i in 4"
        :key="i"
        class="p-6 rounded-xl min-h-[200px] bg-white/70 dark:bg-dark-800/60 border border-gray-200/80 dark:border-dark-700/70 animate-pulse"
      >
        <div class="flex items-start gap-4">
          <div class="w-11 h-11 rounded-xl bg-gray-200 dark:bg-dark-700"></div>
          <div class="flex-1 space-y-2.5">
            <div class="h-5 w-1/3 rounded bg-gray-200 dark:bg-dark-700"></div>
            <div class="h-3 w-1/2 rounded bg-gray-200 dark:bg-dark-700"></div>
          </div>
          <div class="h-7 w-20 rounded-full bg-gray-200 dark:bg-dark-700"></div>
        </div>
        <div class="mt-6 space-y-3">
          <div
            v-for="row in 3"
            :key="row"
            class="h-14 rounded-lg bg-gray-100 dark:bg-dark-900/40"
          ></div>
        </div>
      </div>
    </div>

    <EmptyState
      v-else-if="items.length === 0"
      :title="t('channelStatus.empty.title')"
      :description="t('channelStatus.empty.description')"
    />

    <!-- 单列通栏卡片布局 -->
    <div
      v-else
      class="space-y-3"
    >
      <MonitorProviderCard
        v-for="group in cardGroups"
        :key="group.key"
        :items="group.items"
        :window="window"
        :countdown-seconds="countdownSeconds"
        :timeline-points="timelinePoints"
        :detail-cache="detailCache"
        @card-click="emit('cardClick', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  Provider,
  UserMonitorView,
  UserMonitorDetail,
} from '@/features/channel-monitor-user/data/datasources/channelMonitorUserDatasource'
import EmptyState from '@/common/widgets/feedback/EmptyState.vue'
import MonitorProviderCard from './MonitorProviderCard.vue'

const props = defineProps<{
  items: UserMonitorView[]
  window: '7d' | '15d' | '30d'
  countdownSeconds: number
  timelinePoints: number
  loading: boolean
  detailCache: Record<number, UserMonitorDetail>
}>()

const emit = defineEmits<{
  (e: 'cardClick', item: UserMonitorView): void
}>()

const { t } = useI18n()

interface MonitorCardGroup {
  key: string
  provider: Provider
  items: UserMonitorView[]
}

/**
 * Bucket the flat monitor list by provider so active and passive monitors
 * appear together while their individual modes remain visible in the card.
 * Groups keep their first-seen order to avoid cards jumping around while
 * auto-refresh replaces the list.
 */
const cardGroups = computed<MonitorCardGroup[]>(() => {
  const groups: MonitorCardGroup[] = []
  const indexByKey = new Map<Provider, number>()
  for (const item of props.items) {
    const existing = indexByKey.get(item.provider)
    if (existing === undefined) {
      indexByKey.set(item.provider, groups.length)
      groups.push({ key: item.provider, provider: item.provider, items: [item] })
      continue
    }
    groups[existing].items.push(item)
  }
  return groups
})
</script>
