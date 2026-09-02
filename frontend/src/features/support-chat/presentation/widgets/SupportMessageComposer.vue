<template>
  <form
    class="border-t border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-900 sm:p-4"
    :class="{ 'composer-drop-active': isDropActive }"
    @submit.prevent="submitText"
    @dragenter.prevent="handleDragEnter"
    @dragover.prevent="handleDragOver"
    @dragleave.prevent="handleDragLeave"
    @drop.prevent="handleDrop"
  >
    <div v-if="replyingTo" class="mb-2 flex items-start gap-2 rounded-xl bg-gray-50 px-3 py-2 text-xs dark:bg-dark-800">
      <div class="min-w-0 flex-1">
        <p class="font-medium text-gray-700 dark:text-dark-200">{{ t('supportChat.reply.replying') }} #{{ replyingTo.id }}</p>
        <p class="truncate text-gray-500 dark:text-dark-400">{{ replyingTo.content }}</p>
      </div>
      <button type="button" class="text-gray-500 hover:text-gray-800 dark:hover:text-white" @click="emit('cancelReply')">×</button>
    </div>

    <div v-if="activePanel" class="mb-3 rounded-2xl border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-950/60">
      <div v-if="activePanel === 'stickers'" class="grid max-h-56 grid-cols-6 gap-2 overflow-y-auto sm:grid-cols-10">
        <button
          v-for="emoji in emojiStickers"
          :key="emoji"
          type="button"
          class="h-11 w-11 rounded-xl bg-white text-2xl hover:bg-primary-50 dark:bg-dark-800 dark:hover:bg-primary-900/20"
          @click="sendEmoji(emoji)"
        >
          {{ emoji }}
        </button>
      </div>

      <div v-else-if="activePanel === 'replies'" class="space-y-3">
        <div class="flex items-center justify-between gap-4 rounded-xl border border-gray-200 bg-white px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
          <div>
            <p class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('supportChat.quickReplies.oneClick') }}</p>
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('supportChat.quickReplies.oneClickHint') }}</p>
          </div>
          <Toggle v-model="oneClickReply" data-testid="support-quick-reply-one-click" />
        </div>
        <SupportQuickReplyPanel
          :items="quickReplies"
          :built-in-items="builtInReplies"
          :busy="toolsBusy"
          @use="useQuickReply"
          @create="emit('quickReplyCreate', $event)"
          @update="emit('quickReplyUpdate', $event)"
          @delete="emit('quickReplyDelete', $event)"
          @reorder="emit('quickReplyReorder', $event)"
          @import="emit('quickReplyImport', $event)"
        />
      </div>

      <div v-else-if="activePanel === 'catalog'" class="space-y-3">
        <div class="flex items-center gap-2">
          <button type="button" class="btn btn-sm" :class="catalogScope === 'library' ? 'btn-primary' : 'btn-secondary'" @click="catalogScope = 'library'">
            {{ t('supportChat.assets.library') }}
          </button>
          <button type="button" class="btn btn-sm" :class="catalogScope === 'sticker' ? 'btn-primary' : 'btn-secondary'" @click="catalogScope = 'sticker'">
            {{ t('supportChat.assets.stickers') }}
          </button>
          <input v-model="catalogCollection" maxlength="100" class="input ml-auto max-w-44" :placeholder="t('supportChat.assets.collection')" />
          <button type="button" class="btn btn-secondary btn-sm" :disabled="toolsBusy" @click="catalogFileInput?.click()">
            {{ t('supportChat.assets.addToCatalog') }}
          </button>
          <input ref="catalogFileInput" class="hidden" type="file" accept="image/png,image/jpeg,image/gif,image/webp" @change="handleCatalogFile" />
        </div>
        <div v-if="activeCatalog.length" class="grid max-h-72 grid-cols-3 gap-2 overflow-y-auto sm:grid-cols-5">
          <div v-for="asset in activeCatalog" :key="asset.id" class="group relative">
            <button type="button" class="block w-full" :title="asset.name" @click="sendCatalogAsset(asset)">
              <SupportAssetImage :asset-id="asset.id" scope="admin" :alt="asset.name" container-class="aspect-square rounded-xl" lazy />
            </button>
            <button
              type="button"
              class="absolute right-1 top-1 hidden h-6 w-6 rounded-full bg-red-600 text-xs text-white group-hover:block"
              :title="t('common.delete')"
              @click="emit('catalogDelete', { scope: catalogScope, id: asset.id })"
            >
              ×
            </button>
          </div>
        </div>
        <p v-else class="py-4 text-center text-xs text-gray-500 dark:text-dark-400">{{ t('supportChat.assets.catalogEmpty') }}</p>
      </div>

      <div v-else-if="activePanel === 'transfer'" class="grid gap-3 sm:grid-cols-[180px_minmax(0,1fr)_auto] sm:items-end">
        <label class="text-xs text-gray-600 dark:text-dark-300">
          <span class="mb-1 block">{{ t('supportChat.transfer.amount') }}</span>
          <input v-model.number="transferAmount" type="number" min="0.00000001" max="1000000000" step="0.01" class="input w-full" />
        </label>
        <label class="text-xs text-gray-600 dark:text-dark-300">
          <span class="mb-1 block">{{ t('supportChat.transfer.notes') }}</span>
          <input v-model="transferNotes" maxlength="500" class="input w-full" />
        </label>
        <button type="button" class="btn btn-primary" :disabled="toolsBusy || !validTransfer" @click="submitTransfer">
          {{ t('supportChat.transfer.confirm') }}
        </button>
      </div>
    </div>

    <label class="sr-only" for="support-chat-content">{{ t('supportChat.inputLabel') }}</label>
    <textarea
      id="support-chat-content"
      v-model="draft"
      class="min-h-[92px] w-full resize-none rounded-xl border border-gray-200 bg-white px-4 py-3 text-sm text-gray-900 outline-none placeholder:text-gray-400 focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20 dark:border-dark-700 dark:bg-dark-800 dark:text-white"
      :maxlength="maxLength"
      :placeholder="t('supportChat.inputPlaceholder')"
      :disabled="disabled || sending"
      @compositionstart="isComposing = true"
      @compositionend="isComposing = false"
      @keydown="handleKeydown"
      @paste="handlePaste"
    />

    <div v-if="pendingImages.length" class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
      <div v-for="image in pendingImages" :key="image.id" class="group relative overflow-hidden rounded-xl border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
        <img v-if="image.previewUrl" :src="image.previewUrl" :alt="image.file.name" class="aspect-square w-full object-cover" />
        <div v-else class="flex aspect-square items-center justify-center px-2 text-center text-xs text-gray-500 dark:text-dark-400">
          {{ image.file.name }}
        </div>
        <button
          type="button"
          class="absolute right-1 top-1 inline-flex h-7 w-7 items-center justify-center rounded-full bg-black/65 text-lg text-white hover:bg-red-600"
          :aria-label="t('supportChat.assets.removePending')"
          @click="removePendingImage(image.id)"
        >
          ×
        </button>
      </div>
    </div>

    <div class="mt-2 flex flex-wrap items-center gap-2">
      <button type="button" class="composer-tool" :title="t('supportChat.assets.upload')" :disabled="disabled || sending" @click="messageFileInput?.click()">📎</button>
      <input ref="messageFileInput" class="hidden" type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple @change="handleMessageFile" />
      <button type="button" class="composer-tool" :class="activePanel === 'stickers' ? 'composer-tool-active' : ''" @click="togglePanel('stickers')">😊</button>
      <button v-if="adminMode" type="button" class="composer-tool" :class="activePanel === 'replies' ? 'composer-tool-active' : ''" @click="togglePanel('replies')">
        {{ t('supportChat.quickReplies.short') }}
      </button>
      <button v-if="adminMode" type="button" class="composer-tool" :class="activePanel === 'catalog' ? 'composer-tool-active' : ''" @click="togglePanel('catalog')">
        {{ t('supportChat.assets.catalog') }}
      </button>
      <button v-if="adminMode" type="button" class="composer-tool" :class="activePanel === 'transfer' ? 'composer-tool-active' : ''" @click="togglePanel('transfer')">
        {{ t('supportChat.transfer.action') }}
      </button>
      <span class="ml-auto text-xs text-gray-400">{{ draft.length }}/{{ maxLength }}</span>
      <button type="submit" class="btn btn-primary min-w-24" :disabled="disabled || sending || !canSubmit">
        {{ sending ? t('common.submitting') : t('supportChat.send') }}
      </button>
    </div>
  </form>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import {
  type ChatAsset,
  type ChatMessage,
  type ChatQuickReply,
  type ChatSendMessageInput,
} from '@/features/support-chat/data/datasources/supportChatDatasource'
import SupportAssetImage from '@/features/support-chat/presentation/widgets/SupportAssetImage.vue'
import SupportQuickReplyPanel from '@/features/support-chat/presentation/widgets/SupportQuickReplyPanel.vue'

