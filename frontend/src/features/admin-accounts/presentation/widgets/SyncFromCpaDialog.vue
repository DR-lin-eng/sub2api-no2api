<template>
  <BaseDialog :show="show" :title="t('admin.accounts.cpaImport.title')" width="normal" @close="handleClose">
    <form v-if="step === 'input'" id="sync-from-cpa-form" class="space-y-4" @submit.prevent="handlePreview">
      <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('admin.accounts.cpaImport.description') }}</p>
      <label class="block">
        <span class="input-label">{{ t('admin.accounts.cpaManagementUrl') }}</span>
        <input id="cpa-base-url" v-model="form.base_url" type="url" class="input" required placeholder="https://cpa.example.com" :disabled="busy" />
      </label>
      <label class="block">
        <span class="input-label">{{ t('admin.accounts.cpaManagementKey') }}</span>
        <input id="cpa-password" v-model="form.management_password" type="password" class="input" required autocomplete="off" :disabled="busy" />
      </label>
      <label class="block">
        <span class="input-label">{{ t('admin.accounts.cpaImport.platform') }}</span>
        <select id="cpa-platform" v-model="form.platform" class="input" :disabled="busy">
          <option value="openai">OpenAI / Codex OAuth</option>
          <option value="anthropic">Anthropic / Claude OAuth</option>
          <option value="gemini">Gemini CLI OAuth</option>
          <option value="antigravity">Antigravity OAuth</option>
        </select>
      </label>
      <fieldset :disabled="busy" class="space-y-4">
        <GroupSelector v-model="groupIds" :groups="eligibleGroups" :platform="form.platform" />
        <p class="text-xs text-gray-500">{{ t('admin.accounts.cpaImport.groupHint') }}</p>
        <CPASyncOAuthOptions v-model="oauthOptions" :platform="form.platform" />
        <label class="flex items-center gap-2 text-sm">
          <input id="cpa-apply-existing" v-model="applySettingsToExisting" type="checkbox" />
          {{ t('admin.accounts.cpaImport.applyExisting') }}
        </label>
        <p class="text-xs text-gray-500">{{ t('admin.accounts.cpaImport.updateHint') }}</p>
      </fieldset>
    </form>

    <div v-else-if="step === 'preview' && preview" class="space-y-4">
      <p class="text-sm" data-testid="cpa-preview-summary">{{ t('admin.accounts.cpaImport.previewSummary', { total: preview.total, eligible: preview.accounts.length, skipped: preview.skipped }) }}</p>
      <div v-if="preview.skipped" class="space-y-1 rounded-lg bg-gray-50 p-3 text-xs dark:bg-dark-800">
        <div v-for="(count, reason) in preview.skip_reasons" :key="reason">{{ reasonLabel(reason) }}: {{ count }}</div>
      </div>
      <div class="flex items-center justify-between text-sm">
        <span>{{ t('common.selectedCount', { count: selectedFiles.length }) }}</span>
        <div class="flex gap-3">
          <button type="button" :disabled="busy" @click="selectedFiles = preview.accounts.map(a => a.file_name)">{{ t('admin.accounts.crsSelectAll') }}</button>
          <button type="button" :disabled="busy" @click="selectedFiles = []">{{ t('admin.accounts.crsSelectNone') }}</button>
        </div>
      </div>
      <div class="max-h-72 space-y-1 overflow-auto rounded-lg border border-gray-200 p-2 dark:border-dark-600">
        <label v-for="account in preview.accounts" :key="account.file_name" class="flex items-center gap-2 rounded p-2 text-sm hover:bg-gray-50 dark:hover:bg-dark-700">
          <input v-model="selectedFiles" type="checkbox" :value="account.file_name" :disabled="busy" data-testid="cpa-account-selection" />
          <span class="min-w-0 flex-1 truncate" :title="account.file_name">{{ account.name }}</span>
          <span class="shrink-0 text-xs text-gray-500">{{ t(account.existing ? 'admin.accounts.cpaImport.existing' : 'admin.accounts.cpaImport.newAccount') }}</span>
        </label>
        <p v-if="!preview.accounts.length" class="p-4 text-sm text-gray-500">{{ t('admin.accounts.cpaImport.empty') }}</p>
      </div>
      <p class="text-xs text-gray-500">{{ t('admin.accounts.cpaImport.recheckHint') }}</p>
      <p v-if="syncing" role="status" class="text-sm">{{ t('admin.accounts.cpaImport.progress', { done: processed, total: selectedFiles.length }) }}</p>
    </div>

    <div v-else-if="step === 'result' && result" class="space-y-4">
      <p class="text-sm font-medium" data-testid="cpa-result-summary">{{ t('admin.accounts.syncResultSummary', result) }}</p>
      <p v-if="batchError" role="alert" class="text-sm text-red-600">{{ batchError }}</p>
      <div v-if="resultDetails.length" class="max-h-72 space-y-2 overflow-auto rounded-lg bg-gray-50 p-3 text-xs dark:bg-dark-800">
        <div v-for="(item, index) in resultDetails" :key="index" class="break-words">
          {{ item.file_name }} — {{ reasonLabel(item.reason || item.action) }}<span v-if="item.message">: {{ item.message }}</span>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" type="button" :disabled="busy" @click="handleClose">{{ t(step === 'result' ? 'common.close' : 'common.cancel') }}</button>
        <button v-if="step === 'input'" class="btn btn-primary" type="submit" form="sync-from-cpa-form" :disabled="busy">{{ t(previewing ? 'admin.accounts.crsPreviewing' : 'admin.accounts.crsPreview') }}</button>
        <template v-else-if="step === 'preview'">
          <button class="btn btn-secondary" type="button" :disabled="busy" @click="step = 'input'">{{ t('admin.accounts.crsBack') }}</button>
          <button class="btn btn-primary" type="button" :disabled="busy || !selectedFiles.length" data-testid="cpa-sync" @click="handleSync">{{ t(syncing ? 'admin.accounts.syncing' : 'admin.accounts.syncNow') }}</button>
        </template>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import GroupSelector from '@/common/widgets/data/GroupSelector.vue'
