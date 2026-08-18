<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 pb-4 dark:border-dark-700">
        <div class="min-w-0 text-sm text-gray-500 dark:text-gray-400">
          <span>{{ t('admin.cluster.lastUpdated') }}</span>
          <span class="ml-2 font-medium text-gray-800 dark:text-gray-200">
            {{ status ? formatDateTime(status.observed_at) : '-' }}
          </span>
        </div>
        <div class="flex items-center gap-3">
          <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
            <span>{{ t('admin.cluster.autoRefresh') }}</span>
            <Toggle v-model="autoRefresh" />
          </label>
          <button
            type="button"
            class="inline-flex h-9 w-9 items-center justify-center rounded-lg border border-gray-300 bg-white text-gray-600 transition hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700"
            :title="t('admin.cluster.refresh')"
            :disabled="loading"
            @click="fetchStatus"
          >
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          </button>
        </div>
      </div>

      <div
        v-if="errorMessage"
        class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300"
      >
        {{ errorMessage }}
      </div>

      <ClusterLoadSummary :instances="instances" :summary="status?.summary" />

      <ClusterNodeLoadGrid
        :instances="instances"
        :busy="actionBusy"
        :loading="loading"
        @rename="saveNodeName"
      />

      <ClusterReleasePanel
        :overview="status?.release"
        :deployment="status?.deployment"
        :busy="actionBusy"
        @create="handleCreateRollout"
        @pause="handlePauseRollout"
        @resume="handleResumeRollout"
        @cancel="handleCancelRollout"
        @confirm="handleConfirmRollout"
        @retry="handleRetryTarget"
      />

      <section aria-labelledby="cluster-tasks-title">
        <div class="mb-3 flex items-center justify-between">
          <h2 id="cluster-tasks-title" class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.cluster.tasks.title') }}
          </h2>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ tasks.length }}</span>
        </div>
        <div class="overflow-x-auto rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <table class="min-w-[900px] w-full table-fixed text-left text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/60 dark:text-gray-400">
              <tr>
                <th class="w-64 px-4 py-3 font-medium">{{ t('admin.cluster.tasks.task') }}</th>
                <th class="w-32 px-4 py-3 font-medium">{{ t('admin.cluster.tasks.status') }}</th>
                <th class="w-48 px-4 py-3 font-medium">{{ t('admin.cluster.tasks.node') }}</th>
                <th class="w-44 px-4 py-3 font-medium">{{ t('admin.cluster.tasks.startedAt') }}</th>
                <th class="w-36 px-4 py-3 font-medium">{{ t('admin.cluster.tasks.duration') }}</th>
                <th class="px-4 py-3 font-medium">{{ t('admin.cluster.tasks.error') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="task in tasks" :key="task.run_id" class="align-top">
                <td class="px-4 py-3">
                  <p class="truncate font-mono text-xs font-medium text-gray-900 dark:text-white" :title="task.task_key">{{ task.task_key }}</p>
                  <p class="mt-1 truncate font-mono text-[11px] text-gray-400" :title="task.run_id">{{ task.run_id }}</p>
                </td>
                <td class="px-4 py-3">
                  <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="taskStatusClass(task.status)">
                    {{ taskStatusLabel(task.status) }}
                  </span>
                </td>
                <td class="px-4 py-3">
                  <p class="truncate text-gray-800 dark:text-gray-200" :title="task.node_name">{{ task.node_name }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ formatRelativeTime(task.heartbeat_at) }}</p>
                </td>
                <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ formatDateTime(task.started_at) }}</td>
                <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ taskDuration(task) }}</td>
                <td class="px-4 py-3">
                  <p class="line-clamp-2 break-words text-xs text-red-600 dark:text-red-400" :title="task.error_message">{{ task.error_message || '-' }}</p>
                </td>
              </tr>
              <tr v-if="!loading && tasks.length === 0">
                <td colspan="6" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.cluster.tasks.empty') }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section v-if="status" aria-labelledby="deployment-config-title">
        <h2 id="deployment-config-title" class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.cluster.deployment.title') }}
        </h2>
        <dl class="grid grid-cols-2 overflow-hidden rounded-lg border border-gray-200 bg-white sm:grid-cols-4 dark:border-dark-700 dark:bg-dark-800">
          <div v-for="item in deploymentItems" :key="item.label" class="min-w-0 border-b border-r border-gray-200 p-4 last:border-r-0 dark:border-dark-700">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</dt>
            <dd class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white" :title="item.value">{{ item.value }}</dd>
          </div>
        </dl>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import ClusterLoadSummary from '@/features/admin-cluster/presentation/widgets/ClusterLoadSummary.vue'
import ClusterNodeLoadGrid from '@/features/admin-cluster/presentation/widgets/ClusterNodeLoadGrid.vue'
import ClusterReleasePanel from '@/features/admin-cluster/presentation/widgets/ClusterReleasePanel.vue'
import clusterAPI, {
  type ClusterStatusResponse,
  type ClusterTaskRun,
  type ClusterTaskStatus,
} from '@/features/admin-cluster/data/datasources/adminClusterDatasource'
import { clusterTaskStatusLabel } from '@/features/admin-cluster/presentation/clusterLocale'
import { formatDateTime, formatRelativeTime } from '@/core/utils/format'
import { extractApiErrorMessage } from '@/core/utils/apiError'

