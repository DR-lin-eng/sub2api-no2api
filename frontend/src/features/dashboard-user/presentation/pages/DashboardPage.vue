<template>
  <AppLayout>
    <div class="space-y-6">
      <div
        v-if="statsError"
        role="alert"
        class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300"
      >
        <span>{{ $t('dashboard.statsLoadFailed') }}</span>
        <button
          type="button"
          class="btn btn-secondary btn-sm shrink-0"
          data-test="dashboard-stats-retry"
          :disabled="loading"
          @click="loadStats"
        >
          {{ $t('dashboard.retry') }}
        </button>
      </div>
      <div :aria-busy="loading" :class="{ 'animate-pulse': loading }" class="space-y-4">
        <UserDashboardStats
          :stats="displayStats"
          :balance="user?.balance || 0"
          :available-balance="user?.available_balance"
          :pending-settlement="user?.pending_settlement"
          :frozen-balance="user?.frozen_balance"
          :balance-sync-status="user?.balance_sync_status"
          :is-simple="authStore.isSimpleMode"
          :platform-quotas="platformQuotas"
        />
      </div>
        <section
          ref="chartsSectionRef"
          data-testid="dashboard-charts-section"
          :aria-busy="!chartsComponent && !chartsLoadError"
          class="min-h-[650px] lg:min-h-[360px]"
        >
          <component
            :is="chartsComponent"
            v-if="chartsComponent"
            v-model:startDate="startDate"
            v-model:endDate="endDate"
            v-model:granularity="granularity"
            :loading="loadingCharts"
            :trend="trendData"
            :models="modelStats"
            @dateRangeChange="loadRangeData"
            @granularityChange="loadCharts"
            @refresh="refreshAll"
          />
          <div v-else-if="!chartsLoadError" class="space-y-6" aria-hidden="true">
            <div class="card h-16 animate-pulse"></div>
            <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
              <div class="card h-64 animate-pulse"></div>
              <div class="card h-64 animate-pulse"></div>
            </div>
          </div>
          <div
            v-else
            role="alert"
            class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300"
          >
            <span>{{ $t('common.unknownError') }}</span>
            <button type="button" class="btn btn-secondary btn-sm" @click="loadChartsComponent">
              {{ $t('dashboard.retry') }}
            </button>
          </div>
        </section>
        <section
          ref="apiKeyUsageSectionRef"
          data-testid="dashboard-api-key-usage-section"
          :aria-busy="!apiKeyUsageRequested || loadingApiKeys"
        >
          <UserDashboardApiKeyUsage
            v-if="apiKeyUsageRequested"
            :rows="apiKeyUsageRows"
            :loading="loadingApiKeys"
            :error="apiKeyUsageError"
            @retry="loadApiKeyUsage"
          />
          <div v-else class="card h-40 animate-pulse" aria-hidden="true"></div>
        </section>
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <div
            ref="recentUsageSectionRef"
            data-testid="dashboard-recent-usage-section"
            :aria-busy="!recentUsageRequested || loadingUsage"
            class="lg:col-span-2"
          >
            <UserDashboardRecentUsage v-if="recentUsageRequested" :data="recentUsage" :loading="loadingUsage" />
            <div v-else class="card h-80 animate-pulse" aria-hidden="true"></div>
          </div>
          <div class="lg:col-span-1"><UserDashboardQuickActions /></div>
        </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, type Component } from 'vue'
import { useAuthStore } from '@/features/auth'
import { usageAPI, type UserDashboardStats as UserStatsType } from '@/features/usage/data/datasources/usageDatasource'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import UserDashboardStats from '@/features/dashboard-user/presentation/widgets/UserDashboardStats.vue'
import UserDashboardRecentUsage from '@/features/dashboard-user/presentation/widgets/UserDashboardRecentUsage.vue'
import UserDashboardQuickActions from '@/features/dashboard-user/presentation/widgets/UserDashboardQuickActions.vue'
import UserDashboardApiKeyUsage from '@/features/dashboard-user/presentation/widgets/UserDashboardApiKeyUsage.vue'
import type { UsageLog, TrendDataPoint, ModelStat, PlatformQuotaItem, ApiKey } from '@/types'
import { getMyPlatformQuotas } from '@/features/profile/data/datasources/profileDatasource'
import { keysAPI } from '@/features/keys/data/datasources/keysDatasource'
import { formatDateLocalInput } from '@/core/utils/format'