import { useAppStore } from '@/core/stores/appStore'
import type { AdminGroup } from '@/types'
import type { CPAConnectionParams, CPAPreviewResult, CPASyncResult } from '../../data/dtos/cpaSyncDtos'
import { previewFromCpa } from '../../data/datasources/adminAccountQueries'
import { syncFromCpa } from '../../data/datasources/adminAccountActions'
import { defaultCPAOAuthOptions } from '../cpaSyncOptions'
import CPASyncOAuthOptions from './CPASyncOAuthOptions.vue'

const props = defineProps<{ show: boolean; groups: AdminGroup[] }>()
const emit = defineEmits<{ close: []; synced: [] }>()
const { t, te } = useI18n()
const appStore = useAppStore()
const step = ref<'input' | 'preview' | 'result'>('input')
const previewing = ref(false)
const syncing = ref(false)
const busy = computed(() => previewing.value || syncing.value)
const form = reactive<CPAConnectionParams>({ base_url: '', management_password: '', platform: 'openai' })
const groupIds = ref<number[]>([])
const oauthOptions = ref(defaultCPAOAuthOptions('openai'))
const applySettingsToExisting = ref(false)
const eligibleGroups = computed(() => props.groups.filter(g => (g.platform === form.platform || g.platform === 'composite') && g.status === 'active'))
const preview = ref<CPAPreviewResult | null>(null)
const selectedFiles = ref<string[]>([])
const result = ref<CPASyncResult | null>(null)
const processed = ref(0)
const batchError = ref('')
const resultDetails = computed(() => result.value?.items.filter(item => item.action === 'failed' || item.action === 'skipped' || item.reason) ?? [])
let controller: AbortController | null = null
let generation = 0
let didSync = false
function reasonLabel(reason: string) {
  const key = `admin.accounts.cpaImport.reasons.${reason}`
  return te(key) ? t(key) : reason
}
watch(() => form.platform, (platform) => {
  groupIds.value = []
  oauthOptions.value = defaultCPAOAuthOptions(platform)
  preview.value = null
  selectedFiles.value = []
})
watch(() => props.show, (show) => {
  if (!show) { dispose(); form.management_password = ''; return }
  generation++
  step.value = 'input'
  form.base_url = ''
  form.management_password = ''
  form.platform = 'openai'
  groupIds.value = []
  oauthOptions.value = defaultCPAOAuthOptions('openai')
  applySettingsToExisting.value = false
  preview.value = null
  selectedFiles.value = []
  result.value = null
  batchError.value = ''
  processed.value = 0
  didSync = false
})
function dispose() { generation++; controller?.abort(); controller = null; previewing.value = false; syncing.value = false }
onBeforeUnmount(dispose)
function handleClose() {
  if (busy.value) return
  form.management_password = ''
  if (didSync) emit('synced')
  emit('close')
}
function connection(): CPAConnectionParams { return { ...form, base_url: form.base_url.trim() } }
async function handlePreview() {
  if (busy.value) return
  if (!form.base_url.trim() || !form.management_password.trim()) { appStore.showError(t('admin.accounts.cpaImport.missingFields')); return }
  const current = generation
  controller = new AbortController()
  previewing.value = true
  try {
    const data = await previewFromCpa(connection(), controller.signal)
    if (current !== generation) return
    preview.value = data
    selectedFiles.value = data.accounts.map(a => a.file_name)
    step.value = 'preview'
  } catch {
    if (current === generation) appStore.showError(t('admin.accounts.cpaImport.previewFailed'))
  } finally {
    if (current === generation) previewing.value = false
  }
}
async function handleSync() {
  if (busy.value || !selectedFiles.value.length) return
  const current = generation
  controller = new AbortController()
  syncing.value = true
  processed.value = 0
  batchError.value = ''
  const summary: CPASyncResult = { created: 0, updated: 0, skipped: 0, failed: 0, items: [] }
  // Keep each request bounded. Stop on an uncertain transport result; never
  // automatically retry writes. A fresh preview can reconcile committed work.
  const batchSize = Math.max(1, Math.min(50, preview.value?.batch_size || 50))
  const selected = [...selectedFiles.value]
  try {
    for (let start = 0; start < selected.length; start += batchSize) {
      const data = await syncFromCpa({ ...connection(), selected_files: selected.slice(start, start + batchSize), group_ids: [...groupIds.value], oauth_options: { ...oauthOptions.value }, apply_settings_to_existing: applySettingsToExisting.value }, controller.signal)
      if (current !== generation) return
      summary.created += data.created
      summary.updated += data.updated
      summary.skipped += data.skipped
      summary.failed += data.failed
      summary.items.push(...data.items)
      processed.value += data.items.length
      didSync = true
    }
    if (summary.failed) appStore.showError(t('admin.accounts.syncCompletedWithErrors', { ...summary }))
    else appStore.showSuccess(t('admin.accounts.syncCompleted', { ...summary }))
  } catch {
    if (current !== generation) return
    didSync = true
    batchError.value = t('admin.accounts.cpaImport.batchFailed', { remaining: selected.length - processed.value })
    appStore.showError(batchError.value)
  } finally {
    if (current === generation) { result.value = summary; step.value = 'result'; syncing.value = false; form.management_password = '' }
  }
}
</script>
