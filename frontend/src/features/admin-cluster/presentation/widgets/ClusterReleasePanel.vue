<template>
  <section aria-labelledby="cluster-release-title">
    <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-2">
        <h2 id="cluster-release-title" class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.cluster.release.title') }}
        </h2>
        <span
          class="inline-flex rounded-md px-2 py-1 text-xs font-medium"
          :class="overview?.consistent
            ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300'
            : 'bg-amber-50 text-amber-700 dark:bg-amber-950/50 dark:text-amber-300'"
        >
          {{ overview?.consistent ? t('admin.cluster.release.consistent') : t('admin.cluster.release.inconsistent') }}
        </span>
      </div>
      <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
        <span>{{ t('admin.cluster.release.desired') }}</span>
        <span class="font-mono font-semibold text-gray-800 dark:text-gray-200">
          {{ overview?.state.desired_version ? `v${overview.state.desired_version}` : '-' }}
        </span>
        <span class="text-gray-300 dark:text-dark-600">/</span>
        <span>{{ driverLabel }}</span>
      </div>
    </div>

    <div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
      <div class="flex flex-wrap items-center gap-x-5 gap-y-2 border-b border-gray-100 px-4 py-3 text-xs dark:border-dark-700">
        <span v-if="!overview?.version_counts.length" class="text-gray-500 dark:text-gray-400">-</span>
        <span
          v-for="item in overview?.version_counts ?? []"
          :key="item.version"
          class="inline-flex items-center gap-1.5 text-gray-600 dark:text-gray-300"
        >
          <span class="h-2 w-2 rounded-full" :class="versionDotClass(item.version)" />
          <span class="font-mono">v{{ item.version || '-' }}</span>
          <span class="tabular-nums text-gray-400">{{ item.nodes }}</span>
        </span>
      </div>

      <template v-if="activeRollout">
        <div class="flex flex-wrap items-center justify-between gap-3 px-4 py-4">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-mono text-sm font-semibold text-gray-900 dark:text-white">v{{ activeRollout.target_version }}</span>
              <span class="rounded-md px-2 py-1 text-xs font-medium" :class="rolloutStatusClass(activeRollout.status)">
                {{ rolloutStatusLabel(activeRollout.status) }}
              </span>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ completedTargets }}/{{ activeRollout.targets.length }}
              </span>
            </div>
            <p v-if="activeRollout.error_message" class="mt-1 break-words text-xs text-red-600 dark:text-red-400">
              {{ activeRollout.error_message }}
            </p>
          </div>
          <div class="flex items-center gap-2">
            <button
              v-if="activeRollout.status === 'running'"
              type="button"
              class="h-8 rounded-md border border-gray-300 px-3 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-700"
              :disabled="busy"
              @click="$emit('pause', activeRollout.id)"
            >
              {{ t('admin.cluster.release.pause') }}
            </button>
            <button
              v-if="activeRollout.status === 'paused' && !failedTarget"
              type="button"
              class="inline-flex h-8 items-center gap-1.5 rounded-md bg-primary-500 px-3 text-xs font-medium text-white hover:bg-primary-600 disabled:opacity-50"
              :disabled="busy"
              @click="$emit('resume', activeRollout.id)"
            >
              <Icon name="play" size="xs" />
              {{ t('admin.cluster.release.resume') }}
            </button>
            <button
              type="button"
              class="h-8 rounded-md border border-red-200 px-3 text-xs font-medium text-red-600 hover:bg-red-50 disabled:opacity-50 dark:border-red-900/60 dark:text-red-400 dark:hover:bg-red-950/30"
              :disabled="busy || hasActiveTarget"
              @click="$emit('cancel', activeRollout.id)"
            >
              {{ t('admin.cluster.release.cancel') }}
            </button>
          </div>
        </div>

        <div class="h-1.5 bg-gray-100 dark:bg-dark-700">
          <div class="h-full bg-primary-500 transition-all" :style="{ width: `${progressPercent}%` }" />
        </div>

        <div class="overflow-x-auto">
          <table class="min-w-[760px] w-full table-fixed text-left text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/60 dark:text-gray-400">
              <tr>
                <th class="w-56 px-4 py-3 font-medium">{{ t('admin.cluster.release.node') }}</th>
                <th class="w-32 px-4 py-3 font-medium">{{ t('admin.cluster.release.from') }}</th>
                <th class="w-32 px-4 py-3 font-medium">{{ t('admin.cluster.release.to') }}</th>
                <th class="w-40 px-4 py-3 font-medium">{{ t('admin.cluster.release.state') }}</th>
                <th class="px-4 py-3 font-medium">{{ t('admin.cluster.release.error') }}</th>
                <th class="w-24 px-4 py-3 font-medium"></th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="target in activeRollout.targets" :key="target.node_id">
                <td class="px-4 py-3">
                  <p class="truncate font-medium text-gray-900 dark:text-white">{{ target.node_name }}</p>
                  <p class="mt-0.5 truncate font-mono text-[11px] text-gray-400" :title="target.node_id">{{ target.node_id }}</p>
                </td>
                <td class="px-4 py-3 font-mono text-xs text-gray-600 dark:text-gray-300">v{{ target.source_version }}</td>
                <td class="px-4 py-3 font-mono text-xs text-gray-600 dark:text-gray-300">v{{ target.target_version }}</td>
                <td class="px-4 py-3">
                  <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="targetStatusClass(target.status)">
                    {{ targetStatusLabel(target.status) }}
                  </span>
                </td>
                <td class="px-4 py-3 text-xs text-red-600 dark:text-red-400">{{ target.error_message || '-' }}</td>
                <td class="px-4 py-3 text-right">
                  <button
                    v-if="target.status === 'failed'"
                    type="button"
                    class="inline-flex h-8 w-8 items-center justify-center rounded-md text-gray-500 hover:bg-gray-100 hover:text-gray-800 disabled:opacity-50 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white"
                    :title="t('admin.cluster.release.retry')"
                    :disabled="busy"
                    @click="$emit('retry', activeRollout.id, target.node_id)"
                  >
                    <Icon name="refresh" size="sm" />
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>

      <div v-else class="flex flex-wrap items-end gap-3 px-4 py-4">
        <label class="min-w-[220px] flex-1">
          <span class="mb-1.5 block text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.cluster.release.target') }}</span>
          <input
            v-model.trim="targetVersion"
            type="text"
            class="h-9 w-full rounded-md border border-gray-300 bg-white px-3 text-sm text-gray-900 outline-none focus:border-primary-500 focus:ring-1 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
            :placeholder="t('admin.cluster.release.latestPlaceholder')"
          />
        </label>
        <button
          type="button"
          class="inline-flex h-9 items-center gap-2 rounded-md bg-primary-500 px-4 text-sm font-medium text-white hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
          :disabled="busy || deployment?.mode !== 'multi_instance'"
          @click="$emit('create', targetVersion)"
        >
          <Icon name="upload" size="sm" />
          {{ t('admin.cluster.release.start') }}
        </button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import type {
  ClusterDeploymentStatus,
  ClusterReleaseOverview,
  ClusterRolloutStatus,
  ClusterRolloutTargetStatus,
} from '@/features/admin-cluster/data/datasources/adminClusterDatasource'
import {
  clusterRolloutStatusLabel,
  clusterRolloutTargetStatusLabel,
} from '@/features/admin-cluster/presentation/clusterLocale'

