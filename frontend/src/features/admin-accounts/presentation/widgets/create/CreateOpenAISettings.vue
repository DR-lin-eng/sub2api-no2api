<template>
  <!-- OpenAI/Codex OAuth custom relay -->
  <div
    v-if="form.platform === 'openai' && accountCategory === 'oauth-based'"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.customBaseUrl.label') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaControl.customBaseUrl.openaiHint') }}
          </p>
        </div>
        <button
          type="button"
          data-testid="create-openai-custom-base-url-toggle"
          :aria-pressed="customBaseUrlEnabled"
          @click="customBaseUrlEnabled = !customBaseUrlEnabled"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            customBaseUrlEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              customBaseUrlEnabled ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>
      <div v-if="customBaseUrlEnabled" class="mt-3">
        <input
          v-model="customBaseUrl"
          data-testid="create-openai-custom-base-url"
          type="url"
          class="input"
          :placeholder="t('admin.accounts.quotaControl.customBaseUrl.openaiUrlHint')"
        />
      </div>
    </div>
  </div>

  <!-- OpenAI/Codex OAuth TLS profile -->
  <div
    v-if="form.platform === 'openai' && accountCategory === 'oauth-based'"
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.tlsFingerprint.label') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaControl.tlsFingerprint.hint') }}
          </p>
        </div>
        <button
          type="button"
          @click="tlsFingerprintEnabled = !tlsFingerprintEnabled"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            tlsFingerprintEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              tlsFingerprintEnabled ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>
      <div v-if="tlsFingerprintEnabled" class="mt-3">
        <select v-model="tlsFingerprintProfileId" class="input">
          <option :value="null">{{ t('admin.accounts.quotaControl.tlsFingerprint.defaultProfile') }}</option>
          <option v-if="tlsFingerprintProfiles.length > 0" :value="-1">{{ t('admin.accounts.quotaControl.tlsFingerprint.randomProfile') }}</option>
          <option v-for="p in tlsFingerprintProfiles" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
      </div>
    </div>
  </div>
  <!-- OpenAI 自动透传开关（OAuth/API Key） -->
  <div
    v-if="form.platform === 'openai'"
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
    v-if="form.platform === 'openai' && form.type === 'oauth'"
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
        data-testid="create-openai-flatten-namespaces-toggle"
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

  <!-- OpenAI WS Mode 三态（off/ctx_pool/passthrough） -->
  <div
    v-if="form.platform === 'openai' && (accountCategory === 'oauth-based' || accountCategory === 'apikey')"
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
        <Select v-model="openaiResponsesWebSocketV2Mode" :options="openAIWSModeOptions" />
      </div>
    </div>
  </div>

  <div
    v-if="form.platform === 'openai' && form.type === 'oauth'"
    class="flex items-center justify-between gap-4 border-t border-gray-200 pt-4 dark:border-dark-600"
  >
    <div>
      <label class="input-label mb-0">{{ t('admin.accounts.openai.codexPrewarmContinuation') }}</label>
    </div>
    <Toggle
      v-model="codexPrewarmContinuationEnabled"
      data-testid="create-codex-prewarm-continuation"
      :aria-label="t('admin.accounts.openai.codexPrewarmContinuation')"
    />
  </div>

  <div
    v-if="form.platform === 'openai' && form.type === 'apikey'"
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
      data-testid="create-codex-thinking-tag-normalization"
      :aria-label="t('admin.accounts.openai.codexThinkingTagNormalization')"
    />
  </div>

  <div
    v-if="form.platform === 'openai' && form.type === 'oauth'"
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
        data-testid="create-codex-fingerprint-mode-select"
        :options="codexFingerprintModeOptions"
      />
    </div>
  </div>

  <div
    v-if="form.platform === 'openai' && accountCategory === 'apikey'"
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
  <!-- OpenAI OAuth Codex 官方客户端限制开关 -->
  <div
    v-if="form.platform === 'openai' && (accountCategory === 'oauth-based' || accountCategory === 'apikey')"
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
        @click="toggleOpenAILongContextBilling"
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
    v-if="form.platform === 'openai' && accountCategory === 'oauth-based'"
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

  <!-- OpenAI Compact 能力配置 -->
  <div
    v-if="form.platform === 'openai' && (accountCategory === 'oauth-based' || accountCategory === 'apikey')"
    class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4"
  >
    <div class="flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.openai.compactMode') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.compactModeDesc') }}
        </p>
      </div>
      <div class="w-44">
        <Select v-model="openAICompactMode" :options="openAICompactModeOptions" />
      </div>
    </div>
    <div>
      <label class="input-label">{{ t('admin.accounts.openai.compactModelMapping') }}</label>
      <p class="input-hint">{{ t('admin.accounts.openai.compactModelMappingDesc') }}</p>
      <div v-if="openAICompactModelMappings.length > 0" class="mb-3 space-y-2">
        <div
          v-for="(mapping, index) in openAICompactModelMappings"
          :key="getOpenAICompactModelMappingKey(mapping)"
          class="flex items-center gap-2"
        >
          <input v-model="mapping.from" type="text" class="input flex-1" :placeholder="t('admin.accounts.fromModel')" />
          <span class="text-gray-400">→</span>
          <input v-model="mapping.to" type="text" class="input flex-1" :placeholder="t('admin.accounts.toModel')" />
          <button type="button" @click="removeOpenAICompactModelMapping(index)" class="text-red-500 hover:text-red-700">
            <Icon name="trash" size="sm" />
          </button>
        </div>
      </div>
      <button type="button" @click="addOpenAICompactModelMapping" class="btn btn-secondary text-sm">
        + {{ t('admin.accounts.addMapping') }}
      </button>
    </div>
  </div>

  <!-- OpenAI APIKey Responses API support mode -->
  <div
    v-if="form.platform === 'openai' && accountCategory === 'apikey'"
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
    <p
      v-if="!openAITextGenerationCapabilityEnabled"
      class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
      data-testid="openai-responses-mode-not-applicable"
    >
      {{ t('admin.accounts.openai.responsesModeTextDisabledHint') }}
    </p>
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
import OpenAIPassthroughModelFilterField from "@/features/admin-accounts/presentation/widgets/OpenAIPassthroughModelFilterField.vue"
import Icon from '@/common/widgets/icons/Icon.vue'
import Select from '@/common/widgets/forms/Select.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import type { CreateAccountAdvancedContext } from '../../accountEditorContext'

