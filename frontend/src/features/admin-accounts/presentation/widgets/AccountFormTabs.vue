<template>
  <div role="group" :aria-label="t('admin.accounts.providerSetup.settings')" class="sticky -top-3 z-10 mb-4 overflow-x-auto border-b border-gray-200 bg-white py-2 dark:border-dark-700 dark:bg-dark-800 sm:-top-4">
    <div class="flex w-max max-w-full items-center gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-900">
      <button v-for="tab in tabs" :key="tab" type="button" :data-tour="`account-tab-${tab}`" :aria-pressed="model === tab"
        class="inline-flex items-center gap-1.5 whitespace-nowrap rounded-md px-2.5 py-1.5 text-sm font-medium focus-visible:outline-primary-500 sm:px-3"
        :class="model === tab ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-white'"
        @click="model = tab">
        <Icon :name="icons[tab]" size="sm" class="hidden sm:block" />{{ t(labels[tab]) }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { AccountFormTab } from '../composables/useAccountFormTab'
const props = defineProps<{ openai: boolean }>()
const { t } = useI18n()
const tabs = computed<AccountFormTab[]>(() => props.openai ? ['connection', 'openai', 'scheduling'] : ['connection', 'scheduling'])
const labels = {
  connection: 'admin.accounts.providerSetup.connection',
  openai: 'admin.accounts.providerSetup.openai',
  scheduling: 'admin.accounts.providerSetup.scheduling',
} satisfies Record<AccountFormTab, string>
const icons = { connection: 'key', openai: 'cog', scheduling: 'server' } as const
const model = defineModel<AccountFormTab>({ required: true })
</script>
