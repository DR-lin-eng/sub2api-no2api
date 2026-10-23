<template>
  <div class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-700">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.customModelConfig.template.endpointRewrite') }}
      </label>
      <div class="flex gap-1">
        <button
          v-for="option in MATCH_MODES"
          :key="option"
          type="button"
          :class="modeClass(option)"
          @click="emit('update:mode', option)"
        >
          {{ t(`admin.customModelConfig.template.endpointRewriteModes.${option}`) }}
        </button>
      </div>
    </div>

    <div class="grid items-start gap-3 md:grid-cols-[1fr_auto_1fr]">
      <div>
        <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
          {{ t('admin.customModelConfig.template.sourceEndpoint') }}
        </label>
        <input
          :value="source"
          class="input w-full font-mono text-sm"
          placeholder="/v1/images/edits"
          @input="onSource"
        />
      </div>
      <div class="hidden pt-7 text-gray-400 md:block">→</div>
      <div>
        <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">
          {{ t('admin.customModelConfig.template.targetEndpoint') }}
        </label>
        <input
          :value="target"
          class="input w-full font-mono text-sm"
          placeholder="/v1/images/generations"
          @input="onTarget"
        />
      </div>
    </div>

    <p class="text-xs text-gray-500 dark:text-gray-400">
      {{
        mode === 'prefix'
          ? t('admin.customModelConfig.template.endpointRewritePrefixHint')
          : t('admin.customModelConfig.template.endpointRewriteExactHint')
      }}
    </p>

    <div
      v-if="preview"
      class="rounded-md border border-gray-200 bg-gray-50 px-3 py-2 font-mono text-xs text-gray-600 dark:border-dark-700 dark:bg-dark-800/60 dark:text-gray-400"
    >
      {{ t('admin.customModelConfig.template.endpointRewritePreview') }}: {{ preview }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

type EndpointMatchMode = 'exact' | 'prefix'

const props = defineProps<{
  mode: EndpointMatchMode
  source: string
  target: string
}>()
const emit = defineEmits<{
  'update:mode': [value: EndpointMatchMode]
  'update:source': [value: string]
  'update:target': [value: string]
}>()
const { t } = useI18n()

const MATCH_MODES: EndpointMatchMode[] = ['exact', 'prefix']

function onSource(event: Event) {
  emit('update:source', (event.target as HTMLInputElement).value)
}

function onTarget(event: Event) {
  emit('update:target', (event.target as HTMLInputElement).value)
}

function modeClass(option: EndpointMatchMode): string {
  const base = 'rounded-lg border px-3 py-1 text-xs font-medium transition-colors'
  return option === props.mode
    ? `${base} border-gray-900 bg-gray-900 text-white dark:border-gray-100 dark:bg-gray-100 dark:text-gray-900`
    : `${base} border-gray-200 text-gray-600 hover:text-gray-900 dark:border-dark-700 dark:text-gray-400 dark:hover:text-gray-100`
}

const preview = computed(() => {
  const target = props.target.trim()
  if (!target) return ''
  const source = props.source.trim()
  if (props.mode === 'prefix') {
    const prefix = source.replace(/\/+$/, '')
    const sample = `${prefix}/example`
    return `${sample} → ${target.replace(/\/+$/, '')}${sample.slice(prefix.length)}`
  }
  return `${source || '*'} → ${target}`
})
</script>
