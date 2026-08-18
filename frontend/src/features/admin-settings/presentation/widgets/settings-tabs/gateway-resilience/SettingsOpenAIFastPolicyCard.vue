<template>
  <div class="card">
    <div
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.openaiFastPolicy.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.openaiFastPolicy.description") }}
      </p>
    </div>
    <div class="space-y-5 p-6">
      <!-- Empty state -->
      <div
        v-if="openaiFastPolicyForm.rules.length === 0"
        class="rounded-lg border border-dashed border-gray-200 p-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
      >
        {{ t("admin.settings.openaiFastPolicy.empty") }}
      </div>

      <!-- Rule Cards -->
      <div
        v-for="(rule, ruleIndex) in openaiFastPolicyForm.rules"
        :key="ruleIndex"
        class="rounded-lg border border-gray-200 p-4 dark:border-dark-600"
      >
        <div class="mb-3 flex items-center justify-between">
          <span
            class="text-sm font-medium text-gray-900 dark:text-white"
          >
            {{
              t("admin.settings.openaiFastPolicy.ruleHeader", {
                index: ruleIndex + 1,
              })
            }}
          </span>
          <button
            type="button"
            @click="removeOpenAIFastPolicyRule(ruleIndex)"
            class="rounded p-1 text-red-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
            :title="t('admin.settings.openaiFastPolicy.removeRule')"
          >
            <svg
              class="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              stroke-width="2"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>
        </div>

        <div
          class="mb-4 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-500 dark:text-gray-400"
          :data-testid="`openai-fast-policy-summary-${ruleIndex}`"
        >
          <span class="font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.openaiFastPolicy.modelWhitelist') }}
          </span>
          <span aria-hidden="true">→</span>
          <span class="inline-flex items-center rounded bg-primary-50 px-2 py-0.5 font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
            {{ actionSummary(rule.action) }}
          </span>
          <template v-if="hasTargetModels(rule)">
            <span aria-hidden="true">·</span>
            <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.openaiFastPolicy.fallbackAction') }}</span>
            <span aria-hidden="true">→</span>
            <span class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 font-medium text-gray-700 dark:bg-dark-600 dark:text-gray-300">
              {{ actionSummary(rule.fallback_action || 'pass') }}
            </span>
          </template>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
          <!-- Service Tier -->
          <div>
            <label
              class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
            >
              {{ t("admin.settings.openaiFastPolicy.serviceTier") }}
            </label>
            <Select
              :modelValue="rule.service_tier"
              @update:modelValue="
                rule.service_tier = $event as
                  | 'all'
                  | 'priority'
                  | 'flex'
              "
              :options="openaiFastPolicyTierOptions"
            />
          </div>

          <!-- Action -->
          <div>
            <label
              class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
            >
              {{ t("admin.settings.openaiFastPolicy.action") }}
            </label>
            <Select
              :modelValue="rule.action"
              @update:modelValue="
                rule.action = $event as
                  | 'pass'
                  | 'filter'
                  | 'block'
                  | 'force_priority'
              "
              :options="openaiFastPolicyActionOptions"
            />
          </div>

          <!-- Scope -->
          <div>
            <label
              class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
            >
              {{ t("admin.settings.openaiFastPolicy.scope") }}
            </label>
            <Select
              :modelValue="rule.scope"
              @update:modelValue="
                rule.scope = $event as
                  | 'all'
                  | 'oauth'
                  | 'apikey'
                  | 'bedrock'
              "
              :options="openaiFastPolicyScopeOptions"
            />
          </div>
        </div>

        <!-- User Scope -->
        <div class="mt-3">
          <label
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {{ t("admin.settings.openaiFastPolicy.userIds") }}
          </label>
          <p class="mb-2 text-xs text-gray-400 dark:text-gray-500">
            {{ t("admin.settings.openaiFastPolicy.userIdsHint") }}
          </p>
          <OpenAIFastPolicyUserSelector
            :model-value="rule.user_ids || []"
            @update:model-value="rule.user_ids = $event"
          />
        </div>

        <!-- Error Message (only when action=block) -->
        <div v-if="rule.action === 'block'" class="mt-3">
          <label
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {{ t("admin.settings.openaiFastPolicy.errorMessage") }}
          </label>
          <input
            v-model="rule.error_message"
            type="text"
            class="input"
            :placeholder="
              t(
                'admin.settings.openaiFastPolicy.errorMessagePlaceholder',
              )
            "
          />
          <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
            {{ t("admin.settings.openaiFastPolicy.errorMessageHint") }}
          </p>
        </div>

        <!-- Target Models -->
        <div class="mt-3">
          <label
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {{ t("admin.settings.openaiFastPolicy.modelWhitelist") }}
          </label>
          <p class="mb-2 text-xs text-gray-400 dark:text-gray-500">
            {{
              t("admin.settings.openaiFastPolicy.modelWhitelistHint")
            }}
          </p>
          <div
            v-for="(_, patternIdx) in rule.model_whitelist || []"
            :key="patternIdx"
            class="mb-1.5 flex items-center gap-2"
          >
            <input
              v-model="rule.model_whitelist![patternIdx]"
              type="text"
              class="input input-sm flex-1"
              :placeholder="
                t(
                  'admin.settings.openaiFastPolicy.modelPatternPlaceholder',
                )
              "
            />
            <button
              type="button"
              @click="
                removeOpenAIFastPolicyModelPattern(rule, patternIdx)
              "
              class="shrink-0 rounded p-1 text-red-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
            >
              <svg
                class="h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="2"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M6 18L18 6M6 6l12 12"
                />
              </svg>
            </button>
          </div>
          <button
            type="button"
            @click="addOpenAIFastPolicyModelPattern(rule)"
            class="mb-2 inline-flex items-center gap-1 text-xs text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
          >
            <svg
              class="h-3.5 w-3.5"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              stroke-width="2"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M12 4v16m8-8H4"
              />
            </svg>
            {{ t("admin.settings.openaiFastPolicy.addModelPattern") }}
          </button>
        </div>

        <!-- Other Models Action (only when target models are non-empty) -->
        <div
          v-if="hasTargetModels(rule)"
          class="mt-3"
        >
          <label
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {{ t("admin.settings.openaiFastPolicy.fallbackAction") }}
          </label>
          <Select
            :modelValue="rule.fallback_action || 'pass'"
            @update:modelValue="
              rule.fallback_action = $event as
                | 'pass'
                | 'filter'
                | 'block'
                | 'force_priority'
            "
            :options="openaiFastPolicyActionOptions"
          />
          <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
            {{
              t("admin.settings.openaiFastPolicy.fallbackActionHint")
            }}
          </p>
          <div v-if="rule.fallback_action === 'block'" class="mt-2">
            <input
              v-model="rule.fallback_error_message"
              type="text"
              class="input"
              :placeholder="
                t(
                  'admin.settings.openaiFastPolicy.fallbackErrorMessagePlaceholder',
                )
              "
            />
          </div>
        </div>
      </div>

      <!-- Add Rule Button -->
      <div>
        <button
          type="button"
          @click="addOpenAIFastPolicyRule"
          class="btn btn-secondary btn-sm inline-flex items-center gap-1"
        >
          <svg
            class="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="2"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M12 4v16m8-8H4"
            />
          </svg>
          {{ t("admin.settings.openaiFastPolicy.addRule") }}
        </button>
        <p class="mt-2 text-xs text-gray-400 dark:text-gray-500">
          {{ t("admin.settings.openaiFastPolicy.saveHint") }}
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import Select from '@/common/widgets/forms/Select.vue'
import OpenAIFastPolicyUserSelector from '@/features/admin-settings/presentation/widgets/OpenAIFastPolicyUserSelector.vue'
import { useSettingsPageContext } from '@/features/admin-settings/presentation/composables/settingsPageContext'

const {
  addOpenAIFastPolicyModelPattern,
  addOpenAIFastPolicyRule,
  openaiFastPolicyActionOptions,
  openaiFastPolicyForm,
  openaiFastPolicyScopeOptions,
  openaiFastPolicyTierOptions,
  removeOpenAIFastPolicyModelPattern,
  removeOpenAIFastPolicyRule,
  t,
} = useSettingsPageContext()

function hasTargetModels(rule: { model_whitelist?: string[] }) {
  return Boolean(rule.model_whitelist?.some((pattern) => pattern.trim() !== ''))
}

function actionSummary(action: string) {
  return openaiFastPolicyActionOptions.value.find(option => option.value === action)?.label || action
}
</script>
