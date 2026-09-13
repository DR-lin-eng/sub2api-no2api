<template>
  <AppLayout>
    <main class="mx-auto max-w-[1400px] space-y-5 pb-12">
      <header class="flex flex-wrap items-start justify-between gap-4 border-b border-gray-200 pb-4 dark:border-dark-700">
        <div>
          <div class="flex items-center gap-2">
            <Icon name="chart" size="lg" class="text-primary-600 dark:text-primary-400" />
            <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.accountQuality.title') }}</h1>
            <span class="rounded-md px-2 py-0.5 text-xs font-medium" :class="statusClass">{{ statusLabel }}</span>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accountQuality.description') }}</p>
          <p class="mt-2 text-xs text-gray-400">{{ t('admin.accountInspection.lastRun') }} {{ run?.completed_at ? formatDateTime(run.completed_at) : '-' }} · {{ t('admin.accountInspection.nextRun') }} {{ run?.next_run_at && settings.enabled ? formatDateTime(run.next_run_at) : '-' }}</p>
        </div>
        <div class="flex gap-2">
          <a class="btn btn-secondary inline-flex items-center gap-2" href="/monitor/quality/public" target="_blank" rel="noopener noreferrer">
            <Icon name="externalLink" size="sm" />
            <span>{{ t('admin.accountQuality.openPublic') }}</span>
          </a>
          <button class="btn btn-secondary" type="button" :disabled="loading || running" @click="loadOverview()">{{ t('admin.accountInspection.refresh') }}</button>
          <button class="btn btn-primary" type="button" :disabled="loading || running || run?.status === 'running'" @click="runNow">{{ running || run?.status === 'running' ? t('admin.accountInspection.running') : t('admin.accountQuality.runNow') }}</button>
        </div>
      </header>

      <div v-if="errorMessage || run?.error" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{{ errorMessage || run?.error }}</div>

      <section v-if="run?.progress" class="rounded-lg border border-primary-200 bg-primary-50 p-4 dark:border-primary-900/60 dark:bg-primary-950/30">
        <div class="flex flex-wrap items-center justify-between gap-2 text-sm font-medium"><span>{{ t('admin.accountQuality.progressTitle') }}</span><span>{{ runProgress.completed }} / {{ runProgress.total }}</span></div>
        <div role="progressbar" :aria-label="t('admin.accountQuality.progressTitle')" :aria-valuenow="runProgress.completed" :aria-valuemax="runProgress.total || 1" aria-valuemin="0" class="mt-3 h-2 overflow-hidden rounded-full bg-primary-100 dark:bg-primary-900"><div class="h-full rounded-full bg-primary-600 transition-all" :style="{ width: `${progressPercent}%` }" /></div>
        <p class="mt-2 text-xs text-primary-800 dark:text-primary-200">{{ t('admin.accountQuality.progressCounts', { queued: runProgress.queued || 0, running: runProgress.running || 0 }) }} · {{ t('admin.accountQuality.elapsed') }} {{ elapsed(run?.started_at, run?.completed_at) }}</p>
      </section>

      <section class="border-y border-gray-200 py-5 dark:border-dark-700">
        <div class="mb-4 flex items-center justify-between gap-3">
          <div><h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accountQuality.settingsTitle') }}</h2><p class="mt-1 text-xs text-gray-500">{{ t('admin.accountQuality.settingsCaption') }}</p></div>
          <button class="btn btn-primary" type="button" :disabled="saving || loading" @click="saveSettings">{{ saving ? t('admin.accountInspection.saving') : t('admin.accountInspection.save') }}</button>
        </div>
        <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <label class="toggle-field"><span>{{ t('admin.accountInspection.quality.enabled') }}</span><Toggle :model-value="settings.enabled" @update:model-value="settings.enabled = $event" /></label>
          <label class="toggle-field"><span>{{ t('admin.accountInspection.quality.publicEnabled') }}</span><Toggle :model-value="settings.public_enabled" @update:model-value="settings.public_enabled = $event" /></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.interval') }}</span><select v-model.number="settings.interval_minutes" class="input"><option v-for="value in [5, 10, 15, 30, 60]" :key="value" :value="value">{{ t('admin.accountInspection.settings.minutes', { value }) }}</option></select></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountQuality.timeout') }}</span><input v-model.number="settings.timeout_seconds" class="input" type="number" min="30" max="300" step="10" /><span class="field-hint">{{ t('admin.accountQuality.timeoutHint') }}</span></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.model') }}</span><input v-model="settings.model" class="input" :placeholder="t('admin.accountInspection.quality.modelPlaceholder')" /></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.effort') }}</span><select v-model="settings.effort" class="input"><option v-for="value in ['minimal', 'low', 'medium', 'high', 'xhigh', 'max']" :key="value" :value="value">{{ value }}</option></select></label>
          <label class="toggle-field"><span>{{ t('admin.accountQuality.stage1Enabled') }}</span><Toggle :model-value="settings.stage1_enabled" @update:model-value="settings.stage1_enabled = $event" /></label>
          <label class="toggle-field"><span>{{ t('admin.accountQuality.stage2Enabled') }}</span><Toggle :model-value="settings.stage2_enabled" @update:model-value="settings.stage2_enabled = $event" /></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.sourceGroup') }}</span><select v-model="settings.source_group_id" class="input"><option :value="null">{{ t('admin.accountInspection.quality.allGroups') }}</option><option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }} · {{ group.platform }} (#{{ group.id }})</option></select></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.degradedGroup') }}</span><select v-model="settings.degraded_group_id" class="input"><option :value="null">{{ t('admin.accountInspection.quality.noSwitch') }}</option><option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }} · {{ group.platform }} (#{{ group.id }})</option></select></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.failureThreshold') }}</span><input v-model.number="settings.failure_threshold" class="input" type="number" min="1" /></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.recoveryThreshold') }}</span><input v-model.number="settings.recovery_threshold" class="input" type="number" min="1" /></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.maxConcurrent') }}</span><input v-model.number="settings.max_concurrent" class="input" type="number" min="1" max="4" /></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountQuality.codeMatchThreshold') }}</span><input v-model.number="settings.code_match_threshold" data-testid="code-match-threshold" class="input" type="number" min="1" max="100" step="1" :disabled="!settings.stage2_enabled" /><span class="field-hint">{{ t('admin.accountQuality.codeMatchHint') }}</span></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountQuality.codeMatchNormalClass') }}</span><select v-model="settings.code_match_normal_class" data-testid="code-match-normal-class" class="input" :disabled="!settings.stage2_enabled"><option value="model_a">{{ t('admin.accountQuality.codeMatchModelANormal') }}</option><option value="other">{{ t('admin.accountQuality.codeMatchOtherNormal') }}</option></select></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountQuality.minReasoningTokens') }}</span><input v-model.number="settings.min_reasoning_tokens" class="input" type="number" min="0" max="1000000" step="1" :disabled="!settings.stage1_enabled" /><span class="field-hint">{{ t('admin.accountQuality.minReasoningTokensHint') }}</span></label>
          <p class="text-xs text-gray-500 md:col-span-2 xl:col-span-4">{{ t('admin.accountQuality.stagePolicyHint') }}</p>
          <label class="field-group md:col-span-2 xl:col-span-4"><span class="field-label">{{ t('admin.accountQuality.stage1Prompt') }}</span><textarea v-model="settings.stage1_prompt" class="input min-h-28" maxlength="2000" /></label>
          <label class="field-group md:col-span-2"><span class="field-label">{{ t('admin.accountQuality.stage1Answer') }}</span><input v-model="settings.stage1_answer" class="input" maxlength="100" /></label>
          <label class="field-group md:col-span-2 xl:col-span-4"><span class="field-label">{{ t('admin.accountQuality.stage2Prompt') }}</span><textarea v-model="settings.stage2_prompt" class="input min-h-28" maxlength="2000" /></label>
        </div>
      </section>

      <section class="grid grid-cols-2 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800 sm:grid-cols-3 xl:grid-cols-6">
        <div v-for="item in summaryItems" :key="item.key" class="border-b border-r border-gray-200 px-4 py-3 dark:border-dark-700"><dt class="text-xs text-gray-500">{{ item.label }}</dt><dd class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ item.value }}</dd></div>
      </section>

      <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-wrap items-center justify-between gap-2"><div><h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accountQuality.reasoningDistribution') }}</h2><p class="mt-1 text-xs text-gray-500">{{ t('admin.accountQuality.reasoningDistributionHint') }}</p></div><div class="text-xs text-gray-500">{{ t('admin.accountQuality.reasoningAverage') }}: {{ reasoningAverage }}</div></div>
        <div class="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6"><div v-for="bucket in reasoningBuckets" :key="bucket.key" class="rounded-md bg-gray-50 px-3 py-2 dark:bg-dark-900/60"><div class="text-xs text-gray-500">{{ reasoningBucketLabel(bucket.key) }}</div><div class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ bucket.count }}</div></div></div>
        <p class="mt-3 text-xs text-gray-400">{{ t('admin.accountQuality.reasoningMeasured', { measured: reasoningMeasured, unknown: reasoningUnknown }) }}</p>
      </section>

      <section class="overflow-x-auto rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <table class="w-full min-w-[1100px] text-left text-sm"><thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/60"><tr><th class="px-4 py-3">{{ t('admin.accountInspection.results.account') }}</th><th class="px-4 py-3">{{ t('admin.accountQuality.stageResults') }}</th><th class="px-4 py-3">{{ t('admin.accountInspection.results.state') }}</th><th class="px-4 py-3">{{ t('admin.accountQuality.reasoningTokens') }}</th><th class="px-4 py-3">{{ t('admin.accountInspection.results.action') }}</th><th class="px-4 py-3">{{ t('admin.accountInspection.results.reason') }}</th><th class="px-4 py-3">{{ t('admin.accountQuality.probeLatency') }}</th></tr></thead><tbody class="divide-y divide-gray-100 dark:divide-dark-700"><tr v-for="row in results" :key="row.account_id"><td class="px-4 py-3 font-medium text-gray-900 dark:text-white">{{ row.name }} <span class="text-xs text-gray-400">#{{ row.account_id }} · {{ row.platform }}</span></td><td class="px-4 py-3 text-xs"><p class="mb-2 text-primary-600 dark:text-primary-400">{{ phaseLabel(row.quality_phase) }} · {{ elapsed(row.quality_started_at, row.quality_completed_at) }}</p><span class="mr-3">S1 {{ qualityStageLabel(row.quality_stage1_status) }}</span><span>S2 {{ qualityStageLabel(row.quality_stage2_status) }}</span><p v-if="row.quality_code_match" class="mt-2 text-gray-500">{{ t('admin.accountQuality.codeMatchScore', { score: row.quality_code_match.score, threshold: row.quality_code_match.threshold }) }}</p></td><td class="px-4 py-3"><span :class="qualityStatusClass(row.quality_status)">{{ qualityStatusLabel(row.quality_status) }}</span><span class="ml-2 text-xs text-gray-400">F{{ row.quality_consecutive_failures || 0 }} / P{{ row.quality_consecutive_passes || 0 }}</span></td><td class="px-4 py-3 tabular-nums">{{ row.quality_reasoning_tokens == null ? t('common.unknown') : row.quality_reasoning_tokens.toLocaleString() }}</td><td class="px-4 py-3 text-xs">{{ row.quality_action || '-' }}</td><td class="max-w-md truncate px-4 py-3 text-xs text-gray-500" :title="row.quality_error">{{ row.quality_error || row.quality_label || '-' }}</td><td class="px-4 py-3 tabular-nums">{{ row.quality_latency_ms ? `${Math.round(row.quality_latency_ms)}ms` : '-' }}</td></tr><tr v-if="!loading && !results.length"><td colspan="7" class="px-4 py-10 text-center text-gray-500">{{ run?.status === 'running' ? t('admin.accountQuality.progressPreparing') : t('admin.accountInspection.results.noResults') }}</td></tr></tbody></table>
      </section>
      <div class="flex items-center justify-between text-sm text-gray-500" v-if="(overview?.results.pages || 1) > 1">
        <button class="btn btn-secondary" :disabled="page <= 1 || loading" @click="changePage(-1)">{{ t('common.back') }}</button>
        <span>{{ page }} / {{ overview?.results.pages }} · {{ overview?.results.total }}</span>
        <button class="btn btn-secondary" :disabled="page >= (overview?.results.pages || 1) || loading" @click="changePage(1)">{{ t('common.next') }}</button>
      </div>
    </main>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import { useAppStore } from '@/core/stores/appStore'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import { formatDateTime } from '@/core/utils/format'
