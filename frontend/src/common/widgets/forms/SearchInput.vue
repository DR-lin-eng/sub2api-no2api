<template>
  <div class="relative w-full">
    <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
      <Icon name="search" size="md" class="text-gray-400" />
    </div>
    <input
      :value="modelValue"
      type="text"
      class="input pl-10"
      :placeholder="placeholder"
      @input="handleInput"
      @compositionstart="composing = true"
      @compositionend="handleCompositionEnd"
    />
  </div>
</template>

<script setup lang="ts">
import { useDebounceFn } from '@vueuse/core'
import { ref } from 'vue'
import Icon from '@/common/widgets/icons/Icon.vue'

const props = withDefaults(defineProps<{
  modelValue: string
  placeholder?: string
  debounceMs?: number
}>(), {
  placeholder: 'Search...',
  debounceMs: 300
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'search', value: string): void
}>()

const composing = ref(false)
const debouncedEmitSearch = useDebounceFn((value: string) => {
  if (!composing.value) emit('search', value)
}, props.debounceMs)

const handleInput = (event: Event) => {
  if (composing.value || (event as InputEvent).isComposing) return
  const value = (event.target as HTMLInputElement).value
  emit('update:modelValue', value)
  debouncedEmitSearch(value)
}

const handleCompositionEnd = (event: CompositionEvent) => {
  composing.value = false
  handleInput(event)
}
</script>
