<template>
  <section aria-labelledby="cluster-nodes-title">
    <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-2">
        <h2 id="cluster-nodes-title" class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.cluster.nodes.title') }}
        </h2>
        <span class="text-xs tabular-nums text-gray-400 dark:text-gray-500">{{ instances.length }}</span>
      </div>
      <label class="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
        <span>{{ t('admin.cluster.nodes.sort.label') }}</span>
        <select
          v-model="sortMode"
          class="h-9 rounded-md border border-gray-300 bg-white px-3 text-sm text-gray-700 outline-none focus:border-primary-500 focus:ring-1 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200"
          :aria-label="t('admin.cluster.nodes.sort.label')"
          data-test="node-sort"
        >
          <option value="load">{{ t('admin.cluster.nodes.sort.load') }}</option>
          <option value="cpu">{{ t('admin.cluster.nodes.sort.cpu') }}</option>
          <option value="memory">{{ t('admin.cluster.nodes.sort.memory') }}</option>
          <option value="requests">{{ t('admin.cluster.nodes.sort.requests') }}</option>
          <option value="name">{{ t('admin.cluster.nodes.sort.name') }}</option>
        </select>
      </label>
    </div>

    <div v-if="sortedInstances.length" class="grid gap-4 xl:grid-cols-2">
      <article
        v-for="instance in sortedInstances"
        :key="instance.node_id"
        class="min-w-0 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
        data-test="cluster-node-card"
        :data-node-id="instance.node_id"
      >
        <header class="flex min-h-[92px] items-start justify-between gap-3 px-4 py-4">
          <div class="flex min-w-0 items-start gap-2.5">
            <span class="mt-1.5 h-2.5 w-2.5 flex-none rounded-full" :class="instanceDotClass(instance.status)" />
            <div class="min-w-0">
              <div v-if="editingNodeId === instance.node_id" class="flex items-center gap-1.5">
                <input
                  v-model.trim="editingNodeName"
                  type="text"
                  maxlength="128"
                  class="h-8 min-w-0 flex-1 rounded-md border border-gray-300 bg-white px-2 text-sm text-gray-900 outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
                  @keyup.enter="requestRename(instance.node_id)"
                  @keyup.esc="cancelNodeRename"
                />
                <button
                  type="button"
                  class="inline-flex h-8 w-8 flex-none items-center justify-center rounded-md text-emerald-600 hover:bg-emerald-50 disabled:opacity-50 dark:text-emerald-400 dark:hover:bg-emerald-950/30"
                  :title="t('admin.cluster.nodes.saveName')"
                  :disabled="busy || !editingNodeName"
                  @click="requestRename(instance.node_id)"
                >
                  <Icon name="check" size="sm" />
                </button>
                <button
                  type="button"
                  class="inline-flex h-8 w-8 flex-none items-center justify-center rounded-md text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-700"
                  :title="t('admin.cluster.nodes.cancelRename')"
                  @click="cancelNodeRename"
                >
                  <Icon name="x" size="sm" />
                </button>
              </div>
              <div v-else class="flex min-w-0 flex-wrap items-center gap-2">
                <h3 class="max-w-full truncate text-sm font-semibold text-gray-950 dark:text-white" :title="instance.node_name">
                  {{ instance.node_name }}
                </h3>
                <span
                  class="inline-flex rounded-md px-1.5 py-0.5 text-[11px] font-medium"
                  :class="instanceStatusClass(instance.status)"
                >
                  {{ statusLabel(instance.status) }}
                </span>
                <span v-if="instance.current" class="rounded bg-primary-50 px-1.5 py-0.5 text-[11px] font-medium text-primary-700 dark:bg-primary-950/50 dark:text-primary-300">
                  {{ t('admin.cluster.nodes.current') }}
                </span>
              </div>
              <p class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400" :title="`${instance.hostname} · PID ${instance.process_id}`">
                {{ instance.hostname }} · PID {{ instance.process_id }}
              </p>
              <p class="mt-0.5 truncate font-mono text-[10px] text-gray-400 dark:text-gray-500" :title="instance.node_id">
                {{ instance.node_id }}
              </p>
            </div>
          </div>
          <div class="flex flex-none items-center gap-1.5">
            <span class="rounded-md bg-gray-100 px-2 py-1 font-mono text-[11px] text-gray-600 dark:bg-dark-700 dark:text-gray-300">
              v{{ instance.version || '-' }}
            </span>
            <button
              v-if="editingNodeId !== instance.node_id"
              type="button"
              class="inline-flex h-8 w-8 items-center justify-center rounded-md text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200"
              :title="t('admin.cluster.nodes.rename')"
              @click="beginNodeRename(instance.node_id, instance.node_name)"
            >
              <Icon name="edit" size="xs" />
            </button>
          </div>
        </header>

        <template v-if="instance.load">
          <div class="border-t border-gray-100 px-4 py-4 dark:border-dark-700">
            <div class="grid gap-4 sm:grid-cols-2">
              <div class="min-w-0">
                <div class="mb-2 flex items-baseline justify-between gap-2">
                  <span class="text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.cluster.nodes.cpu') }}</span>
                  <span class="text-sm font-semibold tabular-nums text-gray-950 dark:text-white" data-test="cpu-value">
                    {{ formatPercent(instance.load.cpu_usage_percent) }}
                  </span>
                </div>
                <div class="h-2 overflow-hidden rounded bg-gray-100 dark:bg-dark-700" role="progressbar" :aria-valuenow="instance.load.cpu_usage_percent" aria-valuemin="0" aria-valuemax="100">
                  <div
                    class="h-full rounded transition-[width] duration-300"
                    :class="percentBarClass(instance.load.cpu_usage_percent, 'cpu')"
                    :style="percentBarStyle(instance.load.cpu_usage_percent)"
                  />
                </div>
              </div>
              <div class="min-w-0">
                <div class="mb-2 flex items-baseline justify-between gap-2">
                  <span class="text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.cluster.nodes.memory') }}</span>
                  <span class="text-sm font-semibold tabular-nums text-gray-950 dark:text-white" data-test="memory-value">
                    {{ formatPercent(memoryPercent(instance.load)) }}
                  </span>
                </div>
                <div class="h-2 overflow-hidden rounded bg-gray-100 dark:bg-dark-700" role="progressbar" :aria-valuenow="memoryPercent(instance.load)" aria-valuemin="0" aria-valuemax="100">
                  <div
                    class="h-full rounded transition-[width] duration-300"
                    :class="percentBarClass(memoryPercent(instance.load), 'memory')"
                    :style="percentBarStyle(memoryPercent(instance.load))"
                  />
                </div>
                <p class="mt-1.5 truncate text-[11px] tabular-nums text-gray-400 dark:text-gray-500" :title="formatMemory(instance.load)">
                  {{ formatMemory(instance.load) }}
                </p>
              </div>
            </div>
          </div>

          <dl class="grid grid-cols-2 border-t border-gray-100 sm:grid-cols-4 dark:border-dark-700">
            <div class="min-w-0 border-b border-r border-gray-100 px-4 py-3 sm:border-b-0 dark:border-dark-700">
              <dt class="truncate text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.cluster.nodes.requests') }}</dt>
              <dd class="mt-1 text-base font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatCount(instance.load.in_flight_requests) }}</dd>
            </div>
            <div class="min-w-0 border-b border-gray-100 px-4 py-3 sm:border-b-0 sm:border-r dark:border-dark-700">
              <dt class="truncate text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.cluster.nodes.activeTasks') }}</dt>
              <dd class="mt-1 text-base font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatCount(instance.load.active_tasks) }}</dd>
            </div>
            <div class="min-w-0 border-r border-gray-100 px-4 py-3 dark:border-dark-700">
              <dt class="truncate text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.cluster.nodes.goroutines') }}</dt>
              <dd class="mt-1 text-base font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatCount(instance.load.goroutine_count) }}</dd>
            </div>
            <div class="min-w-0 px-4 py-3">
              <dt class="truncate text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.cluster.nodes.uptime') }}</dt>
              <dd class="mt-1 truncate text-base font-semibold tabular-nums text-gray-900 dark:text-white" :title="formatDateTime(instance.started_at)">
                {{ formatUptime(instance.started_at) }}
              </dd>
            </div>
          </dl>

          <div class="grid border-t border-gray-100 sm:grid-cols-2 dark:border-dark-700">
            <div class="flex min-w-0 items-center justify-between gap-3 border-b border-gray-100 px-4 py-3 sm:border-b-0 sm:border-r dark:border-dark-700">
              <div class="flex min-w-0 items-center gap-2 text-xs">
                <Icon :name="instance.database_ok ? 'checkCircle' : 'xCircle'" size="xs" :class="instance.database_ok ? 'text-emerald-500' : 'text-red-500'" />
                <span class="truncate font-medium" :class="instance.database_ok ? 'text-gray-700 dark:text-gray-200' : 'text-red-600 dark:text-red-400'">
                  {{ t('admin.cluster.nodes.database') }}
                </span>
              </div>
              <div class="flex-none text-right">
                <p class="text-xs font-medium tabular-nums text-gray-700 dark:text-gray-200">
                  {{ formatPool(instance.load.db_connections_active, instance.load.db_connections_max) }}
                </p>
                <p class="text-[10px] text-gray-400">{{ t('admin.cluster.nodes.poolIdle', { count: instance.load.db_connections_idle }) }}</p>
              </div>
            </div>
            <div class="flex min-w-0 items-center justify-between gap-3 px-4 py-3">
              <div class="flex min-w-0 items-center gap-2 text-xs">
                <Icon :name="instance.redis_ok ? 'checkCircle' : 'xCircle'" size="xs" :class="instance.redis_ok ? 'text-emerald-500' : 'text-red-500'" />
                <span class="truncate font-medium" :class="instance.redis_ok ? 'text-gray-700 dark:text-gray-200' : 'text-red-600 dark:text-red-400'">
                  {{ t('admin.cluster.nodes.redis') }}
                </span>
              </div>
              <div class="flex-none text-right">
                <p class="text-xs font-medium tabular-nums text-gray-700 dark:text-gray-200">
                  {{ formatPool(instance.load.redis_connections_active, instance.load.redis_connections_max) }}
                </p>
                <p class="text-[10px] text-gray-400">{{ t('admin.cluster.nodes.poolIdle', { count: instance.load.redis_connections_idle }) }}</p>
              </div>
            </div>
          </div>
        </template>

        <div v-else class="flex min-h-[238px] items-center justify-center border-t border-gray-100 px-4 py-8 dark:border-dark-700">
          <div class="text-center text-gray-400 dark:text-gray-500">
            <Icon name="chartBar" size="lg" class="mx-auto" />
            <p class="mt-2 text-xs">{{ t('admin.cluster.nodes.noMetrics') }}</p>
          </div>
        </div>

        <footer class="flex min-h-[46px] flex-wrap items-center justify-between gap-x-3 gap-y-1 border-t border-gray-100 px-4 py-3 text-[11px] text-gray-500 dark:border-dark-700 dark:text-gray-400">
          <div class="flex items-center gap-2">
            <span>{{ t('admin.cluster.nodes.apiFrontend') }}</span>
            <span class="text-gray-300 dark:text-dark-600">/</span>
            <span :class="instance.worker_enabled ? 'text-emerald-600 dark:text-emerald-400' : ''">
              {{ instance.worker_enabled ? t('admin.cluster.nodes.worker') : workerModeLabel(instance.worker_mode) }}
            </span>
          </div>
          <div class="flex min-w-0 items-center gap-2 tabular-nums">
            <span v-if="instance.load" class="truncate" :title="formatDateTime(instance.load.collected_at)">
              {{ t('admin.cluster.nodes.sampled') }} {{ formatRelativeTime(instance.load.collected_at) }}
            </span>
            <span v-if="instance.load" class="text-gray-300 dark:text-dark-600">/</span>
            <span class="truncate" :title="formatDateTime(instance.last_seen_at)">
              {{ t('admin.cluster.nodes.heartbeat') }} {{ formatRelativeTime(instance.last_seen_at) }}
            </span>
          </div>
        </footer>
      </article>
    </div>

    <div v-else-if="loading" class="grid gap-4 xl:grid-cols-2" aria-hidden="true">
      <div v-for="index in 2" :key="index" class="h-[420px] animate-pulse rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <div class="h-4 w-40 rounded bg-gray-100 dark:bg-dark-700" />
        <div class="mt-8 grid grid-cols-2 gap-4">
          <div class="h-2 rounded bg-gray-100 dark:bg-dark-700" />
          <div class="h-2 rounded bg-gray-100 dark:bg-dark-700" />
        </div>
      </div>
    </div>

    <div v-else class="rounded-lg border border-gray-200 bg-white px-4 py-12 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
      {{ t('admin.cluster.nodes.empty') }}
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import type {
  ClusterInstance,
  ClusterInstanceLoad,
  ClusterInstanceStatus,
} from '@/features/admin-cluster/data/datasources/adminClusterDatasource'
import { clusterInstanceStatusLabel } from '@/features/admin-cluster/presentation/clusterLocale'
import { formatBytes, formatDateTime, formatRelativeTime } from '@/core/utils/format'