type ComposerPanel = 'stickers' | 'replies' | 'catalog' | 'transfer'

interface PendingImage {
  id: string
  file: File
  previewUrl: string
}

const MAX_PENDING_IMAGES = 4
const MAX_IMAGE_BYTES = 5 << 20
const QUICK_REPLY_MODE_KEY = 'support_chat_one_click_reply_v1'
const DRAFT_KEY_PREFIX = 'support_chat_draft_v2:'

const props = withDefaults(defineProps<{
  sending?: boolean
  disabled?: boolean
  maxLength?: number
  adminMode?: boolean
  toolsBusy?: boolean
  replyingTo?: ChatMessage | null
  quickReplies?: ChatQuickReply[]
  libraryAssets?: ChatAsset[]
  stickerAssets?: ChatAsset[]
  draftKey?: string
}>(), {
  sending: false,
  disabled: false,
  maxLength: 10000,
  adminMode: false,
  toolsBusy: false,
  replyingTo: null,
  quickReplies: () => [],
  libraryAssets: () => [],
  stickerAssets: () => [],
  draftKey: 'default',
})

const emit = defineEmits<{
  submit: [input: ChatSendMessageInput]
  upload: [value: { files: File[]; content: string; reply_to_id: number | null }]
  cancelReply: []
  transfer: [value: { amount: number; notes: string }]
  quickReplyCreate: [value: { title: string; content: string }]
  quickReplyUpdate: [value: { id: number; title: string; content: string }]
  quickReplyDelete: [id: number]
  quickReplyReorder: [ids: number[]]
  quickReplyImport: [items: Array<{ title: string; content: string }>]
  catalogCreate: [value: { scope: 'library' | 'sticker'; file: File; collection: string }]
  catalogDelete: [value: { scope: 'library' | 'sticker'; id: number }]
}>()

