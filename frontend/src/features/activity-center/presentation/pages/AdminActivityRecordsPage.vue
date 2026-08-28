<template>
  <AppLayout>
    <div class="space-y-5">
      <div class="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.activityCenter.records.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.records.description') }}</p>
        </div>
        <button type="button" class="btn btn-secondary" :disabled="loading" :title="t('common.refresh')" @click="loadRecords">
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
        </button>
      </div>

      <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900/90">
        <div class="flex flex-wrap gap-3">
          <input v-model="search" class="input min-w-64 flex-1" :placeholder="t('admin.activityCenter.records.searchPlaceholder')" @keyup.enter="resetAndLoad" />
          <select v-model="type" class="input w-44" @change="resetAndLoad">
            <option value="">{{ t('admin.activityCenter.filters.allTypes') }}</option>
            <option value="lottery">{{ t('admin.activityCenter.types.lottery') }}</option>
            <option value="redeem">{{ t('admin.activityCenter.types.redeem') }}</option>
            <option value="custom">{{ t('admin.activityCenter.types.custom') }}</option>
          </select>
          <button type="button" class="btn btn-secondary" @click="resetAndLoad">{{ t('common.search') }}</button>
        </div>
      </section>

      <section class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900/90">
        <div class="overflow-x-auto">
          <table class="min-w-full text-left text-sm">
            <thead class="border-b border-gray-200 bg-gray-50 text-xs font-semibold text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-400">
              <tr><th class="px-4 py-3">{{ t('admin.activityCenter.records.columns.user') }}</th><th class="px-4 py-3">{{ t('admin.activityCenter.records.columns.campaign') }}</th><th class="px-4 py-3">{{ t('admin.activityCenter.records.columns.prize') }}</th><th class="px-4 py-3">{{ t('admin.activityCenter.records.columns.rewardStatus') }}</th><th class="px-4 py-3">{{ t('admin.activityCenter.records.columns.createdAt') }}</th></tr>
            </thead>
            <tbody v-if="records.length" class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="record in records" :key="record.id" class="align-top">
                <td class="px-4 py-3"><div class="font-medium text-gray-900 dark:text-white">{{ record.user_email || record.user_name || `#${record.user_id}` }}</div><div class="mt-1 text-xs text-gray-500">#{{ record.user_id }}</div></td>
                <td class="px-4 py-3"><div class="font-medium text-gray-900 dark:text-white">{{ activityText(record.campaign_title, t, 'admin.activityCenter') }}</div><div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ activityText(record.pool_name, t, 'admin.activityCenter') || typeLabel(record.campaign_type) }}</div></td>
                <td class="max-w-md px-4 py-3"><div class="font-medium text-gray-800 dark:text-dark-200">{{ activityText(record.prize_label, t, 'admin.activityCenter') || t('admin.activityCenter.records.noPrize') }}</div><div v-if="rewardDetail(record)" class="mt-1 break-all text-primary-600 dark:text-primary-300">{{ rewardDetail(record) }}</div></td>
                <td class="px-4 py-3"><span class="badge badge-gray">{{ statusLabel(record.reward_status) }}</span></td>
                <td class="whitespace-nowrap px-4 py-3 text-gray-500 dark:text-dark-400">{{ formatDateTime(record.created_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="!loading && !records.length" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.records.empty') }}</p>
        <Pagination v-if="pagination.total > 0" class="border-t border-gray-100 px-4 py-3 dark:border-dark-700" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="changePage" @update:pageSize="changePageSize" />
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
import adminActivityCenterAPI from '@/features/activity-center/data/datasources/adminActivityCenterDatasource'
import type { ActivityParticipationRecord, UserActivityCampaign } from '@/types'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import Pagination from '@/common/widgets/data/Pagination.vue'
import { activityText } from '@/features/activity-center/presentation/activityCenterText'

const { t } = useI18n()
const appStore = useAppStore()
const records = ref<ActivityParticipationRecord[]>([])
const loading = ref(false)
const search = ref('')
const type = ref('')
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
function typeLabel(value: UserActivityCampaign['type']) { return t(`admin.activityCenter.types.${value}`) }
function statusLabel(value: ActivityParticipationRecord['reward_status']) { return t(`admin.activityCenter.records.rewardStatus.${value}`) }
function rewardDetail(record: ActivityParticipationRecord) {
  if (record.reward_code || record.reward_value) return record.reward_code || record.reward_value || ''
  if (!record.reward_payload_json) return ''
  try {
    const payload = JSON.parse(record.reward_payload_json) as Record<string, unknown>
    return String(payload.code || payload.value || payload.value_amount || '')
  } catch { return '' }
}
async function loadRecords() {
  loading.value = true
  try {
    const response = await adminActivityCenterAPI.listRecords(pagination.page, pagination.page_size, { search: search.value || undefined, type: type.value || undefined })
    records.value = response.items
    pagination.page = response.page
    pagination.page_size = response.page_size
    pagination.total = response.total
  } catch (error: any) { appStore.showError(extractI18nErrorMessage(error, t, 'activityCenter.errors', t('admin.activityCenter.records.failedToLoad'))) } finally { loading.value = false }
}
function resetAndLoad() { pagination.page = 1; void loadRecords() }
function changePage(page: number) { pagination.page = page; void loadRecords() }
function changePageSize(pageSize: number) { pagination.page = 1; pagination.page_size = pageSize; void loadRecords() }
onMounted(() => { void loadRecords() })
</script>
