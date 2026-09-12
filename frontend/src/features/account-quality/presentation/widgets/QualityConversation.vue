<template>
  <section class="min-w-0 space-y-3 rounded-lg border border-slate-200 p-4 dark:border-dark-700">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h4 class="text-sm font-semibold">{{ title }}</h4>
      <span class="text-xs text-slate-500">{{ stageStatus }}</span>
    </div>
    <dl class="space-y-2 text-xs">
      <div><dt class="text-slate-500">{{ t('common.accountQuality.conversationId') }}</dt><dd class="mt-1 break-all font-mono">{{ detail?.conversation_id || t('common.accountQuality.idUnavailable') }}</dd></div>
      <div v-if="detail?.response_id"><dt class="text-slate-500">{{ t('common.accountQuality.responseId') }}</dt><dd class="mt-1 break-all font-mono">{{ detail.response_id }}</dd></div>
      <div v-if="detail"><dt class="text-slate-500">{{ t('common.accountQuality.reasoningTokens') }}</dt><dd class="mt-1 font-mono">{{ detail.reasoning_tokens ?? t('common.unknown') }}</dd></div>
    </dl>
    <div class="border-t border-slate-100 pt-3 dark:border-dark-700">
      <p class="mb-2 text-xs text-slate-500">{{ t('common.accountQuality.answer') }}</p>
      <pre v-if="detail?.answer" class="max-h-72 overflow-y-auto whitespace-pre-wrap break-words rounded-lg bg-slate-50 p-3 font-mono text-sm leading-relaxed [overflow-wrap:anywhere] dark:bg-dark-900">{{ detail.answer }}</pre>
      <p v-else class="text-sm text-slate-400">{{ detail ? t('common.accountQuality.noAnswer') : t('common.accountQuality.noStage') }}</p>
      <p v-if="detail?.answer_truncated" class="mt-2 text-xs text-amber-600">{{ t('common.accountQuality.truncated') }}</p>
    </div>
  </section>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountQualityStageDetail } from '../../data/datasources/accountQualityPublicDatasource'
const props = defineProps<{ title: string; detail?: AccountQualityStageDetail | null }>()
const { t } = useI18n()
const stageStatus = computed(() => {
  if (!props.detail) return '—'
  if (props.detail.status === 'passed') return t('common.accountQuality.normal')
  if (props.detail.status === 'wrong') return t('common.accountQuality.abnormal')
  if (props.detail.status === 'uncertain') return t('common.accountQuality.waiting')
  return t('common.accountQuality.failed')
})
</script>
