<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueDraggable } from 'vue-draggable-plus'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import GroupBadge from '@/common/widgets/data/GroupBadge.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import PlatformIcon from '@/common/widgets/icons/PlatformIcon.vue'
import { platformBadgeLightClass } from '@/core/utils/platformColors'
import type { ApiKeyGroupBinding } from '@/types'
import {
  getKeyGroupProvider,
  KEY_GROUP_PROVIDER_ICONS,
  KEY_GROUP_PROVIDERS,
  type KeyGroupProvider,
} from '../keyGroupProviders'
import type {
  GroupOption,
  KeyQuickSchemeOption,
} from '../keysPageContext'

const props = withDefaults(defineProps<{
  modelValue: ApiKeyGroupBinding[]
  groupOptions: GroupOption[]
  quickSchemes?: KeyQuickSchemeOption[]
  quickSchemeId?: number | null
  filterByProvider?: boolean
}>(), {
  quickSchemes: () => [],
  quickSchemeId: null,
  filterByProvider: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: ApiKeyGroupBinding[]]
  'update:quickSchemeId': [value: number | null]
}>()

const { t } = useI18n()
const showPicker = ref(false)
const showQuickSelect = ref(false)
const searchQuery = ref('')
const createProvider = ref<KeyGroupProvider>('anthropic')

const bindings = computed({
  get: () => props.modelValue,
  set: (value: ApiKeyGroupBinding[]) => emit('update:modelValue', value)
})

const optionByID = computed(() => new Map(props.groupOptions.map(option => [option.value, option])))
const selectedIDs = computed(() => new Set(bindings.value.map(binding => binding.group_id)))
const primaryOption = computed(() => optionByID.value.get(bindings.value[0]?.group_id))
const providerOptions = computed(() => KEY_GROUP_PROVIDERS.map(value => ({
  value,
  count: props.groupOptions.filter(option => getKeyGroupProvider(option.platform) === value).length,
})))
const providerGroupOptions = computed(() => props.filterByProvider
  ? props.groupOptions.filter(option => getKeyGroupProvider(option.platform) === createProvider.value)
  : props.groupOptions)

const addableOptions = computed(() => {
  if (bindings.value.length >= 20 || primaryOption.value?.subscriptionType === 'subscription') {
    return []
  }

  const query = searchQuery.value.trim().toLowerCase()
  return providerGroupOptions.value.filter((option) => {
    if (selectedIDs.value.has(option.value)) return false
    if (primaryOption.value) {
      if (option.platform !== primaryOption.value.platform || option.subscriptionType !== 'standard') {
        return false
      }
    }
    if (!query) return true
    return option.label.toLowerCase().includes(query) || option.description?.toLowerCase().includes(query)
  })
})

const selectCreateProvider = (provider: KeyGroupProvider) => {
  if (createProvider.value === provider) return
  createProvider.value = provider
  if (bindings.value.length > 0) {
    bindings.value = []
  }
  emit('update:quickSchemeId', null)
  searchQuery.value = ''
  showPicker.value = false
  showQuickSelect.value = false
}

const selectedQuickScheme = computed(() =>
  props.quickSchemes.find(scheme => scheme.id === props.quickSchemeId))

const quickSchemeOptions = computed((): KeyQuickSchemeOption[] => {
  const platform = primaryOption.value?.platform
  return props.quickSchemes.filter((scheme) => {
    if (platform) return scheme.platform === platform
    if (props.filterByProvider) return getKeyGroupProvider(scheme.platform) === createProvider.value
    return true
  })
})

const toggleQuickSelect = () => {
  showQuickSelect.value = !showQuickSelect.value
  if (showQuickSelect.value) {
    showPicker.value = false
    searchQuery.value = ''
  }
}

const applyQuickScheme = (scheme: KeyQuickSchemeOption) => {
  bindings.value = []
  emit('update:quickSchemeId', scheme.id)
  showQuickSelect.value = false
}

const clearQuickScheme = () => {
  emit('update:quickSchemeId', null)
  bindings.value = []
}

