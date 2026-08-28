<template>
  <AppLayout>
    <div class="space-y-6">
      <div>
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

            <div class="lg:w-48">
              <div>
                <label class="input-label">{{ t('activityCenter.filters.typeLabel') }}</label>
                <select v-model="typeFilter" class="input">
                  <option value="all">{{ t('activityCenter.filters.allTypes') }}</option>
                  <option value="lottery">{{ t('activityCenter.types.lottery') }}</option>
                  <option value="redeem">{{ t('activityCenter.types.redeem') }}</option>
                  <option value="custom">{{ t('activityCenter.types.custom') }}</option>
                </select>
              </div>
            </div>

            <button
              type="button"
              class="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-gray-300 bg-white text-gray-600 transition-colors hover:bg-gray-50 hover:text-gray-900 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white lg:self-end"
              :disabled="loading"
              :title="t('activityCenter.refresh')"
              :aria-label="t('activityCenter.refresh')"
              @click="loadCampaigns"
            >
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>

          <div v-if="loading" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            <div v-for="n in 4" :key="n" class="h-64 animate-pulse rounded-2xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900/90"></div>
          </div>

          <div v-else-if="filteredCampaigns.length > 0" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            <RouterLink
              v-for="campaign in filteredCampaigns"
              :key="campaign.id"
              :to="`/activity-center/${campaign.id}`"
              class="block h-full rounded-2xl border border-gray-200 bg-white p-4 shadow-sm transition-shadow hover:shadow-md dark:border-dark-700 dark:bg-dark-900/90"
            >
              <div class="aspect-[16/9] overflow-hidden rounded-xl bg-gray-100 dark:bg-dark-800">
                <div
                  v-if="campaign.banner_html"
                  class="activity-banner-html h-full w-full"
                  v-html="sanitizeBannerHtml(campaign.banner_html)"
                ></div>
                <div v-else class="flex h-full items-center justify-center text-sm text-gray-400">
                  {{ t('activityCenter.noBanner') }}
                </div>
              </div>

              <div class="mt-4 flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <h2 class="truncate text-lg font-semibold text-gray-900 dark:text-white">
                    {{ activityText(campaign.title, t) }}
                  </h2>
                  <p class="mt-1 line-clamp-2 text-sm leading-6 text-gray-500 dark:text-dark-400">
                    {{ activityText(campaign.subtitle, t) || t('activityCenter.noSubtitle') }}
                  </p>
                </div>
                <span class="inline-flex shrink-0 items-center rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-200">
                  {{ typeLabel(campaign.type) }}
                </span>
              </div>

              <div class="mt-3 flex flex-wrap gap-2 text-xs text-gray-500 dark:text-dark-400">
                <span v-if="campaign.starts_at || campaign.ends_at" class="rounded-full bg-gray-100 px-2.5 py-1 dark:bg-dark-800">
                  {{ activityTimeRange(campaign) }}
                </span>
                <span class="rounded-full bg-gray-100 px-2.5 py-1 dark:bg-dark-800">
                  {{ campaignSummary(campaign) }}
                </span>
              </div>

              <div class="mt-4 flex items-center justify-between border-t border-gray-100 pt-3 text-sm dark:border-dark-800">
                <span class="text-gray-500 dark:text-dark-400">{{ t('activityCenter.card.open') }}</span>
                <Icon name="arrowRight" size="sm" class="text-primary-500" />
              </div>
            </RouterLink>
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
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import DOMPurify from 'dompurify'
import { useAppStore } from '@/core/stores/appStore'
import { extractI18nErrorMessage } from '@/core/utils/apiError'
import { formatDateTime } from '@/core/utils/format'
import activityCenterAPI from '@/features/activity-center/data/datasources/activityCenterDatasource'
import type { ActivityCampaignConfig, UserActivityCampaign } from '@/types'

import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import { activityText } from '@/features/activity-center/presentation/activityCenterText'

const { t } = useI18n()
const appStore = useAppStore()

const campaigns = ref<UserActivityCampaign[]>([])
const loading = ref(false)
const searchQuery = ref('')
const typeFilter = ref<'all' | UserActivityCampaign['type']>('all')

const filteredCampaigns = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return campaigns.value.filter((item) => {
    const matchesType = typeFilter.value === 'all' || item.type === typeFilter.value
    const haystack = [item.title, item.subtitle, item.content, item.type].join(' ').toLowerCase()
    const matchesQuery = !query || haystack.includes(query)
    return matchesType && matchesQuery
  })
})

function typeLabel(type: UserActivityCampaign['type']) {
  return t(`activityCenter.types.${type}`)
}

function activityTimeRange(item: UserActivityCampaign) {
  if (item.starts_at && item.ends_at) return `${formatTime(item.starts_at)} - ${formatTime(item.ends_at)}`
  if (item.starts_at) return t('activityCenter.fields.startsAtValue', { value: formatTime(item.starts_at) })
  return t('activityCenter.fields.endsAtValue', { value: formatTime(item.ends_at) })
}

function sanitizeBannerHtml(html: string) {
  return DOMPurify.sanitize(html)
}

function parseCampaignConfig(item: UserActivityCampaign): ActivityCampaignConfig {
  if (!item.config_json) return {}
  try {
    return JSON.parse(item.config_json) as ActivityCampaignConfig
  } catch {
    return {}
  }
}

function campaignSummary(item: UserActivityCampaign) {
  if (item.type === 'lottery') {
    const pools = parseCampaignConfig(item).lottery?.pools || []
    const prizeCount = pools.reduce((sum, pool) => sum + (pool.prizes?.length || 0), 0)
    return t('activityCenter.card.lotterySummary', { pools: pools.length, prizes: prizeCount })
  }
  if (item.type === 'redeem') return t('activityCenter.card.redeemSummary')
  return t('activityCenter.card.customSummary')
}

function formatTime(raw?: string) {
  return raw ? formatDateTime(raw) : t('activityCenter.noTime')
}

function resetFilters() {
  searchQuery.value = ''
  typeFilter.value = 'all'
}

async function loadCampaigns() {
  loading.value = true
  try {
    campaigns.value = await activityCenterAPI.list()
  } catch (error: any) {
    console.error('Failed to load activity campaigns', error)
    appStore.showError(extractI18nErrorMessage(error, t, 'activityCenter.errors', t('activityCenter.failedToLoad')))
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void loadCampaigns()
})
</script>

<style scoped>
.activity-banner-html :deep(img),
.activity-banner-html :deep(video),
.activity-banner-html :deep(canvas),
.activity-banner-html :deep(svg) {
  display: block;
  height: 100%;
  width: 100%;
  object-fit: cover;
}

.activity-banner-html :deep(a) {
  color: inherit;
  text-decoration: none;
}
</style>
