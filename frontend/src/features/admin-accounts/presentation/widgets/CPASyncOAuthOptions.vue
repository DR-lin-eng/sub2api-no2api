<template>
  <fieldset class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
    <legend class="px-1 text-sm font-medium">{{ t('admin.accounts.cpaImport.oauthOptions') }}</legend>
    <label v-for="option in booleanOptions" :key="option.key" class="flex items-center gap-2 text-sm">
      <input
        type="checkbox"
        :data-testid="`cpa-option-${option.key}`"
        :checked="modelValue[option.key] === true"
        :disabled="option.key === 'allow_app_server' && !modelValue.codex_cli_only"
        @change="setBoolean(option.key, ($event.target as HTMLInputElement).checked)"
      />
      {{ t(option.label) }}
    </label>
    <template v-if="platform === 'openai'">
      <label v-for="option in selectOptions" :key="option.key" class="block text-sm">
        <span class="input-label">{{ t(option.label) }}</span>
        <select
          class="input"
          :data-testid="`cpa-option-${option.key}`"
          :value="modelValue[option.key]"
          @change="setSelect(option.key, ($event.target as HTMLSelectElement).value)"
        >
          <option v-for="choice in option.choices" :key="choice.value" :value="choice.value">{{ t(choice.label) }}</option>
        </select>
      </label>
    </template>
  </fieldset>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CPAOAuthOptions, CPASyncPlatform } from '../../data/dtos/cpaSyncDtos'

const props = defineProps<{ platform: CPASyncPlatform; modelValue: CPAOAuthOptions }>()
const emit = defineEmits<{ 'update:modelValue': [value: CPAOAuthOptions] }>()
const { t } = useI18n()
type BooleanKey = Exclude<keyof CPAOAuthOptions, 'ws_mode' | 'fingerprint_mode' | 'compact_mode'>
const booleanOptions = computed<Array<{ key: BooleanKey; label: string }>>(() => [
  { key: 'tls_fingerprint', label: 'admin.accounts.quotaControl.tlsFingerprint.label' },
  ...(props.platform === 'anthropic' ? [
    { key: 'session_id_masking' as const, label: 'admin.accounts.quotaControl.sessionIdMasking.label' },
    { key: 'intercept_warmup' as const, label: 'admin.accounts.interceptWarmupRequests' }
  ] : []),
  ...(props.platform === 'openai' ? [
    { key: 'passthrough' as const, label: 'admin.accounts.openai.oauthPassthrough' },
    { key: 'flatten_namespaces' as const, label: 'admin.accounts.openai.flattenNamespaces' },
    { key: 'prewarm_continuation' as const, label: 'admin.accounts.openai.codexPrewarmContinuation' },
    { key: 'codex_cli_only' as const, label: 'admin.accounts.openai.codexCLIOnly' },
    { key: 'allow_app_server' as const, label: 'admin.accounts.openai.codexCLIOnlyAppServer' },
    { key: 'long_context_billing' as const, label: 'admin.accounts.openai.longContextBilling' }
  ] : [])
])
const selectOptions = [
  { key: 'ws_mode' as const, label: 'admin.accounts.openai.wsMode', choices: [
    { value: 'off', label: 'admin.accounts.openai.wsModeOff' },
    { value: 'ctx_pool', label: 'admin.accounts.openai.wsModeCtxPool' },
    { value: 'passthrough', label: 'admin.accounts.openai.wsModePassthrough' },
    { value: 'http_bridge', label: 'admin.accounts.openai.wsModeHttpBridge' }
  ] },
  { key: 'fingerprint_mode' as const, label: 'admin.accounts.openai.codexFingerprintMode', choices: [
    { value: 'off', label: 'admin.accounts.openai.codexFingerprintModeOff' },
    { value: 'device', label: 'admin.accounts.openai.codexFingerprintModeDevice' },
    { value: 'session', label: 'admin.accounts.openai.codexFingerprintModeSession' },
    { value: 'full', label: 'admin.accounts.openai.codexFingerprintModeFull' }
  ] },
  { key: 'compact_mode' as const, label: 'admin.accounts.openai.compactMode', choices: [
    { value: 'auto', label: 'admin.accounts.openai.compactModeAuto' },
    { value: 'force_on', label: 'admin.accounts.openai.compactModeForceOn' },
    { value: 'force_off', label: 'admin.accounts.openai.compactModeForceOff' }
  ] }
]
function setBoolean(key: BooleanKey, value: boolean) {
  const next = { ...props.modelValue, [key]: value }
  if (key === 'codex_cli_only' && !value) next.allow_app_server = false
  emit('update:modelValue', next)
}
function setSelect(key: 'ws_mode' | 'fingerprint_mode' | 'compact_mode', value: string) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}
</script>
