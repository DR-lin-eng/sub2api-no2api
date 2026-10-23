<template>
  <label v-if="field.ui !== 'hidden'" class="flex w-full min-w-0 flex-col gap-1.5">
    <span class="truncate text-xs font-medium text-gray-500 dark:text-gray-400">
      {{ label }}<span v-if="field.required" class="ml-0.5 text-red-500">*</span>
    </span>

    <select
      v-if="field.type === 'enum'"
      :value="textValue"
      class="attribute-select h-9 w-full min-w-0 rounded-lg border border-gray-200 bg-white py-0 pl-2.5 pr-9 text-sm font-medium text-gray-700 shadow-sm outline-none transition-colors hover:border-gray-300 focus:border-gray-400 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200 dark:hover:border-dark-600 dark:focus:border-dark-500"
      @change="onEnumChange"
    >
      <option value="">{{ t('common.selectOption') }}</option>
      <option v-for="option in enumValues" :key="option" :value="option">{{ option }}</option>
    </select>

    <span v-else-if="field.type === 'boolean'" class="flex h-9 items-center">
      <input
        type="checkbox"
        class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
        :checked="booleanValue"
        @change="onBooleanChange"
      />
    </span>

    <input
      v-else-if="field.type === 'integer' || field.type === 'number'"
      type="number"
      class="h-9 w-full min-w-0 rounded-lg border border-gray-200 bg-white px-2.5 text-sm font-medium text-gray-700 shadow-sm outline-none transition-colors hover:border-gray-300 focus:border-gray-400 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200 dark:hover:border-dark-600 dark:focus:border-dark-500"
      :value="numberValue"
      :min="field.min"
      :max="field.max"
      :step="field.type === 'integer' ? 1 : 'any'"
      @input="onNumberInput"
    />

    <input
      v-else
      type="text"
      class="h-9 w-full min-w-0 rounded-lg border border-gray-200 bg-white px-2.5 text-sm font-medium text-gray-700 shadow-sm outline-none transition-colors hover:border-gray-300 focus:border-gray-400 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200 dark:hover:border-dark-600 dark:focus:border-dark-500"
      :value="textValue"
      @input="onTextInput"
    />
  </label>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ModelAttributeField } from './types'

const props = defineProps<{
  field: ModelAttributeField
  modelValue: string | number | boolean | undefined
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string | number | boolean | undefined]
}>()

const { t } = useI18n()

const label = computed(() => props.field.label ?? props.field.key)
const enumValues = computed(() => props.field.values ?? [])

const textValue = computed(() => (
  props.modelValue === undefined ? '' : String(props.modelValue)
))

const numberValue = computed(() => (
  typeof props.modelValue === 'number' ? props.modelValue : ''
))

const booleanValue = computed(() => props.modelValue === true)

function onEnumChange(event: Event) {
  const value = (event.target as HTMLSelectElement).value
  emit('update:modelValue', value === '' ? undefined : value)
}

function onTextInput(event: Event) {
  emit('update:modelValue', (event.target as HTMLInputElement).value)
}

function onNumberInput(event: Event) {
  const raw = (event.target as HTMLInputElement).value
  if (!raw.trim()) {
    emit('update:modelValue', undefined)
    return
  }
  const parsed = Number(raw)
  emit('update:modelValue', Number.isFinite(parsed) ? parsed : undefined)
}

function onBooleanChange(event: Event) {
  emit('update:modelValue', (event.target as HTMLInputElement).checked)
}
</script>

<style scoped>
/* 隐藏原生箭头后自绘，保证箭头与右边缘的间距可控 */
.attribute-select {
  @apply appearance-none bg-no-repeat;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%236b7280' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E");
  background-position: right 0.75rem center;
  background-size: 1rem 1rem;
}

:global(.dark) .attribute-select {
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%239ca3af' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E");
}
</style>
