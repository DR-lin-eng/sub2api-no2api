<template>
  <div class="border-b border-gray-100 pb-5 dark:border-dark-700">
    <div class="flex items-center justify-between gap-4">
      <div>
        <label id="openai-request-timezone-label" class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.settings.gatewayForwarding.openAIRequestTimezone') }}
        </label>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.gatewayForwarding.openAIRequestTimezoneHint') }}
        </p>
      </div>
      <Toggle
        v-model="form.openai_request_timezone_enabled"
        aria-labelledby="openai-request-timezone-label"
        data-testid="openai-request-timezone-toggle"
      />
    </div>
    <div class="mt-4">
      <label for="openai-request-timezone" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.settings.gatewayForwarding.openAIRequestTimezoneTarget') }}
      </label>
      <input
        id="openai-request-timezone"
        v-model="form.openai_request_timezone"
        :disabled="!form.openai_request_timezone_enabled"
        type="text"
        list="openai-request-timezone-options"
        class="input w-full disabled:opacity-50"
        placeholder="Asia/Singapore"
        maxlength="128"
        autocomplete="off"
        spellcheck="false"
        aria-describedby="openai-request-timezone-hint"
        data-testid="openai-request-timezone-input"
      />
      <datalist id="openai-request-timezone-options">
        <option v-for="timezone in timezones" :key="timezone" :value="timezone" />
      </datalist>
      <p id="openai-request-timezone-hint" class="mt-2 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.gatewayForwarding.openAIRequestTimezoneTargetHint') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import Toggle from '@/common/widgets/forms/Toggle.vue'
import { useSettingsPageContext } from '@/features/admin-settings/presentation/composables/settingsPageContext'

const { form, t } = useSettingsPageContext()
const timezones = [
  'Asia/Singapore', 'Asia/Shanghai', 'Asia/Hong_Kong', 'Asia/Taipei',
  'Asia/Tokyo', 'Asia/Seoul', 'Asia/Kolkata', 'Asia/Dubai',
  'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles',
  'Europe/London', 'Europe/Berlin', 'Australia/Sydney', 'Pacific/Auckland', 'UTC',
]
</script>
