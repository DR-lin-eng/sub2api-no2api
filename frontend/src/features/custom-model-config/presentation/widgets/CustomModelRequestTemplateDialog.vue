<template>
  <BaseDialog :show="visible" :title="dialogTitle" width="extra-wide" @close="emit('close')">
    <form class="space-y-5" @submit.prevent="handleSubmit">
      <div class="grid gap-4 md:grid-cols-2">
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.customModelConfig.template.name') }}
            <span class="text-red-500">*</span>
          </label>
          <input
            v-model="form.name"
            class="input w-full"
            :placeholder="t('admin.customModelConfig.template.namePlaceholder')"
            required
            maxlength="100"
          />
        </div>
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.customModelConfig.template.description') }}
          </label>
          <input
            v-model="form.description"
            class="input w-full"
            :placeholder="t('admin.customModelConfig.template.descriptionPlaceholder')"
            maxlength="500"
          />
        </div>
      </div>

      <div>
        <EndpointRewriteEditor
          v-model:mode="form.endpoint_mode"
          v-model:source="form.source_endpoint"
          v-model:target="form.target_endpoint"
        />
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.customModelConfig.template.endpointRewriteHint') }}
        </p>
      </div>

      <div class="max-w-xs">
        <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.customModelConfig.template.contentType') }}
        </label>
        <select v-model="form.content_type" class="input w-full">
          <option value="preserve">{{ t('admin.customModelConfig.template.preserve') }}</option>
          <option value="application/json">application/json</option>
          <option value="multipart/form-data">multipart/form-data</option>
        </select>
      </div>

      <div>
        <div class="mb-2 flex flex-wrap items-center justify-between gap-3">
          <div>
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.customModelConfig.template.headers') }}
            </label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.customModelConfig.template.headersHint') }}
            </p>
          </div>
          <div class="flex items-center gap-2">
            <button type="button" class="btn btn-secondary" @click="loadSample">
              <Icon name="document" size="sm" class="mr-1" />
              {{ t('admin.customModelConfig.template.loadSample') }}
            </button>
            <button type="button" class="btn btn-secondary" @click="addHeader">
              <Icon name="plus" size="sm" class="mr-1" />
              {{ t('admin.customModelConfig.template.addHeader') }}
            </button>
          </div>
        </div>
        <div class="space-y-2">
          <div v-for="(header, index) in form.headers" :key="index" class="grid grid-cols-[1fr_1fr_auto] gap-2">
            <input
              v-model="header.name"
              class="input font-mono text-sm"
              :placeholder="t('admin.customModelConfig.template.headerName')"
            />
            <input
              v-model="header.value"
              class="input font-mono text-sm"
              :placeholder="t('admin.customModelConfig.template.headerValue')"
            />
            <button
              type="button"
              class="px-2 text-gray-500 hover:text-red-600 dark:text-gray-400 dark:hover:text-red-400"
              :title="t('common.delete')"
              @click="removeHeader(index)"
            >
              <Icon name="trash" size="md" />
            </button>
          </div>
          <p v-if="form.headers.length === 0" class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.customModelConfig.template.noHeaders') }}
          </p>
        </div>
      </div>

      <div>
        <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.customModelConfig.template.bodyMode') }}
        </label>
        <select v-model="form.body_mode" class="input w-full">
          <option value="off">{{ t('admin.customModelConfig.template.bodyModes.off') }}</option>
          <option value="merge">{{ t('admin.customModelConfig.template.bodyModes.merge') }}</option>
          <option value="replace">{{ t('admin.customModelConfig.template.bodyModes.replace') }}</option>
        </select>
      </div>

      <div v-if="form.body_mode !== 'off'">
        <div class="mb-1 flex items-center justify-between gap-2">
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.customModelConfig.template.requestBody') }}
          </label>
          <button type="button" class="btn btn-secondary" @click="formatBody">
            <Icon name="document" size="sm" class="mr-1" />
            {{ t('admin.customModelConfig.template.format') }}
          </button>
        </div>
        <textarea
          v-model="form.body_json"
          class="input min-h-48 w-full font-mono text-xs"
          spellcheck="false"
          required
        />
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{
            form.body_mode === 'merge'
              ? t('admin.customModelConfig.template.mergeHint')
              : t('admin.customModelConfig.template.replaceHint')
          }}
        </p>
        <div class="mt-3 rounded-md border border-gray-200 bg-gray-50 p-3 text-xs dark:border-dark-700 dark:bg-dark-800/60">
          <div class="mb-1 font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.customModelConfig.template.variablesTitle') }}
          </div>
          <code class="block whitespace-pre-wrap font-mono text-gray-600 dark:text-gray-400">{{
            REQUEST_TEMPLATE_VARIABLES
          }}</code>
        </div>
      </div>

      <div>
        <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.customModelConfig.template.attributes') }}
        </label>
        <ModelAttributesEditor v-model="form.attributes_json" :hide-candidates="hideCandidates" />
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.customModelConfig.template.attributesHint') }}
        </p>
      </div>

      <div class="flex justify-end gap-3 pt-2">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button type="submit" class="btn btn-primary" :disabled="submitting">
          <Icon v-if="submitting" name="refresh" size="md" class="mr-2 animate-spin" />
          {{ isEdit ? t('common.save') : t('common.create') }}
        </button>
      </div>
    </form>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { REQUEST_TEMPLATE_VARIABLES } from '../requestTemplateLiterals'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import ModelAttributesEditor from '@/common/widgets/model-attributes/ModelAttributesEditor.vue'