const { t } = useI18n()
const status = ref<ClusterStatusResponse | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const autoRefresh = ref(true)
const actionBusy = ref(false)
let refreshTimer: ReturnType<typeof setInterval> | null = null

const instances = computed(() => status.value?.instances ?? [])
const tasks = computed(() => status.value?.tasks ?? [])
const deploymentItems = computed(() => {
  const deployment = status.value?.deployment
  if (!deployment) return []
  return [
    { label: t('admin.cluster.deployment.mode'), value: deployment.mode === 'multi_instance' ? t('admin.cluster.deployment.multiInstance') : t('admin.cluster.deployment.standalone') },
    { label: t('admin.cluster.deployment.nodeName'), value: deployment.node_name },
    { label: t('admin.cluster.deployment.workerMode'), value: workerModeLabel(deployment.worker_mode) },
    { label: t('admin.cluster.deployment.workerResolved'), value: deployment.worker_enabled ? t('admin.cluster.deployment.enabled') : t('admin.cluster.deployment.disabled') },
    { label: t('admin.cluster.deployment.frontend'), value: deployment.frontend_enabled ? t('admin.cluster.deployment.enabled') : t('admin.cluster.deployment.disabled') },
    { label: t('admin.cluster.deployment.heartbeat'), value: t('admin.cluster.deployment.seconds', { value: deployment.heartbeat_interval_seconds }) },
    { label: t('admin.cluster.deployment.staleAfter'), value: t('admin.cluster.deployment.seconds', { value: deployment.stale_after_seconds }) },
    { label: t('admin.cluster.deployment.lease'), value: t('admin.cluster.deployment.seconds', { value: deployment.task_lease_seconds }) },
  ]
})

function workerModeLabel(mode: string): string {
  if (mode === 'true') return t('admin.cluster.deployment.explicitTrue')
  if (mode === 'false') return t('admin.cluster.deployment.explicitFalse')
  return t('admin.cluster.deployment.auto')
}

function taskStatusLabel(value: ClusterTaskStatus): string {
  return clusterTaskStatusLabel(t, value)
}

function taskStatusClass(value: ClusterTaskStatus): string {
  if (value === 'running') return 'bg-blue-50 text-blue-700 dark:bg-blue-950/50 dark:text-blue-300'
  if (value === 'succeeded') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300'
  if (value === 'lost') return 'bg-amber-50 text-amber-700 dark:bg-amber-950/50 dark:text-amber-300'
  return 'bg-red-50 text-red-700 dark:bg-red-950/50 dark:text-red-300'
}

function taskDuration(task: ClusterTaskRun): string {
  const start = new Date(task.started_at).getTime()
  const end = task.finished_at ? new Date(task.finished_at).getTime() : Date.now()
  const seconds = Math.max(0, Math.floor((end - start) / 1000))
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remainder = seconds % 60
  return `${minutes}m ${remainder}s`
}

async function fetchStatus(): Promise<void> {
  if (loading.value) return
  loading.value = true
  errorMessage.value = ''
  try {
    status.value = await clusterAPI.getStatus()
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error, t('admin.cluster.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function saveNodeName(nodeId: string, name: string): Promise<void> {
  if (!name || actionBusy.value) return
  actionBusy.value = true
  errorMessage.value = ''
  try {
    await clusterAPI.renameNode(nodeId, name)
    await fetchStatus()
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error, t('admin.cluster.nodes.renameFailed'))
  } finally {
    actionBusy.value = false
  }
}

async function runRolloutAction(action: () => Promise<unknown>): Promise<void> {
  if (actionBusy.value) return
  actionBusy.value = true
  errorMessage.value = ''
  try {
    await action()
    await fetchStatus()
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error, t('admin.cluster.release.actionFailed'))
  } finally {
    actionBusy.value = false
  }
}

function handleCreateRollout(targetVersion: string): void {
  const target = targetVersion.trim()
  const label = target ? `v${target.replace(/^v/, '')}` : t('admin.cluster.release.latest')
  if (!window.confirm(t('admin.cluster.release.confirmStart', { version: label }))) return
  void runRolloutAction(() => clusterAPI.createRollout(target))
}

function handlePauseRollout(rolloutId: string): void {
  void runRolloutAction(() => clusterAPI.pauseRollout(rolloutId))
}

function handleResumeRollout(rolloutId: string): void {
  void runRolloutAction(() => clusterAPI.resumeRollout(rolloutId))
}

function handleCancelRollout(rolloutId: string): void {
  if (!window.confirm(t('admin.cluster.release.confirmCancel'))) return
  void runRolloutAction(() => clusterAPI.cancelRollout(rolloutId))
}

function handleConfirmRollout(rolloutId: string): void {
  if (!window.confirm(t('admin.cluster.release.confirmLock'))) return
  void runRolloutAction(() => clusterAPI.confirmRollout(rolloutId))
}

function handleRetryTarget(rolloutId: string, nodeId: string): void {
  void runRolloutAction(() => clusterAPI.retryRolloutTarget(rolloutId, nodeId))
}

function startTimer(): void {
  if (refreshTimer) clearInterval(refreshTimer)
  refreshTimer = setInterval(() => {
    if (autoRefresh.value) void fetchStatus()
  }, 15_000)
}

onMounted(() => {
  void fetchStatus()
  startTimer()
})

onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>
