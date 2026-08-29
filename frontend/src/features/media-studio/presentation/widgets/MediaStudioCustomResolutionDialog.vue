<template>
  <BaseDialog
    :show="visible"
    :title="t('mediaStudio.composer.customAspectRatio.title')"
    width="narrow"
    :close-on-click-outside="true"
    @close="emit('close')"
  >
    <form class="space-y-4" @submit.prevent="handleSubmit">
      <div class="grid grid-cols-[1fr_auto_1fr] items-end gap-2">
        <label class="block">
          <span class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('mediaStudio.composer.customAspectRatio.width') }}
          </span>
          <input
            v-model.number="width"
            type="number"
            min="1"
            max="1000"
            step="1"
            inputmode="numeric"
            class="input w-full"
            required
          />
        </label>
        <span class="pb-2 text-gray-400">:</span>
        <label class="block">
          <span class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('mediaStudio.composer.customAspectRatio.height') }}
          </span>
          <input
            v-model.number="height"
            type="number"
            min="1"
            max="1000"
            step="1"
            inputmode="numeric"
            class="input w-full"
            required
          />
        </label>
      </div>

      <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">
        {{ t('mediaStudio.composer.customAspectRatio.hint') }}
      </p>
      <p v-if="!isValid" class="text-xs text-red-600 dark:text-red-300">
        {{ t('mediaStudio.composer.customAspectRatio.invalid') }}
      </p>

      <div class="flex justify-end gap-3 pt-2">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button type="submit" class="btn btn-primary" :disabled="!isValid">
          {{ t('mediaStudio.composer.customAspectRatio.add') }}
        </button>
      </div>
    </form>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'

const props = defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  close: []
  save: [value: string]
}>()

const { t } = useI18n()
const width = ref(1)
const height = ref(1)

const isValid = computed(() => {
  const values = [width.value, height.value]
  return values.every(value => (
    Number.isInteger(value) &&
    value >= 1 &&
    value <= 1000
  )) && width.value / height.value <= 3 && height.value / width.value <= 3
})

watch(() => props.visible, (visible) => {
  if (visible) {
    width.value = 1
    height.value = 1
  }
})

function handleSubmit() {
  if (!isValid.value) return
  emit('save', `${width.value}:${height.value}`)
}
</script>
