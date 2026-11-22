<template>
  <!-- OpenAI 自动透传开关（OAuth/API Key） -->
  <div
    v-if="account?.platform === 'openai' && (account?.type === 'oauth' || account?.type === 'setup-token' || account?.type === 'apikey')"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.openai.oauthPassthrough') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.oauthPassthroughDesc') }}
        </p>
      </div>
      <button
        type="button"
        @click="openaiPassthroughEnabled = !openaiPassthroughEnabled"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          openaiPassthroughEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            openaiPassthroughEnabled ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>
    <OpenAIPassthroughModelFilterField v-if="openaiPassthroughEnabled" v-model="openaiPassthroughModelFilterEnabled" />
  </div>

  <!-- OpenAI Codex namespace flattening compatibility (OAuth only) -->
  <div
    v-if="account?.platform === 'openai' && account?.type === 'oauth'"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.openai.flattenNamespaces') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.flattenNamespacesDesc') }}
        </p>
      </div>
      <button
        type="button"
        data-testid="edit-openai-flatten-namespaces-toggle"
        :aria-pressed="openaiFlattenNamespacesEnabled"
        @click="openaiFlattenNamespacesEnabled = !openaiFlattenNamespacesEnabled"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          openaiFlattenNamespacesEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            openaiFlattenNamespacesEnabled ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>
  </div>

  <!-- Codex PAT standalone web search capability -->
  <div
    v-if="isOpenAIPersonalAccessTokenAccount"
    class="flex items-center justify-between gap-4 border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div>
      <label class="input-label mb-0">{{ t('admin.accounts.openai.codexWebSearch') }}</label>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.openai.codexWebSearchDesc') }}
      </p>
    </div>
    <Toggle
      v-model="codexWebSearchEnabled"
      data-testid="codex-pat-web-search"
      :aria-label="t('admin.accounts.openai.codexWebSearch')"
    />
  </div>

  <OpenAIImageToolPolicy v-model="codexImageToolMode" />
  <!-- OpenAI WS Mode 三态（off/ctx_pool/passthrough） -->
  <div
    v-if="account?.platform === 'openai' && (account?.type === 'oauth' || account?.type === 'setup-token' || account?.type === 'apikey')"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.openai.wsMode') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.wsModeDesc') }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t(openAIWSModeConcurrencyHintKey) }}
        </p>
      </div>
      <div class="w-52">
        <Select v-model="openaiResponsesWebSocketV2Mode" data-testid="edit-openai-ws-mode-select" :options="openAIWSModeOptions" />
      </div>
    </div>
  </div>

  <div
    v-if="account?.platform === 'openai' && account?.type === 'oauth'"
    class="flex items-center justify-between gap-4 border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="min-w-0 flex-1">
      <label class="input-label mb-0">{{ t('admin.accounts.openai.codexFingerprintMode') }}</label>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.openai.codexFingerprintModeDesc') }}
      </p>
    </div>
    <div class="w-52 shrink-0">
      <Select
        v-model="codexFingerprintMode"
        data-testid="edit-codex-fingerprint-mode-select"
        :options="codexFingerprintModeOptions"
      />
    </div>
  </div>

  <div
    v-if="account?.platform === 'openai' && account?.type === 'oauth'"
    class="flex items-center justify-between gap-4 border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div>
      <label class="input-label mb-0">{{ t('admin.accounts.openai.codexPrewarmContinuation') }}</label>
    </div>
    <Toggle
      v-model="codexPrewarmContinuationEnabled"
      data-testid="edit-codex-prewarm-continuation"
      :aria-label="t('admin.accounts.openai.codexPrewarmContinuation')"
    />
  </div>

  <div
    v-if="account?.platform === 'openai' && account?.type === 'apikey'"
    class="flex items-center justify-between gap-4 border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="min-w-0 flex-1">
      <label class="input-label mb-0">{{ t('admin.accounts.openai.codexThinkingTagNormalization') }}</label>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.openai.codexThinkingTagNormalizationDesc') }}
      </p>
    </div>
    <Toggle
      v-model="codexThinkingTagNormalizationEnabled"
      data-testid="edit-codex-thinking-tag-normalization"
      :aria-label="t('admin.accounts.openai.codexThinkingTagNormalization')"
    />
  </div>
  <!-- OpenAI API Key: route image-only Responses requests through Images API -->
  <div
    v-if="account?.platform === 'openai' && account?.type === 'apikey'"
    class="flex items-center justify-between gap-4 border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div>
      <label class="input-label mb-0">{{ t('admin.accounts.openai.forceImageAPI') }}</label>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.openai.forceImageAPIDesc') }}
      </p>
    </div>
    <Toggle
      v-model="openAIForceImageAPIEnabled"
      data-testid="openai-force-image-api"
      :aria-label="t('admin.accounts.openai.forceImageAPI')"
    />
  </div>

  <div
    v-if="account?.platform === 'openai' && account?.type === 'apikey'"
    class="flex items-center justify-between gap-4 border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div>
      <label class="input-label mb-0">{{ t('admin.accounts.upstreamBilling.autoProbe') }}</label>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.upstreamBilling.autoProbeHint') }}
      </p>
    </div>
    <Toggle
      v-model="upstreamBillingAutoProbeEnabled"
      data-testid="upstream-billing-auto-probe"
      :aria-label="t('admin.accounts.upstreamBilling.autoProbe')"
    />
  </div>
  <!-- OpenAI API 长上下文计费开关 -->
  <div
    v-if="account?.platform === 'openai' && !isSparkShadow && (account?.type === 'oauth' || account?.type === 'setup-token' || account?.type === 'apikey')"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="flex items-center justify-between gap-4">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.openai.longContextBilling') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.longContextBillingDesc') }}
        </p>
      </div>
      <button
        type="button"
        data-testid="openai-long-context-billing-toggle"
        role="switch"
        :aria-checked="openAILongContextBillingEnabled"
        @click="openAILongContextBillingEnabled = !openAILongContextBillingEnabled"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          openAILongContextBillingEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            openAILongContextBillingEnabled ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>
  </div>

  <div
    v-if="account?.platform === 'openai' && (account?.type === 'oauth' || account?.type === 'setup-token')"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.openai.codexCLIOnly') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.codexCLIOnlyDesc') }}
        </p>
      </div>
      <button
        type="button"
        @click="codexCLIOnlyEnabled = !codexCLIOnlyEnabled"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          codexCLIOnlyEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            codexCLIOnlyEnabled ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>
    <div
      v-if="codexCLIOnlyEnabled"
      class="mt-4 flex items-center justify-between border-l-2 border-gray-200 pl-4 dark:border-dark-600"
    >
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.openai.codexCLIOnlyAppServer') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.codexCLIOnlyAppServerDesc') }}
        </p>
      </div>
      <button
        type="button"
        @click="codexCLIOnlyAppServerEnabled = !codexCLIOnlyAppServerEnabled"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          codexCLIOnlyAppServerEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            codexCLIOnlyAppServerEnabled ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import OpenAIImageToolPolicy from '../OpenAIImageToolPolicy.vue'
import OpenAIPassthroughModelFilterField from "@/features/admin-accounts/presentation/widgets/OpenAIPassthroughModelFilterField.vue"

import Select from '@/common/widgets/forms/Select.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import type { EditAccountAdvancedContext } from '../../accountEditorContext'

const props = defineProps<{ context: EditAccountAdvancedContext }>()
const {
  account, codexFingerprintMode, codexFingerprintModeOptions, codexPrewarmContinuationEnabled,
  codexThinkingTagNormalizationEnabled, codexCLIOnlyAppServerEnabled, codexCLIOnlyEnabled,
  codexImageToolMode, codexWebSearchEnabled, isOpenAIPersonalAccessTokenAccount, isSparkShadow,
  openAIForceImageAPIEnabled, openAILongContextBillingEnabled, openAIWSModeConcurrencyHintKey,
  openAIWSModeOptions, openaiPassthroughEnabled, openaiPassthroughModelFilterEnabled,
  openaiFlattenNamespacesEnabled, openaiResponsesWebSocketV2Mode, t, upstreamBillingAutoProbeEnabled
} = props.context
</script>
