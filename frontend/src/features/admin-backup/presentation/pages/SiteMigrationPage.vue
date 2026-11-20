<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-6 pb-8">
      <p v-if="!secure" role="alert" class="rounded-lg border border-amber-300 bg-amber-50 p-4 text-sm text-amber-800">{{ t('admin.migration.https') }}</p>
      <form class="card space-y-5 p-5 md:p-6" @submit.prevent="confirming = true">
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.migration.newTask') }}</h2>
        <fieldset :disabled="busy || !secure" class="space-y-3">
          <legend class="text-sm font-medium">{{ t('admin.migration.direction') }}</legend>
          <div class="grid gap-3 sm:grid-cols-2">
            <label v-for="item in directions" :key="item" class="flex cursor-pointer items-center gap-3 rounded-xl border p-3.5" :class="direction === item ? 'border-primary-500 bg-primary-50 dark:bg-primary-950/30' : 'border-gray-200 dark:border-dark-600'">
              <input v-model="direction" type="radio" :value="item" name="direction" class="accent-primary-600" />
              <Icon :name="item === 'push' ? 'upload' : 'download'" size="sm" />
              <span class="text-sm font-semibold">{{ t('admin.migration.' + item) }}</span>
            </label>
          </div>
        </fieldset>

        <div class="grid gap-4 md:grid-cols-2">
          <label class="space-y-1.5 text-sm font-medium">
            <span>{{ t('admin.migration.peer') }}</span>
            <input v-model.trim="peer" class="input w-full" required maxlength="512" autocomplete="off" autocapitalize="none" spellcheck="false" placeholder="gateway.example.com" :disabled="busy || !secure" />
          </label>
          <label class="space-y-1.5 text-sm font-medium">
            <span>{{ t('admin.migration.key') }}</span>
            <input v-model="peerKey" class="input w-full" type="password" required minlength="16" maxlength="256" autocomplete="off" spellcheck="false" :placeholder="t('admin.migration.keyPlaceholder')" :disabled="busy || !secure" />
          </label>
        </div>
        <label class="block space-y-1.5 text-sm font-medium">
          <span>{{ t('admin.migration.scope') }}</span>
          <select v-model="scope" class="input w-full" :disabled="busy || !secure">
            <option v-for="item in scopes" :key="item" :value="item">{{ t('admin.migration.scopes.' + item) }}</option>
          </select>
        </label>
        <div class="rounded-lg bg-gray-50 p-3.5 text-sm text-gray-600 dark:bg-dark-700/60 dark:text-dark-300">
          <p>{{ t('admin.migration.hints.' + scope) }}</p>
          <p class="mt-1 break-words">{{ t('admin.migration.permission') }} <code class="text-xs">{{ grant }}</code></p>
        </div>
        <div class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-200 pt-4 dark:border-dark-700">
          <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.migration.keyPrivacy') }}</span>
          <button type="submit" class="btn btn-primary" :disabled="busy || !secure">{{ t('admin.migration.review') }}</button>
        </div>
      </form>

      <section class="card p-5 md:p-6" :aria-label="t('admin.migration.tasks')">
        <div class="flex items-center justify-between gap-3">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.migration.tasks') }}</h2>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="refresh"><Icon name="refresh" size="sm" />{{ t('admin.migration.refresh') }}</button>
        </div>
        <p v-if="loadError" class="mt-4 text-sm text-red-600" role="alert">{{ loadError }}</p>
        <p v-else-if="!jobs.length" class="py-8 text-center text-sm text-gray-500">{{ t(loading ? 'admin.migration.loading' : 'admin.migration.empty') }}</p>
        <ul v-if="jobs.length" class="mt-3 divide-y divide-gray-200 dark:divide-dark-700">
          <li v-for="job in jobs" :key="job.id" class="space-y-2 py-4">
            <div class="flex flex-wrap items-start justify-between gap-2">
              <div class="min-w-0">
                <strong class="break-all text-sm">{{ t('admin.migration.' + job.direction) }} · {{ job.peer }}</strong>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.migration.scopes.' + job.profile) }} · {{ new Date(job.created_at).toLocaleString() }}</p>
              </div>
              <span class="rounded-full px-2.5 py-1 text-xs font-semibold" :class="phaseStyle(job.phase)">{{ t('admin.migration.phases.' + job.phase) }}</span>
            </div>
            <div v-if="job.total > 0 && !terminal(job.phase)" class="space-y-1">
              <progress class="h-1.5 w-full accent-primary-600" :value="job.acknowledged" :max="job.total" :aria-label="t('admin.migration.progress')" />
              <p class="text-xs text-gray-500">{{ t('admin.migration.blocks', { done: job.acknowledged, total: job.total }) }}</p>
            </div>
            <p v-if="job.phase === 'completed'" class="text-sm">{{ t('admin.migration.counts', { inserted: job.inserted, issues: job.issues }) }}</p>
            <p v-if="job.phase === 'completed' && job.profile === 'clone'" class="break-all text-sm">{{ t('admin.migration.cloneReady', { database: 'sub2api_clone_' + job.id.replace(/-/g, ''), directory: 'site-migration-clones/' + job.id + '/pages' }) }}</p>
            <p v-if="job.phase === 'failed' || job.phase === 'interrupted'" class="text-sm text-amber-700 dark:text-amber-400">{{ t('admin.migration.errors.' + (job.error_code || 'interrupted')) }}</p>
            <div class="flex flex-wrap items-center justify-between gap-2">
              <code class="break-all text-xs text-gray-500 dark:text-dark-400">{{ job.id }}</code>
              <button v-if="job.phase === 'completed'" type="button" class="btn btn-secondary btn-sm" :disabled="busy || !secure" @click="download(job)"><Icon name="download" size="sm" />{{ t('admin.migration.csv') }}</button>
              <button v-else-if="terminal(job.phase)" type="button" class="btn btn-secondary btn-sm" :disabled="busy || !secure" @click="openResume(job)">{{ t('admin.migration.resume') }}</button>
            </div>
          </li>
        </ul>
      </section>

      <ConfirmDialog :show="confirming" :title="t('admin.migration.confirm')" :message="confirmMessage" :confirm-text="t('admin.migration.start')" @cancel="confirming = false" @confirm="submit" />
      <BaseDialog :show="!!resuming" :title="t('admin.migration.resume')" width="narrow" @close="closeResume">
        <form class="space-y-4" @submit.prevent="submitResume">
          <p class="break-all text-sm text-gray-500">{{ resuming?.peer }} · {{ resuming?.id }}</p>
          <label class="block space-y-2 text-sm"><span>{{ t('admin.migration.resumeKey') }}</span><input v-model="resumeKey" required minlength="16" maxlength="256" type="password" class="input w-full" autocomplete="off" /></label>
          <button class="btn btn-primary" type="submit" :disabled="busy || !secure">{{ t('admin.migration.resume') }}</button>
        </form>
      </BaseDialog>
      <TotpStepUpDialog :controller="stepUp" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import ConfirmDialog from '@/common/widgets/feedback/ConfirmDialog.vue'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import { useStepUp, isStepUpBlocked, isStepUpCancelled } from '@/common/composables/useStepUp'
