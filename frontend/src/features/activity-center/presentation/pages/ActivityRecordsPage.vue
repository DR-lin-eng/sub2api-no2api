<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-5">
      <div class="flex items-end justify-between gap-4">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('activityCenter.records.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('activityCenter.records.description') }}</p>
        </div>
        <button type="button" class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 dark:hover:bg-dark-800" :title="t('activityCenter.refresh')" @click="loadRecords">
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
        </button>
      </div>

      <section class="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900/90">
        <div v-if="records.length" class="divide-y divide-gray-100 dark:divide-dark-700">
          <article v-for="record in records" :key="record.id" class="relative px-5 py-4 pr-28">
            <span :class="['absolute right-5 top-4 rounded-full px-2.5 py-1 text-xs font-semibold', recordStatusClass(record)]">{{ recordStatusLabel(record) }}</span>
            <h2 class="font-medium text-gray-900 dark:text-white">{{ activityText(record.campaign_title, t) }}</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ record.pool_name || typeLabel(record.campaign_type) }} · {{ formatTime(record.created_at) }}</p>
            <p v-if="record.prize_label" class="mt-2 text-sm text-gray-700 dark:text-dark-200">{{ activityText(record.prize_label, t) }}</p>
            <p v-if="record.reward_value" class="mt-1 text-sm font-medium text-primary-700 dark:text-primary-300">{{ record.reward_value }}</p>
            <div v-if="record.reward_code" class="mt-2 border-l-2 border-primary-300 pl-2.5 dark:border-primary-600">
              <div class="flex items-center gap-1 text-xs text-gray-500 dark:text-dark-400"><Icon name="key" size="xs" />{{ t('activityCenter.prizeTypes.card') }}</div>
              <ul class="mt-1 space-y-1">
                <li v-for="code in rewardCodes(record)" :key="code" class="flex gap-2 text-sm font-semibold text-gray-800 dark:text-gray-100"><span class="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-primary-400"></span><code class="break-all">{{ code }}</code></li>
              </ul>
            </div>
          </article>
        </div>
        <p v-else class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('activityCenter.records.empty') }}</p>
        <Pagination v-if="pagination.total > pagination.page_size" class="border-t border-gray-100 px-5 py-3 dark:border-dark-700" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="changePage" @update:pageSize="changePageSize" />
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/core/stores/appStore'
import { extractI18nErrorMessage } from '@/core/utils/apiError'
import { formatDateTime } from '@/core/utils/format'
import activityCenterAPI from '@/features/activity-center/data/datasources/activityCenterDatasource'
import type { ActivityParticipationRecord, UserActivityCampaign } from '@/types'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import Pagination from '@/common/widgets/data/Pagination.vue'
import { activityText } from '@/features/activity-center/presentation/activityCenterText'

const { t } = useI18n()
const appStore = useAppStore()
const records = ref<ActivityParticipationRecord[]>([])
const loading = ref(false)
const pagination = reactive({ page: 1, page_size: 10, total: 0 })

function typeLabel(type: UserActivityCampaign['type']) { return t(`activityCenter.types.${type}`) }
function formatTime(raw?: string) { return raw ? formatDateTime(raw) : t('activityCenter.noTime') }
function rewardCodes(record: ActivityParticipationRecord) { return (record.reward_code || '').split(/\r?\n/).map((code) => code.trim()).filter(Boolean) }
function recordStatusLabel(record: ActivityParticipationRecord) {
  if (record.result_status === 'none') return t('activityCenter.records.none')
  if (record.reward_status === 'pending') return t('activityCenter.records.pending')
  if (record.reward_status === 'granted') return t('activityCenter.records.granted')
  if (record.reward_status === 'failed') return t('activityCenter.records.failed')
  return t('activityCenter.records.recorded')
}
function recordStatusClass(record: ActivityParticipationRecord) {
  if (record.result_status === 'none') return 'bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-dark-300'
  if (record.reward_status === 'granted') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (record.reward_status === 'failed') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (record.reward_status === 'pending') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
}
async function loadRecords() {
  loading.value = true
  try {
    const response = await activityCenterAPI.listMyRecords(pagination.page, pagination.page_size)
    records.value = response.items
    pagination.page = response.page
    pagination.page_size = response.page_size
    pagination.total = response.total
  } catch (error: any) {
    appStore.showError(extractI18nErrorMessage(error, t, 'activityCenter.errors', t('activityCenter.records.failedToLoad')))
  } finally { loading.value = false }
}
function changePage(page: number) { pagination.page = page; void loadRecords() }
function changePageSize(pageSize: number) { pagination.page = 1; pagination.page_size = pageSize; void loadRecords() }
onMounted(() => { void loadRecords() })
</script>
