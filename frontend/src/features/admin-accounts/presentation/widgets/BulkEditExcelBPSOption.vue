<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <label class="input-label mb-1" for="bulk-edit-excel-bps-mode">
      {{ t('admin.accounts.openai.excelBpsEnabled') }}
    </label>
    <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.accounts.bulkEdit.excelBpsEligibility') }}
    </p>
    <select
      id="bulk-edit-excel-bps-mode"
      v-model="mode"
      class="input"
      data-testid="bulk-edit-excel-bps-mode"
    >
      <option value="unchanged">{{ t('admin.accounts.bulkEdit.excelBpsUnchanged') }}</option>
      <option value="enabled">{{ t('admin.accounts.bulkEdit.excelBpsEnable') }}</option>
      <option value="disabled">{{ t('admin.accounts.bulkEdit.excelBpsDisable') }}</option>
    </select>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const mode = defineModel<'unchanged' | 'enabled' | 'disabled'>({ required: true })
</script>
