<template>
      <!-- OpenAI passthrough -->
      <div
        v-if="allOpenAIPassthroughCapable"
        class="border-t border-gray-200 pt-4 dark:border-dark-600"
      >
        <div class="mb-3 flex items-center justify-between">
          <div class="flex-1 pr-4">
            <label
              id="bulk-edit-openai-passthrough-label"
              class="input-label mb-0"
              for="bulk-edit-openai-passthrough-enabled"
            >
              {{ t('admin.accounts.openai.oauthPassthrough') }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.oauthPassthroughDesc') }}
            </p>
          </div>
          <input
            v-model="enableOpenAIPassthrough"
            id="bulk-edit-openai-passthrough-enabled"
            type="checkbox"
            aria-controls="bulk-edit-openai-passthrough-body"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div
          id="bulk-edit-openai-passthrough-body"
          :class="!enableOpenAIPassthrough && 'pointer-events-none opacity-50'"
          role="group"
          aria-labelledby="bulk-edit-openai-passthrough-label"
        >
          <button
            id="bulk-edit-openai-passthrough-toggle"
            type="button"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              openaiPassthroughEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
            @click="openaiPassthroughEnabled = !openaiPassthroughEnabled"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                openaiPassthroughEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
      </div>

      <!-- OpenAI Codex namespace flattening compatibility (OAuth only) -->
      <div
        v-if="allOpenAIOAuthOnly"
        class="border-t border-gray-200 pt-4 dark:border-dark-600"
      >
        <div class="mb-3 flex items-center justify-between">
          <div class="flex-1 pr-4">
            <label
              id="bulk-edit-openai-flatten-namespaces-label"
              class="input-label mb-0"
              for="bulk-edit-openai-flatten-namespaces-enabled"
            >
              {{ t('admin.accounts.openai.flattenNamespaces') }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.flattenNamespacesDesc') }}
            </p>
          </div>
          <input
            id="bulk-edit-openai-flatten-namespaces-enabled"
            v-model="enableOpenAIFlattenNamespaces"
            type="checkbox"
            aria-controls="bulk-edit-openai-flatten-namespaces-body"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div
          id="bulk-edit-openai-flatten-namespaces-body"
          :class="!enableOpenAIFlattenNamespaces && 'pointer-events-none opacity-50'"
          role="group"
          aria-labelledby="bulk-edit-openai-flatten-namespaces-label"
        >
          <button
            id="bulk-edit-openai-flatten-namespaces-toggle"
            type="button"
            :aria-pressed="openaiFlattenNamespacesEnabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              openaiFlattenNamespacesEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
            @click="openaiFlattenNamespacesEnabled = !openaiFlattenNamespacesEnabled"
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

      <!-- Base URL (API Key only) -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <label
            id="bulk-edit-base-url-label"
            class="input-label mb-0"
            for="bulk-edit-base-url-enabled"
          >
            {{ t('admin.accounts.baseUrl') }}
          </label>
          <input
            v-model="enableBaseUrl"
            id="bulk-edit-base-url-enabled"
            type="checkbox"
            aria-controls="bulk-edit-base-url"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <input
          v-model="baseUrl"
          id="bulk-edit-base-url"
          type="text"
          :disabled="!enableBaseUrl"
          class="input"
          :class="!enableBaseUrl && 'cursor-not-allowed opacity-50'"
          :placeholder="t('admin.accounts.bulkEdit.baseUrlPlaceholder')"
          aria-labelledby="bulk-edit-base-url-label"
        />
        <GrokBaseUrlPresets
          v-if="allTargetsGrok"
          class="mt-2"
          @select="baseUrl = $event; enableBaseUrl = true"
        />
        <p class="input-hint">
          {{ t('admin.accounts.bulkEdit.baseUrlNotice') }}
        </p>
      </div>

      <!-- Model restriction -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <label
            id="bulk-edit-model-restriction-label"
            class="input-label mb-0"
            for="bulk-edit-model-restriction-enabled"
          >
            {{ t('admin.accounts.modelRestriction') }}
          </label>
          <input
            v-model="enableModelRestriction"
            id="bulk-edit-model-restriction-enabled"
            type="checkbox"
            aria-controls="bulk-edit-model-restriction-body"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>

        <div
          id="bulk-edit-model-restriction-body"
          :class="!enableModelRestriction && 'pointer-events-none opacity-50'"
          role="group"
          aria-labelledby="bulk-edit-model-restriction-label"
        >
          <div
            v-if="isOpenAIModelRestrictionDisabled"
            class="rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20"
          >
            <p class="text-xs text-amber-700 dark:text-amber-400">
              {{ t('admin.accounts.openai.modelRestrictionDisabledByPassthrough') }}
            </p>
          </div>

          <template v-else>
            <!-- Mode Toggle -->
            <div class="mb-4 flex gap-2">
              <button
                type="button"
                :class="[
                  'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                  modelRestrictionMode === 'whitelist'
                    ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                ]"
                @click="modelRestrictionMode = 'whitelist'"
              >
                <svg
                  class="mr-1.5 inline h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
                </svg>
                {{ t('admin.accounts.modelWhitelist') }}
              </button>
              <button
                type="button"
                :class="[
                  'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                  modelRestrictionMode === 'mapping'
                    ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                ]"
                @click="modelRestrictionMode = 'mapping'"
              >
                <svg
                  class="mr-1.5 inline h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"
                  />
                </svg>
                {{ t('admin.accounts.modelMapping') }}
              </button>
            </div>

            <!-- Whitelist Mode -->
            <div v-if="modelRestrictionMode === 'whitelist'">
              <div class="mb-3 rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
                <p class="text-xs text-blue-700 dark:text-blue-400">
                  <svg
                    class="mr-1 inline h-4 w-4"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  {{ t('admin.accounts.selectAllowedModels') }}
                </p>
              </div>

              <ModelWhitelistSelector
                :model-mappings="modelMappings"
                v-model="allowedModels"
                :platforms="targetSelectedPlatforms"
              />

              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.selectedModels', { count: allowedModels.length }) }}
                <span v-if="allowedModels.length === 0">{{
                  t('admin.accounts.supportsAllModels')
                }}</span>
              </p>
            </div>

            <!-- Mapping Mode -->
            <div v-else>
              <div class="mb-3 rounded-lg bg-purple-50 p-3 dark:bg-purple-900/20">
                <p class="text-xs text-purple-700 dark:text-purple-400">
                  <svg
                    class="mr-1 inline h-4 w-4"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  {{ t('admin.accounts.mapRequestModels') }}
                </p>
              </div>

              <!-- Model Mapping List -->
              <div v-if="modelMappings.length > 0" class="mb-3 space-y-2">
                <div
                  v-for="(mapping, index) in modelMappings"
                  :key="index"
                  class="flex items-center gap-2"
                >
                  <input
                    v-model="mapping.from"
                    type="text"
                    class="input flex-1"
                    :placeholder="t('admin.accounts.requestModel')"
                  />
                  <svg
                    class="h-4 w-4 flex-shrink-0 text-gray-400"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M14 5l7 7m0 0l-7 7m7-7H3"
                    />
                  </svg>
                  <input
                    v-model="mapping.to"
                    type="text"
                    class="input flex-1"
                    :placeholder="t('admin.accounts.actualModel')"
                  />
                  <button
                    type="button"
                    class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
                    @click="removeModelMapping(index)"
                  >
                    <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                      />
                    </svg>
                  </button>
                </div>
              </div>

              <button
                type="button"
                class="mb-3 w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
                @click="addModelMapping"
              >
                <svg
                  class="mr-1 inline h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M12 4v16m8-8H4"
                  />
                </svg>
                {{ t('admin.accounts.addMapping') }}
              </button>

              <!-- Quick Add Buttons -->
              <div class="flex flex-wrap gap-2">
                <button
                  v-for="preset in filteredPresets"
                  :key="preset.label"
                  type="button"
                  :class="['rounded-lg px-3 py-1 text-xs transition-colors', preset.color]"
                  @click="addPresetMapping(preset.from, preset.to)"
                >
                  + {{ preset.label }}
                </button>
              </div>
            </div>
          </template>
        </div>
      </div>

      <!-- Custom error codes -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <div>
            <label
              id="bulk-edit-custom-error-codes-label"
              class="input-label mb-0"
              for="bulk-edit-custom-error-codes-enabled"
            >
              {{ t('admin.accounts.customErrorCodes') }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.customErrorCodesHint') }}
            </p>
          </div>
          <input
            v-model="enableCustomErrorCodes"
            id="bulk-edit-custom-error-codes-enabled"
            type="checkbox"
            aria-controls="bulk-edit-custom-error-codes-body"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>

        <div v-if="enableCustomErrorCodes" id="bulk-edit-custom-error-codes-body" class="space-y-3">
          <div class="rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20">
            <p class="text-xs text-amber-700 dark:text-amber-400">
              <Icon name="exclamationTriangle" size="sm" class="mr-1 inline" :stroke-width="2" />
              {{ t('admin.accounts.customErrorCodesWarning') }}
            </p>
          </div>

          <!-- Error Code Buttons -->
          <div class="flex flex-wrap gap-2">
            <button
              v-for="code in commonErrorCodes"
              :key="code.value"
              type="button"
              :class="[
                'rounded-lg px-3 py-1.5 text-sm font-medium transition-colors',
                selectedErrorCodes.includes(code.value)
                  ? 'bg-red-100 text-red-700 ring-1 ring-red-500 dark:bg-red-900/30 dark:text-red-400'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
              ]"
              @click="toggleErrorCode(code.value)"
            >
              {{ code.value }} {{ code.label }}
            </button>
          </div>

          <!-- Manual input -->
          <div class="flex items-center gap-2">
            <input
              v-model="customErrorCodeInput"
              id="bulk-edit-custom-error-code-input"
              type="number"
              min="100"
              max="599"
              class="input flex-1"
              :placeholder="t('admin.accounts.enterErrorCode')"
              aria-labelledby="bulk-edit-custom-error-codes-label"
              @keyup.enter="addCustomErrorCode"
            />
            <button type="button" class="btn btn-secondary px-3" @click="addCustomErrorCode">
              <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 4v16m8-8H4"
                />
              </svg>
            </button>
          </div>

          <!-- Selected codes summary -->
          <div class="flex flex-wrap gap-1.5">
            <span
              v-for="code in selectedErrorCodes.sort((a, b) => a - b)"
              :key="code"
              class="inline-flex items-center gap-1 rounded-full bg-red-100 px-2.5 py-0.5 text-sm font-medium text-red-700 dark:bg-red-900/30 dark:text-red-400"
            >
              {{ code }}
              <button
                type="button"
                class="hover:text-red-900 dark:hover:text-red-300"
                @click="removeErrorCode(code)"
              >
                <Icon name="x" size="xs" class="h-3.5 w-3.5" :stroke-width="2" />
              </button>
            </span>
            <span v-if="selectedErrorCodes.length === 0" class="text-xs text-gray-400">
              {{ t('admin.accounts.noneSelectedUsesDefault') }}
            </span>
          </div>
        </div>
      </div>

      <!-- Intercept warmup requests (Anthropic only) -->
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="flex items-center justify-between">
          <div class="flex-1 pr-4">
            <label
              id="bulk-edit-intercept-warmup-label"
              class="input-label mb-0"
              for="bulk-edit-intercept-warmup-enabled"
            >
              {{ t('admin.accounts.interceptWarmupRequests') }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.interceptWarmupRequestsDesc') }}
            </p>
          </div>
          <input
            v-model="enableInterceptWarmup"
            id="bulk-edit-intercept-warmup-enabled"
            type="checkbox"
            aria-controls="bulk-edit-intercept-warmup-body"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div v-if="enableInterceptWarmup" id="bulk-edit-intercept-warmup-body" class="mt-3">
          <button
            type="button"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              interceptWarmupRequests ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
            @click="interceptWarmupRequests = !interceptWarmupRequests"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                interceptWarmupRequests ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
      </div>

      <!-- Header Override (anthropic/openai apikey only) -->
      <div v-if="allHeaderOverrideCapable" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div class="flex items-center justify-between">
          <div class="flex-1 pr-4">
            <label
              id="bulk-edit-header-override-label"
              class="input-label mb-0"
              for="bulk-edit-header-override-enabled"
            >
              {{ t('admin.accounts.headerOverride.title') }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.headerOverride.hint') }}
            </p>
          </div>
          <input
            v-model="enableHeaderOverride"
            id="bulk-edit-header-override-enabled"
            type="checkbox"
            aria-controls="bulk-edit-header-override-body"
            class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </div>
        <div v-if="enableHeaderOverride" id="bulk-edit-header-override-body" class="mt-3 space-y-3">
          <button
            type="button"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              headerOverrideEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
            @click="headerOverrideEnabled = !headerOverrideEnabled"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                headerOverrideEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>

          <div v-if="headerOverrideEnabled" class="space-y-3">
            <div class="rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
              <p class="text-xs text-blue-700 dark:text-blue-400">
                <Icon name="exclamationCircle" size="sm" class="mr-1 inline" :stroke-width="2" />
                {{ t('admin.accounts.headerOverride.info') }}
              </p>
            </div>

            <p class="text-xs text-amber-600 dark:text-amber-400">
              {{ t('admin.accounts.headerOverride.bulkReplaceHint') }}
            </p>

            <HeaderOverrideEditor
              :rows="headerOverrideRows"
              @update:rows="headerOverrideRows = $event"
            />
          </div>
          <p v-else class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.headerOverride.bulkDisableHint') }}
          </p>
        </div>
      </div>
</template>

<script setup lang="ts">
import GrokBaseUrlPresets from '@/features/admin-accounts/presentation/widgets/GrokBaseUrlPresets.vue'
import HeaderOverrideEditor from '@/features/admin-accounts/presentation/widgets/HeaderOverrideEditor.vue'
import ModelWhitelistSelector from '@/features/admin-accounts/presentation/widgets/ModelWhitelistSelector.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { BulkEditRoutingPolicyContext } from '../bulkEditAccountContext'

const props = defineProps<{ context: BulkEditRoutingPolicyContext }>()
const {
  addCustomErrorCode,
  addModelMapping,
  addPresetMapping,
  allHeaderOverrideCapable,
  allOpenAIOAuthOnly,
  allowedModels,
  allOpenAIPassthroughCapable,
  allTargetsGrok,
  baseUrl,
  commonErrorCodes,
  customErrorCodeInput,
  enableBaseUrl,
  enableCustomErrorCodes,
  enableHeaderOverride,
  enableInterceptWarmup,
  enableModelRestriction,
  enableOpenAIFlattenNamespaces,
  enableOpenAIPassthrough,
  filteredPresets,
  headerOverrideEnabled,
  headerOverrideRows,
  interceptWarmupRequests,
  isOpenAIModelRestrictionDisabled,
  modelMappings,
  modelRestrictionMode,
  openaiFlattenNamespacesEnabled,
  openaiPassthroughEnabled,
  removeErrorCode,
  removeModelMapping,
  selectedErrorCodes,
  t,
  targetSelectedPlatforms,
  toggleErrorCode,
} = props.context
</script>
