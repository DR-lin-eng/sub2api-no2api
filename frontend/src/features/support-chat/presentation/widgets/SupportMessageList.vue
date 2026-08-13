<template>
  <div class="mx-auto w-full max-w-4xl space-y-4">
    <div
      v-for="(message, index) in renderedMessages"
      :key="message.id"
      class="flex"
      :class="message.sender_type === ownSender ? 'justify-end' : 'justify-start'"
    >
      <div class="max-w-[82%] sm:max-w-[68%] lg:max-w-[58%]">
        <div
          class="mb-1 flex items-center gap-2 text-xs text-gray-500 dark:text-dark-400"
          :class="message.sender_type === ownSender ? 'justify-end' : 'justify-start'"
        >
          <span v-if="ownMessageReadState(index)" class="font-medium">
            {{ ownMessageReadState(index) }}
          </span>
          <span v-if="ownMessageReadState(index)">·</span>
          <span>{{ senderLabel(message.sender_type) }}</span>
          <span>·</span>
          <time :datetime="message.created_at">{{ formatTime(message.created_at) }}</time>
        </div>
        <div
          class="rounded-2xl text-sm leading-6 shadow-sm"
          :class="messageBubbleClass(message)"
        >
          <div v-if="message.parsed.html" class="support-chat-message-content" v-html="message.parsed.html"></div>
          <div
            v-if="message.parsed.sticker"
            class="support-chat-sticker"
            :class="message.parsed.html ? 'mt-3' : ''"
            :title="message.parsed.sticker.name"
          >
            <img
              v-if="message.parsed.sticker.url"
              :src="message.parsed.sticker.url"
              :alt="message.parsed.sticker.name"
              class="support-chat-sticker-image"
            />
            <span v-else class="support-chat-sticker-emoji">{{ message.parsed.sticker.emoji }}</span>
            <span class="support-chat-sticker-name">{{ message.parsed.sticker.name }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ChatMessage, ChatSenderType } from '@/features/support-chat/data/datasources/supportChatDatasource'
import { parseSupportMessageContent } from '@/features/support-chat/presentation/utils/supportChatMessageContent'

const props = defineProps<{
  messages: ChatMessage[]
  ownSender: ChatSenderType
  receiverUnreadCount?: number
}>()

const { t, locale } = useI18n()

const orderedMessages = computed(() => {
  return [...props.messages].sort((a, b) => {
    const at = Date.parse(a.created_at) || 0
    const bt = Date.parse(b.created_at) || 0
    if (at !== bt) return at - bt
    return a.id - b.id
  })
})

const renderedMessages = computed(() => {
  return orderedMessages.value.map((message) => ({
    ...message,
    parsed: parseSupportMessageContent(message.content),
  }))
})

function senderLabel(sender: ChatSenderType): string {
  return sender === 'admin' ? t('supportChat.supportAgent') : t('supportChat.user')
}

function formatTime(value: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(locale.value, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

function messageBubbleClass(message: ChatMessage & { parsed: ReturnType<typeof parseSupportMessageContent> }): string {
  const isOwn = message.sender_type === props.ownSender
  const stickerOnly = message.parsed.sticker && !message.parsed.html
  if (stickerOnly) {
    return isOwn
      ? 'border border-primary-200 bg-primary-50 px-3 py-2 text-primary-900 dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-50'
      : 'border border-gray-200 bg-white px-3 py-2 text-gray-900 dark:border-dark-700 dark:bg-dark-800 dark:text-white'
  }
  return isOwn
    ? 'bg-primary-600 px-4 py-3 text-white dark:bg-primary-500'
    : 'border border-gray-200 bg-white px-4 py-3 text-gray-900 dark:border-dark-700 dark:bg-dark-800 dark:text-white'
}

function ownMessageReadState(index: number): string | null {
  const message = renderedMessages.value[index]
  if (!message || message.sender_type !== props.ownSender) return null
  const unreadCount = Math.max(0, props.receiverUnreadCount ?? 0)
  if (unreadCount <= 0) return t('supportChat.read')

  const ownMessages = renderedMessages.value.filter((item) => item.sender_type === props.ownSender)
  const unreadOwnMessages = ownMessages.slice(Math.max(0, ownMessages.length - unreadCount))
  return unreadOwnMessages.some((item) => item.id === message.id) ? t('supportChat.unread') : t('supportChat.read')
}
</script>

<style scoped>
.support-chat-message-content {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.support-chat-message-content :deep(a) {
  text-decoration: underline;
}

.support-chat-message-content :deep(img) {
  display: block;
  max-width: min(18rem, 72vw);
  max-height: 22rem;
  border-radius: 0.75rem;
  object-fit: contain;
}

.support-chat-message-content :deep(p),
.support-chat-message-content :deep(div) {
  margin: 0.25rem 0;
}

.support-chat-message-content :deep(p:first-child),
.support-chat-message-content :deep(div:first-child),
.support-chat-message-content :deep(ul:first-child),
.support-chat-message-content :deep(ol:first-child),
.support-chat-message-content :deep(pre:first-child) {
  margin-top: 0;
}

.support-chat-message-content :deep(p:last-child),
.support-chat-message-content :deep(div:last-child),
.support-chat-message-content :deep(ul:last-child),
.support-chat-message-content :deep(ol:last-child),
.support-chat-message-content :deep(pre:last-child) {
  margin-bottom: 0;
}

.support-chat-message-content :deep(ul),
.support-chat-message-content :deep(ol) {
  margin: 0.35rem 0;
  padding-left: 1.25rem;
}

.support-chat-message-content :deep(ul) {
  list-style: disc;
}

.support-chat-message-content :deep(ol) {
  list-style: decimal;
}

.support-chat-message-content :deep(pre) {
  max-width: 100%;
  overflow-x: auto;
  border-radius: 0.75rem;
  margin: 0.5rem 0;
  padding: 0.75rem;
  background: rgb(17 24 39 / 0.12);
  white-space: pre;
}

.support-chat-message-content :deep(code) {
  border-radius: 0.375rem;
  padding: 0.1rem 0.25rem;
  background: rgb(17 24 39 / 0.12);
}

.support-chat-sticker {
  display: inline-flex;
  min-width: 5.5rem;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.25rem;
  border-radius: 1rem;
}

.support-chat-sticker-emoji {
  font-size: 3.25rem;
  line-height: 1;
  filter: drop-shadow(0 8px 12px rgb(15 23 42 / 0.14));
}

.support-chat-sticker-image {
  width: 5rem;
  height: 5rem;
  object-fit: contain;
  filter: drop-shadow(0 8px 12px rgb(15 23 42 / 0.14));
}

.support-chat-sticker-name {
  max-width: 8rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.75rem;
  font-weight: 600;
  line-height: 1rem;
  opacity: 0.78;
}
</style>
