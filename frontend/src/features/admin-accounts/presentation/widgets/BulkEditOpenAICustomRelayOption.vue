<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="mb-3 flex items-center justify-between gap-4">
      <div class="flex-1">
        <label class="input-label mb-0" for="bulk-edit-openai-custom-relay-enabled">
          {{ t('admin.accounts.quotaControl.customBaseUrl.label') }}
        </label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.quotaControl.customBaseUrl.openaiHint') }}
        </p>
      </div>
      <input
        v-model="enableOpenAICustomRelay"
        id="bulk-edit-openai-custom-relay-enabled"
        type="checkbox"
        class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
      />
    </div>
    <div :class="!enableOpenAICustomRelay && 'pointer-events-none opacity-50'">
      <button
        id="bulk-edit-openai-custom-relay-toggle"
        type="button"
        :aria-pressed="openAICustomRelayEnabled"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          openAICustomRelayEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
        @click="openAICustomRelayEnabled = !openAICustomRelayEnabled"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            openAICustomRelayEnabled ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
      <input
        v-if="openAICustomRelayEnabled"
        v-model="openAICustomRelayURL"
        id="bulk-edit-openai-custom-relay-url"
        type="url"
        class="input mt-3"
        :placeholder="t('admin.accounts.quotaControl.customBaseUrl.openaiUrlHint')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const enableOpenAICustomRelay = defineModel<boolean>('enableUpdate', { required: true })
const openAICustomRelayEnabled = defineModel<boolean>('enabled', { required: true })
const openAICustomRelayURL = defineModel<string>('url', { required: true })
</script>
