<template>
  <BaseDialog
    :show="show"
    :title="dialogTitle"
    width="wide"
    @close="emit('close')"
  >
    <div class="grid gap-5 lg:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
      <section class="min-w-0">
        <div class="mb-3 flex items-center justify-between gap-3">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.groups.compositeRoutes.routes') }}
          </h3>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="routesLoading"
            :title="t('common.refresh')"
            @click="loadRoutes"
          >
            <Icon name="refresh" size="sm" :class="routesLoading ? 'animate-spin' : ''" />
          </button>
        </div>

        <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
          <div
            v-if="routesLoading"
            class="flex h-36 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
          >
            {{ t('common.loading') }}
          </div>
          <div
            v-else-if="routes.length === 0"
            class="flex h-36 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
          >
            {{ t('admin.groups.compositeRoutes.empty') }}
          </div>
          <div v-else class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
              <thead class="bg-gray-50 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                <tr>
                  <th class="px-3 py-2">{{ t('admin.groups.compositeRoutes.publicModel') }}</th>
                  <th class="px-3 py-2">{{ t('admin.groups.compositeRoutes.target') }}</th>
                  <th class="px-3 py-2">{{ t('admin.groups.compositeRoutes.scope') }}</th>
                  <th class="px-3 py-2 text-right">{{ t('admin.groups.columns.actions') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-900">
                <tr
                  v-for="route in routes"
                  :key="route.id"
                  :class="!route.enabled && 'opacity-60'"
                >
                  <td class="max-w-[15rem] px-3 py-2">
                    <div class="break-all font-medium text-gray-900 dark:text-white">
                      {{ route.public_model }}
                    </div>
                    <div class="mt-1 flex flex-wrap items-center gap-1.5">
                      <span class="badge badge-gray">{{ matchLabel(route.match_type) }}</span>
                      <span v-if="!route.enabled" class="badge badge-danger">
                        {{ t('admin.accounts.status.inactive') }}
                      </span>
                    </div>
                  </td>
                  <td class="px-3 py-2">
                    <div class="flex items-center gap-1.5 text-gray-900 dark:text-white">
                      <PlatformIcon :platform="route.target_platform" size="xs" />
                      <span>{{ formatPlatform(route.target_platform) }}</span>
                    </div>
                    <div
                      class="mt-1 break-all text-xs text-gray-500 dark:text-gray-400"
                      :title="!route.upstream_model ? t('admin.groups.compositeRoutes.upstreamModelHint') : undefined"
                    >
                      {{ displayedUpstreamModel(route) }}
                    </div>
                  </td>
                  <td class="px-3 py-2">
                    <div class="text-gray-700 dark:text-gray-300">
                      {{ endpointLabel(route.endpoint) }}
                    </div>
                    <div class="text-xs text-gray-500 dark:text-gray-400">
                      {{ t('admin.groups.compositeRoutes.priority') }}: {{ route.priority }}
                    </div>
                  </td>
                  <td class="px-3 py-2">
                    <div class="flex justify-end gap-1">
                      <button
                        type="button"
                        class="rounded p-1.5 text-gray-500 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                        :title="t('common.edit')"
                        @click="editRoute(route)"
                      >
                        <Icon name="edit" size="sm" />
                      </button>
                      <button
                        type="button"
                        class="rounded p-1.5 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                        :title="t('common.delete')"
                        @click="deleteRoute(route)"
                      >
                        <Icon name="trash" size="sm" />
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <section class="space-y-5">
        <form class="space-y-3" @submit.prevent="saveRoute">
          <div class="flex items-center justify-between gap-3">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{
                editingId
                  ? t('admin.groups.compositeRoutes.editRoute')
                  : t('admin.groups.compositeRoutes.addRoute')
              }}
            </h3>
            <button
              v-if="editingId"
              type="button"
              class="text-xs font-medium text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
              @click="resetRouteForm"
            >
              {{ t('common.cancel') }}
            </button>
          </div>

          <div>
            <label class="input-label">{{ t('admin.groups.compositeRoutes.publicModel') }}</label>
            <input
              v-model.trim="form.public_model"
              data-testid="composite-route-public-model"
              type="text"
              class="input"
              required
              placeholder="openrouter/gpt-5"
            />
          </div>

          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.groups.compositeRoutes.matchType') }}</label>
              <Select v-model="form.match_type" :options="matchOptions" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.groups.compositeRoutes.endpoint') }}</label>
              <Select v-model="form.endpoint" :options="endpointOptions" />
            </div>
          </div>

          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.groups.compositeRoutes.targetPlatform') }}</label>
              <Select v-model="form.target_platform" :options="platformOptions" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.groups.compositeRoutes.priority') }}</label>
              <input
                v-model.number="form.priority"
                type="number"
                min="1"
                step="1"
                class="input"
              />
            </div>
          </div>

          <div>
            <label class="input-label">{{ t('admin.groups.compositeRoutes.upstreamModel') }}</label>
            <input
              v-model.trim="form.upstream_model"
              data-testid="composite-route-upstream-model"
              type="text"
              class="input"
              placeholder="gpt-5"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.groups.compositeRoutes.upstreamModelHint') }}
            </p>
          </div>

          <div>
            <label class="input-label">{{ t('admin.groups.compositeRoutes.notes') }}</label>
            <textarea v-model.trim="form.notes" rows="2" class="input"></textarea>
          </div>

          <div class="flex items-center justify-between gap-3">
            <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input
                v-model="form.enabled"
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
              />
              {{ t('admin.groups.compositeRoutes.enabled') }}
            </label>
            <button type="submit" class="btn btn-primary" :disabled="saving">
              <Icon v-if="!saving" name="check" size="sm" class="mr-2" />
              {{ editingId ? t('common.update') : t('common.create') }}
            </button>
          </div>
        </form>

        <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
          <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.groups.compositeRoutes.preview') }}
          </h3>
          <div class="space-y-3">
            <input
              v-model.trim="previewModel"
              type="text"
              class="input"
              placeholder="openrouter/gpt-5"
              @keyup.enter="previewRoute"
            />
            <div class="flex gap-2">
              <Select v-model="previewEndpoint" :options="endpointOptions" class="min-w-0 flex-1" />
              <button
                type="button"
                class="btn btn-secondary"
                :disabled="previewLoading || !previewModel"
                @click="previewRoute"
              >
                <Icon name="play" size="sm" />
              </button>
            </div>

            <div
              v-if="previewDecision"
              class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-600 dark:bg-dark-800"
            >
              <div class="mb-2 flex items-center gap-2">
                <span :class="['badge', previewDecision.matched ? 'badge-success' : 'badge-danger']">
                  {{
                    previewDecision.matched
                      ? t('admin.groups.compositeRoutes.matched')
                      : t('admin.groups.compositeRoutes.notMatched')
                  }}
                </span>
                <span class="badge badge-gray">{{ sourceLabel(previewDecision.source) }}</span>
              </div>
              <div
                v-if="previewDecision.matched"
                class="space-y-1 text-gray-700 dark:text-gray-300"
              >
                <div>
                  {{ t('admin.groups.compositeRoutes.targetPlatform') }}:
                  {{ formatPlatform(previewDecision.target_platform) }}
                </div>
                <div class="break-all">
                  {{ t('admin.groups.compositeRoutes.upstreamModel') }}:
                  {{ previewDecision.upstream_model }}
                </div>
              </div>
              <div v-else class="text-gray-500 dark:text-gray-400">
                {{ previewDecision.reason }}
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>

    <template #footer>
      <div class="flex justify-end pt-4">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/core/stores/appStore'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import Select from '@/common/widgets/forms/Select.vue'
import PlatformIcon from '@/common/widgets/icons/PlatformIcon.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import {
  createCompositeRoute,
  deleteCompositeRoute,
  updateCompositeRoute,
} from '@/features/admin-groups/data/datasources/adminGroupActions'
import {
  listCompositeRoutes,
  previewCompositeRoute,
} from '@/features/admin-groups/data/datasources/adminGroupQueries'
import type {
  AdminGroup,
  CompositeModelRoute,
  CompositeModelRouteInput,
  CompositeRouteDecision,
  CompositeRouteEndpoint,
  CompositeRouteMatchType,
} from '@/features/admin-groups/data/dtos/adminGroupDtos'
import type { GroupPlatform } from '@/types/group'
import {
  compositeRouteEndpointLabel,
  compositeRouteMatchLabel,
  compositeRouteSourceLabel,
  groupPlatformLabel,
} from '@/features/admin-groups/presentation/groupsLocale'

