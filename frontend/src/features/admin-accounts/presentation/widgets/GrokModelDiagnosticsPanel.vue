<template>
  <section class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600" data-testid="grok-catalog">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h3 class="font-medium">{{ t('admin.accounts.grokCatalog.title') }}</h3>
      <button type="button" class="btn btn-secondary" :disabled="loading" @click="load(true)">
        {{ t('admin.accounts.grokCatalog.refresh') }}
      </button>
    </div>
    <p class="text-xs text-gray-500">{{ t('admin.accounts.grokCatalog.hint') }}</p>
    <p v-if="failed" role="alert" class="text-sm text-amber-600">{{ t('admin.accounts.grokCatalog.failed') }}</p>
    <template v-if="diagnostics">
      <p class="text-sm" data-testid="catalog-state">{{ stateLabels[diagnostics.state] }}</p>
      <p v-if="diagnostics.fetched_at" class="break-all text-xs text-gray-500">{{ t('admin.accounts.grokCatalog.fetched') }}: {{ diagnostics.fetched_at }}</p>
      <details>
        <summary class="cursor-pointer text-sm">{{ t('admin.accounts.grokCatalog.models') }} ({{ diagnostics.models.length }})</summary>
        <ul class="max-h-40 overflow-auto text-xs">
          <li v-for="model in diagnostics.models" :key="model" class="break-all py-1">{{ model }}</li>
        </ul>
      </details>
      <div v-if="diagnostics.mappings.length" class="max-h-60 overflow-auto">
        <table class="w-full text-left text-xs">
          <thead><tr><th class="p-1">{{ t('admin.accounts.grokCatalog.mapping') }}</th><th class="p-1">{{ t('common.status') }}</th></tr></thead>
          <tbody><tr v-for="mapping in diagnostics.mappings" :key="mapping.requested">
            <td class="break-all p-1">{{ mapping.requested }} → {{ mapping.upstream }}</td>
            <td class="p-1">{{ statusLabels[mapping.status] }}</td>
          </tr></tbody>
        </table>
      </div>
    </template>
    <p v-else-if="!loading" class="text-xs text-gray-500">{{ t('admin.accounts.grokCatalog.unavailable') }}</p>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getById } from '@/features/admin-accounts/data/datasources/adminAccountQueries'
import { syncUpstreamModels } from '@/features/admin-accounts/data/datasources/adminAccountActions'
import type { GrokModelDiagnostics } from '@/features/admin-accounts/data/dtos/grokModelDiagnostics'
const props = defineProps<{ accountId: number }>()
const { t } = useI18n()
const stateLabels = computed(() => ({
  fresh: t('admin.accounts.grokCatalog.state.fresh'),
  stale: t('admin.accounts.grokCatalog.state.stale'),
  source_changed: t('admin.accounts.grokCatalog.state.source_changed'),
  unknown: t('admin.accounts.grokCatalog.state.unknown'),
}))
const statusLabels = computed(() => ({
  listed: t('admin.accounts.grokCatalog.status.listed'),
  not_listed: t('admin.accounts.grokCatalog.status.not_listed'),
  unverifiable: t('admin.accounts.grokCatalog.status.unverifiable'),
  out_of_scope: t('admin.accounts.grokCatalog.status.out_of_scope'),
}))
const diagnostics = ref<GrokModelDiagnostics>()
const loading = ref(false)
const failed = ref(false)
let generation = 0
async function load(refresh = false) {
  const current = ++generation
  loading.value = true
  failed.value = false
  try {
    const result = refresh ? await syncUpstreamModels(props.accountId) : await getById(props.accountId)
    if (current === generation) diagnostics.value = result.grok_model_diagnostics
  } catch {
    if (current === generation) failed.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}
watch(() => props.accountId, () => { diagnostics.value = undefined; void load() }, { immediate: true })
onBeforeUnmount(() => { generation += 1 })
</script>
