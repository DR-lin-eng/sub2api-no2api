<template>
  <BaseDialog
    :show="visible"
    :title="t('admin.groups.mediaStudioRouting.title')"
    width="wide"
    @close="emit('close')"
  >
    <div class="space-y-5">
      <p class="text-sm text-gray-600 dark:text-gray-300">
        {{ t('admin.groups.mediaStudioRouting.description') }}
      </p>

      <section class="rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
        <div class="mb-3">
          <h3 class="font-semibold text-gray-900 dark:text-white">
            {{ t('admin.groups.mediaStudioRouting.groupTitle') }}
          </h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.groups.mediaStudioRouting.groupHint') }}
          </p>
        </div>

        <div v-if="availableGroups().length === 0" class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.groups.mediaStudioRouting.noGroups') }}
        </div>

        <div v-else class="space-y-2">
          <div
            v-for="group in availableGroups()"
            :key="group.id"
            class="rounded-lg border border-gray-200 bg-white px-2.5 py-2 dark:border-dark-600 dark:bg-dark-900"
          >
            <div class="flex items-center gap-2">
              <input
                :checked="isSelected(group.id)"
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500 dark:bg-dark-800"
                @change="toggleGroup(group)"
              />
              <div class="min-w-0 flex-1">
                <div class="truncate text-sm font-medium text-gray-900 dark:text-gray-100">{{ group.name }}</div>
                <div class="text-xs text-gray-500 dark:text-gray-400">{{ group.platform }} · #{{ group.id }}</div>
              </div>
              <button
                type="button"
                class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-700 disabled:opacity-30 dark:hover:bg-dark-700 dark:hover:text-gray-200"
                :disabled="!isSelected(group.id) || selectedIndex(group.id) === 0"
                :title="t('admin.groups.mediaStudioRouting.moveUp')"
                @click="moveGroup(group.id, -1)"
              >
                <Icon name="chevronUp" size="xs" />
              </button>
              <button
                type="button"
                class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-700 disabled:opacity-30 dark:hover:bg-dark-700 dark:hover:text-gray-200"
                :disabled="!isSelected(group.id) || selectedIndex(group.id) === selectedIds.length - 1"
                :title="t('admin.groups.mediaStudioRouting.moveDown')"
                @click="moveGroup(group.id, 1)"
              >
                <Icon name="chevronDown" size="xs" />
              </button>
            </div>

            <div v-if="isSelected(group.id)" class="mt-2 border-t border-gray-100 pt-2 dark:border-dark-700">
              <div class="mb-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.groups.mediaStudioRouting.modelsHint') }}
              </div>
              <div v-if="loadingModelGroupIds.has(group.id)" class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.groups.mediaStudioRouting.loadingModels') }}
              </div>
              <div v-else-if="availableModels(group.id).length === 0" class="text-xs text-amber-600 dark:text-amber-300">
                {{ t('admin.groups.mediaStudioRouting.noModels') }}
              </div>
              <div v-else class="max-h-40 space-y-1 overflow-y-auto pr-1">
                <label
                  v-for="model in availableModels(group.id)"
                  :key="model"
                  class="flex cursor-pointer items-center gap-2 rounded-md px-1.5 py-1 text-xs text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-dark-800"
                >
                  <input
                    :checked="selectedModels[group.id]?.includes(model)"
                    type="checkbox"
                    class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500 dark:bg-dark-800"
                    @change="toggleModel(group.id, model)"
                  />
                  <span class="min-w-0 truncate font-mono">{{ model }}</span>
                </label>
              </div>
            </div>
          </div>
        </div>

        <div class="mt-3 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.groups.mediaStudioRouting.selectedCount', { count: selectedIds.length }) }}
        </div>
      </section>

      <div v-if="errorMessage" class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200">
        {{ errorMessage }}
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button type="button" class="btn btn-primary" :disabled="loading || saving" @click="save">
          <Icon v-if="saving" name="refresh" size="sm" class="mr-2 animate-spin" />
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import {
  getAllIncludingInactive,
  getMediaStudioModels,
} from '@/features/admin-groups/data/datasources/adminGroupQueries'
import {
  getMediaStudioGroupRoutes,
  saveMediaStudioGroupRoutes,
  type MediaStudioGroupRoutes,
} from '@/features/admin-groups/data/datasources/mediaStudioGroupRouteDatasource'
import type { AdminGroup } from '@/features/admin-groups/data/dtos/adminGroupDtos'
import {
  isMediaStudioAudioModel,
  isMediaStudioImageModel,
  isMediaStudioVideoModel,
} from '@/features/custom-model-config/domain/services/modelCapabilityService'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()

