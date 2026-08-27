<template>
  <AppLayout>
    <div class="space-y-6">
      <section class="rounded-2xl border border-gray-200 bg-white/95 p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900/90">
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
            <p class="mt-3 text-xs text-gray-400 dark:text-dark-500">
              {{ syncLabel }}
            </p>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary" :disabled="refreshing" @click="handleRefresh">
              <Icon name="refresh" size="sm" :class="refreshing ? 'animate-spin' : ''" />
              <span class="ml-1">{{ t('activityCenter.refresh') }}</span>
            </button>
            <button type="button" class="btn btn-primary" @click="scrollToRules">
              <Icon name="book" size="sm" class="mr-1" />
              {{ t('activityCenter.viewRules') }}
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
                <label class="input-label">{{ t('activityCenter.filters.categoryLabel') }}</label>
                <select v-model="categoryFilter" class="input">
                  <option value="all">{{ t('activityCenter.filters.allCategories') }}</option>
                  <option value="featured">{{ t('activityCenter.filters.featured') }}</option>
                  <option value="rewards">{{ t('activityCenter.filters.rewards') }}</option>
                  <option value="events">{{ t('activityCenter.filters.events') }}</option>
                  <option value="community">{{ t('activityCenter.filters.community') }}</option>
                </select>
              </div>
              <div>
                <label class="input-label">{{ t('activityCenter.filters.sortLabel') }}</label>
                <select v-model="sortMode" class="input">
                  <option value="featured">{{ t('activityCenter.filters.featuredFirst') }}</option>
                  <option value="endingSoon">{{ t('activityCenter.filters.endingSoon') }}</option>
                </select>
              </div>
            </div>
          </div>

          <div class="flex flex-wrap gap-2">
            <button
              v-for="tab in tabs"
              :key="tab.value"
              type="button"
              class="rounded-full border px-4 py-2 text-sm font-medium transition-colors"
              :class="activeTab === tab.value
                ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-500/60 dark:bg-primary-900/30 dark:text-primary-200'
                : 'border-gray-200 bg-white text-gray-600 hover:border-gray-300 hover:text-gray-900 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-300 dark:hover:text-white'"
              @click="activeTab = tab.value"
            >
              {{ tab.label }}
            </button>
          </div>

          <div v-if="filteredActivities.length > 0" class="space-y-3">
            <article
              v-for="activity in filteredActivities"
              :key="activity.id"
              class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm transition-shadow hover:shadow-md dark:border-dark-700 dark:bg-dark-900/90"
            >
              <div class="flex flex-col gap-4">
                <div class="flex flex-wrap items-start justify-between gap-3">
                  <div class="min-w-0">
                    <div class="flex flex-wrap items-center gap-2">
                      <h2 class="truncate text-lg font-semibold text-gray-900 dark:text-white">
                        {{ activity.title }}
                      </h2>
                      <span :class="['inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium', statusTone(activity.status)]">
                        {{ statusLabel(activity.status) }}
                      </span>
                      <span class="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-800 dark:text-dark-300">
                        {{ categoryLabel(activity.category) }}
                      </span>
                    </div>
                    <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-500 dark:text-dark-400">
                      {{ activity.summary }}
                    </p>
                  </div>

                  <button
                    type="button"
                    class="btn btn-secondary btn-sm"
                    @click="selectActivity(activity.id)"
                  >
                    <Icon name="arrowRight" size="sm" class="mr-1" />
                    {{ t('activityCenter.actions.details') }}
                  </button>
                </div>

                <div class="flex flex-wrap gap-2">
                  <span
                    v-for="tag in activity.tags"
                    :key="tag"
                    class="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-500 dark:bg-dark-800 dark:text-dark-300"
                  >
                    {{ tag }}
                  </span>
                </div>

                <div>
                  <div class="mb-2 flex items-center justify-between text-xs text-gray-500 dark:text-dark-400">
                    <span>{{ t('activityCenter.selected.progress') }}</span>
                    <span>{{ activity.progress }}%</span>
                  </div>
                  <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-800">
                    <div class="h-full rounded-full bg-primary-500" :style="{ width: `${activity.progress}%` }"></div>
                  </div>
                </div>

                <div class="grid gap-3 md:grid-cols-3">
                  <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-950/40">
                    <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('activityCenter.selected.reward') }}</p>
                    <p class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ activity.reward }}</p>
                  </div>
                  <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-950/40">
                    <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('activityCenter.selected.deadline') }}</p>
                    <p class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ activity.deadline }}</p>
                  </div>
                  <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-950/40">
                    <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('activityCenter.selected.task') }}</p>
                    <p class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ activity.task }}</p>
                  </div>
                </div>
              </div>
            </article>
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
          <section id="activity-center-rules" class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900/90">
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

          <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900/90">
            <div class="flex items-center justify-between gap-3">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('activityCenter.selected.title') }}</h2>
              <Icon name="badge" size="sm" class="text-primary-600 dark:text-primary-400" />
            </div>

            <div v-if="selectedActivity" class="mt-4 space-y-4">
              <div>
                <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">
                  {{ categoryLabel(selectedActivity.category) }}
                </p>
                <h3 class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">
                  {{ selectedActivity.title }}
                </h3>
                <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-dark-400">
                  {{ selectedActivity.summary }}
                </p>
              </div>

              <div class="grid gap-3 sm:grid-cols-2">
                <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-950/40">
                  <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('activityCenter.selected.reward') }}</p>
                  <p class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ selectedActivity.reward }}</p>
                </div>
                <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-950/40">
                  <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('activityCenter.selected.deadline') }}</p>
                  <p class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ selectedActivity.deadline }}</p>
                </div>
              </div>

              <div>
                <div class="mb-2 flex items-center justify-between text-xs text-gray-500 dark:text-dark-400">
                  <span>{{ t('activityCenter.selected.progress') }}</span>
                  <span>{{ selectedActivity.progress }}%</span>
                </div>
                <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-800">
                  <div class="h-full rounded-full bg-primary-500" :style="{ width: `${selectedActivity.progress}%` }"></div>
                </div>
              </div>

              <div class="rounded-xl border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-700 dark:bg-dark-950/40">
                <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('activityCenter.selected.note') }}</p>
                <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-dark-300">
                  {{ selectedActivity.note }}
                </p>
              </div>
            </div>
          </section>

          <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900/90">
            <div class="flex items-center justify-between gap-3">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('activityCenter.timeline.title') }}</h2>
              <Icon name="calendar" size="sm" class="text-primary-600 dark:text-primary-400" />
            </div>
            <ol class="mt-4 space-y-4">
              <li v-for="(step, index) in timeline" :key="step" class="flex gap-3">
                <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary-50 text-xs font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-200">
                  {{ index + 1 }}
                </span>
                <p class="text-sm leading-6 text-gray-600 dark:text-dark-300">
                  {{ step }}
                </p>
              </li>
            </ol>
          </section>
        </aside>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import Icon from '@/common/widgets/icons/Icon.vue'

