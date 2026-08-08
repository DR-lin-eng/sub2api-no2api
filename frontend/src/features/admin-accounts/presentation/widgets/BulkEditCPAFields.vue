<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="mb-3 flex items-center justify-between gap-4">
      <div>
        <label id="bulk-edit-cpa-label" class="input-label mb-0" for="bulk-edit-cpa-enabled">
          {{ t('admin.accounts.cpaMode') }}
        </label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.cpaModeHint') }}</p>
      </div>
      <input
        v-model="enableCPA"
        id="bulk-edit-cpa-enabled"
        type="checkbox"
        aria-controls="bulk-edit-cpa-body"
        class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
      />
    </div>
    <div id="bulk-edit-cpa-body" class="space-y-3" :class="!enableCPA && 'pointer-events-none opacity-50'">
      <div class="flex items-center justify-between gap-4">
        <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('common.enabled') }}</span>
        <button
          type="button"
          role="switch"
          data-testid="bulk-edit-cpa-mode-toggle"
          :aria-checked="cpaModeEnabled"
          @click="cpaModeEnabled = !cpaModeEnabled"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 rounded-full border-2 border-transparent transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            cpaModeEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span :class="['inline-block h-5 w-5 rounded-full bg-white shadow transition-transform', cpaModeEnabled ? 'translate-x-5' : 'translate-x-0']" />
        </button>
      </div>

      <template v-if="cpaModeEnabled">
        <div class="grid grid-cols-2 gap-1 rounded-md bg-gray-100 p-1 dark:bg-dark-700">
          <button type="button" data-testid="bulk-edit-cpa-use-base-url" :class="modeButtonClass(cpaUseBaseUrl)" @click="cpaUseBaseUrl = true">
            {{ t('admin.accounts.bulkEdit.cpaAddressFollowBaseUrl') }}
          </button>
          <button type="button" data-testid="bulk-edit-cpa-use-custom-url" :class="modeButtonClass(!cpaUseBaseUrl)" @click="cpaUseBaseUrl = false">
            {{ t('admin.accounts.bulkEdit.cpaAddressCustom') }}
          </button>
        </div>
        <div v-if="!cpaUseBaseUrl">
          <label class="input-label">{{ t('admin.accounts.cpaManagementUrl') }}</label>
          <input v-model="cpaManagementUrl" type="url" class="input font-mono" data-testid="bulk-edit-cpa-management-url" placeholder="http://cpa:8317" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.cpaManagementKey') }}</label>
          <input
            v-model="cpaManagementPassword"
            type="password"
            class="input font-mono"
            autocomplete="new-password"
            data-testid="bulk-edit-cpa-management-password"
            data-1p-ignore
            data-lpignore="true"
            data-bwignore="true"
          />
          <p class="input-hint">{{ t('admin.accounts.bulkEdit.cpaPasswordHint') }}</p>
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
            data-testid="bulk-edit-cpa-concurrency-per-credential"
          />
        </div>
        <div class="flex items-center justify-between gap-4">
          <div>
            <p id="bulk-edit-cpa-exclude-abnormal-label" class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.accounts.cpaExcludeAbnormalCredentials') }}
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.cpaExcludeAbnormalCredentialsHint') }}
            </p>
          </div>
          <button
            type="button"
            role="switch"
            data-testid="bulk-edit-cpa-exclude-abnormal-toggle"
            aria-labelledby="bulk-edit-cpa-exclude-abnormal-label"
            :aria-checked="cpaExcludeAbnormalCredentials"
            @click="cpaExcludeAbnormalCredentials = !cpaExcludeAbnormalCredentials"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 rounded-full border-2 border-transparent transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              cpaExcludeAbnormalCredentials ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span :class="['inline-block h-5 w-5 rounded-full bg-white shadow transition-transform', cpaExcludeAbnormalCredentials ? 'translate-x-5' : 'translate-x-0']" />
          </button>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { BulkEditCPAContext } from '../bulkEditAccountContext'

const props = defineProps<{ context: BulkEditCPAContext }>()
const {
  MAX_CPA_CONCURRENCY_PER_CREDENTIAL,
  cpaConcurrencyPerCredential,
  cpaExcludeAbnormalCredentials,
  cpaManagementPassword,
  cpaManagementUrl,
  cpaModeEnabled,
  cpaUseBaseUrl,
  enableCPA,
  t,
} = props.context

const modeButtonClass = (active: boolean) => [
  'rounded px-3 py-1.5 text-xs font-medium transition-colors',
  active
    ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
    : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200',
]
</script>
