<template>
  <div class="mx-auto w-full max-w-4xl space-y-4">
    <div
      v-for="(message, index) in renderedMessages"
      :key="message.id"
      :data-message-id="message.id"
      class="flex"
      :class="message.sender_type === ownSender ? 'justify-end' : 'justify-start'"
    >
      <div class="max-w-[82%] sm:max-w-[68%] lg:max-w-[58%]">
        <div
          class="mb-1 flex items-center gap-2 text-xs text-gray-500 dark:text-dark-400"
          :class="message.sender_type === ownSender ? 'justify-end' : 'justify-start'"
        >
          <span v-if="props.showReadState && ownMessageReadState(index)" class="font-medium">
            {{ ownMessageReadState(index) }}
          </span>
          <span v-if="props.showReadState && ownMessageReadState(index)">·</span>
          <span>{{ senderLabel(message.sender_type) }}</span>
          <span>·</span>
          <time :datetime="message.created_at">{{ formatTime(message.created_at) }}</time>
        </div>

        <!-- 引用消息预览 -->
        <div
          v-if="message.parsed.replyToId && findMessageById(message.parsed.replyToId)"
          class="mb-2 cursor-pointer rounded-lg border-l-4 bg-gray-50 px-3 py-2 text-xs transition-colors hover:bg-gray-100 dark:bg-dark-800 dark:hover:bg-dark-700"
          :class="message.sender_type === ownSender
            ? 'border-primary-400 dark:border-primary-500'
            : 'border-gray-400 dark:border-gray-600'"
          @click="scrollToMessage(message.parsed.replyToId!)"
        >
          <div class="flex items-center gap-1.5 text-gray-500 dark:text-dark-400">
            <svg class="h-3 w-3 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 10h10a8 8 0 018 8v2M3 10l6 6m-6-6l6-6" />
            </svg>
            <span class="font-medium">{{ senderLabel(findMessageById(message.parsed.replyToId)!.sender_type) }}</span>
          </div>
          <div class="mt-1 truncate text-gray-700 dark:text-dark-300">
            {{ getMessagePreviewText(findMessageById(message.parsed.replyToId)!) }}
          </div>
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
          >
            <img
              v-if="message.parsed.sticker.url"
              :src="message.parsed.sticker.url"
              :alt="message.parsed.sticker.name"
              class="support-chat-sticker-image"
            />
            <span v-else class="support-chat-sticker-emoji">{{ message.parsed.sticker.emoji }}</span>
          </div>
        </div>
        <button
          v-if="message.sender_type !== ownSender"
          type="button"
          class="mt-1.5 inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-xs text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-dark-200"
          @click="emit('reply', message)"
        >
          <svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 10h10a8 8 0 018 8v2M3 10l6 6m-6-6l6-6" />
          </svg>
          <span>{{ t('supportChat.composer.replyingTo') }}</span>
        </button>
      </div>
    </div>
  </div>

  <!-- 图片预览模态框 -->
  <Teleport to="body">
    <Transition name="image-preview">
      <div
        v-if="previewImage"
        class="fixed inset-0 z-[9999] flex items-center justify-center bg-black/90 backdrop-blur-sm"
        @click="closePreview"
      >
        <button
          type="button"
          class="absolute right-4 top-4 flex h-10 w-10 items-center justify-center rounded-full bg-white/10 text-3xl leading-none text-white transition-colors hover:bg-white/20"
          @click="closePreview"
        >
          ×
        </button>
        <img
          :src="previewImage"
          alt=""
          class="max-h-[90vh] max-w-[90vw] object-contain"
          @click.stop
        />
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ChatMessage, ChatSenderType } from '@/features/support-chat/data/datasources/supportChatDatasource'
import { parseSupportMessageContent } from '@/features/support-chat/presentation/utils/supportChatMessageContent'

const props = defineProps<{
  messages: ChatMessage[]
  ownSender: ChatSenderType
  receiverUnreadCount?: number
  showReadState?: boolean // 是否显示已读状态，默认 false
}>()

const emit = defineEmits<{
  reply: [message: ChatMessage]
}>()

const { t, locale } = useI18n()

const previewImage = ref<string | null>(null)

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

function findMessageById(id: number): ChatMessage | undefined {
  return props.messages.find((msg) => msg.id === id)
}

function getMessagePreviewText(message: ChatMessage): string {
  const parsed = parseSupportMessageContent(message.content)
  if (parsed.sticker) {
    return `[${parsed.sticker.name}]`
  }
  // 移除 HTML 标签并截取前 50 个字符
  const text = parsed.html.replace(/<[^>]+>/g, '').trim()
  return text.length > 50 ? text.substring(0, 50) + '...' : text
}

function scrollToMessage(messageId: number) {
  const element = document.querySelector(`[data-message-id="${messageId}"]`)
  if (!element) return

  // 滚动到消息
  element.scrollIntoView({ behavior: 'smooth', block: 'center' })

  // 添加高亮动画
  const bubble = element.querySelector('.rounded-2xl') as HTMLElement
  if (!bubble) return

  // 添加高亮类
  bubble.classList.add('message-highlight')

  // 2 秒后移除高亮
  setTimeout(() => {
    bubble.classList.remove('message-highlight')
  }, 2000)
}

function handleImageClick(event: Event) {
  const target = event.target as HTMLElement
  if (target.tagName === 'IMG' && target.closest('.support-chat-message-content')) {
    const img = target as HTMLImageElement
    if (img.src && !img.closest('.support-chat-sticker')) {
      previewImage.value = img.src
    }
  }
}

function closePreview() {
  previewImage.value = null
}

function handleEscKey(event: KeyboardEvent) {
  if (event.key === 'Escape' && previewImage.value) {
    closePreview()
  }
}

onMounted(() => {
  document.addEventListener('click', handleImageClick)
  document.addEventListener('keydown', handleEscKey)
})

onUnmounted(() => {
  document.removeEventListener('click', handleImageClick)
  document.removeEventListener('keydown', handleEscKey)
})
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
  cursor: pointer;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.support-chat-message-content :deep(img:hover) {
  transform: scale(1.02);
  box-shadow: 0 4px 12px rgb(0 0 0 / 0.15);
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

.image-preview-enter-active,
.image-preview-leave-active {
  transition: opacity 0.2s ease;
}

.image-preview-enter-from,
.image-preview-leave-to {
  opacity: 0;
}

.image-preview-enter-active img,
.image-preview-leave-active img {
  transition: transform 0.2s ease;
}

/* 消息高亮动画 */
.message-highlight {
  position: relative;
  animation: highlight-fade 2s ease;
}

.message-highlight::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  background: rgb(59 130 246 / 0.2);
  animation: highlight-expand 0.6s ease;
  pointer-events: none;
}

@keyframes highlight-expand {
  0% {
    transform: scaleX(0);
    opacity: 1;
  }
  100% {
    transform: scaleX(1);
    opacity: 1;
  }
}

@keyframes highlight-fade {
  0%, 30% {
    background-color: rgb(59 130 246 / 0.15);
  }
  100% {
    background-color: transparent;
  }
}

.image-preview-enter-from img,
.image-preview-leave-to img {
  transform: scale(0.9);
}
</style>
