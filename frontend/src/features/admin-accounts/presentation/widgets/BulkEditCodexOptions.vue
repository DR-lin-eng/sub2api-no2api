<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="mb-3 flex items-center justify-between">
      <label
        id="bulk-edit-codex-fingerprint-mode-label"
        class="input-label mb-0"
        for="bulk-edit-codex-fingerprint-mode-enabled"
      >
        {{ t('admin.accounts.openai.codexFingerprintMode') }}
      </label>
      <input
        v-model="enableFingerprint"
        id="bulk-edit-codex-fingerprint-mode-enabled"
        type="checkbox"
        aria-controls="bulk-edit-codex-fingerprint-mode"
        class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
      />
    </div>
    <div
      id="bulk-edit-codex-fingerprint-mode"
      :class="!enableFingerprint && 'pointer-events-none opacity-50'"
    >
      <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.openai.codexFingerprintModeDesc') }}
      </p>
      <Select
        v-model="fingerprintMode"
        data-testid="bulk-codex-fingerprint-mode-select"
        :options="fingerprintModeOptions"
        aria-labelledby="bulk-edit-codex-fingerprint-mode-label"
      />
    </div>
  </div>

  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="mb-3 flex items-center justify-between">
      <label
        id="bulk-edit-codex-prewarm-continuation-label"
        class="input-label mb-0"
        for="bulk-edit-codex-prewarm-continuation-enabled"
      >
        {{ t('admin.accounts.openai.codexPrewarmContinuation') }}
      </label>
      <input
        v-model="enablePrewarm"
        id="bulk-edit-codex-prewarm-continuation-enabled"
        type="checkbox"
        aria-controls="bulk-edit-codex-prewarm-continuation"
        class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
      />
    </div>
    <div
      id="bulk-edit-codex-prewarm-continuation"
      class="flex items-center justify-between gap-4"
      :class="!enablePrewarm && 'pointer-events-none opacity-50'"
    >
      <Toggle
        v-model="prewarmEnabled"
        data-testid="bulk-edit-codex-prewarm-continuation-toggle"
        :aria-label="t('admin.accounts.openai.codexPrewarmContinuation')"
      />
    </div>
  </div>

</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/common/widgets/forms/Select.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import {
  getCodexFingerprintModeOptions,
  type CodexFingerprintMode,
} from '@/features/admin-accounts/presentation/accountFormPolicy'

const { t } = useI18n()

const enableFingerprint = defineModel<boolean>('enableFingerprint', { required: true })
const fingerprintMode = defineModel<CodexFingerprintMode>('fingerprintMode', { required: true })
const enablePrewarm = defineModel<boolean>('enablePrewarm', { required: true })
const prewarmEnabled = defineModel<boolean>('prewarmEnabled', { required: true })

const fingerprintModeOptions = computed(() => getCodexFingerprintModeOptions(t))
</script>
