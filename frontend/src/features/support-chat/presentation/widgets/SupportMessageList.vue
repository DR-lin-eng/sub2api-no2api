<template>
  <div class="space-y-4">
    <div
      v-for="message in orderedMessages"
      :key="message.id"
      class="flex"
      :class="message.sender_type === ownSender ? 'justify-end' : 'justify-start'"
    >
      <div class="group max-w-[88%] sm:max-w-[72%]">
        <div
          class="mb-1 flex items-center gap-2 text-xs text-gray-500 dark:text-dark-400"
          :class="message.sender_type === ownSender ? 'justify-end' : 'justify-start'"
        >
          <span>{{ senderLabel(message.sender_type) }}</span>
          <span>·</span>
          <time :datetime="message.created_at">{{ formatTime(message.created_at) }}</time>
        </div>

        <div
          class="overflow-hidden rounded-2xl text-sm leading-6 shadow-sm"
          :class="message.sender_type === ownSender
            ? 'bg-primary-600 text-white dark:bg-primary-500'
            : 'border border-gray-200 bg-white text-gray-900 dark:border-dark-700 dark:bg-dark-800 dark:text-white'"
        >
          <div
            v-if="!message.recalled_at && message.reply_to_id"
            class="mx-3 mt-3 rounded-xl border-l-2 px-3 py-2 text-xs opacity-80"
            :class="message.sender_type === ownSender ? 'border-white/70 bg-black/10' : 'border-primary-400 bg-gray-50 dark:bg-dark-900'"
          >
            <span class="font-medium">{{ t('supportChat.reply.quote') }} #{{ message.reply_to_id }}</span>
            <p class="mt-0.5 line-clamp-2 whitespace-pre-wrap break-words">{{ replyPreview(message.reply_to_id) }}</p>
          </div>

          <p v-if="message.recalled_at" class="px-4 py-3 italic opacity-75">
            {{ t('supportChat.recall.placeholder') }}
          </p>

          <div v-else-if="message.kind === 'balance_transfer'" class="min-w-64 p-4">
            <p class="text-xs font-medium opacity-75">{{ t('supportChat.transfer.receipt') }}</p>
            <p class="mt-1 text-2xl font-semibold">+{{ formatAmount(transferMetadata(message).amount) }}</p>
            <p class="mt-1 text-xs opacity-80">
              {{ t('supportChat.transfer.balanceAfter', { amount: formatAmount(transferMetadata(message).balance_after) }) }}
            </p>
            <p v-if="transferMetadata(message).notes" class="mt-2 whitespace-pre-wrap break-words text-sm opacity-90">
              {{ transferMetadata(message).notes }}
            </p>
          </div>

          <div v-else-if="message.kind === 'image'" class="p-2">
            <div class="grid max-w-xl gap-2" :class="message.assets.length > 1 ? 'grid-cols-2' : 'grid-cols-1'">
              <SupportAssetImage
                v-for="asset in message.assets"
                :key="asset.id"
                :asset-id="asset.id"
                :scope="assetScope"
                :alt="message.content"
                container-class="min-h-40 rounded-xl"
              />
            </div>
            <p v-if="message.content && message.content !== '[image]'" class="whitespace-pre-wrap break-words px-2 pb-1 pt-2">
              {{ message.content }}
            </p>
          </div>

          <div v-else-if="message.kind === 'sticker'" class="p-3">
            <SupportAssetImage
              v-if="message.assets[0]"
              :asset-id="message.assets[0].id"
              :scope="assetScope"
              :alt="message.content"
              container-class="h-36 w-36 rounded-xl bg-transparent"
            />
            <span v-else class="block px-3 py-2 text-5xl leading-none" role="img" :aria-label="stickerName(message)">
              {{ stickerEmoji(message) }}
            </span>
          </div>

          <p v-else class="whitespace-pre-wrap break-words px-4 py-3">{{ message.content }}</p>
        </div>

        <div
          class="mt-1 flex items-center gap-3 text-[11px] text-gray-400 dark:text-dark-500"
          :class="message.sender_type === ownSender ? 'justify-end' : 'justify-start'"
        >
          <span v-if="message.sender_type === ownSender && !message.recalled_at">
            {{ isPeerRead(message) ? t('supportChat.read.read') : t('supportChat.read.sent') }}
          </span>
          <button v-if="!message.recalled_at" type="button" class="opacity-70 hover:opacity-100 hover:underline" @click="emit('reply', message)">
            {{ t('supportChat.reply.action') }}
          </button>
          <button
            v-if="isRecallable(message)"
            type="button"
            class="opacity-70 hover:text-red-500 hover:opacity-100 hover:underline disabled:cursor-not-allowed disabled:opacity-40"
            :disabled="recallingMessageId === message.id"
            @click="emit('recall', message)"
          >
            {{ recallingMessageId === message.id ? t('common.submitting') : t('supportChat.recall.action') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  type ChatBalanceTransferMetadata,
  type ChatMessage,
  type ChatSenderType,
} from '@/features/support-chat/data/datasources/supportChatDatasource'
import SupportAssetImage from '@/features/support-chat/presentation/widgets/SupportAssetImage.vue'

const props = withDefaults(defineProps<{
  messages: ChatMessage[]
  ownSender: ChatSenderType
  assetScope: 'user' | 'admin'
  peerReadAt?: string | null
  allowRecall?: boolean
  recallingMessageId?: number | null
}>(), {
  peerReadAt: null,
  allowRecall: false,
  recallingMessageId: null,
})

const emit = defineEmits<{
  reply: [message: ChatMessage]
  recall: [message: ChatMessage]
}>()

const { t, locale } = useI18n()

const orderedMessages = computed(() => [...props.messages].sort((a, b) => {
  const at = Date.parse(a.created_at) || 0
  const bt = Date.parse(b.created_at) || 0
  return at === bt ? a.id - b.id : at - bt
}))

const messagesByID = computed(() => new Map(props.messages.map(message => [message.id, message])))

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

function replyPreview(id: number): string {
  const message = messagesByID.value.get(id)
  if (!message) return t('supportChat.reply.notLoaded')
  if (message.recalled_at) return t('supportChat.recall.placeholder')
  if (message.kind === 'image') return t('supportChat.assets.image')
  if (message.kind === 'sticker') return stickerEmoji(message)
  if (message.kind === 'balance_transfer') return t('supportChat.transfer.receipt')
  return message.content.slice(0, 180)
}

function isRecallable(message: ChatMessage): boolean {
  return props.allowRecall && message.sender_type === 'admin' && !message.recalled_at && message.kind !== 'balance_transfer'
}

function stickerEmoji(message: ChatMessage): string {
  const emoji = message.metadata.emoji
  return typeof emoji === 'string' && emoji.trim() ? emoji.slice(0, 16) : message.content.slice(0, 16)
}

function stickerName(message: ChatMessage): string {
  const name = message.metadata.name
  return typeof name === 'string' && name.trim() ? name : t('supportChat.assets.sticker')
}

function transferMetadata(message: ChatMessage): ChatBalanceTransferMetadata {
  const amount = Number(message.metadata.amount)
  const balanceAfter = Number(message.metadata.balance_after)
  return {
    amount: Number.isFinite(amount) ? amount : 0,
    balance_after: Number.isFinite(balanceAfter) ? balanceAfter : 0,
    notes: typeof message.metadata.notes === 'string' ? message.metadata.notes : '',
  }
}

function formatAmount(value: number): string {
  return new Intl.NumberFormat(locale.value, { minimumFractionDigits: 2, maximumFractionDigits: 8 }).format(value)
}

function isPeerRead(message: ChatMessage): boolean {
  const readAt = Date.parse(props.peerReadAt || '')
  const createdAt = Date.parse(message.created_at)
  return Number.isFinite(readAt) && Number.isFinite(createdAt) && createdAt <= readAt
}
</script>
