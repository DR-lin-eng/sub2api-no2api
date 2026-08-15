<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="mb-3 flex items-center justify-between">
      <label
        id="bulk-edit-codex-thinking-tag-label"
        class="input-label mb-0"
        for="bulk-edit-codex-thinking-tag-enabled"
      >
        {{ t('admin.accounts.openai.codexThinkingTagNormalization') }}
      </label>
      <input
        v-model="enableUpdate"
        id="bulk-edit-codex-thinking-tag-enabled"
        type="checkbox"
        aria-controls="bulk-edit-codex-thinking-tag-body"
        class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
      />
    </div>
    <div
      id="bulk-edit-codex-thinking-tag-body"
      class="flex items-center justify-between gap-4"
      :class="!enableUpdate && 'pointer-events-none opacity-50'"
    >
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.openai.codexThinkingTagNormalizationDesc') }}
      </p>
      <Toggle
        v-model="normalizationEnabled"
        data-testid="bulk-codex-thinking-tag-normalization-toggle"
        :aria-label="t('admin.accounts.openai.codexThinkingTagNormalization')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Toggle from '@/common/widgets/forms/Toggle.vue'

const { t } = useI18n()

const enableUpdate = defineModel<boolean>('enableUpdate', { required: true })
const normalizationEnabled = defineModel<boolean>('normalizationEnabled', { required: true })
</script>