const formatRate = (value: number) => Number.isInteger(value) ? value.toString() : value.toFixed(2)

const schemeRateRange = (scheme: KeyQuickSchemeOption): string => {
  const min = scheme.minRate
  const max = scheme.maxRate
  if (min == null && max == null) return '—'
  const low = min ?? max
  const high = max ?? min
  if (low == null || high == null) return '—'
  return low === high ? `${formatRate(low)}x` : `${formatRate(low)}x – ${formatRate(high)}x`
}

watch([() => props.filterByProvider, providerOptions], ([enabled, providers]) => {
  if (!enabled) return
  const selectedAvailable = providers.some(option => option.value === createProvider.value && option.count > 0)
  if (!selectedAvailable) {
    selectCreateProvider(providers.find(option => option.count > 0)?.value ?? 'anthropic')
  }
  if (bindings.value.some(binding => {
    const option = optionByID.value.get(binding.group_id)
    return option && getKeyGroupProvider(option.platform) !== createProvider.value
  })) {
    bindings.value = []
  }
}, { immediate: true })

const optionFor = (groupID: number) => optionByID.value.get(groupID)

const addGroup = (groupID: number) => {
  bindings.value = [...bindings.value, { group_id: groupID, max_rate_multiplier: null }]
  searchQuery.value = ''
  showPicker.value = false
}

const removeGroup = (index: number) => {
  bindings.value = bindings.value.filter((_, currentIndex) => currentIndex !== index)
}

const moveGroup = (index: number, offset: -1 | 1) => {
  const target = index + offset
  if (target < 0 || target >= bindings.value.length) return
  const next = [...bindings.value]
  ;[next[index], next[target]] = [next[target], next[index]]
  bindings.value = next
}

const updateRateCeiling = (index: number, event: Event) => {
  const raw = (event.target as HTMLInputElement).value.trim()
  const value = raw === '' ? null : Number(raw)
  bindings.value = bindings.value.map((binding, currentIndex) => currentIndex === index
    ? { ...binding, max_rate_multiplier: value }
    : binding)
}

const togglePicker = () => {
  showPicker.value = !showPicker.value
  if (showPicker.value) showQuickSelect.value = false
  if (!showPicker.value) searchQuery.value = ''
}
</script>

