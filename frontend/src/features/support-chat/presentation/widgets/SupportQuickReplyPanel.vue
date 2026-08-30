<template>
  <div class="space-y-3">
    <div class="flex flex-wrap gap-2">
      <button
        v-for="item in builtInItems"
        :key="`builtin:${item.title}`"
        type="button"
        class="rounded-xl border border-primary-200 bg-primary-50 px-3 py-2 text-left text-sm text-primary-700 hover:border-primary-400 dark:border-primary-800 dark:bg-primary-900/20 dark:text-primary-200"
        @click="emit('use', item.content)"
      >
        {{ item.title }}
      </button>
      <button
        v-for="item in items"
        :key="item.id"
        type="button"
        class="rounded-xl border border-gray-200 bg-white px-3 py-2 text-left text-sm text-gray-700 hover:border-primary-300 hover:text-primary-700 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200"
        @click="emit('use', item.content)"
      >
        {{ item.title }}
      </button>
      <button type="button" class="rounded-xl border border-dashed border-primary-300 px-3 py-2 text-sm text-primary-700 dark:border-primary-700 dark:text-primary-200" @click="beginCreate">
        {{ t('supportChat.quickReplies.add') }}
      </button>
      <button type="button" class="rounded-xl border border-gray-200 px-3 py-2 text-sm text-gray-600 dark:border-dark-700 dark:text-dark-300" @click="showImport = !showImport">
        {{ t('supportChat.quickReplies.import') }}
      </button>
    </div>

    <div v-if="editing" class="grid gap-2 rounded-xl border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800 sm:grid-cols-[180px_minmax(0,1fr)]">
      <input v-model="title" maxlength="100" class="input" :placeholder="t('supportChat.quickReplies.title')" />
      <textarea v-model="content" maxlength="10000" rows="3" class="input resize-y" :placeholder="t('supportChat.quickReplies.content')"></textarea>
      <div class="flex gap-2 sm:col-span-2 sm:justify-end">
        <button type="button" class="btn btn-secondary btn-sm" @click="cancelEdit">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary btn-sm" :disabled="busy || !canSave" @click="save">{{ t('common.save') }}</button>
      </div>
    </div>

    <div v-if="showImport" class="rounded-xl border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
      <p class="mb-2 text-xs text-gray-500 dark:text-dark-400">{{ t('supportChat.quickReplies.importHint') }}</p>
      <textarea v-model="importText" rows="4" class="input w-full resize-y" :placeholder="t('supportChat.quickReplies.importPlaceholder')"></textarea>
      <div class="mt-2 flex justify-end">
        <button type="button" class="btn btn-primary btn-sm" :disabled="busy || importItems.length === 0" @click="submitImport">
          {{ t('supportChat.quickReplies.import') }}
        </button>
      </div>
    </div>

    <ol v-if="items.length" class="max-h-52 space-y-1 overflow-y-auto">
      <li v-for="(item, index) in items" :key="item.id" class="flex items-center gap-2 rounded-lg bg-gray-50 px-2 py-1.5 text-xs dark:bg-dark-900">
        <span class="min-w-0 flex-1 truncate">{{ item.title }}</span>
        <button type="button" :disabled="busy || index === 0" :title="t('supportChat.quickReplies.moveUp')" @click="move(index, -1)">↑</button>
        <button type="button" :disabled="busy || index === items.length - 1" :title="t('supportChat.quickReplies.moveDown')" @click="move(index, 1)">↓</button>
        <button type="button" class="text-primary-600" :disabled="busy" @click="beginEdit(item)">{{ t('common.edit') }}</button>
        <button type="button" class="text-red-600" :disabled="busy" @click="emit('delete', item.id)">{{ t('common.delete') }}</button>
      </li>
    </ol>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ChatQuickReply } from '@/features/support-chat/data/datasources/supportChatDatasource'

const props = withDefaults(defineProps<{
  items: ChatQuickReply[]
  builtInItems?: Array<{ title: string; content: string }>
  busy?: boolean
}>(), { builtInItems: () => [], busy: false })

const emit = defineEmits<{
  use: [content: string]
  create: [value: { title: string; content: string }]
  update: [value: { id: number; title: string; content: string }]
  delete: [id: number]
  reorder: [ids: number[]]
  import: [items: Array<{ title: string; content: string }>]
}>()

const { t } = useI18n()
const editing = ref(false)
const editingID = ref<number | null>(null)
const title = ref('')
const content = ref('')
const showImport = ref(false)
const importText = ref('')

const canSave = computed(() => title.value.trim().length > 0 && content.value.trim().length > 0)
const importItems = computed(() => importText.value.split(/\r?\n/).map((line) => {
  const separator = line.indexOf('\t')
  if (separator <= 0) return null
  const itemTitle = line.slice(0, separator).trim().slice(0, 100)
  const itemContent = line.slice(separator + 1).trim().slice(0, 10000)
  return itemTitle && itemContent ? { title: itemTitle, content: itemContent } : null
}).filter((item): item is { title: string; content: string } => Boolean(item)).slice(0, 50))

function beginCreate() {
  editingID.value = null
  title.value = ''
  content.value = ''
  editing.value = true
}

function beginEdit(item: ChatQuickReply) {
  editingID.value = item.id
  title.value = item.title
  content.value = item.content
  editing.value = true
}

function cancelEdit() {
  editing.value = false
  editingID.value = null
  title.value = ''
  content.value = ''
}

function save() {
  if (!canSave.value) return
  const value = { title: title.value.trim(), content: content.value.trim() }
  if (editingID.value) emit('update', { id: editingID.value, ...value })
  else emit('create', value)
  cancelEdit()
}

function move(index: number, delta: -1 | 1) {
  const target = index + delta
  if (target < 0 || target >= props.items.length) return
  const ordered = props.items.map(item => item.id)
  ;[ordered[index], ordered[target]] = [ordered[target], ordered[index]]
  emit('reorder', ordered)
}

function submitImport() {
  if (importItems.value.length === 0) return
  emit('import', importItems.value)
  importText.value = ''
  showImport.value = false
}
</script>