const { t } = useI18n()
const draft = ref('')
const isComposing = ref(false)
const activePanel = ref<ComposerPanel | null>(null)
const messageFileInput = ref<HTMLInputElement | null>(null)
const catalogFileInput = ref<HTMLInputElement | null>(null)
const dragDepth = ref(0)
const isDropActive = ref(false)
const catalogScope = ref<'library' | 'sticker'>('library')
const catalogCollection = ref('')
const transferAmount = ref<number | null>(null)
const transferNotes = ref('')
const pendingImages = ref<PendingImage[]>([])
const oneClickReply = ref(readOneClickReply())
const emojiStickers = [
  '👍', '👎', '✅', '❌', '🎉', '🙏', '😊', '😁', '😂', '🥰',
  '🤝', '👏', '💡', '📌', '⏳', '❤️', '🔥', '⭐', '💯', '👀',
  '🫡', '🤔', '😅', '😢', '😮', '🚀', '🛠️', '📣', '💬', '📦',
]

const activeCatalog = computed(() => catalogScope.value === 'library' ? props.libraryAssets : props.stickerAssets)
const canSubmit = computed(() => draft.value.trim().length > 0 || pendingImages.value.length > 0)
const builtInReplies = computed(() => [
  { title: t('supportChat.composer.replyHello'), content: t('supportChat.composer.replyHelloContent') },
  { title: t('supportChat.composer.replyNeedMoreInfo'), content: t('supportChat.composer.replyNeedMoreInfoContent') },
  { title: t('supportChat.composer.replyResolved'), content: t('supportChat.composer.replyResolvedContent') },
])
const validTransfer = computed(() => {
  const amount = Number(transferAmount.value)
  return Number.isFinite(amount) && amount > 0 && amount <= 1_000_000_000
})