type SortMode = 'load' | 'cpu' | 'memory' | 'requests' | 'name'

const props = defineProps<{
  instances: ClusterInstance[]
  busy: boolean
  loading: boolean
}>()

const emit = defineEmits<{
  rename: [nodeId: string, name: string]
}>()

const { t, locale } = useI18n()
const sortMode = ref<SortMode>('load')
const editingNodeId = ref('')
const editingNodeName = ref('')

const statusRanks: Record<ClusterInstanceStatus, number> = {
  online: 0,
  stale: 1,
  stopped: 2,
}

const sortedInstances = computed(() => [...props.instances].sort((left, right) => {
  if (sortMode.value === 'name') return compareNames(left, right)

  const statusDelta = statusRanks[left.status] - statusRanks[right.status]
  if (statusDelta !== 0) return statusDelta

  let delta = 0
  if (sortMode.value === 'cpu') {
    delta = compareDescending(left.load?.cpu_usage_percent, right.load?.cpu_usage_percent)
  } else if (sortMode.value === 'memory') {
    delta = compareDescending(memoryPercent(left.load), memoryPercent(right.load))
  } else if (sortMode.value === 'requests') {
    delta = compareDescending(left.load?.in_flight_requests, right.load?.in_flight_requests)
  } else {
    delta = compareDescending(combinedLoad(left), combinedLoad(right))
    if (delta === 0) delta = compareDescending(left.load?.in_flight_requests, right.load?.in_flight_requests)
    if (delta === 0) delta = compareDescending(left.load?.active_tasks, right.load?.active_tasks)
  }
  return delta || compareNames(left, right)
}))