const authStore = useAuthStore()
const user = computed(() => authStore.user)
const stats = ref<UserStatsType | null>(null)
const loading = ref(false)
const loadingUsage = ref(false)
const loadingCharts = ref(false)
const statsError = ref(false)
const displayStats = computed(() => stats.value ?? ({} as UserStatsType))
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const recentUsage = ref<UsageLog[]>([])
const platformQuotas = ref<PlatformQuotaItem[] | null>(null)
const loadingApiKeys = ref(false)
const apiKeyUsageRequested = ref(false)
const recentUsageRequested = ref(false)
const apiKeyUsageError = ref(false)
const apiKeyUsageRows = ref<Array<{ id: number, name: string, totalTokens: number, actualSpend: number }>>([])
const chartsSectionRef = ref<HTMLElement | null>(null)
const apiKeyUsageSectionRef = ref<HTMLElement | null>(null)
const recentUsageSectionRef = ref<HTMLElement | null>(null)
const chartsComponent = shallowRef<Component | null>(null)
const chartsLoadError = ref(false)
let chartsModuleRequest: Promise<void> | null = null
let deferredSectionObserver: IntersectionObserver | null = null
const deferredSectionCallbacks = new Map<Element, () => void>()
let apiKeyUsageGeneration = 0
const apiKeyUsageRequestConcurrency = 4

async function mapWithConcurrency<T, R>(items: T[], limit: number, worker: (item: T) => Promise<R>): Promise<R[]> {
  const results = new Array<R>(items.length)
  let nextIndex = 0
  const workerCount = Math.min(Math.max(1, limit), items.length)
  await Promise.all(Array.from({ length: workerCount }, async () => {
    while (nextIndex < items.length) {
      const index = nextIndex++
      results[index] = await worker(items[index]!)
    }
  }))
  return results
}

const startDate = ref(formatDateLocalInput(new Date(Date.now() - 6 * 86400000)))
const endDate = ref(formatDateLocalInput(new Date()))
const granularity = ref('day')

async function loadStats(): Promise<void> {
  loading.value = true
  statsError.value = false
  try {
    await authStore.refreshUser()
    stats.value = await usageAPI.getDashboardStats()
  } catch (error) {
    statsError.value = true
    console.error('Failed to load dashboard stats:', error)
  } finally {
    loading.value = false
  }
}

async function loadCharts(): Promise<void> {
  loadingCharts.value = true
  try {
    const [trend, models] = await Promise.all([
      usageAPI.getDashboardTrend({
        start_date: startDate.value,
        end_date: endDate.value,
        granularity: granularity.value as any,
      }),
      usageAPI.getDashboardModels({
        start_date: startDate.value,
        end_date: endDate.value,
      }),
    ])
    trendData.value = trend.trend || []
    modelStats.value = models.models || []
  } catch (error) {
    console.error('Failed to load charts:', error)
  } finally {
    loadingCharts.value = false
  }
}

async function loadRecent(): Promise<void> {
  loadingUsage.value = true
  try {
    const result = await usageAPI.getByDateRange(startDate.value, endDate.value)
    recentUsage.value = result.items.slice(0, 5)
  } catch (error) {
    console.error('Failed to load recent usage:', error)
  } finally {
    loadingUsage.value = false
  }
}

