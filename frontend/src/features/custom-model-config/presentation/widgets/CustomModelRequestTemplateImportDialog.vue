<template>
  <BaseDialog
    :show="visible"
    :title="t('admin.customModelConfig.template.importTitle')"
    width="extra-wide"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div class="rounded-md border border-blue-200 bg-blue-50 p-3 text-xs text-blue-800 dark:border-blue-900/50 dark:bg-blue-900/20 dark:text-blue-200">
        {{ t('admin.customModelConfig.template.importHint') }}
      </div>

      <div>
        <div class="mb-1 flex items-center justify-between gap-2">
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.customModelConfig.template.importInput') }}
          </label>
          <button type="button" class="btn btn-secondary" @click="formatInput">
            <Icon name="document" size="sm" class="mr-1" />
            {{ t('admin.customModelConfig.template.format') }}
          </button>
        </div>
        <textarea
          v-model="source"
          class="input min-h-56 w-full font-mono text-xs"
          spellcheck="false"
          :placeholder="importPlaceholder"
        />
      </div>

      <div v-if="parsedTemplate" class="space-y-2">
        <div class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.customModelConfig.template.importPreview') }}
        </div>
        <div class="grid gap-3 md:grid-cols-2">
          <div class="rounded-md border border-gray-200 p-3 dark:border-dark-700">
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.customModelConfig.template.name') }}
            </div>
            <div class="mt-1 text-sm text-gray-900 dark:text-white">{{ parsedTemplate.name }}</div>
          </div>
          <div class="rounded-md border border-gray-200 p-3 dark:border-dark-700">
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.customModelConfig.template.description') }}
            </div>
            <div class="mt-1 text-sm text-gray-900 dark:text-white">
              {{ parsedTemplate.description || t('admin.customModelConfig.template.noDescription') }}
            </div>
          </div>
        </div>
        <pre class="max-h-64 overflow-auto rounded-md bg-gray-900 p-3 text-xs text-gray-100">{{ formattedAdapter }}</pre>
      </div>

      <p v-if="errorMessage" class="text-sm text-red-600 dark:text-red-400">
        {{ errorMessage }}
      </p>

      <div class="flex justify-end gap-3 pt-2">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-primary"
          :disabled="!source.trim() || submitting"
          @click="handleImport"
        >
          <Icon v-if="submitting" name="refresh" size="md" class="mr-2 animate-spin" />
          {{ t('admin.customModelConfig.template.importAction') }}
        </button>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import { customModelConfigDatasource } from '../../data/datasources/customModelConfigDatasource'
import { REQUEST_TEMPLATE_IMPORT_PLACEHOLDER } from '../requestTemplateLiterals'

defineProps<{ visible: boolean }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()

const source = ref('')
const errorMessage = ref('')
const submitting = ref(false)
const parsedTemplate = ref<{
  name: string
  description: string
  request_adapter: Record<string, unknown>
} | null>(null)

const importPlaceholder = REQUEST_TEMPLATE_IMPORT_PLACEHOLDER
const formattedAdapter = computed(() =>
  parsedTemplate.value ? JSON.stringify(parsedTemplate.value.request_adapter, null, 2) : ''
)

function isObject(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function normalizeAdapter(value: unknown): Record<string, unknown> {
  if (!isObject(value)) {
    throw new Error(t('admin.customModelConfig.template.invalidTemplate'))
  }
  const adapter = { ...value }
  const match = isObject(adapter.match) ? adapter.match : {}
  const upstream = isObject(adapter.upstream) ? adapter.upstream : {}
  const headers = isObject(adapter.headers) ? adapter.headers : {}
  const body = isObject(adapter.body) ? adapter.body : {}
  return {
    version: typeof adapter.version === 'number' ? adapter.version : 1,
    match: {
      endpoint: typeof match.endpoint === 'string' ? match.endpoint : '',
    },
    upstream: {
      path: typeof upstream.path === 'string' ? upstream.path : '',
      content_type: typeof upstream.content_type === 'string' ? upstream.content_type : 'preserve',
    },
    headers: {
      set: isObject(headers.set) ? headers.set : {},
      remove: Array.isArray(headers.remove) ? headers.remove : [],
    },
    body: {
      mode: body.mode === 'merge' || body.mode === 'replace' ? body.mode : 'off',
      value: isObject(body.value) ? body.value : {},
    },
  }
}

function formatInput() {
  errorMessage.value = ''
  parsedTemplate.value = null
  try {
    parsedTemplate.value = parseTemplateSource()
    source.value = JSON.stringify(
      {
        name: parsedTemplate.value.name,
        description: parsedTemplate.value.description,
        request_adapter: parsedTemplate.value.request_adapter,
      },
      null,
      2
    )
  } catch (error) {
    errorMessage.value =
      error instanceof Error
        ? error.message
        : t('admin.customModelConfig.template.invalidTemplate')
  }
}

function parseTemplateSource() {
  const input: unknown = JSON.parse(source.value.trim())
  if (!isObject(input)) throw new Error(t('admin.customModelConfig.template.invalidTemplate'))

  const adapter = isObject(input.request_adapter) ? input.request_adapter : input
  return {
    name:
      typeof input.name === 'string' && input.name.trim()
        ? input.name.trim()
        : t('admin.customModelConfig.template.importedName'),
    description: typeof input.description === 'string' ? input.description.trim() : '',
    request_adapter: normalizeAdapter(adapter),
  }
}

async function handleImport() {
  if (!source.value.trim() || submitting.value) return
  submitting.value = true
  errorMessage.value = ''
  try {
    const template = parseTemplateSource()
    parsedTemplate.value = template
    await customModelConfigDatasource.createTemplate(template)
    emit('saved')
  } catch (error) {
    console.error('Failed to import request template:', error)
    errorMessage.value =
      error instanceof Error
        ? error.message
        : t('admin.customModelConfig.template.importFailed')
  } finally {
    submitting.value = false
  }
}
</script>