function compareNames(left: ClusterInstance, right: ClusterInstance): number {
  return left.node_name.localeCompare(right.node_name, locale.value)
}

function compareDescending(left?: number, right?: number): number {
  const leftValue = typeof left === 'number' && Number.isFinite(left) ? left : -1
  const rightValue = typeof right === 'number' && Number.isFinite(right) ? right : -1
  return rightValue - leftValue
}

function combinedLoad(instance: ClusterInstance): number | undefined {
  if (!instance.load) return undefined
  const values = [instance.load.cpu_usage_percent, memoryPercent(instance.load)]
    .filter((value): value is number => typeof value === 'number' && Number.isFinite(value))
  return values.length > 0 ? Math.max(...values) : undefined
}

function beginNodeRename(nodeId: string, currentName: string): void {
  editingNodeId.value = nodeId
  editingNodeName.value = currentName
}

function cancelNodeRename(): void {
  editingNodeId.value = ''
  editingNodeName.value = ''
}

function requestRename(nodeId: string): void {
  const name = editingNodeName.value.trim()
  if (!name || props.busy) return
  emit('rename', nodeId, name)
  cancelNodeRename()
}

function statusLabel(value: ClusterInstanceStatus): string {
  return clusterInstanceStatusLabel(t, value)
}

function instanceDotClass(value: ClusterInstanceStatus): string {
  if (value === 'online') return 'bg-emerald-500'
  if (value === 'stale') return 'bg-amber-500'
  return 'bg-gray-400'
}

