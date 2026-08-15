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
              <button type="button" class="btn btn-secondary btn-sm px-2 sm:px-3" :disabled="messagesLoading || !selectedConversationID" @click="reloadSelectedMessages">
                <span class="hidden sm:inline">{{ t('common.refresh') }}</span>
                <svg class="h-4 w-4 sm:hidden" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M7.977 14.652H2.985m0 0v.001m0-.001l3.181-3.183a8.25 8.25 0 0113.803 3.7" />
                </svg>
              </button>
            </div>
          </header>

        <div ref="messagePaneRef" class="min-h-0 flex-1 overflow-y-auto bg-gray-50 p-3 dark:bg-dark-950/60 sm:p-5">
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
            :peer-read-at="selectedConversation?.last_read_by_user_at"
            @reply="replyingTo = $event"
          />
        </div>

        <SupportMessageComposer
          :sending="sending"
          :disabled="!selectedConversationID || messagesLoading"
          admin-mode
          :tools-busy="toolsBusy"
          :replying-to="replyingTo"
          :quick-replies="quickReplies"
          :library-assets="libraryAssets"
          :sticker-assets="stickerAssets"
          @submit="handleSend"
          @upload="handleUpload"
          @cancel-reply="replyingTo = null"
          @transfer="handleTransfer"
          @quick-reply-create="handleQuickReplyCreate"
          @quick-reply-update="handleQuickReplyUpdate"
          @quick-reply-delete="handleQuickReplyDelete"
          @quick-reply-reorder="handleQuickReplyReorder"
          @quick-reply-import="handleQuickReplyImport"
          @catalog-create="handleCatalogCreate"
          @catalog-delete="handleCatalogDelete"
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
  createAdminChatCatalogAsset,
  createAdminChatQuickReply,
  deleteAdminChatCatalogAsset,
  deleteAdminChatQuickReply,
  importAdminChatQuickReplies,
  listAdminChatCatalog,
  listAdminChatConversations,
  listAdminChatMessages,
  listAdminChatQuickReplies,
  markAdminChatRead,
  reorderAdminChatQuickReplies,
  sendAdminChatMessage,
  transferAdminChatBalance,
  updateAdminChatQuickReply,
  uploadAdminChatAsset,
  type ChatAsset,
  type ChatConversation,
  type ChatMessage,
  type ChatQuickReply,
  type ChatSendMessageInput,
} from '@/features/support-chat/data/datasources/supportChatDatasource'
import { useSupportChatSocket } from '@/features/support-chat/presentation/composables/useSupportChatSocket'
import { useSupportChatAdminStore } from '@/features/support-chat/presentation/stores/supportChatAdminStore'
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
const replyingTo = ref<ChatMessage | null>(null)
const messagesLoading = ref(false)
const sending = ref(false)
const toolsBusy = ref(false)
const quickReplies = ref<ChatQuickReply[]>([])
const libraryAssets = ref<ChatAsset[]>([])
const stickerAssets = ref<ChatAsset[]>([])
const search = ref('')
const unreadOnly = ref(false)
const socketConnected = ref(false)
const messagePaneRef = ref<HTMLElement | null>(null)
const userProfileDialogOpen = ref(false)
const userProfileLoading = ref(false)
const userProfile = ref<AdminUser | null>(null)
let searchTimer: ReturnType<typeof setTimeout> | null = null
let fallbackPollTimer: ReturnType<typeof setInterval> | null = null
let messageLoadSequence = 0
const SUPPORT_CHAT_RESYNC_MS = 15000
const SUPPORT_CHAT_CONNECTED_RESYNC_MS = 60000
let lastResyncAt = 0
const LEGACY_QUICK_REPLY_KEY = 'support_chat_custom_replies_v1'
const LEGACY_QUICK_REPLY_MIGRATED_KEY = 'support_chat_quick_replies_migrated_v2'

const selectedConversation = computed(() => {
  if (!selectedConversationID.value) return null
  return conversations.value.find((item) => item.id === selectedConversationID.value) ?? null
})

const socket = useSupportChatSocket({
  scope: 'admin',
  onStatusChange: (connected) => {
    socketConnected.value = connected
  },
  onMessage: async (message) => {
    upsertConversationActivity(message)
    if (selectedConversationID.value === message.conversation_id) {
      appendMessage(message)
      if (message.sender_type === 'user') await markSelectedRead()
      await scrollToBottom()
    }
  },
  onReadState: (readState) => {
    const conversation = conversations.value.find(item => item.id === readState.conversation_id)
    if (!conversation) return
    if (readState.reader === 'user') conversation.last_read_by_user_at = readState.read_at
    else conversation.last_read_by_admin_at = readState.read_at
  },
})

