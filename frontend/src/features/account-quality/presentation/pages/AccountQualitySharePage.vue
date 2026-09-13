<template>
  <main class="min-h-screen border-t-4 border-slate-800 bg-slate-50 px-4 py-8 text-slate-800 dark:bg-dark-950 dark:text-slate-100 sm:px-8">
    <div class="mx-auto max-w-6xl space-y-7">
      <header class="flex flex-wrap items-end justify-between gap-4 border-b border-slate-200 pb-6 dark:border-dark-700">
        <div>
          <p class="text-xs font-medium uppercase tracking-[0.2em] text-slate-400">MODEL QUALITY</p>
          <h1 class="mt-2 text-2xl font-semibold tracking-tight sm:text-3xl">{{ t('common.accountQuality.title') }}<span class="text-emerald-500">.</span></h1>
          <p class="mt-3 text-sm text-slate-500">{{ snapshot?.model || t('common.accountQuality.subtitle') }} · {{ t('common.accountQuality.recent') }}</p>
        </div>
        <button class="rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm shadow-sm disabled:opacity-50 dark:border-dark-700 dark:bg-dark-800" :disabled="loading" @click="load">
          {{ loading ? t('common.accountQuality.refreshing') : t('common.accountQuality.refresh') }}
        </button>
      </header>
      <p v-if="error" role="alert" class="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800">{{ t('common.accountQuality.qualityDisabled') }}</p>
      <template v-if="snapshot">
        <section :aria-label="t('common.accountQuality.recent')" class="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <article class="metric"><p>{{ t('common.accountQuality.passRate') }}</p><strong class="text-emerald-600">{{ passRate }}</strong><small>{{ snapshot.passed }} / {{ classified }} · {{ t('common.accountQuality.classified') }}</small></article>
          <article class="metric"><p>{{ t('common.accountQuality.total') }}</p><strong>{{ snapshot.total }}</strong><small>{{ t('common.accountQuality.totalNote') }}</small></article>
          <article class="metric"><p>{{ t('common.accountQuality.anomalies') }}</p><strong>{{ snapshot.degraded + snapshot.uncertain + snapshot.error }}</strong><small>{{ t('common.accountQuality.abnormal') }} {{ snapshot.degraded }} · {{ t('common.accountQuality.failed') }} {{ snapshot.error }} · {{ t('common.accountQuality.waiting') }} {{ snapshot.uncertain }}</small></article>
          <article class="rounded-xl bg-slate-800 p-5 text-white"><p class="text-xs text-slate-300">{{ t('common.accountQuality.nextCheck') }}</p><strong class="mt-4 block font-mono text-3xl tabular-nums">{{ countdown }}</strong><small class="mt-3 block text-slate-300">{{ t('common.accountQuality.frequency', { minutes: snapshot.interval_minutes }) }}</small></article>
        </section>
        <section class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800 sm:p-6">
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 pb-5 dark:border-dark-700">
            <div class="flex items-center gap-3"><span class="rounded-lg border border-slate-200 p-2 text-xs text-slate-400 dark:border-dark-700">01</span><div><h2 class="font-semibold">{{ snapshot.model || t('common.accountQuality.subtitle') }}</h2><p class="mt-1 text-xs text-slate-500">{{ t('common.accountQuality.effort') }} · {{ snapshot.effort }}</p></div></div>
            <span v-if="snapshot.points[0]" class="rounded-md bg-slate-50 px-3 py-1 text-xs dark:bg-dark-900">{{ verdictLabel(snapshot.points[0]) }}</span>
          </div>
          <div class="mt-5 flex flex-wrap justify-between gap-2"><h3 class="text-sm font-medium">{{ t('common.accountQuality.timeline') }}</h3><p class="text-xs text-slate-400">{{ t('common.accountQuality.timelineNote') }}</p></div>
          <div class="mt-5 grid grid-cols-[repeat(auto-fit,minmax(6px,1fr))] gap-1" :aria-label="t('common.accountQuality.timeline')">
            <button v-for="(point, index) in timeline" :key="point.id || index" class="h-10 min-w-0 rounded-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-700" :class="verdictColor[qualityVerdict(point)]" :aria-label="`${qualityTime(point.started_at)} · ${verdictLabel(point)} · ${t('common.accountQuality.details')}`" @click="selected = point" />
          </div>
          <p v-if="!timeline.length" class="py-8 text-center text-sm text-slate-400">{{ t('common.accountQuality.empty') }}</p>
          <div class="mt-3 flex justify-between text-xs text-slate-400"><span>{{ qualityTime(timeline[0]?.started_at) }}</span><span>{{ qualityTime(timeline[timeline.length - 1]?.started_at) }}</span></div>
          <div class="mt-5 flex flex-wrap gap-4 text-xs text-slate-500">
            <span v-for="legend in legends" :key="legend.key" class="flex items-center gap-2"><i class="h-2 w-2 rounded-sm" :class="verdictColor[legend.key]" />{{ legend.label }}</span>
          </div>
        </section>
        <div class="flex flex-wrap justify-between gap-3 text-xs text-slate-500"><span>{{ t('common.accountQuality.lastCheck') }} {{ qualityTime(snapshot.last_run_at) }}</span><span>{{ t('common.accountQuality.nextCheck') }} {{ qualityTime(snapshot.next_run_at) }}</span><span>{{ t('common.accountQuality.timezone') }}</span></div>
        <section class="space-y-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="flex items-center gap-3"><span class="rounded-lg border border-slate-200 p-2 text-xs text-slate-400 dark:border-dark-700">02</span><div><h2 class="text-lg font-semibold">{{ t('common.accountQuality.conversations') }}</h2><p class="mt-1 text-xs text-slate-500">{{ t('common.accountQuality.evidenceHint') }}</p></div></div>
            <label class="text-xs text-slate-500">{{ t('common.accountQuality.filter') }} <select v-model="filter" class="ml-2 rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-dark-700 dark:bg-dark-800"><option value="all">{{ t('common.accountQuality.all') }}</option><option value="images">{{ t('common.accountQuality.images') }}</option><option value="wrong">{{ t('common.accountQuality.abnormal') }}</option><option value="error">{{ t('common.accountQuality.failed') }}</option></select></label>
          </div>
          <div class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            <article v-for="(point, index) in visiblePoints" :key="point.id || index" class="min-w-0 overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="flex items-center justify-between gap-2 px-4 py-3"><time class="text-xs text-slate-500">{{ qualityTime(point.started_at) }}</time><span class="flex items-center gap-1.5 text-xs"><i class="h-1.5 w-1.5 rounded-full" :class="verdictColor[qualityVerdict(point)]" />{{ verdictLabel(point) }}</span></div>
              <div v-if="hasPreview(point)" class="block w-full px-3" role="button" tabindex="0" :aria-label="t('common.accountQuality.enlarge')" @click="selected = point" @keydown.enter="selected = point"><QualityPreview :point="point" /></div>
              <div class="space-y-3 p-4">
                <div class="text-xs"><p class="text-slate-500">{{ t('common.accountQuality.conversationId') }}</p><p class="mt-1 break-all font-mono">{{ point.details?.stage1?.conversation_id || point.details?.stage2?.conversation_id || t('common.accountQuality.idUnavailable') }}</p></div>
                <div><p class="text-xs text-slate-500">{{ t('common.accountQuality.answer') }}</p><p class="mt-1 line-clamp-3 whitespace-pre-wrap break-words text-sm leading-relaxed [overflow-wrap:anywhere]">{{ point.details?.stage1?.answer || point.details?.stage2?.answer || t('common.accountQuality.noAnswer') }}</p></div>
                <div class="flex items-center justify-between gap-2 border-t border-slate-100 pt-3 text-xs text-slate-500 dark:border-dark-700"><span>{{ point.effort || snapshot.effort }} · {{ ((point.latency_ms || 0) / 1000).toFixed(1) }}s</span><button class="font-medium text-emerald-700 dark:text-emerald-400" @click="selected = point">{{ t('common.accountQuality.details') }} ↗</button></div>
              </div>
            </article>
          </div>
          <p v-if="!visiblePoints.length" class="rounded-xl border border-dashed border-slate-200 py-12 text-center text-sm text-slate-400 dark:border-dark-700">{{ t('common.accountQuality.empty') }}</p>
          <button v-if="visiblePoints.length < filteredPoints.length" class="mx-auto block rounded-lg border border-slate-200 px-5 py-2 text-sm dark:border-dark-700" @click="visibleCount += 12">{{ t('common.accountQuality.loadMore') }} ({{ visiblePoints.length }}/{{ filteredPoints.length }})</button>
        </section>
      </template>
      <p v-else-if="loading" role="status" class="py-12 text-center text-sm text-slate-400">{{ t('common.accountQuality.refreshing') }}</p>
      <footer class="border-t border-slate-200 pt-5 text-xs text-slate-400 dark:border-dark-700">{{ t('common.accountQuality.footer', { minutes: snapshot?.interval_minutes || '—' }) }}</footer>
    </div>
    <BaseDialog :show="!!selected" :title="t('common.accountQuality.details')" width="extra-wide" @close="selected = null">
      <div v-if="selected" class="space-y-5">
        <div class="text-xs text-slate-500"><p>{{ selected.model || snapshot?.model }} · {{ selected.effort || snapshot?.effort }} · {{ qualityTime(selected.started_at) }}</p><p class="mt-2 break-all font-mono">{{ t('common.accountQuality.runId') }}: {{ selected.id || '—' }}</p></div>
        <QualityPreview v-if="hasPreview(selected)" :point="selected" />
        <div class="grid gap-4 md:grid-cols-2"><QualityConversation :title="t('common.accountQuality.stage1')" :detail="selected.details?.stage1" /><QualityConversation :title="t('common.accountQuality.stage2')" :detail="selected.details?.stage2" /></div>
      </div>
    </BaseDialog>
  </main>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import QualityPreview from '../widgets/QualityPreview.vue'