async function loadPlatformQuotas(): Promise<void> {
  try {
    const data = await getMyPlatformQuotas()
    platformQuotas.value = data.platform_quotas ?? []
  } catch (error) {
    console.warn('Failed to load platform quotas:', error)
    platformQuotas.value = []
  }
}
const loadChartsComponent = async () => {
  if (chartsModuleRequest) return chartsModuleRequest

  chartsLoadError.value = false
  chartsModuleRequest = import('@/features/dashboard-user/presentation/widgets/UserDashboardCharts.vue')
    .then((module) => {
      chartsComponent.value = module.default
    })
    .catch((error) => {
      chartsLoadError.value = true
      console.error('Failed to load dashboard charts:', error)
    })
    .finally(() => {
      chartsModuleRequest = null
    })

  return chartsModuleRequest
}
const loadApiKeyUsage = async () => {
  const generation = ++apiKeyUsageGeneration
  const range = { startDate: startDate.value, endDate: endDate.value }
  loadingApiKeys.value = true
  apiKeyUsageError.value = false
  try {
    const firstPage = await keysAPI.list(1, 100)
    const remainingPages = Array.from({ length: Math.max(0, firstPage.pages - 1) }, (_, index) => index + 2)
    const pageResponses = await mapWithConcurrency(
      remainingPages,
      apiKeyUsageRequestConcurrency,
      page => keysAPI.list(page, 100)
    )
    const keys: ApiKey[] = [firstPage, ...pageResponses].flatMap(response => response.items)
    if (generation !== apiKeyUsageGeneration) return

    const stats = new Map<number, { total_tokens: number, total_actual_cost: number }>()
    const idBatches = Array.from({ length: Math.ceil(keys.length / 100) }, (_, index) =>
      keys.slice(index * 100, index * 100 + 100).map(key => key.id)
    )
    const usageResponses = await mapWithConcurrency(
      idBatches,
      apiKeyUsageRequestConcurrency,
      ids => usageAPI.getDashboardApiKeysUsage(ids, {
        startDate: range.startDate,
        endDate: range.endDate
      })
    )
    for (const response of usageResponses) {
      Object.values(response.stats).forEach(item => stats.set(item.api_key_id, item))
    }
    if (generation !== apiKeyUsageGeneration) return
    apiKeyUsageRows.value = keys.map(key => ({
      id: key.id,
      name: key.name,
      totalTokens: stats.get(key.id)?.total_tokens ?? 0,
      actualSpend: stats.get(key.id)?.total_actual_cost ?? 0
    }))
  } catch (error) {
    console.error('Failed to load API key usage:', error)
    if (generation === apiKeyUsageGeneration) apiKeyUsageError.value = true
  } finally {
    if (generation === apiKeyUsageGeneration) loadingApiKeys.value = false
  }
}
function loadRangeData(): void {
  void loadCharts()
  if (apiKeyUsageRequested.value) void loadApiKeyUsage()
}

function stopObservingDeferredSections(): void {
  deferredSectionObserver?.disconnect()
  deferredSectionObserver = null
  deferredSectionCallbacks.clear()
}

function refreshAll(): void {
  apiKeyUsageRequested.value = true
  recentUsageRequested.value = true
  stopObservingDeferredSections()
  void loadStats()
  void loadCharts()
  void loadRecent()
  void loadApiKeyUsage()
  void loadPlatformQuotas()
}

function loadChartsSection(): void {
  void loadChartsComponent()
  void loadCharts()
}

function loadApiKeyUsageSection(): void {
  apiKeyUsageRequested.value = true
  void loadApiKeyUsage()
}

function loadRecentUsageSection(): void {
  recentUsageRequested.value = true
  void loadRecent()
}

function observeDeferredSection(element: HTMLElement | null, onVisible: () => void): void {
  if (!element || !deferredSectionObserver) {
    onVisible()
    return
  }

  deferredSectionCallbacks.set(element, onVisible)
  deferredSectionObserver.observe(element)
}

function loadDeferredSectionOnIntersection(entries: IntersectionObserverEntry[]): void {
  entries.forEach((entry) => {
    if (!entry.isIntersecting) return

    const onVisible = deferredSectionCallbacks.get(entry.target)
    if (!onVisible) return

    deferredSectionCallbacks.delete(entry.target)
    deferredSectionObserver?.unobserve(entry.target)
    onVisible()
  })
}

onMounted(() => {
  void loadStats()
  void loadPlatformQuotas()

  if (typeof IntersectionObserver === 'undefined') {
    loadChartsSection()
    loadApiKeyUsageSection()
    loadRecentUsageSection()
    return
  }

  deferredSectionObserver = new IntersectionObserver(loadDeferredSectionOnIntersection, {
    rootMargin: '150px 0px',
  })
  observeDeferredSection(chartsSectionRef.value, loadChartsSection)
  observeDeferredSection(apiKeyUsageSectionRef.value, loadApiKeyUsageSection)
  observeDeferredSection(recentUsageSectionRef.value, loadRecentUsageSection)
})

onBeforeUnmount(() => {
  stopObservingDeferredSections()
})
</script>
