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
          asset-scope="user"
          :peer-read-at="conversation?.last_read_by_admin_at"
          @reply="replyingTo = $event"
        />
      </div>

      <SupportMessageComposer
        ref="composerRef"
        :sending="sending"
        :disabled="loading"
        :replying-to="replyingTo"
        @submit="handleSend"
        @upload="handleUpload"
        @cancel-reply="replyingTo = null"
      />
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
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
  type ChatSendMessageInput,
} from '@/features/support-chat/data/datasources/supportChatDatasource'
import { useSupportChatSocket } from '@/features/support-chat/presentation/composables/useSupportChatSocket'
import { useSupportChatAdminStore } from '@/features/support-chat/presentation/stores/supportChatAdminStore'
import SupportMessageComposer from '@/features/support-chat/presentation/widgets/SupportMessageComposer.vue'
import SupportMessageList from '@/features/support-chat/presentation/widgets/SupportMessageList.vue'

const { t } = useI18n()
const appStore = useAppStore()
const supportChatAdminStore = useSupportChatAdminStore()
const loading = ref(false)
const sending = ref(false)
const messages = ref<ChatMessage[]>([])
const conversation = ref<ChatConversation | null>(null)
const replyingTo = ref<ChatMessage | null>(null)
const messagePaneRef = ref<HTMLElement | null>(null)
const composerRef = ref<InstanceType<typeof SupportMessageComposer> | null>(null)
const socketConnected = ref(false)
let fallbackPollTimer: ReturnType<typeof setInterval> | null = null
const SUPPORT_CHAT_RESYNC_MS = 15000
const SUPPORT_CHAT_CONNECTED_RESYNC_MS = 60000
let lastResyncAt = 0

const socket = useSupportChatSocket({
  scope: 'user',
  onStatusChange: (connected) => {
    socketConnected.value = connected
  },
  onMessage: (message) => {
    appendMessage(message)
    if (message.sender_type === 'admin') {
      appStore.setSupportUserUnread(true)
      supportChatAdminStore.markUserHasUnread()
      void markUserChatRead().then(() => {
        appStore.setSupportUserUnread(false)
        supportChatAdminStore.markUserRead()
      })
    }
    void scrollToBottom()
  },
  onMessageRecalled: (message) => {
    replaceMessage(message)
    if (replyingTo.value?.id === message.id) replyingTo.value = null
  },
  onReadState: (readState) => {
    if (readState.reader === 'admin' && conversation.value?.id === readState.conversation_id) {
      conversation.value.last_read_by_admin_at = readState.read_at
    }
  },
})

function appendMessage(message: ChatMessage) {
  if (messages.value.some((item) => item.id === message.id)) return
  messages.value.push(message)
}

function replaceMessage(message: ChatMessage) {
  const index = messages.value.findIndex((item) => item.id === message.id)
  if (index < 0) {
    messages.value.push(message)
    return
  }
  messages.value[index] = message
  messages.value = [...messages.value]
}

function messageScrollSignature(): string {
  return messages.value.map((message) => `${message.id}:${message.created_at}:${message.recalled_at || ''}`).join('|')
}

function errorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message
  if (typeof error === 'object' && error !== null && 'message' in error) {
    const message = (error as { message?: unknown }).message
    if (typeof message === 'string' && message.trim()) return message
  }
  return fallback
}

async function scrollToBottom() {
  await nextTick()
  const pane = messagePaneRef.value
  if (pane) pane.scrollTop = pane.scrollHeight
}

async function syncMessages(showLoading: boolean) {
  if (showLoading) loading.value = true
  try {
    const [currentConversation, page] = await Promise.all([
      getUserChatConversation(),
      listUserChatMessages({ page: 1, page_size: 100 }),
    ])
    conversation.value = currentConversation
    messages.value = page.items
    lastResyncAt = Date.now()
    if (currentConversation.unread_by_user > 0) {
      await markUserChatRead()
      currentConversation.unread_by_user = 0
      currentConversation.last_read_by_user_at = new Date().toISOString()
      appStore.setSupportUserUnread(false)
      supportChatAdminStore.markUserRead()
    }
    await scrollToBottom()
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.loadFailed')))
  } finally {
    if (showLoading) loading.value = false
  }
}

async function reload() {
  await syncMessages(true)
}

async function resyncMessages() {
  if (loading.value || sending.value) return
  if (socketConnected.value && Date.now() - lastResyncAt < SUPPORT_CHAT_CONNECTED_RESYNC_MS) return
  await syncMessages(false)
}

async function handleSend(input: ChatSendMessageInput) {
  sending.value = true
  try {
    const message = await sendUserChatMessage(input)
    appendMessage(message)
    composerRef.value?.clearDraft()
    replyingTo.value = null
    await scrollToBottom()
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.sendFailed')))
  } finally {
    sending.value = false
  }
}

async function handleUpload(value: { file: File; content: string; reply_to_id: number | null }) {
  if (sending.value) return
  sending.value = true
  try {
    const asset = await uploadUserChatAsset(value.file)
    const message = await sendUserChatMessage({
      content: value.content || '[image]',
      kind: 'image',
      asset_ids: [asset.id],
      reply_to_id: value.reply_to_id,
    })
    appendMessage(message)
    composerRef.value?.clearDraft()
    replyingTo.value = null
    await scrollToBottom()
  } catch (error) {
    appStore.showError(errorMessage(error, t('supportChat.assets.uploadFailed')))
  } finally {
    sending.value = false
  }
}

onMounted(async () => {
  await reload()
  socket.connect()
  fallbackPollTimer = setInterval(() => {
    void resyncMessages()
  }, SUPPORT_CHAT_RESYNC_MS)
})

watch(messageScrollSignature, () => {
  void scrollToBottom()
}, { flush: 'post' })

onBeforeUnmount(() => {
  if (fallbackPollTimer) {
    clearInterval(fallbackPollTimer)
    fallbackPollTimer = null
  }
})
</script>