import TotpStepUpDialog from '@/features/auth/totpStepUpDialog'
import { useAppStore } from '@/core/stores/appStore'
import { downloadMigrationReport, listMigrationJobs, resumeMigration, startMigration, secureMigrationOrigin, type MigrationDirection, type MigrationJob, type MigrationPhase, type MigrationScope } from '../../data/datasources/siteMigrationDatasource'

const { t } = useI18n()
const app = useAppStore()
const stepUp = useStepUp()
const secure = secureMigrationOrigin()
const directions: MigrationDirection[] = ['push', 'pull']
const scopes: MigrationScope[] = ['all', 'login', 'login-quota', 'clone']
const direction = ref<MigrationDirection>('push')
const scope = ref<MigrationScope>('all')
const peer = ref('')
const peerKey = ref('')
const jobs = ref<MigrationJob[]>([])
const resuming = ref<MigrationJob | null>(null)
const resumeKey = ref('')
const confirming = ref(false)
const busy = ref(false)
const loading = ref(false)
const loadError = ref('')
const controller = new AbortController()
let poll: ReturnType<typeof setTimeout> | undefined
let refreshQueued = false
let retry: { id: string; signature: string } | undefined

const grant = computed(() => 'admin.migration.' + (scope.value.startsWith('login') ? 'users.' : '') + (direction.value === 'push' ? 'import' : 'export'))
const confirmMessage = computed(() => t('admin.migration.confirmRoute', { direction: t('admin.migration.' + direction.value), peer: peer.value, scope: t('admin.migration.scopes.' + scope.value) }) + ' ' + t('admin.migration.' + (scope.value === 'clone' ? 'cloneNotice' : 'mergeNotice')))
function terminal(phase: MigrationPhase) { return ['completed', 'interrupted', 'failed'].includes(phase) }
function phaseStyle(phase: MigrationPhase) {
  if (phase === 'completed') return 'bg-green-50 text-green-700 dark:bg-green-900/30 dark:text-green-400'
  if (terminal(phase)) return 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
  return 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
}
function errorMessage(err: unknown) {
  const value = err as { reason?: string; status?: number }
  if (value.reason === 'MIGRATION_HTTPS_REQUIRED') return t('admin.migration.https')
  if (value.reason === 'MIGRATION_PEER_ADDRESS') return t('admin.migration.errors.address')
  if (isStepUpBlocked(err)) return t('stepUp.notEnabled')
  if (value.status === 429) return t('admin.migration.errors.limit')
  if (value.status === 404) return t('admin.migration.errors.checkpoint')
  return t('admin.migration.requestError')
}
async function refresh() {
  if (controller.signal.aborted || !secure) return
  if (loading.value) { refreshQueued = true; return }
  clearTimeout(poll)
  loading.value = true
  try {
    jobs.value = await listMigrationJobs(controller.signal)
    loadError.value = ''
  } catch (e) { if (!controller.signal.aborted) loadError.value = errorMessage(e) }
  finally {
    loading.value = false
    if (!controller.signal.aborted && refreshQueued) {
      refreshQueued = false
      poll = setTimeout(refresh, 0)
    } else if (!controller.signal.aborted && (loadError.value || jobs.value.some(j => !terminal(j.phase)))) {
      poll = setTimeout(refresh, 3000)
    }
  }
}
async function act(run: () => Promise<void>) {
  if (!secure || busy.value) return
  busy.value = true
  try { await run() }
  catch (e) { if (!isStepUpCancelled(e) && !controller.signal.aborted) app.showError(errorMessage(e)) }
  finally { busy.value = false }
}
function openResume(job: MigrationJob) { resumeKey.value = ''; resuming.value = job }
function closeResume() { resumeKey.value = ''; resuming.value = null }
function submit() {
  if (!secure || !confirming.value) return
  confirming.value = false
  void act(async () => {
    const signature = JSON.stringify([peer.value, scope.value, direction.value])
    if (retry?.signature !== signature) retry = { id: crypto.randomUUID(), signature }
    const form = { id: retry.id, peer: peer.value, profile: scope.value, direction: direction.value, key: peerKey.value }
    peerKey.value = ''
    try {
      const job = await stepUp.run(() => startMigration(form))
      retry = undefined
      jobs.value = [job, ...jobs.value.filter(j => j.id !== job.id)]
      await refresh()
    } finally { form.key = '' }
  })
}
function submitResume() {
  const job = resuming.value
  if (!job) return
  void act(async () => {
    const key = resumeKey.value
    closeResume()
    const updated = await stepUp.run(() => resumeMigration(job.id, key))
    jobs.value = [updated, ...jobs.value.filter(j => j.id !== updated.id)]
    await refresh()
  })
}
function download(job: MigrationJob) {
  void act(async () => {
    const blob = await stepUp.run(() => downloadMigrationReport(job.id))
    const href = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = href
    a.download = 'migration-errors-' + job.id + '.csv'
    document.body.appendChild(a)
    a.click()
    a.remove()
    setTimeout(() => URL.revokeObjectURL(href), 1000)
  })
}
onMounted(refresh)
onBeforeUnmount(() => { controller.abort(); clearTimeout(poll); stepUp.onCancel(); peerKey.value = ''; closeResume() })
</script>
