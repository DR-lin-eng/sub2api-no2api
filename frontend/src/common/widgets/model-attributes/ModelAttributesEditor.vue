<template>
  <div class="space-y-3">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div class="inline-flex rounded-lg border border-gray-200 p-0.5 dark:border-dark-700">
        <button type="button" :class="modeButtonClass('visual')" @click="setMode('visual')">
          {{ t('admin.customModelConfig.template.attributesVisual') }}
        </button>
        <button type="button" :class="modeButtonClass('json')" @click="setMode('json')">
          {{ t('admin.customModelConfig.template.attributesJson') }}
        </button>
      </div>
      <button v-if="mode === 'json'" type="button" class="btn btn-secondary" @click="formatJson">
        <Icon name="document" size="sm" class="mr-1" />
        {{ t('admin.customModelConfig.template.format') }}
      </button>
    </div>

    <div v-if="mode === 'visual'" class="space-y-3">
      <div class="flex flex-wrap gap-1">
        <button
          v-for="group in groups"
          :key="group.modality"
          type="button"
          :class="modalityTabClass(group)"
          @click="activeModality = group.modality"
        >
          {{ modalityLabel(group.modality) }}
          <span v-if="group.fields.length > 0" class="ml-1 text-xs font-normal opacity-70">
            {{ group.fields.length }}
          </span>
        </button>
      </div>

      <div
        v-for="group in groups"
        v-show="group.modality === activeModality"
        :key="group.modality"
        class="rounded-lg border border-gray-200 p-3 dark:border-dark-700"
      >
        <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
          <div class="flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ modalityLabel(group.modality) }}
            <span class="text-xs font-normal text-gray-500 dark:text-gray-400">
              ({{ group.fields.length }})
            </span>
          </div>
          <button type="button" class="btn btn-secondary" @click="addField(group)">
            <Icon name="plus" size="sm" class="mr-1" />
            {{ t('admin.customModelConfig.template.attributesAddField') }}
          </button>
        </div>

        <div class="mb-3 flex flex-wrap items-center gap-2 text-xs">
          <span class="font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.customModelConfig.template.attributesRenderMode.label') }}
          </span>
          <div class="inline-flex rounded-lg border border-gray-200 p-0.5 dark:border-dark-700">
            <button
              type="button"
              :class="renderModeButtonClass(group, 'overlay')"
              @click="group.renderMode = 'overlay'"
            >
              {{ t('admin.customModelConfig.template.attributesRenderMode.overlay') }}
            </button>
            <button
              type="button"
              :class="renderModeButtonClass(group, 'replace')"
              @click="group.renderMode = 'replace'"
            >
              {{ t('admin.customModelConfig.template.attributesRenderMode.replace') }}
            </button>
          </div>
          <span class="text-gray-500 dark:text-gray-400">
            {{ t(`admin.customModelConfig.template.attributesRenderMode.hint.${group.renderMode}`) }}
          </span>
        </div>

        <div
          v-if="hideCandidatesFor(group).length > 0"
          class="mb-3 flex flex-wrap items-center gap-x-4 gap-y-2 rounded-md border border-dashed border-gray-200 px-3 py-2 text-xs dark:border-dark-700"
        >
          <span class="font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.customModelConfig.template.attributesHide.label') }}
          </span>
          <label
            v-for="candidate in hideCandidatesFor(group)"
            :key="candidate.key"
            class="flex items-center gap-1.5 text-gray-600 dark:text-gray-300"
          >
            <input
              type="checkbox"
              class="h-3.5 w-3.5 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              :checked="group.hide.includes(candidate.key)"
              @change="onHideToggle(group, candidate.key, $event)"
            />
            {{ candidate.label }}
          </label>
          <span class="text-gray-500 dark:text-gray-400">
            {{ t('admin.customModelConfig.template.attributesHide.hint') }}
          </span>
        </div>

        <p
          v-if="group.fields.length === 0 && group.hide.length === 0"
          class="text-xs text-gray-500 dark:text-gray-400"
        >
          {{ t('admin.customModelConfig.template.attributesNoFields') }}
        </p>

        <div v-else class="space-y-3">
          <div
            v-for="(field, index) in group.fields"
            :key="index"
            class="rounded-md bg-gray-50 p-3 dark:bg-dark-800/60"
          >
            <div class="grid gap-2 md:grid-cols-[1.2fr_0.9fr_1.2fr_0.9fr_auto]">
              <div>
                <span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.customModelConfig.template.attributesFieldKey') }}
                </span>
                <input
                  v-model="field.key"
                  class="input w-full font-mono text-sm"
                  :placeholder="t('admin.customModelConfig.template.attributesFieldKeyPlaceholder')"
                />
              </div>
              <div>
                <span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.customModelConfig.template.attributesFieldType') }}
                </span>
                <select v-model="field.type" class="input w-full">
                  <option v-for="type in FIELD_TYPES" :key="type" :value="type">{{ type }}</option>
                </select>
              </div>
              <div>
                <span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.customModelConfig.template.attributesFieldLabel') }}
                </span>
                <input
                  v-model="field.label"
                  class="input w-full text-sm"
                  :placeholder="t('admin.customModelConfig.template.attributesFieldLabelPlaceholder')"
                />
              </div>
              <div>
                <span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.customModelConfig.template.attributesFieldUI') }}
                </span>
                <select v-model="field.ui" class="input w-full">
                  <option value="auto">
                    {{ t('admin.customModelConfig.template.attributesUI.auto') }}
                  </option>
                  <option value="hidden">
                    {{ t('admin.customModelConfig.template.attributesUI.hidden') }}
                  </option>
                </select>
              </div>
              <div class="flex items-end">
                <button
                  type="button"
                  class="px-2 pb-2.5 text-gray-500 hover:text-red-600 dark:text-gray-400 dark:hover:text-red-400"
                  :title="t('common.delete')"
                  @click="removeField(group, index)"
                >
                  <Icon name="trash" size="md" />
                </button>
              </div>
            </div>

            <div class="mt-2 grid gap-2 md:grid-cols-4">
              <div>
                <span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.customModelConfig.template.attributesFieldDefault') }}
                </span>
                <select
                  v-if="field.type === 'boolean'"
                  v-model="field.defaultValue"
                  class="input w-full"
                >
                  <option value="">
                    {{ t('admin.customModelConfig.template.attributesFieldDefaultUnset') }}
                  </option>
                  <option value="true">true</option>
                  <option value="false">false</option>
                </select>
                <select
                  v-else-if="field.type === 'enum'"
                  v-model="field.defaultValue"
                  class="input w-full"
                >
                  <option value="">
                    {{ t('admin.customModelConfig.template.attributesFieldDefaultUnset') }}
                  </option>
                  <option v-for="option in enumOptions(field)" :key="option" :value="option">
                    {{ option }}
                  </option>
                </select>
                <input
                  v-else
                  v-model="field.defaultValue"
                  class="input w-full font-mono text-sm"
                  :placeholder="t('admin.customModelConfig.template.attributesFieldDefaultUnset')"
                />
              </div>
              <div v-if="isNumeric(field)">
                <span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.customModelConfig.template.attributesFieldMin') }}
                </span>
                <input
                  v-model="field.min"
                  type="number"
                  :step="field.type === 'integer' ? 1 : 'any'"
                  class="input w-full font-mono text-sm"
                  placeholder="-"
                />
              </div>
              <div v-if="isNumeric(field)">
                <span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.customModelConfig.template.attributesFieldMax') }}
                </span>
                <input
                  v-model="field.max"
                  type="number"
                  :step="field.type === 'integer' ? 1 : 'any'"
                  class="input w-full font-mono text-sm"
                  placeholder="-"
                />
              </div>
              <label class="flex items-center gap-2 pb-2.5 text-sm text-gray-700 dark:text-gray-300">
                <input
                  v-model="field.required"
                  type="checkbox"
                  class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                />
                {{ t('admin.customModelConfig.template.attributesFieldRequired') }}
              </label>
            </div>

            <div v-if="field.type === 'enum'" class="mt-2">
              <span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.customModelConfig.template.attributesFieldValues') }}
              </span>
              <input
                v-model="field.values"
                class="input w-full font-mono text-sm"
                :placeholder="t('admin.customModelConfig.template.attributesFieldValuesPlaceholder')"
              />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.customModelConfig.template.attributesFieldValuesHint') }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div v-else class="space-y-2">
      <textarea
        v-model="jsonText"
        class="input min-h-48 w-full font-mono text-xs"
        spellcheck="false"
        placeholder="{}"
        @input="onJsonInput"
      />
      <p v-if="jsonError" class="text-xs text-red-600 dark:text-red-400">{{ jsonError }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'

type FieldType = 'string' | 'integer' | 'number' | 'boolean' | 'enum'
type FieldUI = 'auto' | 'hidden'
type EditorMode = 'visual' | 'json'
// overlay：在宿主硬编码参数之上用同名字段替换对应控件；replace：只渲染模板声明的字段。
type RenderMode = 'overlay' | 'replace'

interface EditableField {
  key: string
  type: FieldType
  values: string
  defaultValue: string
  required: boolean
  min: string
  max: string
  ui: FieldUI
  label: string
}

interface ModalityGroup {
  modality: string
  renderMode: RenderMode
  hide: string[]
  fields: EditableField[]
}

interface ModelAttributeHideCandidate {
  key: string
  label: string
}

const MODALITIES = ['image', 'video', 'audio']
const FIELD_TYPES: FieldType[] = ['string', 'integer', 'number', 'boolean', 'enum']

const props = defineProps<{
  modelValue: string
  // 各模态可隐藏的宿主内置参数，由调用方提供；为空表示该模态不支持隐藏。
  hideCandidates?: Record<string, ModelAttributeHideCandidate[]>
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const { t } = useI18n()

const mode = ref<EditorMode>('visual')
const activeModality = ref(MODALITIES[0])
const groups = ref<ModalityGroup[]>(emptyGroups())
const jsonText = ref(props.modelValue || '{}')
const jsonError = ref('')

let lastEmitted = props.modelValue || '{}'
let syncing = false

function emptyField(): EditableField {
  return {
    key: '',
    type: 'string',
    values: '',
    defaultValue: '',
    required: false,
    min: '',
    max: '',
    ui: 'auto',
    label: '',
  }
}

function emptyGroups(): ModalityGroup[] {
  return MODALITIES.map((modality) => ({ modality, renderMode: 'overlay', hide: [], fields: [] }))
}

function isFieldType(value: unknown): value is FieldType {
  return typeof value === 'string' && (FIELD_TYPES as string[]).includes(value)
}

function parseJson(text: string): { ok: true; value: Record<string, unknown> } | { ok: false } {
  try {
    const parsed: unknown = JSON.parse(text || '{}')
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return { ok: false }
    }
    return { ok: true, value: parsed as Record<string, unknown> }
  } catch {
    return { ok: false }
  }
}

function toGroups(value: Record<string, unknown>): ModalityGroup[] {
  return MODALITIES.map((modality) => {
    const fields: EditableField[] = []
    let renderMode: RenderMode = 'overlay'
    let hide: string[] = []
    const block = value[modality]
    if (block && typeof block === 'object' && !Array.isArray(block)) {
      const rawBlock = block as Record<string, unknown>
      if (rawBlock.mode === 'replace') renderMode = 'replace'
      if (Array.isArray(rawBlock.hide)) {
        hide = rawBlock.hide.filter((key): key is string => typeof key === 'string' && key.trim() !== '')
      }
      const list = rawBlock.fields
      if (Array.isArray(list)) {
        for (const item of list) {
          if (!item || typeof item !== 'object' || Array.isArray(item)) continue
          const raw = item as Record<string, unknown>
          fields.push({
            key: typeof raw.key === 'string' ? raw.key : '',
            type: isFieldType(raw.type) ? raw.type : 'string',
            values: Array.isArray(raw.values) ? raw.values.map(String).join(', ') : '',
            defaultValue:
              raw.default === undefined || raw.default === null ? '' : String(raw.default),
            required: raw.required === true,
            min: typeof raw.min === 'number' ? String(raw.min) : '',
            max: typeof raw.max === 'number' ? String(raw.max) : '',
            ui: raw.ui === 'hidden' ? 'hidden' : 'auto',
            label: typeof raw.label === 'string' ? raw.label : '',
          })
        }
      }
    }
    return { modality, renderMode, hide, fields }
  })
}

function coerceDefault(type: FieldType, raw: string): string | number | boolean | undefined {
  const value = raw.trim()
  if (type === 'boolean') {
    if (value === 'true') return true
    if (value === 'false') return false
    return undefined
  }
  if (type === 'integer' || type === 'number') {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : undefined
  }
  return value
}

function serializeField(field: EditableField): Record<string, unknown> {
  const item: Record<string, unknown> = { key: field.key.trim(), type: field.type }
  if (field.type === 'enum') {
    item.values = field.values
      .split(',')
      .map((value) => value.trim())
      .filter((value) => value !== '')
  }
  if (field.defaultValue.trim() !== '') {
    const value = coerceDefault(field.type, field.defaultValue)
    if (value !== undefined) item.default = value
  }
  if (field.required) item.required = true
  if (field.type === 'integer' || field.type === 'number') {
    const min = Number(field.min)
    if (field.min.trim() !== '' && Number.isFinite(min)) item.min = min
    const max = Number(field.max)
    if (field.max.trim() !== '' && Number.isFinite(max)) item.max = max
  }
  if (field.ui === 'hidden') item.ui = 'hidden'
  if (field.label.trim() !== '') item.label = field.label.trim()
  return item
}

function serialize(groups: ModalityGroup[]): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  for (const group of groups) {
    const fields = group.fields
      .filter((field) => field.key.trim() !== '')
      .map(serializeField)
    if (fields.length === 0 && group.hide.length === 0) continue
    const block: Record<string, unknown> = {}
    if (group.renderMode === 'replace') block.mode = 'replace'
    block.fields = fields
    if (group.hide.length > 0) block.hide = [...group.hide]
    result[group.modality] = block
  }
  return result
}

