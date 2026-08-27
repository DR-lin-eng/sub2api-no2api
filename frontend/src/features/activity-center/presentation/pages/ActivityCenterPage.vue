<template>
  <AppLayout>
    <div class="space-y-6">
      <section class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900/90">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div class="min-w-0">
            <p class="inline-flex items-center gap-1.5 rounded-full bg-primary-50 px-3 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
              <Icon name="sparkles" size="xs" />
              {{ t('activityCenter.heroBadge') }}
            </p>
            <h1 class="mt-3 text-3xl font-semibold tracking-tight text-gray-900 dark:text-white">
              {{ t('activityCenter.title') }}
            </h1>
            <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-500 dark:text-dark-400">
              {{ t('activityCenter.description') }}
            </p>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadCampaigns">
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              <span class="ml-1">{{ t('activityCenter.refresh') }}</span>
            </button>
          </div>
        </div>

        <div class="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <div
            v-for="metric in metrics"
            :key="metric.key"
            class="rounded-xl border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-700 dark:bg-dark-950/40"
          >
            <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">
              {{ metric.label }}
            </p>
            <div class="mt-2 flex items-end justify-between gap-2">
              <span class="text-2xl font-semibold text-gray-900 dark:text-white">{{ metric.value }}</span>
              <Icon :name="metric.icon" size="sm" class="text-primary-600 dark:text-primary-400" />
            </div>
          </div>
        </div>
      </section>

      <div class="grid gap-6 xl:grid-cols-[minmax(0,1.6fr)_minmax(320px,0.9fr)]">
        <section class="space-y-4">
          <div class="flex flex-col gap-3 rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900/90 lg:flex-row lg:items-center">
            <div class="min-w-0 flex-1">
              <label class="input-label">{{ t('common.search') }}</label>
              <div class="relative">
                <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input
                  v-model="searchQuery"
                  type="search"
                  class="input pl-9"
                  :placeholder="t('activityCenter.filters.searchPlaceholder')"
                />
              </div>
            </div>

            <div class="grid gap-3 sm:grid-cols-2 lg:w-[24rem]">
              <div>
                <label class="input-label">{{ t('activityCenter.filters.typeLabel') }}</label>
                <select v-model="typeFilter" class="input">
                  <option value="all">{{ t('activityCenter.filters.allTypes') }}</option>
                  <option value="lottery">{{ t('activityCenter.types.lottery') }}</option>
                  <option value="redeem">{{ t('activityCenter.types.redeem') }}</option>
                  <option value="external_link">{{ t('activityCenter.types.external_link') }}</option>
                  <option value="custom">{{ t('activityCenter.types.custom') }}</option>
                </select>
              </div>
              <div>
                <label class="input-label">{{ t('activityCenter.filters.statusLabel') }}</label>
                <select v-model="statusFilter" class="input">
                  <option value="all">{{ t('activityCenter.filters.allStatus') }}</option>
                  <option value="live">{{ t('activityCenter.tabs.live') }}</option>
                  <option value="scheduled">{{ t('activityCenter.tabs.upcoming') }}</option>
                  <option value="draft">{{ t('activityCenter.status.draft') }}</option>
                </select>
              </div>
            </div>
          </div>

          <div v-if="loading" class="grid gap-4 sm:grid-cols-2">
            <div v-for="n in 4" :key="n" class="h-64 animate-pulse rounded-2xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900/90"></div>
          </div>

          <div v-else-if="filteredCampaigns.length > 0" class="grid gap-4 sm:grid-cols-2">
            <button
              v-for="campaign in filteredCampaigns"
              :key="campaign.id"
              type="button"
              class="text-left"
              @click="selectedCampaignId = campaign.id"
            >
              <article
                class="h-full rounded-2xl border p-4 shadow-sm transition-shadow hover:shadow-md"
                :class="campaign.id === selectedCampaignId
                  ? 'border-primary-500 bg-primary-50/60 dark:border-primary-500/60 dark:bg-primary-900/20'
                  : 'border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900/90'"
              >
                <div class="aspect-[16/9] overflow-hidden rounded-xl bg-gray-100 dark:bg-dark-800">
                  <img
                    v-if="campaign.banner_url"
                    :src="campaign.banner_url"
                    :alt="campaign.title"
                    class="h-full w-full object-cover"
                  />
                  <div v-else class="flex h-full items-center justify-center text-sm text-gray-400">
                    {{ t('activityCenter.noBanner') }}
                  </div>
                </div>

                <div class="mt-4 flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <h2 class="truncate text-lg font-semibold text-gray-900 dark:text-white">
                      {{ campaign.title }}
                    </h2>
                    <p class="mt-1 line-clamp-2 text-sm leading-6 text-gray-500 dark:text-dark-400">
                      {{ campaign.subtitle || t('activityCenter.noSubtitle') }}
                    </p>
                  </div>
                  <span :class="['inline-flex shrink-0 items-center rounded-full px-2.5 py-1 text-xs font-medium', statusTone(campaign)]">
                    {{ statusLabel(campaign) }}
                  </span>
                </div>

                <div class="mt-3 flex flex-wrap gap-2 text-xs text-gray-500 dark:text-dark-400">
                  <span class="rounded-full bg-gray-100 px-2.5 py-1 dark:bg-dark-800">
                    {{ typeLabel(campaign.type) }}
                  </span>
                  <span class="rounded-full bg-gray-100 px-2.5 py-1 dark:bg-dark-800">
                    #{{ campaign.id }}
                  </span>
                  <span class="rounded-full bg-gray-100 px-2.5 py-1 dark:bg-dark-800">
                    {{ t('activityCenter.sortOrder', { value: campaign.sort_order }) }}
                  </span>
                </div>
              </article>
            </button>
          </div>

          <div v-else class="rounded-2xl border border-dashed border-gray-300 bg-white px-6 py-10 text-center dark:border-dark-700 dark:bg-dark-900/80">
            <Icon name="search" size="lg" class="mx-auto text-gray-400" />
            <h3 class="mt-4 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('activityCenter.empty.title') }}
            </h3>
            <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
              {{ t('activityCenter.empty.description') }}
            </p>
            <button type="button" class="btn btn-secondary mt-4" @click="resetFilters">
              {{ t('common.reset') }}
            </button>
          </div>
        </section>

        <aside class="space-y-4">
          <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900/90">
            <div class="flex items-center justify-between gap-3">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('activityCenter.selected.title') }}</h2>
              <Icon name="badge" size="sm" class="text-primary-600 dark:text-primary-400" />
            </div>

            <div v-if="selectedCampaign" class="mt-4 space-y-4">
              <div class="aspect-[16/9] overflow-hidden rounded-xl bg-gray-100 dark:bg-dark-800">
                <img
                  v-if="selectedCampaign.banner_url"
                  :src="selectedCampaign.banner_url"
                  :alt="selectedCampaign.title"
                  class="h-full w-full object-cover"
                />
                <div v-else class="flex h-full items-center justify-center text-sm text-gray-400">
                  {{ t('activityCenter.noBanner') }}
                </div>
              </div>

              <div>
                <div class="flex items-center gap-2">
                  <span :class="['inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium', statusTone(selectedCampaign)]">
                    {{ statusLabel(selectedCampaign) }}
                  </span>
                  <span class="rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-800 dark:text-dark-300">
                    {{ typeLabel(selectedCampaign.type) }}
                  </span>
                </div>
                <h3 class="mt-3 text-xl font-semibold text-gray-900 dark:text-white">
                  {{ selectedCampaign.title }}
                </h3>
                <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-dark-400">
                  {{ selectedCampaign.subtitle || t('activityCenter.noSubtitle') }}
                </p>
              </div>

              <div class="grid gap-3 sm:grid-cols-2">
                <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-950/40">
                  <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('activityCenter.fields.startsAt') }}</p>
                  <p class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ formatTime(selectedCampaign.starts_at) }}</p>
                </div>
                <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-950/40">
                  <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('activityCenter.fields.endsAt') }}</p>
                  <p class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ formatTime(selectedCampaign.ends_at) }}</p>
                </div>
              </div>

              <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-950/40">
                <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('activityCenter.fields.content') }}</p>
                <p class="mt-1 whitespace-pre-wrap text-sm leading-6 text-gray-600 dark:text-dark-300">
                  {{ selectedCampaign.content || t('activityCenter.noContent') }}
                </p>
              </div>

              <div class="grid gap-3 sm:grid-cols-2">
                <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-950/40">
                  <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('activityCenter.fields.refId') }}</p>
                  <p class="mt-1 break-words text-sm font-medium text-gray-900 dark:text-white">
                    {{ selectedCampaign.ref_id || t('activityCenter.noReference') }}
                  </p>
                </div>
                <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-950/40">
                  <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('activityCenter.fields.sortOrder') }}</p>
                  <p class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
                    {{ selectedCampaign.sort_order }}
                  </p>
                </div>
              </div>
            </div>
          </section>

          <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900/90">
            <div class="flex items-center justify-between gap-3">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('activityCenter.rules.title') }}</h2>
              <Icon name="shield" size="sm" class="text-primary-600 dark:text-primary-400" />
            </div>
            <ul class="mt-4 space-y-3 text-sm text-gray-600 dark:text-dark-300">
              <li v-for="rule in rules" :key="rule" class="flex gap-2">
                <Icon name="checkCircle" size="sm" class="mt-0.5 shrink-0 text-emerald-500" />
                <span>{{ rule }}</span>
              </li>
            </ul>
          </section>
        </aside>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/core/stores/appStore'
