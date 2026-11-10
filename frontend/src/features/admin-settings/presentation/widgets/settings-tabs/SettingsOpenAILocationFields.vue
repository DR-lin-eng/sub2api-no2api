<template>
  <div class="border-b border-gray-100 pb-5 dark:border-dark-700">
    <div class="flex items-center justify-between gap-4">
      <div>
        <label id="openai-location-label" class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.settings.gatewayForwarding.openAILocation') }}
        </label>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.gatewayForwarding.openAILocationHint') }}
        </p>
      </div>
      <Toggle
        v-model="form.openai_location_enabled"
        aria-labelledby="openai-location-label"
        data-testid="openai-location-toggle"
      />
    </div>
    <div class="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
      <label class="block">
        <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.settings.gatewayForwarding.openAILocationLatitude') }}
        </span>
        <input
          v-model.number="form.openai_location_latitude"
          class="input w-full disabled:opacity-50"
          type="number"
          min="-90"
          max="90"
          step="any"
          :disabled="!form.openai_location_enabled"
          data-testid="openai-location-latitude"
        />
      </label>
      <label class="block">
        <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.settings.gatewayForwarding.openAILocationLongitude') }}
        </span>
        <input
          v-model.number="form.openai_location_longitude"
          class="input w-full disabled:opacity-50"
          type="number"
          min="-180"
          max="180"
          step="any"
          :disabled="!form.openai_location_enabled"
          data-testid="openai-location-longitude"
        />
      </label>
    </div>
    <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.settings.gatewayForwarding.openAILocationCoordinateHint') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import Toggle from '@/common/widgets/forms/Toggle.vue'
import { useSettingsPageContext } from '@/features/admin-settings/presentation/composables/settingsPageContext'

const { form, t } = useSettingsPageContext()
</script>