function emitJson(text: string) {
  lastEmitted = text
  emit('update:modelValue', text)
}

function applyExternal(text: string) {
  jsonText.value = text
  const parsed = parseJson(text)
  if (!parsed.ok) {
    jsonError.value = t('admin.customModelConfig.template.attributesJsonInvalid')
    mode.value = 'json'
    return
  }
  jsonError.value = ''
  syncing = true
  groups.value = toGroups(parsed.value)
  syncing = false
  const current = groups.value.find((group) => group.modality === activeModality.value)
  if (!current || (current.fields.length === 0 && current.hide.length === 0)) {
    const populated = groups.value.find((group) => group.fields.length > 0 || group.hide.length > 0)
    if (populated) activeModality.value = populated.modality
  }
}

applyExternal(jsonText.value)

watch(
  () => props.modelValue,
  (value) => {
    if (value === lastEmitted) return
    lastEmitted = value
    applyExternal(value)
  }
)

watch(
  groups,
  () => {
    if (syncing) return
    const text = JSON.stringify(serialize(groups.value), null, 2)
    jsonText.value = text
    emitJson(text)
  },
  { deep: true, flush: 'sync' }
)

function modalityLabel(modality: string): string {
  return t(`admin.customModelConfig.template.attributesModalities.${modality}`)
}