import { formatDateTime } from '@/core/utils/format'
import activityCenterAPI from '@/features/activity-center/data/datasources/activityCenterDatasource'
import type { ActivityCampaign } from '@/types'

import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import Icon from '@/common/widgets/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

const campaigns = ref<ActivityCampaign[]>([])
const loading = ref(false)
const searchQuery = ref('')
const typeFilter = ref<'all' | ActivityCampaign['type']>('all')
const statusFilter = ref<'all' | 'live' | 'scheduled' | 'draft'>('all')
const selectedCampaignId = ref<number | null>(null)

const selectedCampaign = computed(() => {
  return campaigns.value.find((item) => item.id === selectedCampaignId.value) ?? campaigns.value[0] ?? null
})

const metrics = computed(() => {
  const now = new Date()
  const live = campaigns.value.filter((item) => isLive(item, now)).length
  const scheduled = campaigns.value.filter((item) => isScheduled(item, now)).length
  const drafts = campaigns.value.filter((item) => item.status === 'draft').length
  const lottery = campaigns.value.filter((item) => item.type === 'lottery').length
  return [
    { key: 'live', label: t('activityCenter.metrics.live'), value: live, icon: 'fire' },
    { key: 'scheduled', label: t('activityCenter.metrics.scheduled'), value: scheduled, icon: 'calendar' },
    { key: 'drafts', label: t('activityCenter.metrics.drafts'), value: drafts, icon: 'document' },
    { key: 'lottery', label: t('activityCenter.metrics.lottery'), value: lottery, icon: 'gift' },
  ] as const
})