type ActivityTab = 'all' | 'live' | 'upcoming' | 'joined'
type ActivityCategory = 'featured' | 'rewards' | 'events' | 'community'
type ActivitySort = 'featured' | 'endingSoon'
type ActivityStatus = 'live' | 'upcoming' | 'ended' | 'joined'

interface ActivityItem {
  id: string
  title: string
  summary: string
  reward: string
  task: string
  deadline: string
  category: ActivityCategory
  status: ActivityStatus
  progress: number
  note: string
  tags: string[]
}

const { t } = useI18n()

const activeTab = ref<ActivityTab>('all')
const categoryFilter = ref<ActivityCategory | 'all'>('all')
const sortMode = ref<ActivitySort>('featured')
const searchQuery = ref('')
const selectedActivityId = ref('spring-bonus')
const refreshing = ref(false)
const lastSyncedAt = ref(new Date())

const tabs = computed(() => ([
  { value: 'all' as const, label: t('activityCenter.tabs.all') },
  { value: 'live' as const, label: t('activityCenter.tabs.live') },
  { value: 'upcoming' as const, label: t('activityCenter.tabs.upcoming') },
  { value: 'joined' as const, label: t('activityCenter.tabs.joined') },
]))

const activityItems = computed<ActivityItem[]>(() => ([
  {
    id: 'spring-bonus',
    title: t('activityCenter.activities.springBonus.title'),
    summary: t('activityCenter.activities.springBonus.summary'),
    reward: t('activityCenter.activities.springBonus.reward'),
    task: t('activityCenter.activities.springBonus.task'),
    deadline: t('activityCenter.activities.springBonus.deadline'),
    category: 'rewards',
    status: 'live',
    progress: 72,
    note: t('activityCenter.activities.springBonus.task'),
    tags: [t('activityCenter.filters.rewards'), t('activityCenter.tabs.live')],
  },
  {
    id: 'referral-sprint',
    title: t('activityCenter.activities.referralSprint.title'),
    summary: t('activityCenter.activities.referralSprint.summary'),
    reward: t('activityCenter.activities.referralSprint.reward'),
    task: t('activityCenter.activities.referralSprint.task'),
    deadline: t('activityCenter.activities.referralSprint.deadline'),
    category: 'community',
    status: 'joined',
    progress: 48,
    note: t('activityCenter.activities.referralSprint.summary'),
    tags: [t('activityCenter.filters.community'), t('activityCenter.tabs.joined')],
  },
  {
    id: 'weekend-checkin',
    title: t('activityCenter.activities.weekendCheckIn.title'),
    summary: t('activityCenter.activities.weekendCheckIn.summary'),
    reward: t('activityCenter.activities.weekendCheckIn.reward'),
    task: t('activityCenter.activities.weekendCheckIn.task'),
    deadline: t('activityCenter.activities.weekendCheckIn.deadline'),
    category: 'events',
    status: 'upcoming',
    progress: 0,
    note: t('activityCenter.activities.weekendCheckIn.summary'),
    tags: [t('activityCenter.filters.events'), t('activityCenter.tabs.upcoming')],
  },
  {
    id: 'community-spotlight',
    title: t('activityCenter.activities.communitySpotlight.title'),
    summary: t('activityCenter.activities.communitySpotlight.summary'),
    reward: t('activityCenter.activities.communitySpotlight.reward'),
    task: t('activityCenter.activities.communitySpotlight.task'),
    deadline: t('activityCenter.activities.communitySpotlight.deadline'),
    category: 'featured',
    status: 'ended',
    progress: 100,
    note: t('activityCenter.activities.communitySpotlight.summary'),
    tags: [t('activityCenter.filters.featured'), t('activityCenter.status.ended')],
  },
]))