function appendMessage(message: ChatMessage) {
  if (messages.value.some((item) => item.id === message.id)) return
  messages.value.push(message)
}

function messageScrollSignature(): string {
  return messages.value.map((message) => `${message.id}:${message.created_at}`).join('|')
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

function upsertConversationActivity(message: ChatMessage) {
  const existing = conversations.value.find((item) => item.id === message.conversation_id)
  if (!existing) {
    void loadConversations()
    return
  }
  existing.last_message_at = message.created_at
  if (message.sender_type === 'user') {
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
  if (conversationsLoading.value) return
  conversationsLoading.value = true
  try {
    const page = await listAdminChatConversations({
      page: 1,
      page_size: 100,
      unread_only: unreadOnly.value,
      search: search.value.trim() || undefined,
    })
    conversations.value = page.items
    lastResyncAt = Date.now()
    conversationTotal.value = page.total
    if (unreadOnly.value) {
      supportChatAdminStore.setUnreadConversationCount(page.total)
    } else {
      void supportChatAdminStore.refreshUnreadIndicator(true)
    }
    if (selectedConversationID.value && !conversations.value.some((item) => item.id === selectedConversationID.value)) {
      selectedConversationID.value = null
      messages.value = []
    }
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.loadFailed')))
  } finally {
    conversationsLoading.value = false
  }
}

async function selectConversation(conversationID: number) {
  if (selectedConversationID.value === conversationID) return
  selectedConversationID.value = conversationID
  replyingTo.value = null
  messages.value = []
  await reloadSelectedMessages()
  await markSelectedRead()
}

function backToConversationList() {
  messageLoadSequence += 1
  selectedConversationID.value = null
  messages.value = []
  replyingTo.value = null
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
  if (!selectedConversationID.value) return
  const conversationID = selectedConversationID.value
  const sequence = ++messageLoadSequence
  messagesLoading.value = true
  try {
    const page = await listAdminChatMessages(conversationID, { page: 1, page_size: 100 })
    if (sequence !== messageLoadSequence || selectedConversationID.value !== conversationID) return
    messages.value = page.items
    await scrollToBottom()
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.loadFailed')))
  } finally {
    if (sequence === messageLoadSequence) messagesLoading.value = false
  }
}

async function markSelectedRead(conversationID = selectedConversationID.value) {
  if (!conversationID) return
  await markAdminChatRead(conversationID)
  const existing = conversations.value.find((item) => item.id === conversationID)
  if (existing) {
    existing.unread_by_admin = 0
    existing.last_read_by_admin_at = new Date().toISOString()
  }
  await supportChatAdminStore.refreshUnreadIndicator(true)
  appStore.setSupportInboxUnread(supportChatAdminStore.hasUnread)
}

async function handleSend(input: ChatSendMessageInput) {
  if (!selectedConversationID.value) return
  const conversationID = selectedConversationID.value
  sending.value = true
  try {
    const message = await sendAdminChatMessage(conversationID, input)
    if (selectedConversationID.value === conversationID) appendMessage(message)
    upsertConversationActivity(message)
    if (selectedConversationID.value === conversationID) {
      replyingTo.value = null
      await markSelectedRead(conversationID)
      await scrollToBottom()
    }
    void supportChatAdminStore.refreshUnreadIndicator(true)
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.sendFailed')))
  } finally {
    sending.value = false
  }
}

async function handleUpload(value: { file: File; content: string; reply_to_id: number | null }) {
  if (!selectedConversationID.value || sending.value) return
  const conversationID = selectedConversationID.value
  sending.value = true
  try {
    const asset = await uploadAdminChatAsset(conversationID, value.file)
    const message = await sendAdminChatMessage(conversationID, {
      content: value.content || '[image]',
      kind: 'image',
      asset_ids: [asset.id],
      reply_to_id: value.reply_to_id,
    })
    if (selectedConversationID.value === conversationID) appendMessage(message)
    upsertConversationActivity(message)
    if (selectedConversationID.value === conversationID) {
      replyingTo.value = null
      await scrollToBottom()
    }
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.assets.uploadFailed')))
  } finally {
    sending.value = false
  }
}

async function handleTransfer(value: { amount: number; notes: string }) {
  if (!selectedConversationID.value || sending.value) return
  const conversationID = selectedConversationID.value
  sending.value = true
  try {
    const result = await transferAdminChatBalance(conversationID, value.amount, value.notes)
    if (selectedConversationID.value === conversationID) appendMessage(result.message)
    upsertConversationActivity(result.message)
    if (selectedConversationID.value === conversationID) await scrollToBottom()
    appStore.showSuccess(t('supportChat.transfer.success'))
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.transfer.failed')))
  } finally {
    sending.value = false
  }
}

async function loadSupportTools() {
  await migrateLegacyQuickReplies()
  try {
    const [replies, library, stickers] = await Promise.all([
      listAdminChatQuickReplies(),
      listAdminChatCatalog('library'),
      listAdminChatCatalog('sticker'),
    ])
    quickReplies.value = replies
    libraryAssets.value = library
    stickerAssets.value = stickers
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.toolsLoadFailed')))
  }
}

async function migrateLegacyQuickReplies() {
  try {
    if (localStorage.getItem(LEGACY_QUICK_REPLY_MIGRATED_KEY) === 'true') return
    const raw = localStorage.getItem(LEGACY_QUICK_REPLY_KEY)
    const parsed = raw ? JSON.parse(raw) as unknown : []
    const items = Array.isArray(parsed) ? parsed.map((value) => {
      if (!value || typeof value !== 'object') return null
      const record = value as Record<string, unknown>
      const title = typeof record.title === 'string' ? record.title.trim().slice(0, 100) : ''
      const content = typeof record.content === 'string' ? record.content.trim().slice(0, 10000) : ''
      return title && content ? { title, content } : null
    }).filter((item): item is { title: string; content: string } => Boolean(item)).slice(0, 50) : []
    if (items.length > 0) await importAdminChatQuickReplies(items)
    localStorage.removeItem(LEGACY_QUICK_REPLY_KEY)
    localStorage.setItem(LEGACY_QUICK_REPLY_MIGRATED_KEY, 'true')
  } catch {
    // Keep legacy data for a future retry without blocking the support inbox.
  }
}

async function runTool(operation: () => Promise<void>) {
  if (toolsBusy.value) return
  toolsBusy.value = true
  try {
    await operation()
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.toolActionFailed')))
  } finally {
    toolsBusy.value = false
  }
}

function handleQuickReplyCreate(value: { title: string; content: string }) {
  void runTool(async () => {
    await createAdminChatQuickReply(value.title, value.content)
    quickReplies.value = await listAdminChatQuickReplies()
  })
}

function handleQuickReplyUpdate(value: { id: number; title: string; content: string }) {
  void runTool(async () => {
    await updateAdminChatQuickReply(value.id, value.title, value.content)
    quickReplies.value = await listAdminChatQuickReplies()
  })
}

function handleQuickReplyDelete(id: number) {
  void runTool(async () => {
    await deleteAdminChatQuickReply(id)
    quickReplies.value = await listAdminChatQuickReplies()
  })
}

function handleQuickReplyReorder(ids: number[]) {
  void runTool(async () => {
    await reorderAdminChatQuickReplies(ids)
    quickReplies.value = await listAdminChatQuickReplies()
  })
}

function handleQuickReplyImport(items: Array<{ title: string; content: string }>) {
  void runTool(async () => {
    quickReplies.value = await importAdminChatQuickReplies(items)
  })
}

function handleCatalogCreate(value: { scope: 'library' | 'sticker'; file: File; collection: string }) {
  void runTool(async () => {
    await createAdminChatCatalogAsset(value.scope, value.file, value.collection)
    const items = await listAdminChatCatalog(value.scope)
    if (value.scope === 'library') libraryAssets.value = items
    else stickerAssets.value = items
  })
}

function handleCatalogDelete(value: { scope: 'library' | 'sticker'; id: number }) {
  void runTool(async () => {
    await deleteAdminChatCatalogAsset(value.scope, value.id)
    const items = await listAdminChatCatalog(value.scope)
    if (value.scope === 'library') libraryAssets.value = items
    else stickerAssets.value = items
  })
}

async function resyncMessages() {
  if (sending.value) return
  if (socketConnected.value && Date.now() - lastResyncAt < SUPPORT_CHAT_CONNECTED_RESYNC_MS) return
  await loadConversations()
  if (selectedConversationID.value) {
    await reloadSelectedMessages()
  }
}

watch([search, unreadOnly], () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    void loadConversations()
  }, 250)
})

watch(messageScrollSignature, () => {
  void scrollToBottom()
}, { flush: 'post' })

onMounted(async () => {
  await Promise.all([loadConversations(), loadSupportTools()])
  socket.connect()
  fallbackPollTimer = setInterval(() => {
    void resyncMessages()
  }, SUPPORT_CHAT_RESYNC_MS)
})

onBeforeUnmount(() => {
  if (searchTimer) {
    clearTimeout(searchTimer)
    searchTimer = null
  }
  if (fallbackPollTimer) {
    clearInterval(fallbackPollTimer)
    fallbackPollTimer = null
  }
})
</script>
