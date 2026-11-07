<template>
  <section class="rounded-xl border border-gray-200 p-4 dark:border-dark-500" data-testid="openai-usage-details">
    <button type="button" class="flex w-full items-center justify-between gap-3 text-left" :aria-expanded="expanded" @click="expanded = !expanded">
      <span>
        <span class="block font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.accounts.upstreamUsage.title') }}</span>
        <span class="mt-1 block text-xs text-gray-500">{{ t('admin.accounts.upstreamUsage.description') }}</span>
      </span>
      <span class="shrink-0 text-sm text-primary-600">{{ t(expanded ? 'admin.accounts.upstreamUsage.collapse' : 'admin.accounts.upstreamUsage.expand') }}</span>
    </button>
    <OpenAIUsageDetailsContent v-if="expanded" :key="account.id" :account="account" />
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import OpenAIUsageDetailsContent from './OpenAIUsageDetailsContent.vue'

defineProps<{ account: Account }>()
const { t } = useI18n()
const expanded = ref(false)
</script>
