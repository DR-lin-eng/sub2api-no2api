<template>
  <AppLayout>
    <section class="grid h-[calc(100dvh-5.5rem)] min-h-0 grid-cols-1 gap-4 lg:h-[calc(100vh-8rem)] lg:grid-cols-[360px_minmax(0,1fr)]">
      <div class="min-h-0" :class="selectedConversationID ? 'hidden lg:block' : 'block'">
        <AdminConversationList
          v-model:search="search"
          v-model:unread-only="unreadOnly"
          class="h-full"
          :conversations="conversations"
          :selected-id="selectedConversationID"
          :loading="conversationsLoading"
          :total="conversationTotal"
          @select="selectConversation"
          @view-user="openUserProfile"
          @refresh="loadConversations"
        />
      </div>

      <div class="min-h-0" :class="!selectedConversationID ? 'hidden lg:block' : 'block'">
        <div class="flex h-full min-h-0 flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <header class="flex items-center justify-between gap-2 border-b border-gray-200 px-3 py-3 dark:border-dark-700 sm:gap-3 sm:px-5 sm:py-4">
            <button
              v-if="selectedConversationID"
              type="button"
              class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-gray-200 text-gray-600 transition-colors hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:border-dark-700 dark:text-dark-300 dark:hover:bg-dark-800 lg:hidden"
              :title="t('common.back')"
              @click="backToConversationList"
            >
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8">
                <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
              </svg>
            </button>

            <div class="min-w-0 flex-1">
              <h1 class="truncate text-lg font-semibold text-gray-900 dark:text-white">
                <button
                  v-if="selectedConversation"
                  type="button"
                  class="-mx-1 -my-0.5 max-w-full truncate rounded-md px-1 py-0.5 text-left transition-colors hover:bg-primary-50 hover:text-primary-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:hover:bg-primary-900/20 dark:hover:text-primary-200"
                  :title="t('supportChat.userProfile.viewProfile')"
                  @click="openUserProfile(selectedConversation)"
                >
                  {{ displayUser(selectedConversation) }}
                </button>
                <span v-else>{{ t('supportChat.adminTitle') }}</span>
              </h1>
              <p class="truncate text-sm text-gray-500 dark:text-dark-400">
                {{ selectedConversation ? selectedConversation.user_email || `#${selectedConversation.user_id}` : t('supportChat.adminDescription') }}
              </p>
            </div>
            <div class="flex shrink-0 items-center gap-1 text-xs sm:gap-2">
              <span class="inline-flex items-center gap-1 rounded-full px-2 py-1 font-medium sm:px-2.5" :class="socketConnected ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-200' : 'bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-dark-300'">
                <span class="h-2 w-2 rounded-full" :class="socketConnected ? 'bg-emerald-500' : 'bg-gray-400'"></span>
                <span class="hidden sm:inline">{{ socketConnected ? t('supportChat.connected') : t('supportChat.offline') }}</span>
              </span>
              <button type="button" class="btn btn-secondary btn-sm px-2 sm:px-3" :disabled="messagesLoading || olderMessagesLoading || !selectedConversationID" @click="reloadSelectedMessages">
                <span class="hidden sm:inline">{{ t('common.refresh') }}</span>
                <svg class="h-4 w-4 sm:hidden" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M7.977 14.652H2.985m0 0v.001m0-.001l3.181-3.183a8.25 8.25 0 0113.803 3.7" />
                </svg>
              </button>
              <button
                v-if="selectedConversationID"
                type="button"
                class="btn btn-secondary btn-sm px-2 sm:px-3"
                :disabled="markingUnread || selectedConversation?.manually_unread_by_admin"
                @click="markSelectedUnread"
              >
                <span class="hidden sm:inline">{{ selectedConversation?.manually_unread_by_admin ? t('supportChat.markedUnread') : t('supportChat.markUnread') }}</span>
                <span class="sm:hidden">{{ selectedConversation?.manually_unread_by_admin ? '!' : '•' }}</span>
              </button>
            </div>
          </header>

        <div
          ref="messagePaneRef"
          class="min-h-0 flex-1 overflow-y-auto bg-gray-50 p-3 dark:bg-dark-950/60 sm:p-5"
          @scroll="handleMessagePaneScroll"
        >
          <div v-if="selectedConversationID && hasOlderMessages" class="sticky top-0 z-10 mb-3 flex justify-center">
            <button type="button" class="btn btn-secondary btn-sm shadow-sm" :disabled="messagesLoading || olderMessagesLoading" @click="loadOlderMessages">
              {{ olderMessagesLoading ? t('common.loading') : t('supportChat.loadOlder') }}
            </button>
          </div>
          <div v-if="!selectedConversationID" class="flex h-full flex-col items-center justify-center text-center text-gray-500 dark:text-dark-400">
            <div class="mb-3 rounded-2xl bg-primary-50 p-4 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300">
              <svg class="h-8 w-8" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M8.625 12a.375.375 0 11-.75 0 .375.375 0 01.75 0zm3.75 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zm3.75 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z" />
                <path stroke-linecap="round" stroke-linejoin="round" d="M21 12c0 4.556-4.03 8.25-9 8.25a9.77 9.77 0 01-2.555-.337A5.972 5.972 0 015.41 21a5.969 5.969 0 01-.474-.018 4.48 4.48 0 00.978-2.025c.09-.457-.133-.901-.467-1.226C3.93 16.253 3 14.224 3 12c0-4.556 4.03-8.25 9-8.25s9 3.694 9 8.25z" />
              </svg>
            </div>
            <p class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('supportChat.selectConversationTitle') }}</p>
            <p class="mt-1 text-sm">{{ t('supportChat.selectConversationDescription') }}</p>
          </div>
          <div v-else-if="messagesLoading && messages.length === 0" class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-dark-400">
            {{ t('common.loading') }}
          </div>
          <div v-else-if="messages.length === 0" class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-dark-400">
            {{ t('supportChat.noMessagesYet') }}
          </div>
          <SupportMessageList
            v-else
            :messages="messages"
            own-sender="admin"
            asset-scope="admin"
            :receiver-unread-count="selectedConversation?.unread_by_user ?? 0"
            :show-read-state="true"
            allow-recall
            :recalling-message-id="recallingMessageID"
            @reply="handleReply"
            @recall="requestRecall"
          />
        </div>

        <SupportMessageComposer
          :sending="sending"
          :disabled="!selectedConversationID || messagesLoading"
          :draft-key="selectedConversationID ? `admin:${selectedConversationID}` : 'admin:none'"
          :clear-nonce="composerClearNonce"
          :image-library="imageLibrary"
          :stickers="stickers"
          :replying-to="replyingTo"
          :conversation="selectedConversation"
          show-assistant-tools
          @submit="handleSend"
          @submit-rich="handleRichSend"
          @library-image-add-selected="handleLibraryImageAddSelected"
          @library-image-delete="handleLibraryImageDelete"
          @sticker-add-selected="handleStickerAddSelected"
          @sticker-delete="handleStickerDelete"
          @cancel-reply="handleCancelReply"
          @transfer="handleTransfer"
        />
        </div>
      </div>
    </section>

    <SupportUserProfileDialog
      :show="userProfileDialogOpen"
      :loading="userProfileLoading"
      :user="userProfile"
      @close="closeUserProfile"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import { useAppStore } from '@/core/stores/appStore'
