<template>
  <AppLayout>
    <section class="mx-auto flex h-[calc(100vh-8rem)] max-w-5xl flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
      <header class="flex items-center justify-between gap-3 border-b border-gray-200 px-5 py-4 dark:border-dark-700">
        <div>
          <h1 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('supportChat.title') }}</h1>
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('supportChat.description') }}</p>
        </div>
        <div class="flex items-center gap-2 text-xs">
          <span class="inline-flex items-center gap-1 rounded-full px-2.5 py-1 font-medium" :class="socketConnected ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-200' : 'bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-dark-300'">
            <span class="h-2 w-2 rounded-full" :class="socketConnected ? 'bg-emerald-500' : 'bg-gray-400'"></span>
            {{ socketConnected ? t('supportChat.connected') : t('supportChat.offline') }}
          </span>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="reload">
            {{ t('common.refresh') }}
          </button>
        </div>
      </header>

      <div ref="messagePaneRef" class="min-h-0 flex-1 overflow-y-auto bg-gray-50 p-5 dark:bg-dark-950/60">
        <div v-if="loading && messages.length === 0" class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-dark-400">
          {{ t('common.loading') }}
        </div>
        <div v-else-if="messages.length === 0" class="flex h-full flex-col items-center justify-center text-center text-gray-500 dark:text-dark-400">
          <div class="mb-3 rounded-2xl bg-primary-50 p-4 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300">
            <svg class="h-8 w-8" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M20.25 8.511c.884.284 1.5 1.128 1.5 2.097v4.286c0 1.136-.847 2.1-1.98 2.193-.34.027-.68.052-1.02.072v3.091l-3-3c-1.354 0-2.694-.055-4.02-.163a2.115 2.115 0 01-.825-.242m9.345-8.334a2.126 2.126 0 00-.476-.095 48.64 48.64 0 00-8.048 0c-1.131.094-1.976 1.057-1.976 2.192v4.286c0 .837.46 1.58 1.155 1.951m9.345-8.334V6.637c0-1.621-1.152-3.026-2.76-3.235A48.455 48.455 0 0011.25 3c-2.115 0-4.198.137-6.24.402-1.608.209-2.76 1.614-2.76 3.235v6.226c0 1.621 1.152 3.026 2.76 3.235.577.075 1.157.14 1.74.194V21l4.155-4.155" />
            </svg>
          </div>
          <p class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('supportChat.emptyTitle') }}</p>
          <p class="mt-1 text-sm">{{ t('supportChat.emptyDescription') }}</p>
        </div>
        <SupportMessageList
          v-else
          :messages="messages"
          own-sender="user"
          :receiver-unread-count="conversation?.unread_by_admin ?? 0"
          @reply="handleReply"
        />
      </div>

      <SupportMessageComposer
        :sending="sending"
        :disabled="loading"
        draft-key="user:support"
        :clear-nonce="composerClearNonce"
        :replying-to="replyingTo"
        @submit="handleSend"
        @submit-rich="handleRichSend"
        @cancel-reply="handleCancelReply"
      />
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import { useAppStore } from '@/core/stores/appStore'
import {
  getUserChatConversation,
  listUserChatMessages,
  markUserChatRead,
  sendUserChatMessage,
  uploadUserChatAsset,
  type ChatConversation,
  type ChatMessage,
} from '@/features/support-chat/data/datasources/supportChatDatasource'
import { useSupportChatSocket } from '@/features/support-chat/presentation/composables/useSupportChatSocket'
import { useSupportChatAdminStore } from '@/features/support-chat/presentation/stores/supportChatAdminStore'
import {
  buildImageMessageContent,
  buildStickerMessageContent,
} from '@/features/support-chat/presentation/utils/supportChatMessageContent'
import SupportMessageComposer from '@/features/support-chat/presentation/widgets/SupportMessageComposer.vue'
import SupportMessageList from '@/features/support-chat/presentation/widgets/SupportMessageList.vue'

const { t } = useI18n()
const appStore = useAppStore()
const supportChatAdminStore = useSupportChatAdminStore()
const loading = ref(false)
const sending = ref(false)
const messages = ref<ChatMessage[]>([])
const conversation = ref<ChatConversation | null>(null)
const messagePaneRef = ref<HTMLElement | null>(null)
const socketConnected = ref(false)
const composerClearNonce = ref(0)
const pageVisible = ref(!document.hidden)
const replyingTo = ref<ChatMessage | null>(null)
let optimisticMessageSeed = 0