type ConcreteGroupPlatform = Exclude<GroupPlatform, 'composite'>

interface RouteFormState {
  public_model: string
  match_type: CompositeRouteMatchType
  target_platform: ConcreteGroupPlatform
  upstream_model: string
  endpoint: CompositeRouteEndpoint
  priority: number
  enabled: boolean
  notes: string
}

const props = defineProps<{
  show: boolean
  group: AdminGroup | null
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const routes = ref<CompositeModelRoute[]>([])
const routesLoading = ref(false)
const saving = ref(false)
const editingId = ref<number | null>(null)
const previewModel = ref('')
const previewEndpoint = ref<CompositeRouteEndpoint>('any')
const previewLoading = ref(false)
const previewDecision = ref<CompositeRouteDecision | null>(null)
let loadSequence = 0
let previewSequence = 0
let modalGeneration = 0

const form = reactive<RouteFormState>({
  public_model: '',
  match_type: 'exact',
  target_platform: 'openai',
  upstream_model: '',
  endpoint: 'any',
  priority: 100,
  enabled: true,
  notes: '',
})

const dialogTitle = computed(() =>
  props.group
    ? t('admin.groups.compositeRoutes.titleWithGroup', { name: props.group.name })
    : t('admin.groups.compositeRoutes.title'),
)

const platformOptions = computed(() => [
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' },
  { value: 'grok', label: 'Grok' },
])

const endpointOptions = computed(() => [
  { value: 'any', label: t('admin.groups.compositeRoutes.endpoints.any') },
  { value: 'messages', label: t('admin.groups.compositeRoutes.endpoints.messages') },
  { value: 'count_tokens', label: t('admin.groups.compositeRoutes.endpoints.countTokens') },
  { value: 'responses', label: t('admin.groups.compositeRoutes.endpoints.responses') },
  { value: 'chat_completions', label: t('admin.groups.compositeRoutes.endpoints.chatCompletions') },
  { value: 'embeddings', label: t('admin.groups.compositeRoutes.endpoints.embeddings') },
  { value: 'images', label: t('admin.groups.compositeRoutes.endpoints.images') },
  { value: 'gemini', label: t('admin.groups.compositeRoutes.endpoints.gemini') },
])

const matchOptions = computed(() => [
  { value: 'exact', label: t('admin.groups.compositeRoutes.match.exact') },
  { value: 'prefix', label: t('admin.groups.compositeRoutes.match.prefix') },
])

function resetRouteForm(): void {
  editingId.value = null
  Object.assign(form, {
    public_model: '',
    match_type: 'exact',
    target_platform: 'openai',
    upstream_model: '',
    endpoint: 'any',
    priority: 100,
    enabled: true,
    notes: '',
  })
}

function resetModalState(): void {
  modalGeneration += 1
  loadSequence += 1
  previewSequence += 1
  routes.value = []
  previewModel.value = ''
  previewEndpoint.value = 'any'
  previewDecision.value = null
  routesLoading.value = false
  previewLoading.value = false
  saving.value = false
  resetRouteForm()
}

function isCurrentModal(generation: number, groupId: number): boolean {
  return generation === modalGeneration && props.show && props.group?.id === groupId
}

function matchLabel(matchType: CompositeRouteMatchType): string {
  return compositeRouteMatchLabel(t, matchType)
}

function endpointLabel(endpoint: CompositeRouteEndpoint): string {
  return compositeRouteEndpointLabel(t, endpoint)
}

function formatPlatform(platform: string): string {
  return platform ? groupPlatformLabel(t, platform) : '—'
}

function sourceLabel(source: string): string {
  return source ? compositeRouteSourceLabel(t, source) : '—'
}

function displayedUpstreamModel(route: CompositeModelRoute): string {
  if (route.upstream_model) return route.upstream_model
  if (route.match_type === 'prefix') {
    return t('admin.groups.compositeRoutes.passthroughRequestedModel')
  }
  return route.public_model
}

function routePayload(): CompositeModelRouteInput {
  return {
    public_model: form.public_model.trim(),
    match_type: form.match_type,
    target_platform: form.target_platform,
    upstream_model: form.upstream_model.trim(),
    endpoint: form.endpoint,
    priority: Number(form.priority) || 100,
    enabled: form.enabled,
    notes: form.notes.trim(),
  }
}

async function loadRoutes(): Promise<void> {
  const groupId = props.group?.id
  if (!props.show || !groupId) return
  const sequence = ++loadSequence
  routesLoading.value = true
  try {
    const result = await listCompositeRoutes(groupId)
    if (sequence !== loadSequence || !props.show || props.group?.id !== groupId) return
    routes.value = [...result].sort((left, right) =>
      left.priority !== right.priority ? left.priority - right.priority : left.id - right.id,
    )
  } catch (error: unknown) {
    if (sequence !== loadSequence) return
    appStore.showError(
      extractApiErrorMessage(error, t('admin.groups.compositeRoutes.failedToLoad')),
    )
  } finally {
    if (sequence === loadSequence) routesLoading.value = false
  }
}

function editRoute(route: CompositeModelRoute): void {
  editingId.value = route.id
  Object.assign(form, {
    public_model: route.public_model,
    match_type: route.match_type,
    target_platform: route.target_platform,
    upstream_model: route.upstream_model || '',
    endpoint: route.endpoint,
    priority: route.priority || 100,
    enabled: route.enabled,
    notes: route.notes || '',
  })
}

async function saveRoute(): Promise<void> {
  const groupId = props.group?.id
  if (!groupId || saving.value) return
  if (!form.public_model.trim()) {
    appStore.showError(t('admin.groups.compositeRoutes.publicModelRequired'))
    return
  }
  const generation = modalGeneration
  const editingRouteId = editingId.value
  saving.value = true
  try {
    const payload = routePayload()
    if (editingRouteId) {
      await updateCompositeRoute(groupId, editingRouteId, payload)
    } else {
      await createCompositeRoute(groupId, payload)
    }
    if (!isCurrentModal(generation, groupId)) return
    appStore.showSuccess(
      t(
        editingRouteId
          ? 'admin.groups.compositeRoutes.routeUpdated'
          : 'admin.groups.compositeRoutes.routeCreated',
      ),
    )
    resetRouteForm()
    await loadRoutes()
  } catch (error: unknown) {
    if (!isCurrentModal(generation, groupId)) return
    appStore.showError(
      extractApiErrorMessage(error, t('admin.groups.compositeRoutes.failedToSave')),
    )
  } finally {
    if (isCurrentModal(generation, groupId)) saving.value = false
  }
}

async function deleteRoute(route: CompositeModelRoute): Promise<void> {
  const groupId = props.group?.id
  if (!groupId || !window.confirm(t('admin.groups.compositeRoutes.deleteConfirm'))) return
  const generation = modalGeneration
  try {
    await deleteCompositeRoute(groupId, route.id)
    if (!isCurrentModal(generation, groupId)) return
    if (editingId.value === route.id) resetRouteForm()
    appStore.showSuccess(t('admin.groups.compositeRoutes.routeDeleted'))
    await loadRoutes()
  } catch (error: unknown) {
    if (!isCurrentModal(generation, groupId)) return
    appStore.showError(
      extractApiErrorMessage(error, t('admin.groups.compositeRoutes.failedToDelete')),
    )
  }
}

async function previewRoute(): Promise<void> {
  const groupId = props.group?.id
  const model = previewModel.value.trim()
  if (!groupId || !model) return
  const sequence = ++previewSequence
  previewLoading.value = true
  try {
    const decision = await previewCompositeRoute(groupId, {
      model,
      endpoint: previewEndpoint.value,
    })
    if (sequence === previewSequence && props.show && props.group?.id === groupId) {
      previewDecision.value = decision
    }
  } catch (error: unknown) {
    if (sequence !== previewSequence) return
    appStore.showError(
      extractApiErrorMessage(error, t('admin.groups.compositeRoutes.failedToPreview')),
    )
  } finally {
    if (sequence === previewSequence) previewLoading.value = false
  }
}

watch(
  () => [props.show, props.group?.id] as const,
  ([show, groupId]) => {
    resetModalState()
    if (show && groupId) void loadRoutes()
  },
  { immediate: true },
)
</script>