function togglePanel(panel: ComposerPanel) {
  if (props.disabled || (panel !== 'stickers' && !props.adminMode)) return
  activePanel.value = activePanel.value === panel ? null : panel
}

function replyID(): number | null {
  return props.replyingTo?.id || null
}

function submitText() {
  if (isComposing.value) return
  const content = draft.value.trim()
  if ((!content && pendingImages.value.length === 0) || props.disabled || props.sending) return
  if (pendingImages.value.length > 0) {
    emit('upload', {
      files: pendingImages.value.map(image => image.file),
      content,
      reply_to_id: replyID(),
    })
    return
  }
  emit('submit', { content, kind: 'text', reply_to_id: replyID() })
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.shiftKey || event.altKey || event.ctrlKey || event.metaKey) return
  if (isComposing.value || event.isComposing || event.keyCode === 229) return
  event.preventDefault()
  submitText()
}

function clearDraft() {
  draft.value = ''
  clearPendingImages()
  removeStoredDraft(props.draftKey)
}

function sendEmoji(emoji: string) {
  emit('submit', {
    content: emoji,
    kind: 'sticker',
    sticker: { name: emoji, emoji },
    reply_to_id: replyID(),
  })
  activePanel.value = null
}

function sendCatalogAsset(asset: ChatAsset) {
  emit('submit', {
    content: asset.scope === 'sticker' ? '[sticker]' : '[image]',
    kind: asset.scope === 'sticker' ? 'sticker' : 'image',
    asset_ids: [asset.id],
    reply_to_id: replyID(),
  })
  activePanel.value = null
}

function useQuickReply(content: string) {
  const next = content.trim().slice(0, props.maxLength)
  if (oneClickReply.value && next && pendingImages.value.length === 0 && !props.disabled && !props.sending) {
    emit('submit', { content: next, kind: 'text', reply_to_id: replyID() })
  } else {
    draft.value = next
  }
  activePanel.value = null
}

function readOneClickReply(): boolean {
  try {
    return localStorage.getItem(QUICK_REPLY_MODE_KEY) === 'true'
  } catch {
    return false
  }
}

function persistOneClickReply(enabled: boolean) {
  try {
    localStorage.setItem(QUICK_REPLY_MODE_KEY, String(enabled))
  } catch {
    // Storage can be unavailable in hardened browser contexts.
  }
}

function storedDraftKey(key: string): string {
  return `${DRAFT_KEY_PREFIX}${key || 'default'}`
}

function readStoredDraft(key: string): string {
  try {
    return (localStorage.getItem(storedDraftKey(key)) || '').slice(0, props.maxLength)
  } catch {
    return ''
  }
}

function persistDraft(key: string, value: string) {
  try {
    const storageKey = storedDraftKey(key)
    if (value) localStorage.setItem(storageKey, value.slice(0, props.maxLength))
    else localStorage.removeItem(storageKey)
  } catch {
    // Draft persistence is best effort and never blocks sending.
  }
}

function removeStoredDraft(key: string) {
  try {
    localStorage.removeItem(storedDraftKey(key))
  } catch {
    // Ignore inaccessible storage.
  }
}

function imageFiles(files: Iterable<File> | null | undefined): File[] {
  if (!files) return []
  return Array.from(files).filter(file => {
    const type = file.type.toLowerCase()
    const supportedType = type.startsWith('image/') || type === '' || type === 'application/octet-stream'
    return supportedType && file.size > 0 && file.size <= MAX_IMAGE_BYTES
  })
}

function createPendingImage(file: File): PendingImage {
  let previewUrl = ''
  try {
    previewUrl = URL.createObjectURL(file)
  } catch {
    // File name remains visible when this browser cannot create object URLs.
  }
  return {
    id: `${file.name}:${file.size}:${file.lastModified}:${Math.random().toString(36).slice(2)}`,
    file,
    previewUrl,
  }
}