interface PendingImage {
  type: 'imageFile' | 'imageUrl'
  file?: File
  previewUrl?: string
  url?: string
  name: string
}

interface PendingSticker {
  id?: string
  name: string
  url?: string
  emoji?: string
}

interface ComposerSubmitPayload {
  text: string
  images?: PendingImage[]
  sticker?: PendingSticker | null
  replyTo?: number
}

const socket = useSupportChatSocket({
  scope: 'user',
  onStatusChange: (connected) => {
    socketConnected.value = connected
  },
  onMessage: (message) => {
    appendMessage(message)
    if (message.sender_type === 'admin') {
      // Mark as unread, will be cleared when user views the page
      appStore.setSupportUserUnread(true)
      supportChatAdminStore.markUserHasUnread()
      if (conversation.value) {
        conversation.value.unread_by_user += 1
      }
      // Only auto-mark as read if user is actively viewing the page
      if (pageVisible.value) {
        void markAsReadIfVisible()
      }
    }
    void scrollToBottom()
  },
  onReadState: (conversationID, reader) => {
    // When admin marks as read, clear unread_by_admin for this conversation
    if (reader === 'admin' && conversation.value?.id === conversationID) {
      conversation.value.unread_by_admin = 0
    }
  },
})

function appendMessage(message: ChatMessage) {
  if (messages.value.some((item) => item.id === message.id)) return
  messages.value.push(message)
}

function mergeMessages(nextMessages: ChatMessage[]): boolean {
  let changed = false
  for (const message of nextMessages) {
    const index = messages.value.findIndex((item) => item.id === message.id)
    if (index >= 0) {
      messages.value[index] = { ...messages.value[index], ...message }
    } else {
      messages.value.push(message)
      changed = true
    }
  }
  return changed
}

function messageScrollSignature(): string {
  return messages.value.map((message) => `${message.id}:${message.created_at}`).join('|')
}

function errorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message
  if (typeof error === 'object' && error !== null && 'message' in error) {
    const message = (error as { message?: unknown }).message
    if (typeof message === 'string' && message.trim()) return message
  }
  return fallback
}

function imageMessageContent(url: string, name: string): string {
  return buildImageMessageContent(url, name, t('supportChat.composer.imageAlt'))
}

function stickerMessageContent(sticker: { id?: string; name: string; url?: string; emoji?: string }): string {
  return buildStickerMessageContent(sticker as { id: string; name: string; url?: string; emoji?: string })
}

function nextOptimisticMessageID(): number {
  optimisticMessageSeed += 1
  return -(Date.now() + optimisticMessageSeed)
}

function appendOptimisticMessage(content: string): ChatMessage | null {
  if (!conversation.value || !content) return null
  const message: ChatMessage = {
    id: nextOptimisticMessageID(),
    conversation_id: conversation.value.id,
    sender_type: 'user',
    sender_id: conversation.value.user_id,
    content,
    created_at: new Date().toISOString(),
  }
  appendMessage(message)
  return message
}

function replaceMessage(messageID: number, nextMessage: ChatMessage) {
  messages.value = messages.value.filter((item) => item.id === messageID || item.id !== nextMessage.id)
  const index = messages.value.findIndex((item) => item.id === messageID)
  if (index >= 0) {
    messages.value.splice(index, 1, nextMessage)
    return
  }
  appendMessage(nextMessage)
}

async function scrollToBottom() {
  await nextTick()
  const pane = messagePaneRef.value
  if (pane) pane.scrollTop = pane.scrollHeight
}

async function markAsReadIfVisible() {
  // Only mark as read if page is visible and user has scrolled near bottom
  if (!pageVisible.value || !conversation.value) return
  const pane = messagePaneRef.value
  if (!pane) return

  // Check if user is near bottom (within 100px)
  const isNearBottom = pane.scrollHeight - pane.scrollTop - pane.clientHeight < 100
  if (!isNearBottom) return

  // Mark as read
  await markUserChatRead()
  if (conversation.value) conversation.value.unread_by_user = 0
  appStore.setSupportUserUnread(false)
  supportChatAdminStore.markUserRead()
}

function handleVisibilityChange() {
  pageVisible.value = !document.hidden
  if (pageVisible.value) {
    void markAsReadIfVisible()
  }
}

