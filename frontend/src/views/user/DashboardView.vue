<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-12"><LoadingSpinner /></div>
      <template v-else-if="stats">
        <UserDashboardStats :stats="stats" :balance="user?.balance || 0" :is-simple="authStore.isSimpleMode" :platform-quotas="platformQuotas" />
        <UserDashboardCharts v-model:startDate="startDate" v-model:endDate="endDate" v-model:granularity="granularity" :loading="loadingCharts" :trend="trendData" :models="modelStats" @dateRangeChange="loadRangeData" @granularityChange="loadCharts" @refresh="refreshAll" />
        <UserDashboardApiKeyUsage :rows="apiKeyUsageRows" :loading="loadingApiKeys" :error="apiKeyUsageError" @retry="loadApiKeyUsage" />
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <div class="lg:col-span-2"><UserDashboardRecentUsage :data="recentUsage" :loading="loadingUsage" /></div>
          <div class="lg:col-span-1"><UserDashboardQuickActions /></div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'; import { useAuthStore } from '@/stores/auth'; import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import AppLayout from '@/components/layout/AppLayout.vue'; import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'; import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'; import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
import UserDashboardApiKeyUsage from '@/components/user/dashboard/UserDashboardApiKeyUsage.vue'
import type { UsageLog, TrendDataPoint, ModelStat, PlatformQuotaItem, ApiKey } from '@/types'
import { getMyPlatformQuotas } from '@/api/user'
import { keysAPI } from '@/api/keys'
import { formatDateLocalInput } from '@/utils/format'

const authStore = useAuthStore(); const user = computed(() => authStore.user)
const stats = ref<UserStatsType | null>(null); const loading = ref(false); const loadingUsage = ref(false); const loadingCharts = ref(false)
const trendData = ref<TrendDataPoint[]>([]); const modelStats = ref<ModelStat[]>([]); const recentUsage = ref<UsageLog[]>([])
const platformQuotas = ref<PlatformQuotaItem[] | null>(null)
const loadingApiKeys = ref(false)
const apiKeyUsageError = ref(false)
const apiKeyUsageRows = ref<Array<{ id: number, name: string, totalTokens: number, actualSpend: number }>>([])
let apiKeyUsageGeneration = 0

const startDate = ref(formatDateLocalInput(new Date(Date.now() - 6 * 86400000))); const endDate = ref(formatDateLocalInput(new Date())); const granularity = ref('day')

const loadStats = async () => { loading.value = true; try { await authStore.refreshUser(); stats.value = await usageAPI.getDashboardStats() } catch (error) { console.error('Failed to load dashboard stats:', error) } finally { loading.value = false } }
const loadCharts = async () => { loadingCharts.value = true; try { const res = await Promise.all([usageAPI.getDashboardTrend({ start_date: startDate.value, end_date: endDate.value, granularity: granularity.value as any }), usageAPI.getDashboardModels({ start_date: startDate.value, end_date: endDate.value })]); trendData.value = res[0].trend || []; modelStats.value = res[1].models || [] } catch (error) { console.error('Failed to load charts:', error) } finally { loadingCharts.value = false } }
const loadRecent = async () => { loadingUsage.value = true; try { const res = await usageAPI.getByDateRange(startDate.value, endDate.value); recentUsage.value = res.items.slice(0, 5) } catch (error) { console.error('Failed to load recent usage:', error) } finally { loadingUsage.value = false } }
const loadPlatformQuotas = async () => { try { const data = await getMyPlatformQuotas(); platformQuotas.value = data.platform_quotas ?? [] } catch (error) { console.warn('Failed to load platform quotas:', error); platformQuotas.value = [] } }
const loadApiKeyUsage = async () => {
  const generation = ++apiKeyUsageGeneration
  const range = { startDate: startDate.value, endDate: endDate.value }
  loadingApiKeys.value = true
  apiKeyUsageError.value = false
  try {
    const keys: ApiKey[] = []
    let page = 1
    let pages = 1
    do {
      const response = await keysAPI.list(page, 100)
      keys.push(...response.items)
      pages = response.pages
      page += 1
    } while (page <= pages)

    const stats = new Map<number, { total_tokens: number, total_actual_cost: number }>()
    for (let offset = 0; offset < keys.length; offset += 100) {
      const ids = keys.slice(offset, offset + 100).map(key => key.id)
      const response = await usageAPI.getDashboardApiKeysUsage(ids, {
        startDate: range.startDate,
        endDate: range.endDate
      })
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
const loadRangeData = () => { loadCharts(); loadApiKeyUsage() }
const refreshAll = () => { loadStats(); loadCharts(); loadRecent(); loadApiKeyUsage(); loadPlatformQuotas() }

onMounted(() => { refreshAll() })
</script>