function isNumeric(field: EditableField): boolean {
  return field.type === 'integer' || field.type === 'number'
}

function enumOptions(field: EditableField): string[] {
  return field.values
    .split(',')
    .map((value) => value.trim())
    .filter((value) => value !== '')
}

function addField(group: ModalityGroup) {
  group.fields.push(emptyField())
}

function removeField(group: ModalityGroup, index: number) {
  group.fields.splice(index, 1)
}

function hideCandidatesFor(group: ModalityGroup): ModelAttributeHideCandidate[] {
  return props.hideCandidates?.[group.modality] ?? []
}

function onHideToggle(group: ModalityGroup, key: string, event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  const next = group.hide.filter((item) => item !== key)
  if (checked) next.push(key)
  group.hide = next
}

function modeButtonClass(target: EditorMode): string {
  const base = 'rounded-md px-3 py-1 text-xs font-medium transition-colors'
  return mode.value === target
    ? `${base} bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900`
    : `${base} text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100`
}

function renderModeButtonClass(group: ModalityGroup, target: RenderMode): string {
  const base = 'rounded-md px-2 py-0.5 text-xs font-medium transition-colors'
  return group.renderMode === target
    ? `${base} bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900`
    : `${base} text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100`
}

function modalityTabClass(group: ModalityGroup): string {
  const base = 'rounded-lg border px-3 py-1 text-xs font-medium transition-colors'
  return group.modality === activeModality.value
    ? `${base} border-gray-900 bg-gray-900 text-white dark:border-gray-100 dark:bg-gray-100 dark:text-gray-900`
    : `${base} border-gray-200 text-gray-600 hover:text-gray-900 dark:border-dark-700 dark:text-gray-400 dark:hover:text-gray-100`
}

function setMode(next: EditorMode) {
  if (next === mode.value) return
  if (next === 'visual') {
    const parsed = parseJson(jsonText.value)
    if (!parsed.ok) {
      jsonError.value = t('admin.customModelConfig.template.attributesJsonInvalid')
      return
    }
    jsonError.value = ''
    syncing = true
    groups.value = toGroups(parsed.value)
    syncing = false
  } else {
    const text = JSON.stringify(serialize(groups.value), null, 2)
    jsonText.value = text
    emitJson(text)
  }
  mode.value = next
}

function onJsonInput(event: Event) {
  const value = (event.target as HTMLTextAreaElement).value
  jsonText.value = value
  emitJson(value)
  jsonError.value = parseJson(value).ok
    ? ''
    : t('admin.customModelConfig.template.attributesJsonInvalid')
}

function formatJson() {
  const parsed = parseJson(jsonText.value)
  if (!parsed.ok) {
    jsonError.value = t('admin.customModelConfig.template.attributesJsonInvalid')
    return
  }
  const text = JSON.stringify(parsed.value, null, 2)
  jsonText.value = text
  emitJson(text)
  jsonError.value = ''
}
</script>