import { getById as getAdminUserById } from '@/features/admin-users/data/datasources/adminUsersDatasource'
import {
  createAdminChatImageLibraryItem,
  createAdminChatSticker,
  deleteAdminChatImageLibraryItem,
  deleteAdminChatSticker,
  listAdminChatConversations,
  listAdminChatImageLibrary,
  listAdminChatMessages,
  listAdminChatStickers,
  markAdminChatRead,
  markAdminChatUnread,
  recallAdminChatMessage,
  sendAdminChatRichMessage,
  transferAdminChatBalance,
  uploadAdminChatAsset,
  type ChatConversation,
  type ChatSendMessageInput,
  type ChatImageLibraryItem,
  type ChatMessage,
  type ChatStickerItem,
} from '@/features/support-chat/data/datasources/supportChatDatasource'
import { useSupportChatSocket } from '@/features/support-chat/presentation/composables/useSupportChatSocket'
import {
  chatMessagePageCount,
  loadChatMessagePages,
  mergeChatMessages,
  SUPPORT_CHAT_MESSAGE_PAGE_SIZE,
} from '@/features/support-chat/presentation/composables/supportChatHistory'
import { useSupportChatAdminStore } from '@/features/support-chat/presentation/stores/supportChatAdminStore'
import {
  buildImageMessageContent,
  buildStickerMessageContent,
} from '@/features/support-chat/presentation/utils/supportChatMessageContent'
import AdminConversationList from '@/features/support-chat/presentation/widgets/AdminConversationList.vue'
import SupportMessageComposer from '@/features/support-chat/presentation/widgets/SupportMessageComposer.vue'
import SupportMessageList from '@/features/support-chat/presentation/widgets/SupportMessageList.vue'
import SupportUserProfileDialog from '@/features/support-chat/presentation/widgets/SupportUserProfileDialog.vue'
import type { AdminUser } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()
const supportChatAdminStore = useSupportChatAdminStore()
const conversations = ref<ChatConversation[]>([])
const conversationTotal = ref(0)
const conversationsLoading = ref(false)
const selectedConversationID = ref<number | null>(null)
const messages = ref<ChatMessage[]>([])
const messagesLoading = ref(false)
const olderMessagesLoading = ref(false)
const sending = ref(false)
const recallingMessageID = ref<number | null>(null)
const markingUnread = ref(false)
const search = ref('')
const unreadOnly = ref(false)
const socketConnected = ref(false)
const messagePaneRef = ref<HTMLElement | null>(null)
const userProfileDialogOpen = ref(false)
const userProfileLoading = ref(false)
const userProfile = ref<AdminUser | null>(null)
const composerClearNonce = ref(0)
const imageLibrary = ref<ChatImageLibraryItem[]>([])
const stickers = ref<ChatStickerItem[]>([])
const replyingTo = ref<ChatMessage | null>(null)
const selectedConversationSnapshot = ref<ChatConversation | null>(null)
const messagePage = ref(1)
const messagePages = ref(1)
const stickMessagesToBottom = ref(true)
let searchTimer: ReturnType<typeof setTimeout> | null = null
let optimisticMessageSeed = 0
let conversationsRequestID = 0

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

