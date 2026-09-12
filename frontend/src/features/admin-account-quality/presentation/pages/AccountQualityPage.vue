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
          <button class="btn btn-secondary" type="button" :disabled="loading || running" @click="loadOverview">{{ t('admin.accountInspection.refresh') }}</button>
          <button class="btn btn-primary" type="button" :disabled="loading || running" @click="runNow">{{ running ? t('admin.accountInspection.running') : t('admin.accountQuality.runNow') }}</button>
        </div>
      </header>

      <div v-if="errorMessage" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{{ errorMessage }}</div>

      <section class="border-y border-gray-200 py-5 dark:border-dark-700">
        <div class="mb-4 flex items-center justify-between gap-3">
          <div><h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accountQuality.settingsTitle') }}</h2><p class="mt-1 text-xs text-gray-500">{{ t('admin.accountQuality.settingsCaption') }}</p></div>
          <button class="btn btn-primary" type="button" :disabled="saving || loading" @click="saveSettings">{{ saving ? t('admin.accountInspection.saving') : t('admin.accountInspection.save') }}</button>
        </div>
        <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <label class="toggle-field"><span>{{ t('admin.accountInspection.quality.enabled') }}</span><Toggle :model-value="settings.enabled" @update:model-value="settings.enabled = $event" /></label>
          <label class="toggle-field"><span>{{ t('admin.accountInspection.quality.publicEnabled') }}</span><Toggle :model-value="settings.public_enabled" @update:model-value="settings.public_enabled = $event" /></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.interval') }}</span><select v-model.number="settings.interval_minutes" class="input"><option v-for="value in [5, 10, 15, 30, 60]" :key="value" :value="value">{{ t('admin.accountInspection.settings.minutes', { value }) }}</option></select></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.model') }}</span><input v-model="settings.model" class="input" :placeholder="t('admin.accountInspection.quality.modelPlaceholder')" /></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.effort') }}</span><select v-model="settings.effort" class="input"><option v-for="value in ['minimal', 'low', 'medium', 'high', 'xhigh', 'max']" :key="value" :value="value">{{ value }}</option></select></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.sourceGroup') }}</span><select v-model="settings.source_group_id" class="input"><option :value="null">{{ t('admin.accountInspection.quality.allGroups') }}</option><option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }} · {{ group.platform }} (#{{ group.id }})</option></select></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.degradedGroup') }}</span><select v-model="settings.degraded_group_id" class="input"><option :value="null">{{ t('admin.accountInspection.quality.noSwitch') }}</option><option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }} · {{ group.platform }} (#{{ group.id }})</option></select></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.failureThreshold') }}</span><input v-model.number="settings.failure_threshold" class="input" type="number" min="1" /></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.recoveryThreshold') }}</span><input v-model.number="settings.recovery_threshold" class="input" type="number" min="1" /></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.maxConcurrent') }}</span><input v-model.number="settings.max_concurrent" class="input" type="number" min="1" max="4" /></label>
          <label class="field-group"><span class="field-label">{{ t('admin.accountInspection.quality.minConfidence') }}</span><input v-model.number="settings.min_confidence" class="input" type="number" min="0" max="1" step="0.01" /></label>
          <label class="field-group md:col-span-2 xl:col-span-4"><span class="field-label">{{ t('admin.accountInspection.quality.prompt') }}</span><input v-model="settings.prompt" class="input" maxlength="500" /></label>
        </div>
      </section>

      <section class="grid grid-cols-2 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800 sm:grid-cols-3 xl:grid-cols-6">
        <div v-for="item in summaryItems" :key="item.key" class="border-b border-r border-gray-200 px-4 py-3 dark:border-dark-700"><dt class="text-xs text-gray-500">{{ item.label }}</dt><dd class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ item.value }}</dd></div>
      </section>

      <section class="overflow-x-auto rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <table class="w-full min-w-[900px] text-left text-sm"><thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/60"><tr><th class="px-4 py-3">{{ t('admin.accountInspection.results.account') }}</th><th class="px-4 py-3">{{ t('admin.accountInspection.quality.healthy') }}</th><th class="px-4 py-3">{{ t('admin.accountInspection.results.action') }}</th><th class="px-4 py-3">{{ t('admin.accountInspection.results.reason') }}</th><th class="px-4 py-3">{{ t('admin.accountInspection.results.ttft') }}</th></tr></thead><tbody class="divide-y divide-gray-100 dark:divide-dark-700"><tr v-for="row in results" :key="row.account_id"><td class="px-4 py-3 font-medium text-gray-900 dark:text-white">{{ row.name }} <span class="text-xs text-gray-400">#{{ row.account_id }} · {{ row.platform }}</span></td><td class="px-4 py-3"><span :class="qualityStatusClass(row.quality_status)">{{ qualityStatusLabel(row.quality_status) }}</span><span class="ml-2 text-xs text-gray-400">F{{ row.quality_consecutive_failures || 0 }} / P{{ row.quality_consecutive_passes || 0 }}</span></td><td class="px-4 py-3 text-xs">{{ row.quality_action || '-' }}</td><td class="max-w-md truncate px-4 py-3 text-xs text-gray-500" :title="row.quality_error">{{ row.quality_error || row.quality_label || '-' }}</td><td class="px-4 py-3 tabular-nums">{{ row.quality_latency_ms ? `${Math.round(row.quality_latency_ms)}ms` : '-' }}</td></tr><tr v-if="!loading && !results.length"><td colspan="5" class="px-4 py-10 text-center text-gray-500">{{ t('admin.accountInspection.results.noResults') }}</td></tr></tbody></table>
      </section>
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