const props = defineProps<{ context: CreateAccountAdvancedContext }>()
const {
  accountCategory, addOpenAICompactModelMapping, codexFingerprintMode, codexFingerprintModeOptions,
  codexPrewarmContinuationEnabled, codexThinkingTagNormalizationEnabled,
  codexCLIOnlyAppServerEnabled, codexCLIOnlyEnabled, customBaseUrl, customBaseUrlEnabled, form,
  getOpenAICompactModelMappingKey, openAICompactMode, openAICompactModeOptions,
  openAICompactModelMappings, openAIEndpointCapabilities, openAIEndpointCapabilityOptions,
  openAIForceImageAPIEnabled, openAILongContextBillingEnabled, openAIResponsesMode,
  openAIResponsesModeOptions, openAITextGenerationCapabilityEnabled, openAIWSModeConcurrencyHintKey,
  openAIWSModeOptions, openaiPassthroughEnabled, openaiPassthroughModelFilterEnabled,
  openaiFlattenNamespacesEnabled, openaiResponsesWebSocketV2Mode, removeOpenAICompactModelMapping,
  t, tlsFingerprintEnabled, tlsFingerprintProfileId, tlsFingerprintProfiles,
  toggleOpenAIEndpointCapability, toggleOpenAILongContextBilling
} = props.context
</script>
