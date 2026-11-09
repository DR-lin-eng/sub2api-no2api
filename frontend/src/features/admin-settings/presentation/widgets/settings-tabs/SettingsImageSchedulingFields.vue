<template>
  <div class="card flex items-center justify-between gap-4 p-6">
    <div>
      <label id="image-independent-scheduling-label" class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.settings.gatewayForwarding.imageIndependentScheduling') }}
      </label>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.gatewayForwarding.imageIndependentSchedulingHint') }}
      </p>
    </div>
    <Toggle
      v-model="form.image_independent_scheduling_enabled"
      aria-labelledby="image-independent-scheduling-label"
      data-testid="image-independent-scheduling-toggle"
    />
  </div>
</template>

<script setup lang="ts">
import Toggle from '@/common/widgets/forms/Toggle.vue'
import { useSettingsPageContext } from '@/features/admin-settings/presentation/composables/settingsPageContext'

const { form, t } = useSettingsPageContext()
</script>