import EndpointRewriteEditor from './EndpointRewriteEditor.vue'
import { customModelConfigDatasource } from '../../data/datasources/customModelConfigDatasource'
import { MEDIA_STUDIO_HIDE_CANDIDATES } from '@/features/media-studio/data/datasources/mediaStudioDatasource'
import type { CustomModelRequestTemplate } from '../../domain/entities/customModelConfig'

type BodyMode = 'off' | 'merge' | 'replace'
type ContentTypeMode = 'preserve' | 'application/json' | 'multipart/form-data'
type EndpointMatchMode = 'exact' | 'prefix'
type HeaderEntry = { name: string; value: string }

const props = defineProps<{
  visible: boolean
  template?: CustomModelRequestTemplate | null
}>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()

const submitting = ref(false)
const form = ref(emptyForm())
const isEdit = computed(() => !!props.template)
// 媒体工坊内置参数候选，交由通用编辑器按模态渲染勾选项。
const hideCandidates = computed(() =>
  Object.fromEntries(
    Object.entries(MEDIA_STUDIO_HIDE_CANDIDATES).map(([modality, keys]) => [
      modality,
      keys.map(key => ({
        key,
        label: t(`admin.customModelConfig.template.attributesHide.options.${key}`, key),
      })),
    ])
  )
)
const dialogTitle = computed(() =>
  isEdit.value
    ? t('admin.customModelConfig.template.editTitle')
    : t('admin.customModelConfig.template.createTitle')
)

function emptyForm() {
  return {
    name: '',
    description: '',
    endpoint_mode: 'exact' as EndpointMatchMode,
    source_endpoint: '',
    target_endpoint: '',
    content_type: 'preserve' as ContentTypeMode,
    headers: [] as HeaderEntry[],
    body_mode: 'off' as BodyMode,
    body_json: '{}',
    attributes_json: '{}',
  }
}

function objectValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {}
}

watch(
  () => props.template,
  (template) => {
    if (!template) {
      form.value = emptyForm()
      return
    }
    const adapter = objectValue(template.request_adapter)
    const match = objectValue(adapter.match)
    const upstream = objectValue(adapter.upstream)
    const headers = objectValue(objectValue(adapter.headers).set)
    const body = objectValue(adapter.body)
    const mode: BodyMode = body.mode === 'merge' || body.mode === 'replace' ? body.mode : 'off'
    const contentType: ContentTypeMode =
      upstream.content_type === 'application/json' || upstream.content_type === 'multipart/form-data'
        ? upstream.content_type
        : 'preserve'
    form.value = {
      name: template.name,
      description: template.description,
      endpoint_mode: match.mode === 'prefix' ? 'prefix' : 'exact',
      source_endpoint: typeof match.endpoint === 'string' ? match.endpoint : '',
      target_endpoint: typeof upstream.path === 'string' ? upstream.path : '',
      content_type: contentType,
      headers: Object.entries(headers).map(([name, value]) => ({
        name,
        value: typeof value === 'string' ? value : '',
      })),
      body_mode: mode,
      body_json: JSON.stringify(objectValue(body.value), null, 2),
      attributes_json: JSON.stringify(objectValue(template.attributes), null, 2),
    }
  },
  { immediate: true }
)

function addHeader() {
  form.value.headers.push({ name: '', value: '' })
}

function removeHeader(index: number) {
  form.value.headers.splice(index, 1)
}

