<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import Select from '@/common/widgets/forms/Select.vue'
import PlatformIcon from '@/common/widgets/icons/PlatformIcon.vue'
import { platformLabel } from '@/core/utils/platformColors'
import type { GroupPlatform } from '@/types/group'
import type {
  RoutingScheme,
  RoutingSchemeDraft,
  RoutingSchemeGroupOption,
  RoutingSchemeItem,
  RoutingSchemeStatus,
} from '../../data/dtos/routingSchemeDtos'
import { listGroupScores } from '../../data/datasources/routingSchemeQueries'
import RoutingSchemeGroupPicker from './RoutingSchemeGroupPicker.vue'

const props = withDefaults(defineProps<{
  show: boolean
  scheme?: RoutingScheme | null
  groups: RoutingSchemeGroupOption[]
  saving?: boolean
}>(), {
  scheme: null,
  saving: false,
})

const emit = defineEmits<{
  close: []
  save: [draft: RoutingSchemeDraft]
}>()

const { t } = useI18n()
const isEdit = computed(() => !!props.scheme)

const DEFAULT_TTFT_WEIGHT = 40
const DEFAULT_ERROR_WEIGHT = 60

const form = ref<RoutingSchemeDraft>({
  name: '',
  description: '',
  platform: 'deepseek',
  status: 'active',
  sort_order: 10,
  ttft_weight: DEFAULT_TTFT_WEIGHT,
  error_weight: DEFAULT_ERROR_WEIGHT,
  items: [],
})

const reset = () => {
  const source = props.scheme
  form.value = source
    ? {
        name: source.name,
        description: source.description,
        platform: source.platform,
        status: source.status,
        sort_order: source.sort_order,
        ttft_weight: source.ttft_weight,
        error_weight: source.error_weight,
        items: source.items.map(item => ({ ...item })),
      }
    : {
        name: '',
        description: '',
        platform: props.groups[0]?.platform ?? 'deepseek',
        status: 'active',
        sort_order: 10,
        ttft_weight: DEFAULT_TTFT_WEIGHT,
        error_weight: DEFAULT_ERROR_WEIGHT,
        items: [],
      }
}

watch(() => props.show, (show) => {
  if (show) reset()
}, { immediate: true })

const clampWeight = (value: unknown): number => {
  const numeric = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numeric)) return 0
  return Math.min(100, Math.max(0, Math.round(numeric)))
}

// Weights only matter through their ratio; zero/zero falls back to the backend
// default so the preview always matches what the scheduler will use.
const scoreWeights = computed(() => {
  let ttft = clampWeight(form.value.ttft_weight)
  let errors = clampWeight(form.value.error_weight)
  let valid = true
  if (ttft + errors === 0) {
    ttft = DEFAULT_TTFT_WEIGHT
    errors = DEFAULT_ERROR_WEIGHT
    valid = false
  }
  const ttftShare = Math.round((ttft / (ttft + errors)) * 100)
  return { ttft, errors, ttftShare, errorShare: 100 - ttftShare, valid }
})

let scoreRequestSeq = 0
const mergeGroupScores = async () => {
  const ids = Array.from(new Set(
    form.value.items.map(item => item.group_id).filter(id => id > 0),
  ))
  if (ids.length === 0) return
  const seq = ++scoreRequestSeq
  try {
    const scores = await listGroupScores(ids, scoreWeights.value.ttft, scoreWeights.value.errors)
    if (seq !== scoreRequestSeq) return
    const byID = new Map(scores.map(score => [score.group_id, score]))
    form.value.items = form.value.items.map((item) => {
      const score = byID.get(item.group_id)
      if (!score) return item
      return {
        ...item,
        score: score.score,
        p95_ttft_ms: score.p95_ttft_ms,
        failure_rate: score.failure_rate,
        failure_count: score.failure_count,
        sample_count: score.sample_count,
        cold_start: score.cold_start,
      }
    })
  } catch {
    // Keep the last known metrics; the editor still works without them.
  }
}

// Pull read-only scores as soon as the selected groups or the score weights
// change, including before the scheme is saved for the first time.
watch(
  () => `${form.value.items.map(item => item.group_id).join(',')}`
    + `|${scoreWeights.value.ttft}|${scoreWeights.value.errors}`,
  () => { void mergeGroupScores() },
)

const platformOptions = computed(() => {
  const platforms = new Set<GroupPlatform>(props.groups.map(group => group.platform))
  if (props.scheme) platforms.add(props.scheme.platform)
  return Array.from(platforms)
    .sort((a, b) => platformLabel(a).localeCompare(platformLabel(b)))
    .map(platform => ({ value: platform, label: platformLabel(platform) }))
})

const statusOptions = computed(() => [
  { value: 'active' as RoutingSchemeStatus, label: t('admin.routingSchemes.status.active') },
  { value: 'disabled' as RoutingSchemeStatus, label: t('admin.routingSchemes.status.disabled') },
])

const groupsForPlatform = computed(() =>
  props.groups.filter(group => group.platform === form.value.platform),
)

const selectedPlatformLabel = computed(() => platformLabel(form.value.platform))
const totalRules = computed(() =>
  form.value.items.reduce((sum, item) => sum + (item.rules?.length ?? 0), 0),
)

const setItems = (items: RoutingSchemeItem[]) => {
  form.value.items = items
}

const changePlatform = (value: string | number | boolean | null) => {
  if (isEdit.value) return
  const platform = value as GroupPlatform
  if (form.value.platform === platform) return
  form.value.platform = platform
  form.value.items = []
}

