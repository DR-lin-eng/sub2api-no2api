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
    <div v-if="detail?.code_match" class="space-y-2 rounded-lg bg-slate-50 p-3 text-xs dark:bg-dark-900" data-testid="code-match-result">
      <p class="font-medium">{{ t('common.accountQuality.codeMatchScore', { score: detail.code_match.score, threshold: detail.code_match.threshold }) }}</p>
      <p>{{ detail.code_match.is_model_a ? t('common.accountQuality.modelAMatched') : t('common.accountQuality.modelANotMatched') }} · {{ detail.code_match.normal_class === 'other' ? t('common.accountQuality.otherNormal') : t('common.accountQuality.modelANormal') }}</p>
      <p class="text-slate-500">{{ t('common.accountQuality.codeMatchHint') }}</p>
      <details v-if="detail.code_match.matched_signals.length"><summary class="cursor-pointer">{{ t('common.accountQuality.matchedSignals') }} ({{ detail.code_match.matched_signals.length }}/{{ detail.code_match.matched_signals.length + detail.code_match.missing_signals.length }})</summary><ul class="mt-2 list-inside list-disc space-y-1"><li v-for="signal in detail.code_match.matched_signals" :key="signal">{{ signalLabels[signal] || t('common.unknown') }}</li></ul></details>
    </div>
    <p v-if="detail?.preview_status === 'error'" class="text-xs text-amber-600">{{ t('common.accountQuality.previewFailed') }}</p>
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
const signalLabels = computed<Record<string, string>>(() => ({
  js_trig_leg_animation: t('common.accountQuality.signals.trig'),
  defs_use_reuse: t('common.accountQuality.signals.reuse'),
  reduced_motion_js: t('common.accountQuality.signals.motion'),
  play_pause_button: t('common.accountQuality.signals.pause'),
  title_desc_aria_labelledby: t('common.accountQuality.signals.title'),
  visibilitychange_listener: t('common.accountQuality.signals.visibility'),
  multiword_kebab_naming: t('common.accountQuality.signals.naming'),
  no_root_css_vars: t('common.accountQuality.signals.palette'),
  pure_svg_scene: t('common.accountQuality.signals.scene'),
}))
const stageStatus = computed(() => {
  if (!props.detail) return '—'
  if (props.detail.status === 'passed') return t('common.accountQuality.normal')
  if (props.detail.status === 'wrong') return t('common.accountQuality.abnormal')
  if (props.detail.status === 'uncertain') return t('common.accountQuality.waiting')
  return t('common.accountQuality.failed')
})
</script>