const metrics = computed(() => {
  const items = activityItems.value
  return [
    { key: 'live', label: t('activityCenter.metrics.live'), value: items.filter((item) => item.status === 'live').length, icon: 'fire' },
    { key: 'rewards', label: t('activityCenter.metrics.rewards'), value: items.filter((item) => item.status !== 'ended').length, icon: 'gift' },
    { key: 'joined', label: t('activityCenter.metrics.joined'), value: items.filter((item) => item.status === 'joined').length, icon: 'users' },
    { key: 'endingSoon', label: t('activityCenter.metrics.endingSoon'), value: items.filter((item) => item.status === 'live' || item.status === 'upcoming').length, icon: 'clock' },
  ] as const
})

const rules = computed(() => [
  t('activityCenter.rules.one'),
  t('activityCenter.rules.two'),
  t('activityCenter.rules.three'),
  t('activityCenter.rules.four'),
])

const timeline = computed(() => [
  t('activityCenter.timeline.one'),
  t('activityCenter.timeline.two'),
  t('activityCenter.timeline.three'),
])

const selectedActivity = computed(() => filteredActivities.value.find((item) => item.id === selectedActivityId.value) ?? filteredActivities.value[0] ?? activityItems.value[0] ?? null)

const filteredActivities = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  const items = activityItems.value.filter((item) => {
    const matchesTab =
      activeTab.value === 'all' ||
      (activeTab.value === 'live' && item.status === 'live') ||
      (activeTab.value === 'upcoming' && item.status === 'upcoming') ||
      (activeTab.value === 'joined' && item.status === 'joined')
    const matchesCategory = categoryFilter.value === 'all' || item.category === categoryFilter.value
    const haystack = [item.title, item.summary, item.reward, item.task, item.deadline, ...item.tags].join(' ').toLowerCase()
    const matchesQuery = !query || haystack.includes(query)
    return matchesTab && matchesCategory && matchesQuery
  })

  const sorted = [...items]
  if (sortMode.value === 'endingSoon') {
    sorted.sort((a, b) => a.progress - b.progress)
  } else {
    const priority: Record<ActivityStatus, number> = { live: 0, joined: 1, upcoming: 2, ended: 3 }
    sorted.sort((a, b) => priority[a.status] - priority[b.status] || b.progress - a.progress)
  }

  return sorted
})

const syncLabel = computed(() => {
  const time = lastSyncedAt.value.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  return `${t('common.lastUpdated')}: ${time}`
})

function statusLabel(status: ActivityStatus) {
  return t(`activityCenter.status.${status}`)
}

function categoryLabel(category: ActivityCategory) {
  return t(`activityCenter.filters.${category}`)
}

function statusTone(status: ActivityStatus) {
  switch (status) {
    case 'live':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-200'
    case 'joined':
      return 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-200'
    case 'upcoming':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-200'
    case 'ended':
      return 'bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-dark-300'
  }
}

function selectActivity(id: string) {
  selectedActivityId.value = id
}

function resetFilters() {
  activeTab.value = 'all'
  categoryFilter.value = 'all'
  sortMode.value = 'featured'
  searchQuery.value = ''
}

function scrollToRules() {
  document.getElementById('activity-center-rules')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

function handleRefresh() {
  refreshing.value = true
  resetFilters()
  lastSyncedAt.value = new Date()
  window.setTimeout(() => {
    refreshing.value = false
  }, 240)
}
</script>