const rules = computed(() => [
  t('activityCenter.rules.one'),
  t('activityCenter.rules.two'),
  t('activityCenter.rules.three'),
  t('activityCenter.rules.four'),
])

const filteredCampaigns = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  const now = new Date()
  return campaigns.value.filter((item) => {
    const matchesType = typeFilter.value === 'all' || item.type === typeFilter.value
    const matchesStatus =
      statusFilter.value === 'all' ||
      (statusFilter.value === 'live' && isLive(item, now)) ||
      (statusFilter.value === 'scheduled' && isScheduled(item, now)) ||
      (statusFilter.value === 'draft' && item.status === 'draft')
    const haystack = [item.title, item.subtitle, item.ref_id, item.content, item.type, item.status].join(' ').toLowerCase()
    const matchesQuery = !query || haystack.includes(query)
    return matchesType && matchesStatus && matchesQuery
  }).sort((a, b) => a.sort_order - b.sort_order || b.id - a.id)
})

watch(
  filteredCampaigns,
  (items) => {
    if (items.length === 0) {
      selectedCampaignId.value = null
      return
    }
    if (!selectedCampaignId.value || !items.some((item) => item.id === selectedCampaignId.value)) {
      selectedCampaignId.value = items[0].id
    }
  },
  { immediate: true }
)

function isLive(item: ActivityCampaign, now: Date) {
  const start = item.starts_at ? new Date(item.starts_at) : null
  const end = item.ends_at ? new Date(item.ends_at) : null
  return item.status === 'active' && (!start || start <= now) && (!end || end > now)
}

function isScheduled(item: ActivityCampaign, now: Date) {
  const start = item.starts_at ? new Date(item.starts_at) : null
  return item.status === 'active' && !!start && start > now
}

function statusLabel(item: ActivityCampaign) {
  const now = new Date()
  if (isLive(item, now)) return t('activityCenter.status.live')
  if (isScheduled(item, now)) return t('activityCenter.status.scheduled')
  if (item.status === 'draft') return t('activityCenter.status.draft')
  return t('activityCenter.status.ended')
}

function statusTone(item: ActivityCampaign) {
  const now = new Date()
  if (isLive(item, now)) {
    return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-200'
  }
  if (isScheduled(item, now)) {
    return 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-200'
  }
  if (item.status === 'draft') {
    return 'bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-dark-300'
  }
  return 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-200'
}

function typeLabel(type: ActivityCampaign['type']) {
  return t(`activityCenter.types.${type}`)
}

function formatTime(raw?: string) {
  return raw ? formatDateTime(raw) : t('activityCenter.noTime')
}

function resetFilters() {
  searchQuery.value = ''
  typeFilter.value = 'all'
  statusFilter.value = 'all'
}

async function loadCampaigns() {
  loading.value = true
  try {
    campaigns.value = await activityCenterAPI.list()
  } catch (error: any) {
    console.error('Failed to load activity campaigns', error)
    appStore.showError(error?.message || t('activityCenter.failedToLoad'))
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void loadCampaigns()
})
</script>