const canSave = computed(() =>
  form.value.name.trim().length > 0
  && form.value.items.length > 0
  && scoreWeights.value.valid,
)

const submit = () => {
  if (!canSave.value || props.saving) return
  const weights = scoreWeights.value
  emit('save', {
    ...form.value,
    name: form.value.name.trim(),
    description: form.value.description.trim(),
    ttft_weight: weights.ttft,
    error_weight: weights.errors,
    items: form.value.items.map(item => ({ ...item })),
  })
}
</script>

<template>
  <BaseDialog
    :show="show"
    :title="t(isEdit ? 'admin.routingSchemes.editTitle' : 'admin.routingSchemes.createTitle')"
    width="extra-wide"
    @close="emit('close')"
  >
    <form id="routing-scheme-form" class="space-y-5" @submit.prevent="submit">
      <div class="grid gap-5 lg:grid-cols-[minmax(0,0.62fr)_minmax(0,1.38fr)]">
        <div class="space-y-4">
          <div>
            <label class="input-label">{{ t('admin.routingSchemes.form.name') }}</label>
            <input
              v-model="form.name"
              type="text"
              required
              maxlength="100"
              class="input"
              :placeholder="t('admin.routingSchemes.form.namePlaceholder')"
            />
          </div>

          <div>
            <label class="input-label">{{ t('admin.routingSchemes.form.platform') }}</label>
            <Select
              :model-value="form.platform"
              :options="platformOptions"
              :disabled="isEdit"
              :placeholder="t('admin.routingSchemes.form.platformPlaceholder')"
              @update:model-value="changePlatform"
            />
            <p class="input-hint">{{ t('admin.routingSchemes.form.platformHint') }}</p>
          </div>

          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="input-label">{{ t('admin.routingSchemes.form.status') }}</label>
              <Select
                v-model="form.status"
                :options="statusOptions"
                :placeholder="t('admin.routingSchemes.form.statusPlaceholder')"
              />
            </div>
            <div>
              <label class="input-label">{{ t('admin.routingSchemes.form.sortOrder') }}</label>
              <input
                v-model.number="form.sort_order"
                type="number"
                step="1"
                class="input"
                min="0"
              />
            </div>
          </div>

          <div>
            <label class="input-label">{{ t('admin.routingSchemes.form.scoreWeights') }}</label>
            <div class="grid grid-cols-2 gap-3">
              <div>
                <label class="input-label">{{ t('admin.routingSchemes.form.ttftWeight') }}</label>
                <input
                  v-model.number="form.ttft_weight"
                  type="number"
                  step="1"
                  class="input"
                  min="0"
                  max="100"
                />
                <p class="input-hint">
                  {{ t('admin.routingSchemes.form.weightShare', { percent: scoreWeights.ttftShare }) }}
                </p>
              </div>
              <div>
                <label class="input-label">{{ t('admin.routingSchemes.form.errorWeight') }}</label>
                <input
                  v-model.number="form.error_weight"
                  type="number"
                  step="1"
                  class="input"
                  min="0"
                  max="100"
                />
                <p class="input-hint">
                  {{ t('admin.routingSchemes.form.weightShare', { percent: scoreWeights.errorShare }) }}
                </p>
              </div>
            </div>
            <p class="input-hint">
              {{ t('admin.routingSchemes.form.scoreWeightHint') }}
            </p>
          </div>

          <div>
            <label class="input-label">{{ t('admin.routingSchemes.form.description') }}</label>
            <textarea
              v-model="form.description"
              rows="3"
              class="input"
              :placeholder="t('admin.routingSchemes.form.descriptionPlaceholder')"
            />
          </div>

          <div class="rounded-xl border border-primary-100 bg-primary-50/60 p-4 dark:border-primary-800/60 dark:bg-primary-500/10">
            <div class="flex items-center gap-2 text-sm font-semibold text-primary-800 dark:text-primary-200">
              <PlatformIcon :platform="form.platform" size="sm" />
              {{ selectedPlatformLabel }}
            </div>
            <div class="mt-2 grid grid-cols-2 gap-3 text-xs text-primary-700 dark:text-primary-300">
              <div>
                <div class="text-primary-500 dark:text-primary-400">{{ t('admin.routingSchemes.summary.groups') }}</div>
                <div class="mt-0.5 text-lg font-semibold">{{ form.items.length }}</div>
              </div>
              <div>
                <div class="text-primary-500 dark:text-primary-400">{{ t('admin.routingSchemes.summary.rules') }}</div>
                <div class="mt-0.5 text-lg font-semibold">{{ totalRules }}</div>
              </div>
            </div>
          </div>
        </div>

        <div>
          <div class="mb-2 flex items-center justify-between">
            <label class="input-label mb-0">{{ t('admin.routingSchemes.form.groups') }}</label>
            <span class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.routingSchemes.form.groupsHint') }}
            </span>
          </div>
          <RoutingSchemeGroupPicker
            :groups="groupsForPlatform"
            :model-value="form.items"
            @update:model-value="setItems"
          />
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex items-center justify-between gap-3 pt-4">
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.routingSchemes.form.requiredHint') }}
        </p>
        <div class="flex gap-3">
          <button type="button" class="btn btn-secondary" @click="emit('close')">
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            form="routing-scheme-form"
            class="btn btn-primary"
            :disabled="!canSave || saving"
          >
            {{ saving ? t('common.submitting') : t(isEdit ? 'common.update' : 'common.create') }}
          </button>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>
