<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import ConfirmDialog from '@/common/widgets/feedback/ConfirmDialog.vue'
import EmptyState from '@/common/widgets/feedback/EmptyState.vue'
import GroupBadge from '@/common/widgets/data/GroupBadge.vue'
import Select from '@/common/widgets/forms/Select.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import PlatformIcon from '@/common/widgets/icons/PlatformIcon.vue'
import { useAppStore } from '@/core/stores/appStore'
import { formatDateTime } from '@/core/utils/format'
import { platformLabel } from '@/core/utils/platformColors'
import { getAllIncludingInactive } from '@/features/admin-groups/data/datasources/adminGroupQueries'
import type { GroupPlatform } from '@/types/group'
import type {
  RoutingScheme,
  RoutingSchemeDraft,
  RoutingSchemeGroupOption,
  RoutingSchemeItem,
} from '../../data/dtos/routingSchemeDtos'
import {
  ensurePreviewSchemes,
  useRoutingSchemePreviewStore,
} from '../stores/routingSchemePreviewStore'
import { previewGroupScore, scorePercent } from '../../data/routingScorePreview'
import RoutingSchemeEditorDialog from '../widgets/RoutingSchemeEditorDialog.vue'

const { t } = useI18n()
const appStore = useAppStore()
const store = useRoutingSchemePreviewStore()

const groups = ref<RoutingSchemeGroupOption[]>([])
const loading = ref(false)
const search = ref('')
const platform = ref<GroupPlatform | ''>('')
const status = ref<'active' | 'disabled' | ''>('')
const showEditor = ref(false)
const editingScheme = ref<RoutingScheme | null>(null)
const saving = ref(false)
const deleteTarget = ref<RoutingScheme | null>(null)

const schemeGroups = computed(() => groups.value)
const groupMap = computed(() => new Map(groups.value.map(group => [group.id, group])))

const seedLabels = computed(() => ({
  stable: t('admin.routingSchemes.seed.stable'),
  value: t('admin.routingSchemes.seed.value'),
  cheap: t('admin.routingSchemes.seed.cheap'),
  stableDescription: t('admin.routingSchemes.seed.stableDescription'),
  valueDescription: t('admin.routingSchemes.seed.valueDescription'),
  cheapDescription: t('admin.routingSchemes.seed.cheapDescription'),
}))

const platformOptions = computed(() => {
  const values = new Set<GroupPlatform>(groups.value.map(group => group.platform))
  for (const scheme of store.schemes.value) values.add(scheme.platform)
  return [
    { value: '', label: t('admin.routingSchemes.filters.allPlatforms') },
    ...Array.from(values)
      .sort((a, b) => platformLabel(a).localeCompare(platformLabel(b)))
      .map(value => ({ value, label: platformLabel(value) })),
  ]
})

const statusOptions = computed(() => [
  { value: '', label: t('admin.routingSchemes.filters.allStatus') },
  { value: 'active', label: t('admin.routingSchemes.status.active') },
  { value: 'disabled', label: t('admin.routingSchemes.status.disabled') },
])

const filteredSchemes = computed(() => {
  const query = search.value.trim().toLowerCase()
  return store.schemes.value.filter((scheme) => {
    if (platform.value && scheme.platform !== platform.value) return false
    if (status.value && scheme.status !== status.value) return false
    if (!query) return true
    return scheme.name.toLowerCase().includes(query)
      || scheme.description.toLowerCase().includes(query)
      || platformLabel(scheme.platform).toLowerCase().includes(query)
  })
})

const groupsFor = (scheme: RoutingScheme) => scheme.items
  .map(item => ({ ...item, group: groupMap.value.get(item.group_id) }))
  .filter((item): item is RoutingSchemeItem & { group: RoutingSchemeGroupOption } => !!item.group)

const ruleCount = (scheme: RoutingScheme) =>
  scheme.items.reduce((sum, item) => sum + (item.rules?.length ?? 0), 0)

const scoresFor = (scheme: RoutingScheme) => scheme.items.map(item =>
  previewGroupScore(item.group_id, groupMap.value.get(item.group_id)?.name ?? ''),
)

const averageScore = (scheme: RoutingScheme): number => {
  const scores = scoresFor(scheme)
  if (scores.length === 0) return 0
  return Math.round(scores.reduce((sum, score) => sum + score.overall, 0) / scores.length)
}

const percentOf = (scheme: RoutingScheme, groupID: number): number => {
  const scores = scoresFor(scheme)
  const current = scores[scheme.items.findIndex(item => item.group_id === groupID)]
  return scorePercent(current?.overall ?? 0, scores.map(score => score.overall))
}

