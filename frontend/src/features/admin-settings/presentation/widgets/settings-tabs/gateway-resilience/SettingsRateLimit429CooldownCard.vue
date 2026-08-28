<template>
  <div class="card">
    <div
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.rateLimit429Cooldown.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.rateLimit429Cooldown.description") }}
      </p>
    </div>
    <div class="space-y-5 p-6">
      <div
        v-if="rateLimit429CooldownLoading"
        class="flex items-center gap-2 text-gray-500"
      >
        <div
          class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
        ></div>
        {{ t("common.loading") }}
      </div>

      <template v-else>
        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-gray-900 dark:text-white">{{
              t("admin.settings.rateLimit429Cooldown.enabled")
            }}</label>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.rateLimit429Cooldown.enabledHint") }}
            </p>
          </div>
          <Toggle v-model="rateLimit429CooldownForm.enabled" />
        </div>

        <div
          v-if="rateLimit429CooldownForm.enabled"
          class="space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700"
        >
          <div>
            <label
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{
                t(
                  "admin.settings.rateLimit429Cooldown.cooldownSeconds",
                )
              }}
            </label>
            <input
              v-model.number="rateLimit429CooldownForm.cooldown_seconds"
              type="number"
              min="1"
              max="7200"
              class="input w-32"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                t(
                  "admin.settings.rateLimit429Cooldown.cooldownSecondsHint",
                )
              }}
            </p>
          </div>
        </div>

        <div class="border-t border-gray-100 pt-4 dark:border-dark-700">
          <div class="flex items-center justify-between">
            <div>
              <label class="font-medium text-gray-900 dark:text-white">{{
                t("admin.settings.rateLimit429Cooldown.autoDisable")
              }}</label>
              <p class="text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.rateLimit429Cooldown.autoDisableHint") }}
              </p>
            </div>
            <Toggle v-model="rateLimit429CooldownForm.auto_disable_enabled" />
          </div>

          <div
            v-if="rateLimit429CooldownForm.auto_disable_enabled"
            class="mt-4"
          >
            <label
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t("admin.settings.rateLimit429Cooldown.autoDisableThreshold") }}
            </label>
            <input
              v-model.number="rateLimit429CooldownForm.auto_disable_threshold"
              type="number"
              min="1"
              max="100"
              class="input w-32"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.rateLimit429Cooldown.autoDisableThresholdHint") }}
            </p>
          </div>
        </div>

        <div
          class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700"
        >
          <button
            type="button"
            @click="saveRateLimit429CooldownSettings"
            :disabled="rateLimit429CooldownSaving"
            class="btn btn-primary btn-sm"
          >
            <svg
              v-if="rateLimit429CooldownSaving"
              class="mr-1 h-4 w-4 animate-spin"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              ></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
            {{
              rateLimit429CooldownSaving
                ? t("common.saving")
                : t("common.save")
            }}
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
  rateLimit429CooldownForm,
  rateLimit429CooldownLoading,
  rateLimit429CooldownSaving,
  saveRateLimit429CooldownSettings,
  t,
} = useSettingsPageContext()
</script>
