<template>
  <form class="border-t border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-900 sm:p-4" @submit.prevent="submitText">
    <div v-if="replyingTo" class="mb-2 flex items-start gap-2 rounded-xl bg-gray-50 px-3 py-2 text-xs dark:bg-dark-800">
      <div class="min-w-0 flex-1">
        <p class="font-medium text-gray-700 dark:text-dark-200">{{ t('supportChat.reply.replying') }} #{{ replyingTo.id }}</p>
        <p class="truncate text-gray-500 dark:text-dark-400">{{ replyingTo.content }}</p>
      </div>
      <button type="button" class="text-gray-500 hover:text-gray-800 dark:hover:text-white" @click="emit('cancelReply')">×</button>
    </div>

    <div v-if="activePanel" class="mb-3 rounded-2xl border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-950/60">
      <div v-if="activePanel === 'stickers'" class="flex flex-wrap gap-2">
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

      <SupportQuickReplyPanel
        v-else-if="activePanel === 'replies'"
        :items="quickReplies"
        :busy="toolsBusy"
        @use="useQuickReply"
        @create="emit('quickReplyCreate', $event)"
        @update="emit('quickReplyUpdate', $event)"
        @delete="emit('quickReplyDelete', $event)"
        @reorder="emit('quickReplyReorder', $event)"
        @import="emit('quickReplyImport', $event)"
      />

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
    />

    <div class="mt-2 flex flex-wrap items-center gap-2">
      <button type="button" class="composer-tool" :title="t('supportChat.assets.upload')" :disabled="disabled || sending" @click="messageFileInput?.click()">📎</button>
      <input ref="messageFileInput" class="hidden" type="file" accept="image/png,image/jpeg,image/gif,image/webp" @change="handleMessageFile" />
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
      <button type="submit" class="btn btn-primary min-w-24" :disabled="disabled || sending || !draft.trim()">
        {{ sending ? t('common.submitting') : t('supportChat.send') }}
      </button>
    </div>
  </form>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  type ChatAsset,
  type ChatMessage,
  type ChatQuickReply,
  type ChatSendMessageInput,
} from '@/features/support-chat/data/datasources/supportChatDatasource'
import SupportAssetImage from '@/features/support-chat/presentation/widgets/SupportAssetImage.vue'
import SupportQuickReplyPanel from '@/features/support-chat/presentation/widgets/SupportQuickReplyPanel.vue'

type ComposerPanel = 'stickers' | 'replies' | 'catalog' | 'transfer'

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
})

const emit = defineEmits<{
  submit: [input: ChatSendMessageInput]
  upload: [value: { file: File; content: string; reply_to_id: number | null }]
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
const catalogScope = ref<'library' | 'sticker'>('library')
const catalogCollection = ref('')
const transferAmount = ref<number | null>(null)
const transferNotes = ref('')
const emojiStickers = ['👍', '✅', '🎉', '🙏', '😊', '🤝', '💡', '📌', '⏳', '❤️']

const activeCatalog = computed(() => catalogScope.value === 'library' ? props.libraryAssets : props.stickerAssets)
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
  if (!content || props.disabled || props.sending) return
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
  draft.value = content.slice(0, props.maxLength)
  activePanel.value = null
}

function handleMessageFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  emit('upload', { file, content: draft.value.trim(), reply_to_id: replyID() })
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

defineExpose({ clearDraft })
</script>

<style scoped>
.composer-tool {
  @apply inline-flex h-9 min-w-9 items-center justify-center rounded-xl border border-gray-200 bg-white px-2 text-xs font-medium text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700;
}

.composer-tool-active {
  @apply border-primary-300 bg-primary-50 text-primary-700 dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-200;
}
</style>