import QualityConversation from '../widgets/QualityConversation.vue'
import { qualityTime, qualityVerdict, verdictColor, hasPreview } from '../qualityDisplay'
import { getPublicSnapshot, type AccountQualityPublicPoint, type AccountQualityPublicSnapshot } from '../../data/datasources/accountQualityPublicDatasource'
const { t } = useI18n()
const snapshot = ref<AccountQualityPublicSnapshot | null>(null)
const selected = ref<AccountQualityPublicPoint | null>(null)
const loading = ref(false)
const error = ref(false)
const filter = ref('all')
const visibleCount = ref(12)
const now = ref(Date.now())
let serverOffset = 0
let disposed = false
let timer: ReturnType<typeof setInterval> | undefined
let clock: ReturnType<typeof setInterval> | undefined
const timeline = computed(() => [...(snapshot.value?.points || [])].reverse())
const classified = computed(() => (snapshot.value?.passed || 0) + (snapshot.value?.degraded || 0))
const passRate = computed(() => classified.value ? `${((snapshot.value!.passed / classified.value) * 100).toFixed(1)}%` : '—')
const filteredPoints = computed(() => (snapshot.value?.points || []).filter(p => filter.value === 'all' || (filter.value === 'images' ? hasPreview(p) : qualityVerdict(p) === filter.value)))
const visiblePoints = computed(() => filteredPoints.value.slice(0, visibleCount.value))
const legends = computed(() => [
  { key: 'passed' as const, label: t('common.accountQuality.normal') },
  { key: 'wrong' as const, label: t('common.accountQuality.abnormal') },
  { key: 'uncertain' as const, label: t('common.accountQuality.waiting') },
  { key: 'error' as const, label: t('common.accountQuality.failed') }
])
function verdictLabel(point: AccountQualityPublicPoint) { return legends.value.find(v => v.key === qualityVerdict(point))!.label }
const countdown = computed(() => {
  const next = Date.parse(snapshot.value?.next_run_at || '')
  if (!Number.isFinite(next)) return '—'
  const seconds = Math.max(0, Math.ceil((next - now.value - serverOffset) / 1000))
  if (!seconds) return t('common.accountQuality.due')
  return `${String(Math.floor(seconds / 60)).padStart(2, '0')} : ${String(seconds % 60).padStart(2, '0')}`
})
watch(filter, () => { visibleCount.value = 12 })
async function load() {
  if (loading.value) return
  loading.value = true
  try {
    const value = await getPublicSnapshot()
    if (disposed) return
    snapshot.value = value
    const serverTime = Date.parse(value.now)
    serverOffset = Number.isFinite(serverTime) ? serverTime - Date.now() : 0
    error.value = false
  } catch {
    if (!disposed) { error.value = true; snapshot.value = null; selected.value = null }
  } finally { loading.value = false }
}
onMounted(() => {
  void load()
  timer = setInterval(() => void load(), 20000)
  clock = setInterval(() => { now.value = Date.now() }, 1000)
})
onBeforeUnmount(() => { disposed = true; clearInterval(timer); clearInterval(clock) })
</script>
<style scoped>
.metric { @apply rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800; }
.metric p { @apply text-xs text-slate-500; }
.metric strong { @apply mt-4 block text-3xl font-semibold tabular-nums; }
.metric small { @apply mt-3 block text-xs leading-relaxed text-slate-400; }
</style>