const loadGroups = async () => {
  loading.value = true
  try {
    const all = await getAllIncludingInactive()
    groups.value = all
      .filter(group => group.platform !== 'composite' && group.platform !== 'multimodal')
      .map(group => ({
        id: group.id,
        name: group.name,
        platform: group.platform,
        status: group.status,
        subscription_type: group.subscription_type,
        rate_multiplier: group.rate_multiplier,
        description: group.description ?? null,
      }))
    ensurePreviewSchemes(groups.value, seedLabels.value)
  } catch {
    appStore.showError(t('admin.routingSchemes.loadFailed'))
  } finally {
    loading.value = false
  }
}

const openCreate = () => {
  editingScheme.value = null
  showEditor.value = true
}

const openEdit = (scheme: RoutingScheme) => {
  editingScheme.value = scheme
  showEditor.value = true
}

const closeEditor = () => {
  if (saving.value) return
  showEditor.value = false
  editingScheme.value = null
}

const handleSave = (draft: RoutingSchemeDraft) => {
  saving.value = true
  try {
    if (editingScheme.value) {
      store.updateScheme(editingScheme.value.id, draft)
      appStore.showSuccess(t('admin.routingSchemes.updateSuccess'))
    } else {
      store.createScheme(draft)
      appStore.showSuccess(t('admin.routingSchemes.createSuccess'))
    }
    closeEditor()
  } catch {
    appStore.showError(t('admin.routingSchemes.saveFailed'))
  } finally {
    saving.value = false
  }
}

const toggleScheme = (scheme: RoutingScheme) => {
  store.toggleScheme(scheme.id)
  appStore.showSuccess(t(
    scheme.status === 'active'
      ? 'admin.routingSchemes.disabledSuccess'
      : 'admin.routingSchemes.enabledSuccess',
  ))
}

const confirmDelete = () => {
  if (!deleteTarget.value) return
  store.deleteScheme(deleteTarget.value.id)
  appStore.showSuccess(t('admin.routingSchemes.deleteSuccess'))
  deleteTarget.value = null
}

onMounted(loadGroups)
</script>

