<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('dashboard.apiKeyUsage') }}</h2>
    </div>
    <div v-if="error" class="flex items-center justify-between gap-4 border-b border-red-100 bg-red-50 px-6 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300">
      <span>{{ t('dashboard.apiKeyUsageLoadFailed') }}</span>
      <button type="button" class="btn btn-secondary btn-sm shrink-0" @click="emit('retry')">
        <Icon name="refresh" size="sm" />
        {{ t('dashboard.retry') }}
      </button>
    </div>
    <div v-if="loading" class="flex items-center justify-center py-12"><LoadingSpinner size="lg" /></div>
    <div v-else-if="rows.length === 0 && !error" class="p-6">
      <EmptyState :title="t('dashboard.noApiKeys')" :description="t('dashboard.noApiKeysDescription')" />
    </div>
    <div v-else-if="rows.length > 0" class="overflow-x-auto">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 text-gray-500 dark:bg-dark-800/50 dark:text-gray-400">
          <tr>
            <th class="px-6 py-3 text-left font-medium">{{ t('dashboard.apiKeyName') }}</th>
            <th class="px-6 py-3 text-right font-medium">{{ t('dashboard.tokens') }}</th>
            <th class="px-6 py-3 text-right font-medium">{{ t('dashboard.actualSpend') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in rows" :key="row.id" class="border-t border-gray-100 dark:border-dark-700">
            <td class="px-6 py-3 font-medium text-gray-900 dark:text-white">{{ row.name }}</td>
            <td class="px-6 py-3 text-right text-gray-600 dark:text-gray-300">{{ row.totalTokens.toLocaleString() }}</td>
            <td class="px-6 py-3 text-right text-green-600 dark:text-green-400">${{ row.actualSpend.toFixed(4) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'

defineProps<{ rows: Array<{ id: number, name: string, totalTokens: number, actualSpend: number }>, loading: boolean, error: boolean }>()
const emit = defineEmits<{ retry: [] }>()
const { t } = useI18n()
</script>