import { getAll as getAllGroups } from '@/features/admin-groups/data/datasources/adminGroupQueries'
import { getOverview, runQualityMonitoring, updateSettings } from '../../data/datasources/accountQualityDatasource'
import type { AccountQualityOverview, AccountQualityResult, AccountQualitySettings } from '../../data/dtos/accountQualityDtos'

const { t } = useI18n(); const appStore = useAppStore(); const loading = ref(false); const saving = ref(false); const running = ref(false); const errorMessage = ref(''); const overview = ref<AccountQualityOverview | null>(null); const results = computed<AccountQualityResult[]>(() => overview.value?.results.items ?? []); const run = computed(() => overview.value?.run); const runProgress = computed(() => run.value?.progress ?? { total: 0, completed: 0 }); const progressPercent = computed(() => { const progress = runProgress.value; return progress.total <= 0 ? 0 : Math.min(100, Math.round((progress.completed / progress.total) * 100)) })
const settings = reactive<AccountQualitySettings>({ enabled: false, interval_minutes: 10, timeout_seconds: 120, model: '', effort: 'medium', stage1_enabled: true, stage1_prompt: '', stage1_answer: '21', stage2_enabled: true, stage2_prompt: '', failure_threshold: 2, recovery_threshold: 2, degraded_group_id: null, source_group_id: null, max_concurrent: 4, min_confidence: 0.85, code_match_threshold: 55, code_match_normal_class: 'model_a', min_reasoning_tokens: 0, public_enabled: false }); const groups = ref<Array<{ id: number; name: string; platform?: string }>>([]); let timer: ReturnType<typeof setInterval> | null = null
const statusLabel = computed(() => run.value?.status === 'running' ? t('admin.accountInspection.status.running') : run.value?.status === 'failed' ? t('admin.accountInspection.status.failed') : run.value?.status === 'succeeded' ? t('admin.accountInspection.status.succeeded') : t('admin.accountInspection.status.idle')); const statusClass = computed(() => run.value?.status === 'failed' ? 'bg-red-50 text-red-700' : run.value?.status === 'succeeded' ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-600'); const summaryItems = computed(() => { const s = run.value?.summary ?? { inspected: 0, passed: 0, degraded: 0, uncertain: 0, errors: 0, switched: 0, reasoning_token_distribution: { average_tokens: null, measured_accounts: 0, unknown_accounts: 0, buckets: [] } }; return [{ key: 'inspected', label: t('admin.accountInspection.summary.inspected'), value: s.inspected }, { key: 'passed', label: t('admin.accountQuality.passed'), value: s.passed }, { key: 'degraded', label: t('admin.accountInspection.summary.qualityDegraded'), value: s.degraded }, { key: 'uncertain', label: t('admin.accountQuality.uncertain'), value: s.uncertain }, { key: 'errors', label: t('admin.accountQuality.errors'), value: s.errors }, { key: 'switched', label: t('admin.accountInspection.summary.qualitySwitched'), value: s.switched }] })
const reasoningDistribution = computed(() => run.value?.summary.reasoning_token_distribution ?? { average_tokens: null, measured_accounts: 0, unknown_accounts: 0, buckets: [] }); const reasoningBuckets = computed(() => reasoningDistribution.value.buckets); const reasoningAverage = computed(() => reasoningDistribution.value.average_tokens == null ? '-' : reasoningDistribution.value.average_tokens.toFixed(1)); const reasoningMeasured = computed(() => reasoningDistribution.value.measured_accounts); const reasoningUnknown = computed(() => reasoningDistribution.value.unknown_accounts)
function reasoningBucketLabel(key: string): string { const labels: Record<string, string> = { '0_49': t('admin.accountQuality.reasoningBuckets.0_49'), '50_99': t('admin.accountQuality.reasoningBuckets.50_99'), '100_249': t('admin.accountQuality.reasoningBuckets.100_249'), '250_499': t('admin.accountQuality.reasoningBuckets.250_499'), '500_999': t('admin.accountQuality.reasoningBuckets.500_999'), '1000_plus': t('admin.accountQuality.reasoningBuckets.1000_plus') }; return labels[key] ?? key }
function qualityStageLabel(status?: string): string { if (status === 'running') return t('admin.accountInspection.running'); if (status === 'queued') return t('admin.accountQuality.queued'); if (status === 'passed') return t('admin.accountQuality.stagePassed'); if (status === 'wrong') return t('admin.accountQuality.stageWrong'); if (status === 'uncertain') return t('admin.accountQuality.uncertain'); if (status === 'error') return t('admin.accountQuality.errors'); if (status === 'disabled') return t('admin.accountQuality.stageDisabled'); return '-' }
function qualityStatusLabel(status?: string): string { if (status === 'running') return t('admin.accountInspection.running'); if (status === 'queued') return t('admin.accountQuality.queued'); if (status === 'disabled') return t('admin.accountQuality.stageDisabled'); if (status === 'healthy') return t('admin.accountQuality.passed'); if (status === 'degraded') return t('admin.accountInspection.summary.qualityDegraded'); if (status === 'uncertain') return t('admin.accountQuality.uncertain'); if (status === 'error') return t('admin.accountQuality.errors'); return '-' }
function qualityStatusClass(status?: string): string { if (status === 'degraded') return 'text-amber-600'; if (status === 'error') return 'text-red-600'; if (status === 'uncertain') return 'text-gray-500'; return 'text-emerald-600' }
const page = ref(1)
const now = ref(Date.now())
let elapsedTimer: ReturnType<typeof setInterval> | undefined
let disposed = false
function elapsed(start?: string | null, end?: string | null): string {
  const started = Date.parse(start || '')
  if (!Number.isFinite(started)) return '—'
  const duration = Math.max(0, Math.floor(((end ? Date.parse(end) : now.value) - started) / 1000))
  return `${Math.floor(duration / 60)}m ${duration % 60}s`
}
function phaseLabel(phase?: string): string {
  const labels: Record<string, string> = {
    queued: t('admin.accountQuality.queued'),
    stage1: t('admin.accountQuality.stage1Running'),
    stage2: t('admin.accountQuality.stage2Running'),
    rendering: t('admin.accountQuality.rendering'),
    saving: t('admin.accountQuality.persisting'),
    complete: t('admin.accountQuality.completed'),
    skipped: t('admin.accountQuality.stageDisabled'),
    cancelled: t('admin.accountQuality.cancelled')
  }
  return labels[phase || ''] || '—'
}
async function loadOverview(refreshSettings = true) {
  if (loading.value || disposed) return
  loading.value = true
  try {
    const data = await getOverview({ page: page.value, page_size: 50 })
    if (disposed) return
    const priorStatus = run.value?.status
    overview.value = data
    if (refreshSettings) { Object.assign(settings, data.settings); errorMessage.value = '' }
    if (priorStatus === 'running' && data.run.status === 'succeeded') appStore.showSuccess(t('admin.accountQuality.runCompleted', { count: data.run.summary.degraded }))
  } catch (error) { errorMessage.value = extractApiErrorMessage(error, t('admin.accountQuality.loadFailed')) }
  finally { loading.value = false }
}
async function changePage(delta: number) { page.value += delta; await loadOverview(false) }
async function saveSettings() {
  saving.value = true
  try { Object.assign(settings, await updateSettings({ ...settings })); errorMessage.value = ''; appStore.showSuccess(t('admin.accountQuality.saved')) }
  catch (error) { errorMessage.value = extractApiErrorMessage(error, t('admin.accountQuality.saveFailed')) }
  finally { saving.value = false }
}
async function runNow() {
  running.value = true
  errorMessage.value = ''
  page.value = 1
  try {
    overview.value = await runQualityMonitoring()
    if (overview.value.run.status === 'running') appStore.showSuccess(t('admin.accountQuality.runStarted'))
    else appStore.showSuccess(t('admin.accountQuality.runCompleted', { count: overview.value.run.summary.degraded }))
  } catch (error) { errorMessage.value = extractApiErrorMessage(error, t('admin.accountQuality.runFailed')) }
  finally { running.value = false }
}
onMounted(() => {
  void loadOverview()
  void getAllGroups().then(items => { groups.value = items.filter(item => item.status === 'active' && (item.platform === 'openai' || item.platform === 'gemini')) }).catch(() => { groups.value = [] })
  timer = setInterval(() => { if (run.value?.status === 'running') void loadOverview(false) }, 5000)
  elapsedTimer = setInterval(() => { now.value = Date.now() }, 1000)
})
onBeforeUnmount(() => { disposed = true; if (timer) clearInterval(timer); clearInterval(elapsedTimer) })
</script>

<style scoped>
.field-group { display: flex; flex-direction: column; gap: 0.35rem; }
.field-label { color: rgb(107 114 128); font-size: 0.75rem; line-height: 1rem; }
.field-hint { color: rgb(156 163 175); font-size: 0.7rem; line-height: 1rem; }
.toggle-field { display: flex; align-items: center; justify-content: space-between; gap: .75rem; border-radius: .375rem; background: rgb(249 250 251); padding: .625rem .75rem; font-size: .875rem; color: rgb(55 65 81); }
</style>
