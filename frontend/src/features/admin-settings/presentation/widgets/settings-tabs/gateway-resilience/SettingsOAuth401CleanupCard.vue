<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.oauth401Cleanup.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.oauth401Cleanup.description") }}
      </p>
    </div>

    <div class="space-y-5 p-6">
      <div
        v-if="oauth401CleanupLoading"
        class="text-sm text-gray-500 dark:text-gray-400"
      >
        {{ t("common.loading") }}
      </div>

      <template v-else>
        <div class="flex items-center justify-between gap-4">
          <div>
            <label class="font-medium text-gray-900 dark:text-white">
              {{ t("admin.settings.oauth401Cleanup.enabled") }}
            </label>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.oauth401Cleanup.enabledHint") }}
            </p>
          </div>
          <Toggle
            v-model="oauth401CleanupForm.enabled"
            data-testid="oauth-401-auto-delete-toggle"
          />
        </div>

        <p
          v-if="oauth401CleanupForm.enabled"
          class="border-l-2 border-red-500 pl-3 text-sm text-red-700 dark:text-red-300"
        >
          {{ t("admin.settings.oauth401Cleanup.warning") }}
        </p>

        <div class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700">
          <button
            type="button"
            class="btn btn-danger btn-sm"
            :disabled="oauth401CleanupSaving"
            @click="saveOAuth401CleanupSettings"
          >
            {{ oauth401CleanupSaving ? t("common.saving") : t("common.save") }}
          </button>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import Toggle from '@/common/widgets/forms/Toggle.vue'
import { useSettingsPageContext } from '@/features/admin-settings/presentation/composables/settingsPageContext'

const {
  oauth401CleanupSaving,
  oauth401CleanupForm,
  oauth401CleanupLoading,
  saveOAuth401CleanupSettings,
  t,
} = useSettingsPageContext()
</script>
