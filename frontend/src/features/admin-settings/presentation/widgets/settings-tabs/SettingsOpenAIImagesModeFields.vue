<template>
  <div class="flex items-center justify-between gap-4 border-b border-gray-100 pb-5 dark:border-dark-700">
    <div>
      <label id="openai-images-legacy-label" class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.settings.gatewayForwarding.openAIImagesLegacy') }}
      </label>
      <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.gatewayForwarding.openAIImagesLegacyHint') }}
      </p>
    </div>
    <Toggle v-model="form.openai_images_legacy_enabled" aria-labelledby="openai-images-legacy-label" data-testid="openai-images-legacy-toggle" />
  </div>
</template>

<script setup lang="ts">
import Toggle from '@/common/widgets/forms/Toggle.vue'
import { useSettingsPageContext } from '@/features/admin-settings/presentation/composables/settingsPageContext'

const { form, t } = useSettingsPageContext()
</script>
