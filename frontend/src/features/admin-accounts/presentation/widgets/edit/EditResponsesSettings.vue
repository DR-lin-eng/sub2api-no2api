<template>
  <!-- OpenAI APIKey Responses API support mode -->
    <div
      v-if="isUpstreamBillingProbeEligible(account.platform, account.type)"
      class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600"
    >
      <div class="flex items-center justify-between gap-4">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.openai.responsesMode') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openai.responsesModeDesc') }}
          </p>
        </div>
        <div class="w-56">
          <Select
            v-model="openAIResponsesMode"
            :options="openAIResponsesModeOptions"
            :disabled="!openAITextGenerationCapabilityEnabled"
            data-testid="openai-responses-mode-select"
          />
        </div>
      </div>
      <div
        v-if="openAITextGenerationCapabilityEnabled"
        class="rounded-lg bg-gray-50 px-3 py-2 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300"
      >
        <span class="font-medium">{{ t(openAIResponsesStatusKey) }}</span>
      </div>
      <div
        v-else
        class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
        data-testid="openai-responses-mode-not-applicable"
      >
        {{ t('admin.accounts.openai.responsesModeTextDisabledHint') }}
      </div>
      <div>
        <label class="input-label mb-2 block">{{ t('admin.accounts.openai.endpointCapabilities') }}</label>
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <label
            v-for="option in openAIEndpointCapabilityOptions"
            :key="option.value"
            class="flex cursor-pointer items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600"
          >
            <input
              type="checkbox"
              class="rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
              :data-testid="`openai-endpoint-capability-${option.value}`"
              :checked="openAIEndpointCapabilities.includes(option.value)"
              @change="toggleOpenAIEndpointCapability(option.value, $event)"
            />
            <span class="text-gray-700 dark:text-gray-200">{{ option.label }}</span>
          </label>
        </div>
        <p class="input-hint">{{ t('admin.accounts.openai.endpointCapabilitiesDesc') }}</p>
      </div>
    </div>
</template>

<script setup lang="ts">
import Select from '@/common/widgets/forms/Select.vue'
import { isUpstreamBillingProbeEligible } from '../../upstreamBillingProbeEligibility'
import type { EditAccountAdvancedContext } from '../../accountEditorContext'
const props = defineProps<{ context: EditAccountAdvancedContext }>()
const { account, openAITextGenerationCapabilityEnabled, openAIResponsesMode, openAIResponsesModeOptions, openAIResponsesStatusKey, openAIEndpointCapabilityOptions, openAIEndpointCapabilities, toggleOpenAIEndpointCapability, t } = props.context
</script>