function instanceStatusClass(value: ClusterInstanceStatus): string {
  if (value === 'online') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300'
  if (value === 'stale') return 'bg-amber-50 text-amber-700 dark:bg-amber-950/50 dark:text-amber-300'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}

function workerModeLabel(mode: string): string {
  if (mode === 'true') return t('admin.cluster.deployment.explicitTrue')
  if (mode === 'false') return t('admin.cluster.deployment.explicitFalse')
  return t('admin.cluster.deployment.auto')
}

function memoryPercent(load?: ClusterInstanceLoad): number | undefined {
  if (!load) return undefined
  if (typeof load.memory_usage_percent === 'number' && Number.isFinite(load.memory_usage_percent)) {
    return load.memory_usage_percent
  }
  if (typeof load.memory_used_bytes === 'number' && typeof load.memory_limit_bytes === 'number' && load.memory_limit_bytes > 0) {
    return load.memory_used_bytes / load.memory_limit_bytes * 100
  }
  return undefined
}

function clampPercent(value?: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, value))
}

function formatPercent(value?: number): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  const normalized = clampPercent(value)
  return `${normalized.toFixed(Number.isInteger(normalized) ? 0 : 1)}%`
}

function percentBarStyle(value?: number): Record<string, string> {
  return { width: `${clampPercent(value)}%` }
}