const selectedConversation = computed(() => {
  if (!selectedConversationID.value) return null
  return conversations.value.find((item) => item.id === selectedConversationID.value)
    ?? selectedConversationSnapshot.value
})

const hasOlderMessages = computed(() => messagePage.value < messagePages.value)

const socket = useSupportChatSocket({
  scope: 'admin',
  onStatusChange: (connected) => {
    socketConnected.value = connected
  },
  onMessage: async (message) => {
    const isCurrentConversation = selectedConversationID.value === message.conversation_id

    // Update conversation list
    upsertConversationActivity(message, isCurrentConversation)

    // If user sent a message, they've read all admin messages
    if (message.sender_type === 'user') {
      const existing = conversations.value.find((item) => item.id === message.conversation_id)
      if (existing) existing.unread_by_user = 0
    }

    // If viewing this conversation, append message and mark as read
    if (isCurrentConversation) {
      appendMessage(message)
      if (message.sender_type === 'user') {
        void markSelectedRead()
      }
      if (stickMessagesToBottom.value) await scrollToBottom()
    }
  },
  onMessageRecalled: (message) => {
    if (selectedConversationID.value === message.conversation_id) replaceMessage(message.id, message)
  },
  onReadState: (conversationID, reader) => {
    // When user marks as read, clear unread_by_user for that conversation
    if (reader === 'user') {
      const existing = conversations.value.find((item) => item.id === conversationID)
      if (existing) existing.unread_by_user = 0
    }
  },
})