<template>
  <div class="space-y-3" data-test="key-group-bindings-editor">
    <fieldset v-if="props.filterByProvider" data-test="key-group-provider-filter">
      <legend class="input-label">{{ t('keys.providerLabel') }}</legend>
      <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <label
          v-for="provider in providerOptions"
          :key="provider.value"
          class="relative min-w-0"
          :class="provider.count === 0 ? 'cursor-not-allowed' : 'cursor-pointer'"
        >
          <input
            type="radio"
            name="key-provider"
            :value="provider.value"
            :checked="createProvider === provider.value"
            :disabled="provider.count === 0"
            class="peer sr-only"
            @change="selectCreateProvider(provider.value)"
          />
          <span
            class="flex h-full min-h-20 flex-col items-center justify-center gap-2 rounded-lg border border-gray-200 bg-white px-2 py-2 text-center transition-colors peer-checked:border-primary-500 peer-checked:bg-primary-50/60 peer-checked:ring-1 peer-checked:ring-primary-500 peer-focus-visible:outline peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-primary-500 peer-disabled:opacity-40 dark:border-dark-600 dark:bg-dark-800 dark:peer-checked:border-primary-500 dark:peer-checked:bg-primary-500/10"
            :class="provider.count > 0 && 'hover:border-primary-300 dark:hover:border-primary-700'"
          >
            <span class="flex h-6 items-center justify-center gap-1" aria-hidden="true">
              <span
                v-for="platform in KEY_GROUP_PROVIDER_ICONS[provider.value]"
                :key="platform"
                class="flex h-6 w-6 items-center justify-center rounded-md"
                :class="platformBadgeLightClass(platform)"
              >
                <PlatformIcon :platform="platform" size="md" />
              </span>
            </span>
            <span class="max-w-full text-xs font-semibold leading-4 text-gray-800 dark:text-gray-100">
              {{ t(`keys.providers.${provider.value}`) }}
            </span>
          </span>
          <span
            v-if="createProvider === provider.value"
            class="absolute right-1.5 top-1.5 flex h-4 w-4 items-center justify-center rounded-full bg-primary-500 text-white"
            aria-hidden="true"
          >
            <Icon name="check" size="xs" :stroke-width="3" />
          </span>
        </label>
      </div>
      <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400" aria-live="polite">
        {{ props.groupOptions.length === 0
          ? t('common.noGroupsAvailable')
          : t(`keys.providerHints.${createProvider}`) }}
      </p>
    </fieldset>

    <VueDraggable
      v-if="!selectedQuickScheme"
      v-model="bindings"
      :animation="180"
      handle=".key-group-drag-handle"
      class="space-y-2"
      data-test="key-group-bindings-list"
    >
      <div
        v-for="(binding, index) in bindings"
        :key="binding.group_id"
        class="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-800 sm:grid-cols-[auto_minmax(0,1fr)_minmax(9.5rem,auto)_auto]"
        :data-test="`key-group-binding-${binding.group_id}`"
      >
        <button
          type="button"
          class="key-group-drag-handle cursor-grab touch-none rounded p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 active:cursor-grabbing dark:hover:bg-dark-700 dark:hover:text-gray-200"
          :title="t('keys.groupBindings.dragToReorder')"
          :aria-label="t('keys.groupBindings.dragToReorder')"
        >
          <Icon name="arrowsUpDown" size="sm" :stroke-width="2" />
        </button>

        <div class="min-w-0">
          <div class="flex min-w-0 flex-wrap items-center gap-1.5">
            <span
              :class="[
                'shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-semibold',
                index === 0
                  ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
              ]"
            >
              {{ index === 0
                ? t('keys.groupBindings.primary')
                : t('keys.groupBindings.fallbackPosition', { position: index }) }}
            </span>
            <GroupBadge
              v-if="optionFor(binding.group_id)"
              class="max-w-full"
              :name="optionFor(binding.group_id)!.label"
              :platform="optionFor(binding.group_id)!.platform"
              :subscription-type="optionFor(binding.group_id)!.subscriptionType"
              :rate-multiplier="optionFor(binding.group_id)!.rate"
              :user-rate-multiplier="optionFor(binding.group_id)!.userRate"
              :peak-rate-enabled="optionFor(binding.group_id)!.peakRateEnabled"
              :peak-start="optionFor(binding.group_id)!.peakStart"
              :peak-end="optionFor(binding.group_id)!.peakEnd"
              :peak-rate-multiplier="optionFor(binding.group_id)!.peakRateMultiplier"
            />
            <span v-else class="text-sm font-medium text-gray-900 dark:text-white">
              #{{ binding.group_id }}
            </span>
          </div>
          <p
            v-if="optionFor(binding.group_id)?.description"
            class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400"
            :title="optionFor(binding.group_id)?.description || undefined"
          >
            {{ optionFor(binding.group_id)?.description }}
          </p>
        </div>

        <label class="col-span-2 flex min-w-0 items-center justify-end gap-2 sm:col-span-1">
          <span class="whitespace-nowrap text-xs text-gray-500 dark:text-gray-400">
            {{ t('keys.groupBindings.rateProtection') }}
          </span>
          <input
            :value="binding.max_rate_multiplier ?? ''"
            type="number"
            min="0.000001"
            step="any"
            class="input h-9 w-28 min-w-0 py-1 text-center"
            :placeholder="t('keys.groupBindings.unlimited')"
            :aria-label="t('keys.groupBindings.rateProtectionFor', { group: optionFor(binding.group_id)?.label || binding.group_id })"
            :data-test="`key-group-rate-ceiling-${binding.group_id}`"
            @input="updateRateCeiling(index, $event)"
          />
        </label>

        <div class="flex items-center justify-end gap-0.5">
          <div class="flex flex-col">
            <button
              type="button"
              class="rounded p-0.5 text-gray-400 hover:bg-gray-100 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-30 dark:hover:bg-dark-700 dark:hover:text-gray-200"
              :disabled="index === 0"
              :title="t('keys.groupBindings.moveUp')"
              :aria-label="t('keys.groupBindings.moveUp')"
              :data-test="`key-group-move-up-${binding.group_id}`"
              @click="moveGroup(index, -1)"
            >
              <Icon name="chevronUp" size="xs" :stroke-width="2" />
            </button>
            <button
              type="button"
              class="rounded p-0.5 text-gray-400 hover:bg-gray-100 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-30 dark:hover:bg-dark-700 dark:hover:text-gray-200"
              :disabled="index === bindings.length - 1"
              :title="t('keys.groupBindings.moveDown')"
              :aria-label="t('keys.groupBindings.moveDown')"
              :data-test="`key-group-move-down-${binding.group_id}`"
              @click="moveGroup(index, 1)"
            >
              <Icon name="chevronDown" size="xs" :stroke-width="2" />
            </button>
          </div>
          <button
            type="button"
            class="rounded p-2 text-red-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-950/30"
            :title="t('keys.groupBindings.removeGroup')"
            :aria-label="t('keys.groupBindings.removeGroup')"
            :data-test="`key-group-remove-${binding.group_id}`"
            @click="removeGroup(index)"
          >
            <Icon name="trash" size="sm" :stroke-width="2" />
          </button>
        </div>
      </div>
    </VueDraggable>

    <div v-if="!selectedQuickScheme" class="flex flex-wrap items-start gap-2">
      <div class="relative">
        <button
          v-if="bindings.length < 20 && primaryOption?.subscriptionType !== 'subscription'"
          type="button"
          class="btn btn-secondary btn-sm"
          data-test="key-group-add"
          @click="togglePicker"
        >
          <Icon name="plus" size="sm" :stroke-width="2" />
          {{ t('keys.groupBindings.addGroup') }}
        </button>

      <div
        v-if="showPicker"
        class="absolute left-0 top-full z-20 mt-2 w-[min(24rem,calc(100vw-2rem))] overflow-hidden rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
        data-test="key-group-picker"
      >
        <div class="border-b border-gray-100 p-2 dark:border-dark-700">
          <div class="relative">
            <Icon name="search" size="sm" class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-400" />
            <input
              v-model="searchQuery"
              type="search"
              class="input h-9 pl-8"
              :placeholder="t('keys.searchGroup')"
              data-test="key-group-picker-search"
              @keydown.esc="showPicker = false"
            />
          </div>
        </div>
        <div class="max-h-52 overflow-y-auto p-1">
          <button
            v-for="option in addableOptions"
            :key="option.value"
            type="button"
            class="flex w-full items-start gap-2 rounded-md px-2 py-2 text-left hover:bg-gray-50 dark:hover:bg-dark-700"
            :data-test="`key-group-option-${option.value}`"
            @click="addGroup(option.value)"
          >
            <div class="min-w-0 flex-1">
              <GroupBadge
                class="max-w-full"
                :name="option.label"
                :platform="option.platform"
                :subscription-type="option.subscriptionType"
                :rate-multiplier="option.rate"
                :user-rate-multiplier="option.userRate"
                :peak-rate-enabled="option.peakRateEnabled"
                :peak-start="option.peakStart"
                :peak-end="option.peakEnd"
                :peak-rate-multiplier="option.peakRateMultiplier"
              />
              <p v-if="option.description" class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">
                {{ option.description }}
              </p>
            </div>
          </button>
          <p v-if="addableOptions.length === 0" class="px-3 py-5 text-center text-sm text-gray-500 dark:text-gray-400">
            {{ t('keys.noGroupFound') }}
          </p>
        </div>
      </div>
      </div>

      <div v-if="primaryOption?.subscriptionType !== 'subscription'" class="relative">
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          data-test="key-group-quick-select"
          @click="toggleQuickSelect"
        >
          <Icon name="bolt" size="sm" :stroke-width="2" />
          {{ t('keys.groupBindings.quickSelect') }}
        </button>

      </div>
    </div>

    <div
      v-if="selectedQuickScheme"
      class="rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-800"
      data-test="key-quick-scheme-selected"
    >
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-1.5">
            <span
              v-if="selectedQuickScheme.platform"
              class="flex h-5 w-5 shrink-0 items-center justify-center rounded"
              :class="platformBadgeLightClass(selectedQuickScheme.platform)"
              aria-hidden="true"
            >
              <PlatformIcon :platform="selectedQuickScheme.platform" size="sm" />
            </span>
            <span class="text-sm font-medium text-gray-900 dark:text-white">
              {{ selectedQuickScheme.name }}
            </span>
            <span class="rounded-md bg-primary-50 px-1.5 py-0.5 text-[10px] font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
              {{ schemeRateRange(selectedQuickScheme) }}
            </span>
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ selectedQuickScheme.description || t('keys.groupBindings.quickSelectNoDescription') }}
          </p>
        </div>
      </div>
      <div class="mt-3 flex flex-wrap gap-2">
        <button type="button" class="btn btn-secondary btn-sm" @click="toggleQuickSelect">
          <Icon name="bolt" size="sm" :stroke-width="2" />
          {{ t('keys.groupBindings.quickSelectReselect') }}
        </button>
        <button type="button" class="btn btn-secondary btn-sm" @click="clearQuickScheme">
          <Icon name="grid" size="sm" :stroke-width="2" />
          {{ t('keys.groupBindings.quickSelectCustom') }}
        </button>
      </div>
    </div>

    <BaseDialog
      :show="showQuickSelect"
      :title="t('keys.groupBindings.quickSelectTitle')"
      width="normal"
      @close="showQuickSelect = false"
    >
      <div class="space-y-3">
        <div v-if="quickSchemeOptions.length > 0" class="space-y-2">
          <button
            v-for="scheme in quickSchemeOptions"
            :key="scheme.id"
            type="button"
            class="w-full rounded-xl border border-gray-200 bg-white p-4 text-left transition-colors hover:border-primary-300 hover:bg-primary-50/50 dark:border-dark-600 dark:bg-dark-900 dark:hover:border-primary-700 dark:hover:bg-primary-500/10"
            :data-test="`key-group-quick-scheme-${scheme.id}`"
            @click="applyQuickScheme(scheme)"
          >
            <div class="flex items-center justify-between gap-3">
              <span class="flex min-w-0 items-center gap-2">
                <span
                  v-if="scheme.platform"
                  class="flex h-5 w-5 shrink-0 items-center justify-center rounded"
                  :class="platformBadgeLightClass(scheme.platform)"
                  aria-hidden="true"
                >
                  <PlatformIcon :platform="scheme.platform" size="sm" />
                </span>
                <span class="min-w-0 truncate text-base font-semibold text-gray-900 dark:text-white">
                  {{ scheme.name }}
                </span>
              </span>
              <span class="flex-shrink-0 rounded-full bg-primary-50 px-2 py-0.5 text-xs font-medium text-primary-700 dark:bg-primary-500/10 dark:text-primary-300">
                {{ schemeRateRange(scheme) }}
              </span>
            </div>
            <p class="mt-1 text-sm leading-6 text-gray-500 dark:text-gray-400">
              {{ scheme.description || t('keys.groupBindings.quickSelectNoDescription') }}
            </p>
          </button>
        </div>

        <p
          v-else
          class="rounded-lg border border-dashed border-gray-200 px-3 py-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
        >
          {{ t('keys.groupBindings.quickSelectEmpty') }}
        </p>
      </div>

      <template #footer>
        <div class="flex justify-end">
          <button type="button" class="btn btn-secondary" @click="showQuickSelect = false">
            {{ t('common.close') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <p v-if="!selectedQuickScheme" class="input-hint">{{ t('keys.groupBindings.orderHint') }}</p>
    <p v-if="!selectedQuickScheme && primaryOption?.subscriptionType === 'subscription'" class="input-hint">
      {{ t('keys.groupBindings.subscriptionSingleHint') }}
    </p>
  </div>
</template>
