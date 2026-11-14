<template>
  <fieldset class="flex flex-wrap items-end gap-3 text-xs" :disabled="disabled" data-testid="gpt-image-output-fields">
    <label v-if="visible('output_format')" class="flex flex-col gap-1 text-gray-600 dark:text-gray-300">
      {{ t('common.imageOutput.format') }}
      <select :value="modelValue?.output_format || ''" class="input min-w-[110px] text-sm" data-testid="image-output-format" @change="update('output_format', ($event.target as HTMLSelectElement).value)">
        <option value="">{{ t('common.imageOutput.default') }}</option>
        <option value="png">PNG</option><option value="jpeg">JPEG</option><option value="webp">WebP</option>
        <option value="svg" :disabled="!allowSvg">SVG</option><option value="gif">GIF</option>
      </select>
    </label>
    <template v-if="modelValue?.output_format === 'gif'">
      <label v-if="visible('frame_count')" class="flex flex-col gap-1 text-gray-600 dark:text-gray-300">
        {{ t('common.imageOutput.frames') }}
        <input :value="modelValue.frame_count ?? 9" type="number" min="2" max="16" step="1" class="input w-24 text-sm" data-testid="image-frame-count" @change="update('frame_count', Number(($event.target as HTMLInputElement).value))" />
      </label>
      <label v-if="visible('frame_duration_ms')" class="flex flex-col gap-1 text-gray-600 dark:text-gray-300">
        {{ t('common.imageOutput.duration') }}
        <input :value="modelValue.frame_duration_ms ?? 200" type="number" min="20" max="2000" step="10" class="input w-28 text-sm" data-testid="image-frame-duration" @change="update('frame_duration_ms', Number(($event.target as HTMLInputElement).value))" />
      </label>
      <p class="w-full text-gray-500 dark:text-gray-400">{{ t('common.imageOutput.gifHint') }}</p>
    </template>
    <template v-if="modelValue?.output_format === 'svg'">
      <label v-if="visible('svg')" class="flex flex-col gap-1 text-gray-600 dark:text-gray-300">
        {{ t('common.imageOutput.author') }}
        <input :value="modelValue.svg?.model || ''" class="input max-w-[240px] text-sm" :placeholder="t('common.imageOutput.authorDefault')" data-testid="image-svg-author" @change="updateAuthor(($event.target as HTMLInputElement).value)" />
      </label>
      <label v-if="visible('svg') && modelValue.svg?.model" class="flex flex-col gap-1 text-gray-600 dark:text-gray-300">
        {{ t('common.imageOutput.reasoning') }}
        <select :value="modelValue.svg.reasoning?.effort || ''" class="input text-sm" @change="updateAuthor(modelValue.svg!.model, ($event.target as HTMLSelectElement).value)">
          <option value="">{{ t('common.imageOutput.default') }}</option>
          <option v-for="effort in ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max']" :key="effort" :value="effort">{{ effort }}</option>
        </select>
      </label>
      <p class="w-full text-gray-500 dark:text-gray-400">{{ t('common.imageOutput.svgHint') }}</p>
    </template>
    <p v-if="!allowSvg" class="w-full text-gray-500 dark:text-gray-400">{{ t('common.imageOutput.svgEditHint') }}</p>
  </fieldset>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { normalizeGPTImageOutput, type GPTImageOutputOptions } from '@/core/utils/gptImageOutput'

const props = withDefaults(defineProps<{ modelValue?: GPTImageOutputOptions; disabled?: boolean; allowSvg?: boolean; hiddenFields?: string[] }>(), { allowSvg: true })
const emit = defineEmits<{ 'update:modelValue': [value: GPTImageOutputOptions] }>()
const { t } = useI18n()
const visible = (key: string) => !props.hiddenFields?.includes(key)
function update(key: string, value: unknown) { emit('update:modelValue', normalizeGPTImageOutput({ ...props.modelValue, [key]: value })) }
function updateAuthor(model: string, effort?: string) { update('svg', { model, reasoning: effort ? { effort } : undefined }) }
</script>
