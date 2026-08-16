<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueDraggable } from 'vue-draggable-plus'
import GroupBadge from '@/common/widgets/data/GroupBadge.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { ApiKeyGroupBinding } from '@/types'
import type { GroupOption } from '../keysPageContext'

const props = defineProps<{
  modelValue: ApiKeyGroupBinding[]
  groupOptions: GroupOption[]
}>()

const emit = defineEmits<{
  'update:modelValue': [value: ApiKeyGroupBinding[]]
}>()

const { t } = useI18n()
const showPicker = ref(false)
const searchQuery = ref('')

const bindings = computed({
  get: () => props.modelValue,
  set: (value: ApiKeyGroupBinding[]) => emit('update:modelValue', value)
})

const optionByID = computed(() => new Map(props.groupOptions.map(option => [option.value, option])))
const selectedIDs = computed(() => new Set(bindings.value.map(binding => binding.group_id)))
const primaryOption = computed(() => optionByID.value.get(bindings.value[0]?.group_id))

const addableOptions = computed(() => {
  if (bindings.value.length >= 20 || primaryOption.value?.subscriptionType === 'subscription') {
    return []
  }

  const query = searchQuery.value.trim().toLowerCase()
  return props.groupOptions.filter((option) => {
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
  if (!showPicker.value) searchQuery.value = ''
}
</script>

<template>
  <div class="space-y-3" data-test="key-group-bindings-editor">
    <VueDraggable
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
        class="mt-2 overflow-hidden rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
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

    <p class="input-hint">{{ t('keys.groupBindings.orderHint') }}</p>
    <p v-if="primaryOption?.subscriptionType === 'subscription'" class="input-hint">
      {{ t('keys.groupBindings.subscriptionSingleHint') }}
    </p>
  </div>
</template>
