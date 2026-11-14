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

const form = ref<RoutingSchemeDraft>({
  name: '',
  description: '',
  platform: 'deepseek',
  status: 'active',
  sort_order: 10,
  sort_mode: 'score',
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
        sort_mode: source.sort_mode ?? 'score',
        items: source.items.map(item => ({ ...item })),
      }
    : {
        name: '',
        description: '',
        platform: props.groups[0]?.platform ?? 'deepseek',
        status: 'active',
        sort_order: 10,
        sort_mode: 'score',
        items: [],
      }
}

watch(() => props.show, (show) => {
  if (show) reset()
}, { immediate: true })

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
  && form.value.items.length > 0,
)

const submit = () => {
  if (!canSave.value || props.saving) return
  emit('save', {
    ...form.value,
    name: form.value.name.trim(),
    description: form.value.description.trim(),
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
            :sort-mode="form.sort_mode"
            @update:model-value="setItems"
            @update:sort-mode="form.sort_mode = $event"
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