<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-6">
      <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div class="max-w-2xl">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.routingSchemes.title') }}
              </h2>
              <span class="rounded-full bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-700 dark:bg-amber-500/10 dark:text-amber-300">
                {{ t('admin.routingSchemes.previewBadge') }}
              </span>
            </div>
            <p class="mt-1 text-sm leading-6 text-gray-500 dark:text-gray-400">
              {{ t('admin.routingSchemes.description') }}
            </p>
            <p class="mt-2 text-xs leading-5 text-amber-600 dark:text-amber-400">
              {{ t('admin.routingSchemes.previewNotice') }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button class="btn btn-secondary" :disabled="loading" @click="loadGroups">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-primary" :disabled="groups.length === 0" @click="openCreate">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.routingSchemes.create') }}
            </button>
          </div>
        </div>

        <div class="mt-4 flex flex-wrap items-center gap-3">
          <div class="relative w-full sm:w-64">
            <Icon name="search" size="md" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input
              v-model="search"
              type="search"
              class="input pl-10"
              :placeholder="t('admin.routingSchemes.searchPlaceholder')"
            />
          </div>
          <Select v-model="platform" :options="platformOptions" class="w-44" />
          <Select v-model="status" :options="statusOptions" class="w-40" />
          <span class="ml-auto text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.routingSchemes.count', { count: filteredSchemes.length }) }}
          </span>
        </div>
      </section>

      <div v-if="filteredSchemes.length > 0" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        <article
          v-for="scheme in filteredSchemes"
          :key="scheme.id"
          class="flex flex-col rounded-2xl border bg-white p-5 shadow-sm transition-colors dark:bg-dark-900"
          :class="scheme.status === 'active'
            ? 'border-gray-200 dark:border-dark-700'
            : 'border-dashed border-gray-300 opacity-80 dark:border-dark-600'"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="flex min-w-0 items-center gap-3">
              <span class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-gray-100 dark:bg-dark-800">
                <PlatformIcon :platform="scheme.platform" size="lg" />
              </span>
              <div class="min-w-0">
                <div class="flex items-center gap-2">
                  <h3 class="truncate text-base font-semibold text-gray-900 dark:text-white">
                    {{ scheme.name }}
                  </h3>
                  <span
                    class="rounded-full px-2 py-0.5 text-[11px] font-medium"
                    :class="scheme.status === 'active'
                      ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
                      : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'"
                  >
                    {{ scheme.status === 'active'
                      ? t('admin.routingSchemes.status.active')
                      : t('admin.routingSchemes.status.disabled') }}
                  </span>
                </div>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ platformLabel(scheme.platform) }}
                  <span class="mx-1">·</span>
                  {{ t('admin.routingSchemes.sortOrderValue', { value: scheme.sort_order }) }}
                </p>
              </div>
            </div>
            <div class="flex flex-shrink-0 items-center gap-1">
              <button
                class="rounded-lg p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                :title="t('common.edit')"
                @click="openEdit(scheme)"
              >
                <Icon name="edit" size="sm" />
              </button>
              <button
                class="rounded-lg p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-amber-600 dark:hover:bg-dark-700 dark:hover:text-amber-400"
                :title="scheme.status === 'active'
                  ? t('admin.routingSchemes.disable')
                  : t('admin.routingSchemes.enable')"
                @click="toggleScheme(scheme)"
              >
                <Icon :name="scheme.status === 'active' ? 'ban' : 'checkCircle'" size="sm" />
              </button>
              <button
                class="rounded-lg p-2 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-950/30"
                :title="t('common.delete')"
                @click="deleteTarget = scheme"
              >
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>

          <p
            class="mt-3 line-clamp-2 min-h-[2.5rem] text-sm leading-5 text-gray-600 dark:text-gray-300"
            :class="{ 'text-gray-400 dark:text-gray-500': !scheme.description }"
          >
            {{ scheme.description || t('admin.routingSchemes.noDescription') }}
          </p>

          <div class="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
            <span>{{ t('admin.routingSchemes.groupCount', { count: groupsFor(scheme).length }) }}</span>
            <span>{{ t('admin.routingSchemes.avgScore', { score: averageScore(scheme) }) }}</span>
            <span>{{ t('admin.routingSchemes.policy.ruleCount', { count: ruleCount(scheme) }) }}</span>
          </div>

          <div class="mt-2 flex h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
            <span
              v-for="(item, index) in groupsFor(scheme)"
              :key="item.group_id"
              class="h-full"
              :class="index === 0
                ? 'bg-primary-500'
                : index === 1
                  ? 'bg-sky-400'
                  : index === 2
                    ? 'bg-emerald-400'
                    : 'bg-amber-400'"
              :style="{ width: `${percentOf(scheme, item.group_id)}%` }"
            />
          </div>

          <div class="mt-3 flex flex-wrap gap-1.5">
            <span
              v-for="item in groupsFor(scheme).slice(0, 3)"
              :key="item.group_id"
              class="inline-flex items-center gap-1"
            >
              <GroupBadge
                :name="item.group.name"
                :platform="item.group.platform"
                :subscription-type="item.group.subscription_type || 'standard'"
                :rate-multiplier="item.group.rate_multiplier"
                truncate
              />
              <span class="rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-gray-400">
                {{ percentOf(scheme, item.group_id) }}%
              </span>
            </span>
            <span
              v-if="groupsFor(scheme).length > 3"
              class="inline-flex items-center rounded-md bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-500 dark:bg-dark-700 dark:text-gray-400"
            >
              +{{ groupsFor(scheme).length - 3 }}
            </span>
          </div>

          <div class="mt-auto flex items-center justify-between border-t border-gray-100 pt-3 text-[11px] text-gray-400 dark:border-dark-800 dark:text-gray-500">
            <span>{{ t('admin.routingSchemes.updatedAt', { time: formatDateTime(scheme.updated_at) }) }}</span>
            <span>{{ t('admin.routingSchemes.previewTag') }}</span>
          </div>
        </article>
      </div>

      <div v-else class="rounded-2xl border border-dashed border-gray-300 bg-white p-4 dark:border-dark-600 dark:bg-dark-900">
        <EmptyState
          :title="t('admin.routingSchemes.emptyTitle')"
          :description="t('admin.routingSchemes.emptyDescription')"
          :action-text="groups.length > 0 ? t('admin.routingSchemes.create') : undefined"
          @action="openCreate"
        />
      </div>
    </div>

    <RoutingSchemeEditorDialog
      :show="showEditor"
      :scheme="editingScheme"
      :groups="schemeGroups"
      :saving="saving"
      @close="closeEditor"
      @save="handleSave"
    />

    <ConfirmDialog
      :show="!!deleteTarget"
      :title="t('admin.routingSchemes.deleteTitle')"
      :message="t('admin.routingSchemes.deleteMessage', { name: deleteTarget?.name || '' })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="deleteTarget = null"
    />
  </AppLayout>
</template>
