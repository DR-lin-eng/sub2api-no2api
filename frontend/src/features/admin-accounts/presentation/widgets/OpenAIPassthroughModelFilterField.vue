<template>
  <label class="mt-3 flex items-start gap-2 text-sm">
    <input v-model="enabled" type="checkbox" data-testid="openai-passthrough-model-filter" class="mt-1 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
    <span>
      {{ t('admin.accounts.openai.passthroughModelFilter') }}
      <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openai.passthroughModelFilterDesc') }}</span>
    </span>
  </label>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const enabled = defineModel<boolean>({ required: true })
const { t } = useI18n()
</script>
