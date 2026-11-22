<template>
  <div class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600">
      <div class="space-y-2">
        <div class="flex items-center justify-between">
          <label class="input-label mb-0">{{ t('admin.accounts.autoPause5hDisabled') }}</label>
          <button
            type="button"
            :aria-label="t('admin.accounts.autoPause5hDisabled')" :aria-pressed="autoPause5hDisabled"
              @click="autoPause5hDisabled = !autoPause5hDisabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              autoPause5hDisabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
            data-testid="auto-pause-5h-disabled"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                autoPause5hDisabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
        <p class="input-hint">{{ t('admin.accounts.autoPauseDisabledHint') }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.autoPause5hThreshold') }}</label>
        <input
          v-model.number="autoPause5hThreshold"
          type="number"
          min="0"
          max="100"
          step="0.1"
          class="input"
          :disabled="autoPause5hDisabled"
          data-testid="auto-pause-5h-threshold"
        />
        <p class="input-hint">{{ t('admin.accounts.autoPauseThresholdHint') }}</p>
      </div>
      <div class="space-y-2">
        <div class="flex items-center justify-between">
          <label class="input-label mb-0">{{ t('admin.accounts.autoPause7dDisabled') }}</label>
          <button
            type="button"
            :aria-label="t('admin.accounts.autoPause7dDisabled')" :aria-pressed="autoPause7dDisabled"
              @click="autoPause7dDisabled = !autoPause7dDisabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              autoPause7dDisabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
            data-testid="auto-pause-7d-disabled"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                autoPause7dDisabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
        <p class="input-hint">{{ t('admin.accounts.autoPauseDisabledHint') }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.autoPause7dThreshold') }}</label>
        <input
          v-model.number="autoPause7dThreshold"
          type="number"
          min="0"
          max="100"
          step="0.1"
          class="input"
          :disabled="autoPause7dDisabled"
          data-testid="auto-pause-7d-threshold"
        />
        <p class="input-hint">{{ t('admin.accounts.autoPauseThresholdHint') }}</p>
      </div>
    </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
const autoPause5hDisabled = defineModel<boolean>('autoPause5hDisabled', { required: true })
const autoPause7dDisabled = defineModel<boolean>('autoPause7dDisabled', { required: true })
const autoPause5hThreshold = defineModel<number | null>('autoPause5hThreshold', { required: true })
const autoPause7dThreshold = defineModel<number | null>('autoPause7dThreshold', { required: true })
</script>
