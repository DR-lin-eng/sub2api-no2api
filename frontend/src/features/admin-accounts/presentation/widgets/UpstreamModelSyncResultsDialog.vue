<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.bulkActions.syncUpstreamModels')"
    width="extra-wide"
    :close-on-escape="!running"
    :show-close-button="!running"
    @close="$emit('close')"
  >
    <p class="mb-3 text-sm text-gray-600 dark:text-gray-300">
      {{ t('admin.accounts.bulkActions.modelSyncHint') }}
    </p>
    <p role="status" aria-live="polite" class="mb-3 text-sm font-medium text-gray-900 dark:text-gray-100">
      {{ t('admin.accounts.bulkActions.modelSyncProgress', { completed, total }) }}
      · {{ t('admin.accounts.bulkActions.modelSyncSummary', summary) }}
    </p>
    <div class="max-h-96 overflow-auto">
      <table class="w-full text-left text-sm">
        <thead class="text-gray-500 dark:text-gray-400">
          <tr>
            <th class="p-2">{{ t('common.name') }}</th>
            <th class="p-2">{{ t('common.status') }}</th>
            <th class="p-2">{{ t('admin.accounts.bulkActions.modelSyncModels') }}</th>
            <th class="p-2">{{ t('admin.accounts.bulkActions.modelSyncAdded') }}</th>
            <th class="p-2">{{ t('admin.accounts.bulkActions.modelSyncDetail') }}</th>
          </tr>
        </thead>
        <tbody class="text-gray-900 dark:text-gray-100">
          <tr v-for="result in visibleResults" :key="result.account_id" class="border-t border-gray-100 dark:border-dark-700">
            <td class="p-2">{{ result.account_name || result.account_id }}</td>
            <td class="p-2" :class="result.status === 'success' ? 'text-green-600 dark:text-green-400' : 'text-amber-600 dark:text-amber-400'">
              {{ statusLabel(result.status) }}
            </td>
            <td class="p-2">{{ result.status === 'success' ? result.model_count : '—' }}</td>
            <td class="p-2">{{ result.status === 'success' ? result.added_count : '—' }}</td>
            <td class="p-2">{{ result.error || t('admin.accounts.bulkActions.modelSyncSaved') }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="pageCount > 1" class="mt-3 flex items-center justify-end gap-3 text-sm">
      <button class="btn btn-secondary btn-sm" :disabled="page === 1" @click="page--">{{ t('admin.accounts.bulkActions.modelSyncPrevious') }}</button>
      <span>{{ page }} / {{ pageCount }}</span>
      <button class="btn btn-secondary btn-sm" :disabled="page === pageCount" @click="page++">{{ t('admin.accounts.bulkActions.modelSyncNext') }}</button>
    </div>
    <template #footer>
      <button class="btn btn-primary" :disabled="running" @click="$emit('close')">{{ t('common.close') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import type { UpstreamModelSyncResult } from '@/features/admin-accounts/data/dtos/adminAccountDtos'

const props = defineProps<{
  show: boolean
  running: boolean
  completed: number
  total: number
  results: UpstreamModelSyncResult[]
}>()
defineEmits(['close'])
const { t } = useI18n()
function statusLabel(status: UpstreamModelSyncResult['status']) {
  switch (status) {
    case 'success': return t('admin.accounts.bulkActions.modelSyncStatus.success')
    case 'failed': return t('admin.accounts.bulkActions.modelSyncStatus.failed')
    case 'skipped': return t('admin.accounts.bulkActions.modelSyncStatus.skipped')
  }
}
const page = ref(1)
const pageCount = computed(() => Math.max(1, Math.ceil(props.results.length / 50)))
const visibleResults = computed(() => props.results.slice((page.value - 1) * 50, page.value * 50))
const summary = computed(() => ({
  success: props.results.filter(result => result.status === 'success').length,
  failed: props.results.filter(result => result.status === 'failed').length,
  skipped: props.results.filter(result => result.status === 'skipped').length
}))
watch(() => props.show, show => { if (show) page.value = 1 })
</script>