function percentBarClass(value: number | undefined, kind: 'cpu' | 'memory'): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 'bg-gray-300 dark:bg-dark-600'
  if (value >= 90) return 'bg-red-500'
  if (value >= 75) return 'bg-amber-500'
  return kind === 'cpu' ? 'bg-sky-500' : 'bg-teal-500'
}

function formatMemory(load: ClusterInstanceLoad): string {
  if (typeof load.memory_used_bytes !== 'number') return '-'
  const used = formatBytes(Math.max(0, load.memory_used_bytes), 1)
  if (typeof load.memory_limit_bytes !== 'number' || load.memory_limit_bytes <= 0) return used
  return `${used} / ${formatBytes(load.memory_limit_bytes, 1)}`
}

function formatCount(value: number): string {
  return new Intl.NumberFormat(locale.value).format(value)
}

function formatPool(active: number, max: number): string {
  if (max <= 0) return formatCount(active)
  return `${formatCount(active)} / ${formatCount(max)}`
}

function formatUptime(startedAt: string): string {
  const started = new Date(startedAt).getTime()
  if (!Number.isFinite(started)) return '-'
  const totalMinutes = Math.max(0, Math.floor((Date.now() - started) / 60_000))
  const days = Math.floor(totalMinutes / 1440)
  const hours = Math.floor((totalMinutes % 1440) / 60)
  const minutes = totalMinutes % 60
  if (days > 0) return t('admin.cluster.nodes.uptimeDaysHours', { days, hours })
  if (hours > 0) return t('admin.cluster.nodes.uptimeHoursMinutes', { hours, minutes })
  if (minutes > 0) return t('admin.cluster.nodes.uptimeMinutes', { minutes })
  return t('admin.cluster.nodes.uptimeLessThanMinute')
}
</script>