const groups = ref<AdminGroup[]>([])
const loading = ref(false)
const saving = ref(false)
const errorMessage = ref('')
const modelCandidates = ref<Map<number, string[]>>(new Map())
const loadingModelGroupIds = ref<Set<number>>(new Set())
const selectedIds = ref<number[]>([])
const selectedModels = reactive<Record<number, string[]>>({})

function availableGroups(): AdminGroup[] {
  return groups.value.filter(group => group.status === 'active')
}

function isSelected(id: number): boolean {
  return selectedIds.value.includes(id)
}

function selectedIndex(id: number): number {
  return selectedIds.value.indexOf(id)
}

async function toggleGroup(group: AdminGroup): Promise<void> {
  const index = selectedIndex(group.id)
  if (index >= 0) {
    selectedIds.value.splice(index, 1)
    delete selectedModels[group.id]
  } else {
    selectedIds.value.push(group.id)
    await loadGroupModels(group)
    if (!selectedModels[group.id]) selectedModels[group.id] = []
  }
}

function moveGroup(id: number, offset: number): void {
  const index = selectedIndex(id)
  const nextIndex = index + offset
  if (index < 0 || nextIndex < 0 || nextIndex >= selectedIds.value.length) return
  const [item] = selectedIds.value.splice(index, 1)
  selectedIds.value.splice(nextIndex, 0, item)
}

function applyRoutes(routes: MediaStudioGroupRoutes): void {
  selectedIds.value = routes.map(entry => entry.group_id)
  for (const key of Object.keys(selectedModels)) delete selectedModels[Number(key)]
  for (const entry of routes) selectedModels[entry.group_id] = [...(entry.models || [])]
}

function buildRoutes(): MediaStudioGroupRoutes {
  return selectedIds.value.map((group_id, priority) => ({
    group_id,
    priority,
    enabled: true,
    models: selectedModels[group_id] || [],
  }))
}

function availableModels(groupId: number): string[] {
  const candidates = modelCandidates.value.get(groupId) || []
  const configured = selectedModels[groupId] || []
  const mediaModels = candidates.filter(model =>
    isMediaStudioImageModel(model) ||
    isMediaStudioVideoModel(model) ||
    isMediaStudioAudioModel(model),
  )
  return [...new Set([...mediaModels, ...configured])]
}

function toggleModel(groupId: number, model: string): void {
  const current = selectedModels[groupId] || []
  selectedModels[groupId] = current.includes(model)
    ? current.filter(item => item !== model)
    : [...current, model]
}

async function loadGroupModels(group: AdminGroup): Promise<void> {
  if (modelCandidates.value.has(group.id) || loadingModelGroupIds.value.has(group.id)) return
  loadingModelGroupIds.value = new Set([...loadingModelGroupIds.value, group.id])
  try {
    const models = await getMediaStudioModels(group.id, group.platform)
    const next = new Map(modelCandidates.value)
    next.set(group.id, models)
    modelCandidates.value = next
  } finally {
    const nextLoading = new Set(loadingModelGroupIds.value)
    nextLoading.delete(group.id)
    loadingModelGroupIds.value = nextLoading
  }
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [allGroups, routes] = await Promise.all([
      getAllIncludingInactive(),
      getMediaStudioGroupRoutes(),
    ])
    groups.value = allGroups
    applyRoutes(routes)
    const selectedGroupIds = new Set(selectedIds.value)
    await Promise.all(
      allGroups
        .filter(group => selectedGroupIds.has(group.id))
        .map(group => loadGroupModels(group)),
    )
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : t('admin.groups.mediaStudioRouting.loadFailed')
  } finally {
    loading.value = false
  }
}

async function save(): Promise<void> {
  saving.value = true
  errorMessage.value = ''
  try {
    const missingModels = selectedIds.value.some(groupId => (selectedModels[groupId] || []).length === 0)
    if (missingModels) {
      errorMessage.value = t('admin.groups.mediaStudioRouting.modelRequired')
      return
    }
    await saveMediaStudioGroupRoutes(buildRoutes())
    emit('saved')
    emit('close')
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : t('admin.groups.mediaStudioRouting.saveFailed')
  } finally {
    saving.value = false
  }
}

watch(
  () => props.visible,
  (visible) => {
    if (visible) void load()
  },
)
</script>