function addPendingImages(files: Iterable<File> | null | undefined): number {
  if (props.disabled || props.sending) return 0
  const remaining = MAX_PENDING_IMAGES - pendingImages.value.length
  if (remaining <= 0) return 0
  const existing = new Set(pendingImages.value.map(image => `${image.file.name}:${image.file.size}:${image.file.lastModified}`))
  const accepted = imageFiles(files)
    .filter(file => {
      const key = `${file.name}:${file.size}:${file.lastModified}`
      if (existing.has(key)) return false
      existing.add(key)
      return true
    })
    .slice(0, remaining)
  pendingImages.value.push(...accepted.map(createPendingImage))
  return accepted.length
}

function revokePendingImage(image: PendingImage) {
  if (!image.previewUrl) return
  try {
    URL.revokeObjectURL(image.previewUrl)
  } catch {
    // Ignore browsers without object URL support.
  }
}

function removePendingImage(id: string) {
  const index = pendingImages.value.findIndex(image => image.id === id)
  if (index < 0) return
  const [removed] = pendingImages.value.splice(index, 1)
  revokePendingImage(removed)
}

function clearPendingImages() {
  pendingImages.value.forEach(revokePendingImage)
  pendingImages.value = []
}

function handleMessageFile(event: Event) {
  const input = event.target as HTMLInputElement
  addPendingImages(input.files)
  input.value = ''
}

function handleCatalogFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  emit('catalogCreate', { scope: catalogScope.value, file, collection: catalogCollection.value.trim() })
}

function submitTransfer() {
  if (!validTransfer.value) return
  emit('transfer', { amount: Number(transferAmount.value), notes: transferNotes.value.trim() })
  transferAmount.value = null
  transferNotes.value = ''
  activePanel.value = null
}

function hasFilePayload(dataTransfer: DataTransfer | null | undefined): boolean {
  if (!dataTransfer) return false
  return dataTransfer.files.length > 0 || Array.from(dataTransfer.types || []).includes('Files')
}

function handleDragEnter(event: DragEvent) {
  if (!hasFilePayload(event.dataTransfer)) return
  dragDepth.value += 1
  isDropActive.value = true
}

function handleDragOver(event: DragEvent) {
  const dataTransfer = event.dataTransfer
  if (!dataTransfer || !hasFilePayload(dataTransfer)) return
  dataTransfer.dropEffect = 'copy'
  isDropActive.value = true
}

function handleDragLeave(event: DragEvent) {
  if (!hasFilePayload(event.dataTransfer)) return
  dragDepth.value = Math.max(0, dragDepth.value - 1)
  if (dragDepth.value === 0) isDropActive.value = false
}

function handleDrop(event: DragEvent) {
  dragDepth.value = 0
  isDropActive.value = false
  addPendingImages(event.dataTransfer?.files)
}

function handlePaste(event: ClipboardEvent) {
  const clipboard = event.clipboardData
  if (!clipboard) return
  const files = [
    ...Array.from(clipboard.files || []),
    ...Array.from(clipboard.items || [])
    .filter(item => item.kind === 'file')
    .map(item => item.getAsFile())
    .filter((candidate): candidate is File => Boolean(candidate)),
  ]
  if (addPendingImages(files) > 0) event.preventDefault()
}

watch(oneClickReply, persistOneClickReply)

watch(draft, value => {
  persistDraft(props.draftKey, value)
})

watch(() => props.draftKey, (next, previous) => {
  if (previous && previous !== next) persistDraft(previous, draft.value)
  clearPendingImages()
  draft.value = readStoredDraft(next)
}, { immediate: true })

onBeforeUnmount(clearPendingImages)

defineExpose({ clearDraft })
</script>

<style scoped>
.composer-tool {
  @apply inline-flex h-9 min-w-9 items-center justify-center rounded-xl border border-gray-200 bg-white px-2 text-xs font-medium text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700;
}

.composer-tool-active {
  @apply border-primary-300 bg-primary-50 text-primary-700 dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-200;
}

.composer-drop-active {
  @apply border-primary-400 bg-primary-50/40 dark:border-primary-600 dark:bg-primary-900/10;
}
</style>
