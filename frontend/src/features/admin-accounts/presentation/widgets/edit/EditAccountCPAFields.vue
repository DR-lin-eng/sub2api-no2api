<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="mb-3 flex items-center justify-between gap-4">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.cpaMode') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.cpaModeHint') }}
        </p>
      </div>
      <button
        type="button"
        role="switch"
        data-testid="cpa-mode-toggle"
        :aria-checked="cpaModeEnabled"
        @click="cpaModeEnabled = !cpaModeEnabled"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          cpaModeEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            cpaModeEnabled ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>

    <div v-if="cpaModeEnabled" class="space-y-3" data-testid="cpa-mode-settings">
      <div class="grid grid-cols-2 gap-1 rounded-md bg-gray-100 p-1 dark:bg-dark-700">
        <button
          type="button"
          data-testid="cpa-use-base-url"
          :class="modeButtonClass(cpaUseBaseUrl)"
          @click="cpaUseBaseUrl = true"
        >
          {{ t('admin.accounts.cpaUseBaseUrl') }}
        </button>
        <button
          type="button"
          data-testid="cpa-use-custom-url"
          :class="modeButtonClass(!cpaUseBaseUrl)"
          @click="cpaUseBaseUrl = false"
        >
          {{ t('admin.accounts.cpaUseCustomUrl') }}
        </button>
      </div>
      <div v-if="cpaUseBaseUrl" class="input bg-gray-50 font-mono text-sm text-gray-600 dark:bg-dark-700 dark:text-gray-300">
        {{ editBaseUrl }}
      </div>
      <div v-else>
        <label class="input-label">{{ t('admin.accounts.cpaManagementUrl') }}</label>
        <input
          v-model="cpaManagementUrl"
          type="url"
          class="input font-mono"
          data-testid="cpa-management-url"
          placeholder="http://cpa:8317"
        />
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.cpaManagementKey') }}</label>
        <input
          v-model="cpaManagementKey"
          type="password"
          class="input font-mono"
          autocomplete="new-password"
          data-testid="cpa-management-key"
          data-1p-ignore
          data-lpignore="true"
          data-bwignore="true"
          :placeholder="t('admin.accounts.leaveEmptyToKeep')"
        />
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.cpaConcurrencyPerCredential') }}</label>
        <input
          v-model.number="cpaConcurrencyPerCredential"
          type="number"
          min="1"
          :max="MAX_CPA_CONCURRENCY_PER_CREDENTIAL"
          step="1"
          class="input"
          data-testid="cpa-concurrency-per-credential"
        />
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.cpaConcurrencyHint', { seconds: CPA_SNAPSHOT_INTERVAL_SECONDS }) }}
        </p>
      </div>
      <div class="flex items-center justify-between gap-4">
        <div>
          <p id="cpa-exclude-abnormal-label" class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.accounts.cpaExcludeAbnormalCredentials') }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.cpaExcludeAbnormalCredentialsHint') }}
          </p>
        </div>
        <button
          type="button"
          role="switch"
          data-testid="cpa-exclude-abnormal-toggle"
          aria-labelledby="cpa-exclude-abnormal-label"
          :aria-checked="cpaExcludeAbnormalCredentials"
          @click="cpaExcludeAbnormalCredentials = !cpaExcludeAbnormalCredentials"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            cpaExcludeAbnormalCredentials ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              cpaExcludeAbnormalCredentials ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>
      <button
        type="button"
        class="btn btn-secondary inline-flex items-center gap-2"
        data-testid="cpa-test-connection"
        :disabled="isTestingCPA"
        @click="testCPAConnection"
      >
        <Icon :name="isTestingCPA ? 'refresh' : 'play'" size="sm" :class="isTestingCPA && 'animate-spin'" />
        {{ isTestingCPA ? t('admin.accounts.cpaTestingConnection') : t('admin.accounts.cpaTestConnection') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import Icon from '@/common/widgets/icons/Icon.vue'
import type { EditAccountCredentialContext } from '../../accountEditorContext'

const props = defineProps<{ context: EditAccountCredentialContext }>()
const {
  CPA_SNAPSHOT_INTERVAL_SECONDS,
  MAX_CPA_CONCURRENCY_PER_CREDENTIAL,
  cpaConcurrencyPerCredential,
  cpaExcludeAbnormalCredentials,
  cpaManagementKey,
  cpaManagementUrl,
  cpaModeEnabled,
  cpaUseBaseUrl,
  editBaseUrl,
  isTestingCPA,
  t,
  testCPAConnection,
} = props.context

const modeButtonClass = (active: boolean) => [
  'rounded px-3 py-1.5 text-xs font-medium transition-colors',
  active
    ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
    : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200',
]
</script>