async function syncMessages(showLoading: boolean) {
  if (showLoading) loading.value = true
  try {
    conversation.value = await getUserChatConversation()
    const page = await listUserChatMessages({ page: 1, page_size: 100 })
    if (showLoading) {
      messages.value = page.items
    } else {
      mergeMessages(page.items)
    }
    await scrollToBottom()
    // Mark as read only if user is viewing the page
    if (page.total > 0 && pageVisible.value) {
      await markAsReadIfVisible()
    }
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.loadFailed')))
  } finally {
    if (showLoading) loading.value = false
  }
}

async function reload() {
  await syncMessages(true)
}

async function handleSend(content: string) {
  sending.value = true
  try {
    const message = await sendUserChatMessage(content)
    appendMessage(message)
    if (conversation.value) conversation.value.unread_by_admin += 1
    composerClearNonce.value += 1
    await scrollToBottom()
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.sendFailed')))
  } finally {
    sending.value = false
  }
}

async function handleRichSend(payload: ComposerSubmitPayload) {
  sending.value = true
  let optimisticMessage: ChatMessage | null = null
  const shouldRevokePreviewUrls: string[] = []
  try {
    let content = payload.text.trim()

    // 处理回复
    if (payload.replyTo) {
      const replyPrefix = `[reply:${payload.replyTo}]`
      content = content ? `${replyPrefix}\n${content}` : replyPrefix
    }

    // 处理多图上传
    if (payload.images && payload.images.length > 0) {
      const imageContents: string[] = []

      for (const img of payload.images) {
        if (img.type === 'imageFile' && img.previewUrl && img.name) {
          // 先显示 optimistic 预览
          imageContents.push(imageMessageContent(img.previewUrl, img.name))
          shouldRevokePreviewUrls.push(img.previewUrl)
        } else if (img.type === 'imageUrl' && img.url && img.name) {
          // 直接使用 URL
          imageContents.push(imageMessageContent(img.url, img.name))
        }
      }

      // 显示 optimistic 消息
      if (imageContents.length > 0) {
        const optimisticContent = content ? `${content}\n${imageContents.join('\n')}` : imageContents.join('\n')
        optimisticMessage = appendOptimisticMessage(optimisticContent)
        composerClearNonce.value += 1
        await scrollToBottom()
      }

      // 上传所有图片文件
      const uploadedContents: string[] = []
      for (const img of payload.images) {
        if (img.type === 'imageFile' && img.file) {
          const asset = await uploadUserChatAsset(img.file)
          uploadedContents.push(imageMessageContent(asset.url, asset.name || img.name))
        } else if (img.type === 'imageUrl' && img.url && img.name) {
          uploadedContents.push(imageMessageContent(img.url, img.name))
        }
      }
      if (uploadedContents.length > 0) {
        content = content ? `${content}\n${uploadedContents.join('\n')}` : uploadedContents.join('\n')
      }
    }
    // 处理表情包
    else if (payload.sticker) {
      const stickerContent = stickerMessageContent(payload.sticker)
      content = content ? `${content}\n${stickerContent}` : stickerContent
      optimisticMessage = appendOptimisticMessage(content)
      composerClearNonce.value += 1
      await scrollToBottom()
    }

    if (!content) return
    const message = await sendUserChatMessage(content)
    if (optimisticMessage) {
      replaceMessage(optimisticMessage.id, message)
    } else {
      appendMessage(message)
    }
    if (conversation.value) conversation.value.unread_by_admin += 1
    composerClearNonce.value += 1
    replyingTo.value = null
    await scrollToBottom()
  } catch (error) {
    if (optimisticMessage) {
      const optimisticMessageID = optimisticMessage.id
      messages.value = messages.value.filter((item) => item.id !== optimisticMessageID)
    }
    appStore.showError(errorMessage(error, t('supportChat.sendFailed')))
  } finally {
    for (const url of shouldRevokePreviewUrls) {
      URL.revokeObjectURL(url)
    }
    sending.value = false
  }
}

function handleReply(message: ChatMessage) {
  replyingTo.value = message
}

function handleCancelReply() {
  replyingTo.value = null
}

onMounted(async () => {
  await reload()
  socket.connect()
  document.addEventListener('visibilitychange', handleVisibilityChange)

  // Mark as read when user scrolls
  const pane = messagePaneRef.value
  if (pane) {
    pane.addEventListener('scroll', () => {
      void markAsReadIfVisible()
    })
  }
})

onUnmounted(() => {
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})

watch(messageScrollSignature, () => {
  void scrollToBottom()
}, { flush: 'post' })

</script>