function loadSample() {
  form.value = {
    name: t('admin.customModelConfig.template.sampleName'),
    description: t('admin.customModelConfig.template.sampleDescription'),
    endpoint_mode: 'exact',
    source_endpoint: '/v1/images/edits',
    target_endpoint: '/v1/images/generations',
    content_type: 'application/json',
    headers: [],
    body_mode: 'merge',
    body_json: JSON.stringify(
      {
        image: '{{request.input_images}}',
        quality: 'high',
        response_format: null,
      },
      null,
      2
    ),
    attributes_json: '{}',
  }
}

function formatBody() {
  try {
    form.value.body_json = JSON.stringify(JSON.parse(form.value.body_json || '{}'), null, 2)
  } catch {
    alert(t('admin.customModelConfig.template.invalidJson'))
  }
}

function parseRequestBody(): Record<string, unknown> {
  if (form.value.body_mode === 'off') return {}
  const parsed: unknown = JSON.parse(form.value.body_json || '{}')
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('request body must be an object')
  }
  return parsed as Record<string, unknown>
}

const ATTRIBUTE_MODALITIES = ['image', 'video', 'audio']

function parseAttributesJson(): Record<string, unknown> {
  const parsed: unknown = JSON.parse(form.value.attributes_json || '{}')
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('attributes must be an object')
  }
  const result: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(parsed as Record<string, unknown>)) {
    const modality = key.trim().toLowerCase()
    if (!ATTRIBUTE_MODALITIES.includes(modality)) {
      throw new Error(`unsupported modality ${key}`)
    }
    if (modality in result) {
      throw new Error(`duplicate modality ${key}`)
    }
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      throw new Error(`modality ${modality} must be an object`)
    }
    const block = value as Record<string, unknown>
    for (const blockKey of Object.keys(block)) {
      if (blockKey !== 'fields' && blockKey !== 'mode' && blockKey !== 'hide') {
        throw new Error(`modality ${modality} only accepts "mode", "fields" and "hide"`)
      }
    }
    if (block.fields !== undefined && !Array.isArray(block.fields)) {
      throw new Error(`${modality}.fields must be an array`)
    }
    if (
      block.hide !== undefined
      && (!Array.isArray(block.hide) || block.hide.some(item => typeof item !== 'string' || item.trim() === ''))
    ) {
      throw new Error(`${modality}.hide must be a string array`)
    }
    const normalized: Record<string, unknown> = { fields: block.fields ?? [] }
    if (Array.isArray(block.hide) && block.hide.length > 0) {
      normalized.hide = block.hide
    }
    if (block.mode !== undefined) {
      const mode = typeof block.mode === 'string' ? block.mode.trim().toLowerCase() : ''
      if (mode !== 'overlay' && mode !== 'replace') {
        throw new Error(`modality ${modality} has unsupported render mode`)
      }
      normalized.mode = mode
    }
    result[modality] = normalized
  }
  return result
}

async function handleSubmit() {
  if (submitting.value || !form.value.name.trim()) return

  let bodyValue: Record<string, unknown>
  try {
    bodyValue = parseRequestBody()
  } catch {
    alert(t('admin.customModelConfig.template.invalidJson'))
    return
  }

  let attributes: Record<string, unknown>
  try {
    attributes = parseAttributesJson()
  } catch {
    alert(t('admin.customModelConfig.template.attributesInvalid'))
    return
  }

  const headerSet: Record<string, string> = Object.create(null) as Record<string, string>
  for (const header of form.value.headers) {
    const name = header.name.trim()
    if (!name) continue
    headerSet[name] = header.value.trim()
  }

  const match: Record<string, unknown> = { endpoint: form.value.source_endpoint.trim() }
  if (form.value.endpoint_mode === 'prefix') {
    match.mode = 'prefix'
  }

  const requestAdapter: Record<string, unknown> = {
    version: 1,
    match,
    upstream: {
      path: form.value.target_endpoint.trim(),
      content_type: form.value.content_type,
    },
    headers: { set: headerSet, remove: [] },
    body: {
      mode: form.value.body_mode,
      value: bodyValue,
    },
  }

  submitting.value = true
  try {
    const payload = {
      name: form.value.name.trim(),
      description: form.value.description.trim(),
      request_adapter: requestAdapter,
      attributes,
    }
    if (props.template) {
      await customModelConfigDatasource.updateTemplate(props.template.id, payload)
    } else {
      await customModelConfigDatasource.createTemplate(payload)
    }
    emit('saved')
  } catch (error) {
    console.error('Failed to save request template:', error)
    alert(t('admin.customModelConfig.template.saveFailed'))
  } finally {
    submitting.value = false
  }
}
</script>