const props = defineProps<{
  overview?: ClusterReleaseOverview
  deployment?: ClusterDeploymentStatus
  busy: boolean
}>()

defineEmits<{
  create: [targetVersion: string]
  pause: [rolloutId: string]
  resume: [rolloutId: string]
  cancel: [rolloutId: string]
  retry: [rolloutId: string, nodeId: string]
}>()

const { t } = useI18n()
const targetVersion = ref('')
const activeRollout = computed(() => props.overview?.active_rollout)
const completedTargets = computed(() => activeRollout.value?.targets.filter((target) => target.status === 'succeeded').length ?? 0)
const progressPercent = computed(() => {
  const total = activeRollout.value?.targets.length ?? 0
  return total > 0 ? Math.round((completedTargets.value / total) * 100) : 0
})
const failedTarget = computed(() => activeRollout.value?.targets.find((target) => target.status === 'failed'))
const hasActiveTarget = computed(() => activeRollout.value?.targets.some((target) => ['draining', 'installing', 'restarting', 'verifying'].includes(target.status)) ?? false)
const driverLabel = computed(() => props.deployment?.update_driver === 'binary'
  ? t('admin.cluster.release.binaryDriver')
  : t('admin.cluster.release.externalDriver'))

function versionDotClass(version: string): string {
  if (version === props.overview?.state.desired_version) return 'bg-emerald-500'
  return 'bg-amber-500'
}

function rolloutStatusLabel(status: ClusterRolloutStatus): string {
  return clusterRolloutStatusLabel(t, status)
}

function targetStatusLabel(status: ClusterRolloutTargetStatus): string {
  return clusterRolloutTargetStatusLabel(t, status)
}

function rolloutStatusClass(status: ClusterRolloutStatus): string {
  if (status === 'completed') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300'
  if (status === 'paused') return 'bg-amber-50 text-amber-700 dark:bg-amber-950/50 dark:text-amber-300'
  if (status === 'cancelled') return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  return 'bg-blue-50 text-blue-700 dark:bg-blue-950/50 dark:text-blue-300'
}

function targetStatusClass(status: ClusterRolloutTargetStatus): string {
  if (status === 'succeeded') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300'
  if (status === 'failed') return 'bg-red-50 text-red-700 dark:bg-red-950/50 dark:text-red-300'
  if (status === 'pending') return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  if (status === 'cancelled') return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
  return 'bg-blue-50 text-blue-700 dark:bg-blue-950/50 dark:text-blue-300'
}
</script>