function appendMessage(message: ChatMessage) {
  if (messages.value.some((item) => item.id === message.id)) return
  stickMessagesToBottom.value = isMessagePaneNearBottom()
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

function isMessagePaneNearBottom(): boolean {
  const pane = messagePaneRef.value
  if (!pane) return true
  return pane.scrollHeight - pane.scrollTop - pane.clientHeight < 80
}

function displayUser(conversation: ChatConversation): string {
  return conversation.user_username || conversation.user_email || t('supportChat.unknownUser')
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
  if (!selectedConversationID.value || !content) return null
  const message: ChatMessage = {
    id: nextOptimisticMessageID(),
    conversation_id: selectedConversationID.value,
    sender_type: 'admin',
    sender_id: 0,
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

function upsertConversationActivity(message: ChatMessage, isCurrentlyViewing = false) {
  const existing = conversations.value.find((item) => item.id === message.conversation_id)
  if (!existing) {
    void loadConversations()
    return
  }
  existing.last_message_at = message.created_at
  if (message.sender_type === 'user' && !isCurrentlyViewing) {
    // Only increment unread if admin is NOT currently viewing this conversation
    existing.unread_by_admin += 1
    appStore.setSupportInboxUnread(true)
    supportChatAdminStore.markHasUnread()
  }
  conversations.value = [...conversations.value].sort((a, b) => {
    const at = Date.parse(a.last_message_at || a.updated_at) || 0
    const bt = Date.parse(b.last_message_at || b.updated_at) || 0
    if (at !== bt) return bt - at
    return b.id - a.id
  })
}

async function scrollToBottom() {
  await nextTick()
  const pane = messagePaneRef.value
  if (pane) pane.scrollTop = pane.scrollHeight
}

async function loadConversations() {
  const requestID = ++conversationsRequestID
  conversationsLoading.value = true
  try {
    const page = await listAdminChatConversations({
      page: 1,
      page_size: 100,
      unread_only: unreadOnly.value,
      search: search.value.trim() || undefined,
    })
    if (requestID !== conversationsRequestID) return
    conversations.value = page.items
    conversationTotal.value = page.total
    if (unreadOnly.value) {
      supportChatAdminStore.setUnreadConversationCount(page.total)
    } else {
      void supportChatAdminStore.refreshUnreadIndicator(true)
    }
    if (selectedConversationID.value && !conversations.value.some((item) => item.id === selectedConversationID.value)) {
      selectedConversationSnapshot.value = conversations.value.find((item) => item.id === selectedConversationID.value) ?? selectedConversationSnapshot.value
    }
  } catch (error) {
    if (requestID === conversationsRequestID) {
      appStore.showError(errorMessage(error, t('supportChat.loadFailed')))
    }
  } finally {
    if (requestID === conversationsRequestID) {
      conversationsLoading.value = false
    }
  }
}

async function selectConversation(conversationID: number) {
  if (selectedConversationID.value === conversationID) return
  selectedConversationID.value = conversationID
  selectedConversationSnapshot.value = conversations.value.find((item) => item.id === conversationID) ?? null
  messages.value = []
  messagePage.value = 1
  messagePages.value = 1
  stickMessagesToBottom.value = true
  await reloadSelectedMessages()
  await markSelectedRead()
}

function backToConversationList() {
  selectedConversationID.value = null
  selectedConversationSnapshot.value = null
  messages.value = []
  messagePage.value = 1
  messagePages.value = 1
}

async function openUserProfile(conversation: ChatConversation) {
  userProfileDialogOpen.value = true
  userProfile.value = null
  userProfileLoading.value = true
  try {
    userProfile.value = await getAdminUserById(conversation.user_id)
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.userProfile.loadFailed')))
    userProfileDialogOpen.value = false
  } finally {
    userProfileLoading.value = false
  }
}

function closeUserProfile() {
  userProfileDialogOpen.value = false
  userProfile.value = null
}

async function reloadSelectedMessages() {
  await syncSelectedMessages(true)
}

async function syncSelectedMessages(showLoading: boolean) {
  if (!selectedConversationID.value || messagesLoading.value) return
  if (showLoading) messagesLoading.value = true
  try {
    const requestedPage = Math.max(1, messagePage.value)
    const history = await loadChatMessagePages(
      (page) => listAdminChatMessages(selectedConversationID.value!, { page, page_size: SUPPORT_CHAT_MESSAGE_PAGE_SIZE }),
      requestedPage,
    )
    if (showLoading) {
      messages.value = history.items
      messagePage.value = history.page
      messagePages.value = history.pages
      if (stickMessagesToBottom.value) await scrollToBottom()
    } else if (mergeMessages(history.items) && stickMessagesToBottom.value) {
      await scrollToBottom()
    }
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.loadFailed')))
  } finally {
    if (showLoading) messagesLoading.value = false
  }
}

async function loadOlderMessages() {
  const conversationID = selectedConversationID.value
  if (!conversationID || olderMessagesLoading.value || messagesLoading.value || !hasOlderMessages.value) return
  const nextPage = messagePage.value + 1
  const pane = messagePaneRef.value
  const previousHeight = pane?.scrollHeight ?? 0
  const previousTop = pane?.scrollTop ?? 0
  olderMessagesLoading.value = true
  stickMessagesToBottom.value = false
  try {
    const page = await listAdminChatMessages(conversationID, { page: nextPage, page_size: SUPPORT_CHAT_MESSAGE_PAGE_SIZE })
    if (selectedConversationID.value !== conversationID) return
    messages.value = mergeChatMessages(messages.value, page.items)
    messagePage.value = Math.max(messagePage.value, nextPage)
    messagePages.value = Math.max(messagePages.value, chatMessagePageCount(page))
    await nextTick()
    if (pane) pane.scrollTop = previousTop + (pane.scrollHeight - previousHeight)
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.loadFailed')))
  } finally {
    olderMessagesLoading.value = false
  }
}

function handleMessagePaneScroll(event: Event) {
  const pane = event.target as HTMLElement | null
  if (!pane) return
  stickMessagesToBottom.value = pane.scrollHeight - pane.scrollTop - pane.clientHeight < 80
  if (pane.scrollTop <= 80) void loadOlderMessages()
}

async function markSelectedRead() {
  if (!selectedConversationID.value) return
  await markAdminChatRead(selectedConversationID.value)
  const existing = conversations.value.find((item) => item.id === selectedConversationID.value)
  const target = existing ?? selectedConversationSnapshot.value
  if (target) {
    target.unread_by_admin = 0
    target.manually_unread_by_admin = false
  }
  await supportChatAdminStore.refreshUnreadIndicator(true)
  appStore.setSupportInboxUnread(supportChatAdminStore.hasUnread)
}

async function markSelectedUnread() {
  if (!selectedConversationID.value || markingUnread.value) return
  const existing = conversations.value.find((item) => item.id === selectedConversationID.value)
  if (existing?.manually_unread_by_admin) return
  markingUnread.value = true
  try {
    await markAdminChatUnread(selectedConversationID.value)
    if (existing) existing.manually_unread_by_admin = true
    await supportChatAdminStore.refreshUnreadIndicator(true)
    appStore.setSupportInboxUnread(supportChatAdminStore.hasUnread)
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.markUnreadFailed')))
  } finally {
    markingUnread.value = false
  }
}

function requestRecall(message: ChatMessage) {
  if (!selectedConversationID.value || message.sender_type !== 'admin' || message.recalled_at || message.kind === 'balance_transfer') return
  if (!window.confirm(t('supportChat.recall.confirmMessage'))) return
  void confirmRecall(message)
}

async function confirmRecall(message: ChatMessage) {
  if (recallingMessageID.value) return
  recallingMessageID.value = message.id
  try {
    const recalled = await recallAdminChatMessage(message.conversation_id, message.id)
    replaceMessage(message.id, recalled)
    appStore.showSuccess(t('supportChat.recall.success'))
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.recall.failed')))
  } finally {
    recallingMessageID.value = null
  }
}

async function handleSend(content: string) {
  if (!selectedConversationID.value) return
  sending.value = true
  try {
    const message = await sendAdminChatRichMessage(selectedConversationID.value, { content, kind: 'text' })
    appendMessage(message)
    upsertConversationActivity(message, true)
    const existing = conversations.value.find((item) => item.id === message.conversation_id)
    if (existing) existing.unread_by_user += 1
    composerClearNonce.value += 1
    await markSelectedRead()
    await scrollToBottom()
    void supportChatAdminStore.refreshUnreadIndicator(true)
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.sendFailed')))
  } finally {
    sending.value = false
  }
}

async function handleTransfer(value: { amount: number; notes: string }) {
  if (!selectedConversationID.value || sending.value) return
  sending.value = true
  try {
    const result = await transferAdminChatBalance(selectedConversationID.value, value.amount, value.notes)
    if (result.message) {
      appendMessage(result.message)
      upsertConversationActivity(result.message, true)
      const existing = conversations.value.find((item) => item.id === result.message!.conversation_id)
      if (existing) existing.unread_by_user += 1
      await markSelectedRead()
      if (stickMessagesToBottom.value) await scrollToBottom()
    }
    appStore.showSuccess(t('supportChat.composer.transferSuccess', { amount: value.amount, newBalance: result.balance }))
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.composer.transferFailed')))
  } finally {
    sending.value = false
  }
}

async function handleRichSend(payload: ComposerSubmitPayload) {
  if (!selectedConversationID.value) return
  sending.value = true
  let optimisticMessage: ChatMessage | null = null
  const shouldRevokePreviewUrls: string[] = []
  try {
    let content = payload.text.trim()
    let structuredInput: ChatSendMessageInput | null = null

    // 处理回复
    if (payload.replyTo) structuredInput = { kind: 'text', reply_to_id: payload.replyTo }

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
      const assetIDs: number[] = []
      for (const img of payload.images) {
        if (img.type === 'imageFile' && img.file) {
          const asset = await uploadAdminChatAsset(img.file, selectedConversationID.value)
          uploadedContents.push(imageMessageContent(asset.url, asset.name || img.name))
          if (asset.id) assetIDs.push(asset.id)
        } else if (img.type === 'imageUrl' && img.url && img.name) {
          uploadedContents.push(imageMessageContent(img.url, img.name))
          const assetID = Number((img.url.match(/\/assets\/(\d+)$/) || [])[1])
          if (Number.isFinite(assetID) && assetID > 0) assetIDs.push(assetID)
        }
      }
      if (uploadedContents.length > 0) {
        content = content ? `${content}\n${uploadedContents.join('\n')}` : uploadedContents.join('\n')
      }
      if (assetIDs.length > 0) structuredInput = { ...structuredInput, kind: 'image', asset_ids: assetIDs, content: payload.text.trim() || undefined }
    }
    // 处理表情包
    else if (payload.sticker) {
      const stickerContent = stickerMessageContent(payload.sticker)
      content = content ? `${content}\n${stickerContent}` : stickerContent
      optimisticMessage = appendOptimisticMessage(content)
      composerClearNonce.value += 1
      if (stickMessagesToBottom.value) await scrollToBottom()
      const stickerID = Number(payload.sticker.id)
      structuredInput = Number.isFinite(stickerID) && stickerID > 0
        ? { ...structuredInput, kind: 'sticker', asset_ids: [stickerID], content: payload.text.trim() || undefined }
        : { ...structuredInput, kind: 'sticker', content: payload.text.trim() || undefined, sticker: { name: payload.sticker.name, emoji: payload.sticker.emoji } }
    }

    if (!content) return
    const message = await sendAdminChatRichMessage(selectedConversationID.value, structuredInput?.kind
      ? { ...structuredInput, content: structuredInput.content ?? (structuredInput.kind === 'text' ? content : undefined) }
      : { content, kind: 'text' })
    if (optimisticMessage) {
      replaceMessage(optimisticMessage.id, message)
    } else {
      appendMessage(message)
    }
    upsertConversationActivity(message)
    const existing = conversations.value.find((item) => item.id === message.conversation_id)
    if (existing) existing.unread_by_user += 1
    await markSelectedRead()
    await scrollToBottom()
    composerClearNonce.value += 1
    replyingTo.value = null
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

async function handleLibraryImageAddSelected(file: File) {
  sending.value = true
  try {
    const item = await createAdminChatImageLibraryItem({
      file,
      name: file.name,
      category: t('supportChat.composer.defaultImageCategory'),
    })
    imageLibrary.value = [item, ...imageLibrary.value]
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.imageUploadFailed')))
  } finally {
    sending.value = false
  }
}

async function handleLibraryImageDelete(image: { id: string }) {
  try {
    await deleteAdminChatImageLibraryItem(image.id)
    imageLibrary.value = imageLibrary.value.filter((item) => item.id !== image.id)
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.imageDeleteFailed')))
  }
}

async function handleStickerAddSelected(file: File) {
  sending.value = true
  try {
    const item = await createAdminChatSticker({
      file,
      name: file.name,
      group: t('supportChat.composer.defaultStickerGroup'),
    })
    stickers.value = [item, ...stickers.value]
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.stickerUploadFailed')))
  } finally {
    sending.value = false
  }
}

async function handleStickerDelete(sticker: { id: string }) {
  try {
    await deleteAdminChatSticker(sticker.id)
    stickers.value = stickers.value.filter((item) => item.id !== sticker.id)
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.stickerDeleteFailed')))
  }
}

async function loadImageLibrary() {
  try {
    imageLibrary.value = await listAdminChatImageLibrary()
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.loadFailed')))
  }
}

async function loadStickers() {
  try {
    stickers.value = await listAdminChatStickers()
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.loadFailed')))
  }
}

watch([search, unreadOnly], () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    void loadConversations()
  }, 250)
})

watch(messageScrollSignature, () => {
  if (stickMessagesToBottom.value) void scrollToBottom()
}, { flush: 'post' })

onMounted(async () => {
  await loadConversations()
  void loadImageLibrary()
  void loadStickers()
  socket.connect()
})

onBeforeUnmount(() => {
  conversationsRequestID += 1
  if (searchTimer) {
    clearTimeout(searchTimer)
    searchTimer = null
  }
})
</script>