const { t } = useI18n(); const appStore = useAppStore(); const loading = ref(false); const saving = ref(false); const running = ref(false); const errorMessage = ref(''); const overview = ref<AccountQualityOverview | null>(null); const results = computed<AccountQualityResult[]>(() => overview.value?.results.items ?? []); const run = computed(() => overview.value?.run)
const settings = reactive<AccountQualitySettings>({ enabled: false, interval_minutes: 10, model: '', effort: 'medium', prompt: '', failure_threshold: 2, recovery_threshold: 2, degraded_group_id: null, source_group_id: null, max_concurrent: 4, min_confidence: 0.85, public_enabled: false }); const groups = ref<Array<{ id: number; name: string; platform?: string }>>([]); let timer: ReturnType<typeof setInterval> | null = null
const statusLabel = computed(() => run.value?.status === 'running' ? t('admin.accountInspection.status.running') : run.value?.status === 'failed' ? t('admin.accountInspection.status.failed') : run.value?.status === 'succeeded' ? t('admin.accountInspection.status.succeeded') : t('admin.accountInspection.status.idle')); const statusClass = computed(() => run.value?.status === 'failed' ? 'bg-red-50 text-red-700' : run.value?.status === 'succeeded' ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-600'); const summaryItems = computed(() => { const s = run.value?.summary ?? { inspected: 0, passed: 0, degraded: 0, uncertain: 0, errors: 0, switched: 0 }; return [{ key: 'inspected', label: t('admin.accountInspection.summary.inspected'), value: s.inspected }, { key: 'passed', label: t('admin.accountQuality.passed'), value: s.passed }, { key: 'degraded', label: t('admin.accountInspection.summary.qualityDegraded'), value: s.degraded }, { key: 'uncertain', label: t('admin.accountQuality.uncertain'), value: s.uncertain }, { key: 'errors', label: t('admin.accountQuality.errors'), value: s.errors }, { key: 'switched', label: t('admin.accountInspection.summary.qualitySwitched'), value: s.switched }] })
function qualityStatusLabel(status?: string): string { if (status === 'healthy') return t('admin.accountQuality.passed'); if (status === 'degraded') return t('admin.accountInspection.summary.qualityDegraded'); if (status === 'uncertain') return t('admin.accountQuality.uncertain'); if (status === 'error') return t('admin.accountQuality.errors'); return '-' }
function qualityStatusClass(status?: string): string { if (status === 'degraded') return 'text-amber-600'; if (status === 'error') return 'text-red-600'; if (status === 'uncertain') return 'text-gray-500'; return 'text-emerald-600' }
async function loadOverview() { loading.value = true; errorMessage.value = ''; try { const data = await getOverview(); overview.value = data; Object.assign(settings, data.settings) } catch (error) { errorMessage.value = extractApiErrorMessage(error, t('admin.accountQuality.loadFailed')) } finally { loading.value = false } }
async function saveSettings() { saving.value = true; try { Object.assign(settings, await updateSettings({ ...settings })); appStore.showSuccess(t('admin.accountQuality.saved')) } catch (error) { errorMessage.value = extractApiErrorMessage(error, t('admin.accountQuality.saveFailed')) } finally { saving.value = false } }
async function runNow() { running.value = true; try { overview.value = await runQualityMonitoring(); Object.assign(settings, overview.value.settings); appStore.showSuccess(t('admin.accountQuality.runCompleted', { count: overview.value.run.summary.degraded })) } catch (error) { errorMessage.value = extractApiErrorMessage(error, t('admin.accountQuality.runFailed')) } finally { running.value = false; await loadOverview() } }
onMounted(() => { void loadOverview(); void getAllGroups().then(items => { groups.value = items.filter(item => item.status === 'active' && (item.platform === 'openai' || item.platform === 'gemini')) }).catch(() => { groups.value = [] }); timer = setInterval(() => { if (run.value?.status === 'running') void loadOverview() }, 5000) }); onBeforeUnmount(() => { if (timer) clearInterval(timer) })
</script>

<style scoped>
.field-group { display: flex; flex-direction: column; gap: 0.35rem; }
.field-label { color: rgb(107 114 128); font-size: 0.75rem; line-height: 1rem; }
.toggle-field { display: flex; align-items: center; justify-content: space-between; gap: .75rem; border-radius: .375rem; background: rgb(249 250 251); padding: .625rem .75rem; font-size: .875rem; color: rgb(55 65 81); }
</style>
