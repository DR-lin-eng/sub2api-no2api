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

      <div class="grid gap-4 md:grid-cols-3">
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.customModelConfig.template.sourceEndpoint') }}
          </label>
          <input v-model="form.source_endpoint" class="input w-full font-mono text-sm" placeholder="/v1/images/edits" />
        </div>
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.customModelConfig.template.targetEndpoint') }}
          </label>
          <input
            v-model="form.target_endpoint"
            class="input w-full font-mono text-sm"
            placeholder="/v1/images/generations"
          />
        </div>
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.customModelConfig.template.contentType') }}
          </label>
          <select v-model="form.content_type" class="input w-full">
            <option value="preserve">{{ t('admin.customModelConfig.template.preserve') }}</option>
            <option value="application/json">application/json</option>
            <option value="multipart/form-data">multipart/form-data</option>
          </select>
        </div>
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
import { customModelConfigDatasource } from '../../data/datasources/customModelConfigDatasource'
import type { CustomModelRequestTemplate } from '../../domain/entities/customModelConfig'

type BodyMode = 'off' | 'merge' | 'replace'
type ContentTypeMode = 'preserve' | 'application/json' | 'multipart/form-data'
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
const dialogTitle = computed(() =>
  isEdit.value
    ? t('admin.customModelConfig.template.editTitle')
    : t('admin.customModelConfig.template.createTitle')
)

function emptyForm() {
  return {
    name: '',
    description: '',
    source_endpoint: '',
    target_endpoint: '',
    content_type: 'preserve' as ContentTypeMode,
    headers: [] as HeaderEntry[],
    body_mode: 'off' as BodyMode,
    body_json: '{}',
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
      source_endpoint: typeof match.endpoint === 'string' ? match.endpoint : '',
      target_endpoint: typeof upstream.path === 'string' ? upstream.path : '',
      content_type: contentType,
      headers: Object.entries(headers).map(([name, value]) => ({
        name,
        value: typeof value === 'string' ? value : '',
      })),
      body_mode: mode,
      body_json: JSON.stringify(objectValue(body.value), null, 2),
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

async function handleSubmit() {
  if (submitting.value || !form.value.name.trim()) return

  let bodyValue: Record<string, unknown>
  try {
    bodyValue = parseRequestBody()
  } catch {
    alert(t('admin.customModelConfig.template.invalidJson'))
    return
  }

  const headerSet: Record<string, string> = {}
  for (const header of form.value.headers) {
    const name = header.name.trim()
    if (!name) continue
    headerSet[name] = header.value.trim()
  }

  const requestAdapter: Record<string, unknown> = {
    version: 1,
    match: { endpoint: form.value.source_endpoint.trim() },
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
