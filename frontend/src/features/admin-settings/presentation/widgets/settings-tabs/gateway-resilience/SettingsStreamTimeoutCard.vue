<template>
  <div class="card">
    <div
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.streamTimeout.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.streamTimeout.description") }}
      </p>
    </div>
    <div class="space-y-5 p-6">
      <!-- Loading State -->
      <div
        v-if="streamTimeoutLoading"
        class="flex items-center gap-2 text-gray-500"
      >
        <div
          class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
        ></div>
        {{ t("common.loading") }}
      </div>

      <template v-else>
        <div class="space-y-4 border-b border-gray-100 pb-5 dark:border-dark-700">
          <div>
            <h3 class="font-medium text-gray-900 dark:text-white">
              {{ t("admin.settings.streamTimeout.openAIStartupTitle") }}
            </h3>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.streamTimeout.openAIStartupDescription") }}
            </p>
          </div>

          <div class="grid gap-4 md:grid-cols-3">
            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t("admin.settings.streamTimeout.firstOutputTimeout") }}
              </label>
              <input
                v-model.number="streamTimeoutForm.openai_first_output_timeout_seconds"
                type="number"
                min="0"
                max="600"
                data-testid="openai-first-output-timeout"
                class="input w-full"
              />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.streamTimeout.firstOutputTimeoutHint") }}
              </p>
            </div>

            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t("admin.settings.streamTimeout.highEffortFirstOutputTimeout") }}
              </label>
              <input
                v-model.number="streamTimeoutForm.openai_high_effort_first_output_timeout_seconds"
                type="number"
                min="0"
                max="1800"
                data-testid="openai-high-effort-first-output-timeout"
                class="input w-full"
              />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.streamTimeout.highEffortFirstOutputTimeoutHint") }}
              </p>
            </div>

            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t("admin.settings.streamTimeout.keepaliveInterval") }}
              </label>
              <input
                v-model.number="streamTimeoutForm.stream_keepalive_interval_seconds"
                type="number"
                min="0"
                max="30"
                data-testid="openai-stream-keepalive-interval"
                class="input w-full"
              />
              <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.streamTimeout.keepaliveIntervalHint") }}
              </p>
            </div>
          </div>
        </div>

        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-gray-900 dark:text-white">{{
              t("admin.settings.streamTimeout.responseHeaderEnabled")
            }}</label>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.streamTimeout.responseHeaderEnabledHint") }}
            </p>
          </div>
          <Toggle
            v-model="streamTimeoutForm.response_header_timeout_degradation_enabled"
          />
        </div>

        <div
          v-if="streamTimeoutForm.response_header_timeout_degradation_enabled"
          class="border-t border-gray-100 pt-4 dark:border-dark-700"
        >
          <label
            class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.settings.streamTimeout.timeoutSeconds") }}
          </label>
          <input
            v-model.number="streamTimeoutForm.response_header_timeout_seconds"
            type="number"
            min="1"
            max="300"
            class="input w-32"
          />
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.streamTimeout.timeoutSecondsHint") }}
          </p>
        </div>

        <!-- Enable Stream Timeout -->
        <div
          class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700"
        >
          <div>
            <label class="font-medium text-gray-900 dark:text-white">{{
              t("admin.settings.streamTimeout.enabled")
            }}</label>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.streamTimeout.enabledHint") }}
            </p>
          </div>
          <Toggle v-model="streamTimeoutForm.enabled" />
        </div>

        <!-- Settings - Only show when enabled -->
        <div
          v-if="streamTimeoutForm.enabled"
          class="space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700"
        >
          <!-- Action -->
          <div>
            <label
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t("admin.settings.streamTimeout.action") }}
            </label>
            <select
              v-model="streamTimeoutForm.action"
              class="input w-64"
            >
              <option value="temp_unsched">
                {{
                  t("admin.settings.streamTimeout.actionTempUnsched")
                }}
              </option>
              <option value="error">
                {{ t("admin.settings.streamTimeout.actionError") }}
              </option>
              <option value="none">
                {{ t("admin.settings.streamTimeout.actionNone") }}
              </option>
            </select>
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.streamTimeout.actionHint") }}
            </p>
          </div>

          <!-- Temp Unsched Minutes (only show when action is temp_unsched) -->
          <div v-if="streamTimeoutForm.action === 'temp_unsched'">
            <label
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t("admin.settings.streamTimeout.tempUnschedMinutes") }}
            </label>
            <input
              v-model.number="streamTimeoutForm.temp_unsched_minutes"
              type="number"
              min="1"
              max="60"
              class="input w-32"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                t("admin.settings.streamTimeout.tempUnschedMinutesHint")
              }}
            </p>
          </div>

          <!-- Threshold Count -->
          <div>
            <label
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{ t("admin.settings.streamTimeout.thresholdCount") }}
            </label>
            <input
              v-model.number="streamTimeoutForm.threshold_count"
              type="number"
              min="1"
              max="10"
              class="input w-32"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.streamTimeout.thresholdCountHint") }}
            </p>
          </div>

          <!-- Threshold Window Minutes -->
          <div>
            <label
              class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              {{
                t("admin.settings.streamTimeout.thresholdWindowMinutes")
              }}
            </label>
            <input
              v-model.number="
                streamTimeoutForm.threshold_window_minutes
              "
              type="number"
              min="1"
              max="60"
              class="input w-32"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                t(
                  "admin.settings.streamTimeout.thresholdWindowMinutesHint",
                )
              }}
            </p>
          </div>
        </div>

        <!-- Save Button -->
        <div
          class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700"
        >
          <button
            type="button"
            @click="saveStreamTimeoutSettings"
            :disabled="streamTimeoutSaving"
            class="btn btn-primary btn-sm"
          >
            <svg
              v-if="streamTimeoutSaving"
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
              streamTimeoutSaving
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
  saveStreamTimeoutSettings,
  streamTimeoutForm,
  streamTimeoutLoading,
  streamTimeoutSaving,
  t,
} = useSettingsPageContext()
</script>
