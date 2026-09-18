<template>
  <AppLayout>
    <main class="mx-auto max-w-[1600px] space-y-5 pb-12">
      <header class="flex flex-wrap items-start justify-between gap-4 border-b border-gray-200 pb-4 dark:border-dark-700">
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <Icon name="chartBar" size="lg" class="text-primary-600 dark:text-primary-400" />
            <h1 class="text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('admin.stateDiagnostics.title') }}
            </h1>
            <span
              class="rounded-md px-2 py-0.5 text-xs font-medium"
              :class="snapshot?.enabled ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'"
            >
              {{ snapshot?.enabled ? t('admin.stateDiagnostics.enabled') : t('admin.stateDiagnostics.disabled') }}
            </span>
          </div>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.stateDiagnostics.description') }}
          </p>
          <p class="mt-2 text-xs text-gray-400 dark:text-gray-500">
            {{ t('admin.stateDiagnostics.scope') }} · {{ t('admin.stateDiagnostics.updatedAt') }} {{ formatTimestamp(snapshot?.generated_at) }}
            <span v-if="snapshot"> · {{ t('admin.stateDiagnostics.targetLength', { value: snapshot.target_length }) }}</span>
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <label class="inline-flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
            <Toggle :model-value="autoRefresh" @update:model-value="autoRefresh = $event" />
            <span>{{ t('admin.stateDiagnostics.autoRefresh') }}</span>
          </label>
          <button
            type="button"
            class="btn btn-secondary inline-flex items-center gap-2"
            :disabled="loading"
            :title="t('admin.stateDiagnostics.refresh')"
            data-testid="state-diagnostics-refresh"
            @click="loadData"
          >
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
            <span>{{ t('admin.stateDiagnostics.refresh') }}</span>
          </button>
        </div>
      </header>

      <div
        v-if="errorMessage"
        class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300"
        data-testid="state-diagnostics-error"
      >
        {{ errorMessage }}
      </div>

      <section aria-label="State diagnostics summary" class="grid grid-cols-2 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800 sm:grid-cols-3 xl:grid-cols-6">
        <div v-for="item in summaryItems" :key="item.key" class="border-b border-r border-gray-200 px-4 py-3 last:border-r-0 dark:border-dark-700 xl:border-b-0">
          <dt class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</dt>
          <dd class="mt-1 text-lg font-semibold tabular-nums text-gray-900 dark:text-white">{{ item.value }}</dd>
        </div>
      </section>

      <section class="border-y border-gray-200 py-4 dark:border-dark-700" aria-labelledby="state-pool-health-title">
        <div class="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 id="state-pool-health-title" class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.stateDiagnostics.poolHealth') }}
            </h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.stateDiagnostics.poolHealthHint') }}</p>
          </div>
          <span class="text-xs tabular-nums text-gray-500 dark:text-gray-400">
            {{ t('admin.stateDiagnostics.coverage') }} {{ coveragePercent }}%
          </span>
        </div>
        <div class="mt-3 flex h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700" data-testid="state-diagnostics-health-bar">
          <span v-for="segment in healthSegments" :key="segment.key" :class="segment.className" :style="{ width: `${segment.width}%` }" />
        </div>
        <div class="mt-3 flex flex-wrap gap-x-5 gap-y-2 text-xs text-gray-600 dark:text-gray-300">
          <span v-for="segment in healthSegments" :key="`${segment.key}-legend`" class="inline-flex items-center gap-1.5">
            <span class="h-2 w-2 rounded-full" :class="segment.dotClass" />
            {{ segment.label }} <strong class="font-semibold tabular-nums">{{ segment.count }}</strong>
          </span>
        </div>
      </section>

      <section class="space-y-3" aria-labelledby="state-pool-table-title">
        <div class="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 id="state-pool-table-title" class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.stateDiagnostics.accountPool') }}
            </h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.stateDiagnostics.accountPoolHint', { shown: filteredRows.length, total: rows.length }) }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <div class="relative">
              <Icon name="search" size="sm" class="pointer-events-none absolute left-2.5 top-2.5 text-gray-400" />
              <input
                v-model="search"
                class="input w-56 pl-8"
                :placeholder="t('admin.stateDiagnostics.searchPlaceholder')"
                data-testid="state-diagnostics-search"
              />
            </div>
            <select v-model="healthFilter" class="input w-36" data-testid="state-diagnostics-health-filter">
              <option value="all">{{ t('admin.stateDiagnostics.filters.all') }}</option>
              <option value="ready">{{ t('admin.stateDiagnostics.filters.ready') }}</option>
              <option value="attention">{{ t('admin.stateDiagnostics.filters.attention') }}</option>
              <option value="unobserved">{{ t('admin.stateDiagnostics.filters.unobserved') }}</option>
              <option value="blocked">{{ t('admin.stateDiagnostics.filters.blocked') }}</option>
            </select>
            <select v-model="modelFilter" class="input w-44" data-testid="state-diagnostics-model-filter">
              <option value="">{{ t('admin.stateDiagnostics.filters.allModels') }}</option>
              <option v-for="model in models" :key="model" :value="model">{{ model }}</option>
            </select>
          </div>
        </div>

        <div class="overflow-x-auto rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <table class="min-w-[1100px] w-full table-fixed text-left text-sm" data-testid="state-diagnostics-account-table">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/60 dark:text-gray-400">
              <tr>
                <th class="w-64 px-4 py-3 font-medium">{{ t('admin.stateDiagnostics.columns.account') }}</th>
                <th class="w-32 px-4 py-3 font-medium">{{ t('admin.stateDiagnostics.columns.poolStatus') }}</th>
                <th class="w-56 px-4 py-3 font-medium">{{ t('admin.stateDiagnostics.columns.models') }}</th>
                <th class="w-48 px-4 py-3 font-medium">{{ t('admin.stateDiagnostics.columns.state') }}</th>
                <th class="w-48 px-4 py-3 font-medium">{{ t('admin.stateDiagnostics.columns.response') }}</th>
                <th class="w-48 px-4 py-3 font-medium">{{ t('admin.stateDiagnostics.columns.probe') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="row in filteredRows" :key="row.account.id" class="align-top">
                <td class="px-4 py-3">
                  <p class="truncate font-medium text-gray-900 dark:text-white" :title="row.account.name">{{ row.account.name }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">#{{ row.account.id }} · {{ row.account.platform }} · {{ row.account.type }}</p>
                  <p v-if="row.account.scheduling_disabled_reason || row.account.temp_unschedulable_reason" class="mt-1 truncate text-xs text-amber-600 dark:text-amber-300" :title="row.account.scheduling_disabled_reason || row.account.temp_unschedulable_reason || ''">
                    {{ row.account.scheduling_disabled_reason || row.account.temp_unschedulable_reason }}
                  </p>
                </td>
                <td class="px-4 py-3">
                  <span class="rounded-md px-2 py-1 text-xs font-medium" :class="healthClass(row.health)">{{ healthLabel(row.health) }}</span>
                  <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                    {{ row.account.schedulable ? t('admin.stateDiagnostics.schedulable') : t('admin.stateDiagnostics.notSchedulable') }}
                  </p>
                  <p v-if="row.account.current_concurrency != null" class="mt-1 text-xs tabular-nums text-gray-400 dark:text-gray-500">
                    {{ row.account.current_concurrency }} / {{ row.account.concurrency }} {{ t('admin.stateDiagnostics.inFlight') }}
                  </p>
                </td>
                <td class="px-4 py-3">
                  <div v-if="row.observations.length" class="flex flex-wrap gap-1">
                    <span v-for="item in row.observations" :key="`${row.account.id}:${item.model}`" class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200">
                      {{ item.model }}
                    </span>
                  </div>
                  <span v-else class="text-xs text-gray-400">{{ t('admin.stateDiagnostics.noObservation') }}</span>
                  <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                    {{ row.healthyObservations }} / {{ row.observations.length }} {{ t('admin.stateDiagnostics.healthyStates') }}
                  </p>
                </td>
                <td class="px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                  <template v-if="row.observations.length">
                    <div v-for="item in row.observations" :key="`${row.account.id}:${item.model}:state`" class="mb-2 last:mb-0">
                      <div class="flex items-center gap-1.5">
                        <span class="h-2 w-2 rounded-full" :class="isHealthy(item) ? 'bg-emerald-500' : 'bg-amber-500'" />
                        <span>{{ item.model }}</span>
                      </div>
                      <p class="mt-1 text-gray-500 dark:text-gray-400">
                        {{ observationLabel(item) }} · {{ item.state.token_characters }} {{ t('admin.stateDiagnostics.characters') }}
                      </p>
                      <p v-if="item.state_digest" class="mt-1 font-mono text-[11px] text-gray-400 dark:text-gray-500">{{ item.state_digest }}</p>
                    </div>
                  </template>
                  <span v-else class="text-gray-400">-</span>
                </td>
                <td class="px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                  <template v-if="row.observations.length">
                    <div v-for="item in row.observations" :key="`${row.account.id}:${item.model}:response`" class="mb-2 last:mb-0">
                      <p>{{ formatTimestamp(item.last_response_at) }}</p>
                      <p class="mt-1 text-gray-500 dark:text-gray-400">
                        {{ item.last_response_had_state ? t('admin.stateDiagnostics.responseHasState') : t('admin.stateDiagnostics.responseMissingState') }}
                      </p>
                      <p class="mt-1 text-gray-500 dark:text-gray-400">
                        {{ cipherLabel(item.encrypted_content.classification) }} · {{ t('admin.stateDiagnostics.rotations', { count: item.rotation.count }) }}
                      </p>
                    </div>
                  </template>
                  <span v-else class="text-gray-400">-</span>
                </td>
                <td class="px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                  <template v-if="row.observations.length">
                    <div v-for="item in row.observations" :key="`${row.account.id}:${item.model}:probe`" class="mb-2 last:mb-0">
                      <p>{{ probeLabel(item) }}</p>
                      <p class="mt-1 text-gray-500 dark:text-gray-400">{{ formatTimestamp(item.probe.next_probe_at) }}</p>
                      <p v-if="item.proxy_enabled" class="mt-1 text-gray-500 dark:text-gray-400">{{ t('admin.stateDiagnostics.proxyEnabled') }}</p>
                    </div>
                  </template>
                  <span v-else class="text-gray-400">-</span>
                </td>
              </tr>
              <tr v-if="!loading && filteredRows.length === 0">
                <td colspan="6" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.stateDiagnostics.empty') }}
                </td>
              </tr>
              <tr v-if="loading">
                <td colspan="6" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-gray-400">
                  <span class="inline-flex items-center gap-2"><Icon name="refresh" size="sm" class="animate-spin" /> {{ t('common.loading') }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </main>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import type { CodexTurnStateObservation, CodexTurnStateObservability } from '@/features/admin-settings/data/dtos/adminSettingsDtos'
import type { StateDiagnosticsFilter } from '../../data/dtos/stateDiagnosticsDtos'
import { getStateDiagnostics, listStatePoolAccounts } from '../../data/datasources/stateDiagnosticsQueries'
import { buildAccountRows, filterAccountRows, isHealthyObservation, probePhase } from '../composables/stateDiagnosticsTransforms'

const { t } = useI18n()
const snapshot = ref<CodexTurnStateObservability | null>(null)
const accounts = ref<Awaited<ReturnType<typeof listStatePoolAccounts>>>([])
const loading = ref(true)
const errorMessage = ref('')
const search = ref('')
const healthFilter = ref<StateDiagnosticsFilter>('all')
const modelFilter = ref('')
const autoRefresh = ref(false)
let refreshTimer: ReturnType<typeof setInterval> | null = null
let requestController: AbortController | null = null

const rows = computed(() => buildAccountRows(accounts.value, snapshot.value?.items ?? []))
const filteredRows = computed(() => filterAccountRows(rows.value, search.value, healthFilter.value, modelFilter.value))
const models = computed(() => [...new Set((snapshot.value?.items ?? []).map((item) => item.model))].sort())
const coveragePercent = computed(() => rows.value.length ? Math.round((rows.value.filter((row) => row.observations.length > 0).length / rows.value.length) * 100) : 0)
const countFor = (health: StateDiagnosticsFilter) => rows.value.filter((row) => row.health === health).length
const summaryItems = computed(() => [
  { key: 'pool', label: t('admin.stateDiagnostics.summary.pool'), value: rows.value.length },
  { key: 'schedulable', label: t('admin.stateDiagnostics.summary.schedulable'), value: rows.value.filter((row) => row.account.status === 'active' && row.account.schedulable).length },
  { key: 'ready', label: t('admin.stateDiagnostics.summary.ready'), value: countFor('ready') },
  { key: 'attention', label: t('admin.stateDiagnostics.summary.attention'), value: countFor('attention') },
  { key: 'unobserved', label: t('admin.stateDiagnostics.summary.unobserved'), value: countFor('unobserved') },
  { key: 'rotations', label: t('admin.stateDiagnostics.summary.rotations'), value: (snapshot.value?.items ?? []).reduce((total, item) => total + item.rotation.count, 0) },
])
const healthSegments = computed(() => {
  const total = Math.max(rows.value.length, 1)
  return [
    { key: 'ready', count: countFor('ready'), label: t('admin.stateDiagnostics.filters.ready'), width: (countFor('ready') / total) * 100, className: 'bg-emerald-500', dotClass: 'bg-emerald-500' },
    { key: 'attention', count: countFor('attention'), label: t('admin.stateDiagnostics.filters.attention'), width: (countFor('attention') / total) * 100, className: 'bg-amber-500', dotClass: 'bg-amber-500' },
    { key: 'unobserved', count: countFor('unobserved'), label: t('admin.stateDiagnostics.filters.unobserved'), width: (countFor('unobserved') / total) * 100, className: 'bg-gray-400', dotClass: 'bg-gray-400' },
    { key: 'blocked', count: countFor('blocked'), label: t('admin.stateDiagnostics.filters.blocked'), width: (countFor('blocked') / total) * 100, className: 'bg-red-500', dotClass: 'bg-red-500' },
  ]
})

function formatTimestamp(value?: string | null): string {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString()
}

function healthLabel(health: Exclude<StateDiagnosticsFilter, 'all'>): string {
  switch (health) {
    case 'ready': return t('admin.stateDiagnostics.filters.ready')
    case 'attention': return t('admin.stateDiagnostics.filters.attention')
    case 'unobserved': return t('admin.stateDiagnostics.filters.unobserved')
    case 'blocked': return t('admin.stateDiagnostics.filters.blocked')
  }
}

function healthClass(health: Exclude<StateDiagnosticsFilter, 'all'>): string {
  if (health === 'ready') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
  if (health === 'attention') return 'bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300'
  if (health === 'blocked') return 'bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}

function isHealthy(item: CodexTurnStateObservation): boolean {
  return isHealthyObservation(item)
}

function probeLabel(item: CodexTurnStateObservation): string {
  switch (probePhase(item)) {
    case 'running': return t('admin.stateDiagnostics.probeRunning')
    case 'queued': return t('admin.stateDiagnostics.probeQueued')
    case 'scheduled': return t('admin.stateDiagnostics.probeScheduled')
    case 'unscheduled': return t('admin.stateDiagnostics.probeUnscheduled')
  }
}

function observationLabel(item: CodexTurnStateObservation): string {
  if (item.state.expired) return t('admin.stateDiagnostics.observation.expired')
  if (!item.state.valid) return t('admin.stateDiagnostics.observation.invalid')
  if (!item.length_match) return t('admin.stateDiagnostics.observation.lengthMismatch')
  if (item.rotation.count > 0) return t('admin.stateDiagnostics.observation.rotation')
  if (item.last_response_at && !item.last_response_had_state) return t('admin.stateDiagnostics.observation.missing')
  return t('admin.stateDiagnostics.observation.valid')
}

function cipherLabel(classification: string): string {
  switch (classification) {
    case 'baseline': return t('admin.stateDiagnostics.cipher.baseline')
    case 'plus_16_hint': return t('admin.stateDiagnostics.cipher.plus16')
    case 'other': return t('admin.stateDiagnostics.cipher.other')
    default: return t('admin.stateDiagnostics.cipher.unknown')
  }
}

async function loadData(): Promise<void> {
  requestController?.abort()
  requestController = new AbortController()
  loading.value = true
  errorMessage.value = ''
  try {
    const [nextSnapshot, nextAccounts] = await Promise.all([
      getStateDiagnostics(),
      listStatePoolAccounts(requestController.signal),
    ])
    snapshot.value = nextSnapshot
    accounts.value = nextAccounts
  } catch (error: unknown) {
    if ((error as { code?: string })?.code === 'ERR_CANCELED') return
    errorMessage.value = extractApiErrorMessage(error, t('admin.stateDiagnostics.loadFailed'))
  } finally {
    loading.value = false
  }
}

function updateRefreshTimer(): void {
  if (refreshTimer) clearInterval(refreshTimer)
  refreshTimer = autoRefresh.value ? setInterval(() => void loadData(), 15000) : null
}

watch(autoRefresh, updateRefreshTimer)
onMounted(loadData)
onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer)
  requestController?.abort()
})
</script>
