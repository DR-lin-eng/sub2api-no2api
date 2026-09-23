<template>
  <div>
    <div
      v-if="loading && items.length === 0"
      class="grid gap-5 grid-cols-1 lg:grid-cols-2"
    >
      <div
        v-for="i in 4"
        :key="i"
        class="p-5 rounded-2xl min-h-[240px] bg-white/70 dark:bg-dark-800/60 border border-gray-200/80 dark:border-dark-700/70 animate-pulse"
      >
        <div class="flex items-start gap-3">
          <div class="w-9 h-9 rounded-xl bg-gray-200 dark:bg-dark-700"></div>
          <div class="flex-1 space-y-2">
            <div class="h-4 w-2/3 rounded bg-gray-200 dark:bg-dark-700"></div>
            <div class="h-3 w-1/2 rounded bg-gray-200 dark:bg-dark-700"></div>
          </div>
          <div class="h-6 w-16 rounded-full bg-gray-200 dark:bg-dark-700"></div>
        </div>
        <div class="mt-5 space-y-3">
          <div
            v-for="row in 3"
            :key="row"
            class="h-12 rounded-xl bg-gray-100 dark:bg-dark-900/40"
          ></div>
        </div>
      </div>
    </div>

    <EmptyState
      v-else-if="items.length === 0"
      :title="t('channelStatus.empty.title')"
      :description="t('channelStatus.empty.description')"
    />

    <div
      v-else
      class="grid gap-5 grid-cols-1 lg:grid-cols-2"
    >
      <MonitorProviderCard
        v-for="group in providerGroups"
        :key="group.provider"
        :items="group.items"
        :window="window"
        :countdown-seconds="countdownSeconds"
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
  loading: boolean
  detailCache: Record<number, UserMonitorDetail>
}>()

const emit = defineEmits<{
  (e: 'cardClick', item: UserMonitorView): void
}>()

const { t } = useI18n()

/**
 * Bucket the flat monitor list by provider so every group watching the same AI
 * family lands in one card. Providers keep their first-seen order to avoid
 * cards jumping around while auto-refresh replaces the list.
 */
const providerGroups = computed<{ provider: Provider; items: UserMonitorView[] }[]>(() => {
  const groups: { provider: Provider; items: UserMonitorView[] }[] = []
  const indexByProvider = new Map<Provider, number>()
  for (const item of props.items) {
    const existing = indexByProvider.get(item.provider)
    if (existing === undefined) {
      indexByProvider.set(item.provider, groups.length)
      groups.push({ provider: item.provider, items: [item] })
      continue
    }
    groups[existing].items.push(item)
  }
  return groups
})
</script>
