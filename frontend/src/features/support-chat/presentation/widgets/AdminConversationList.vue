<template>
  <div class="flex h-full flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
    <div class="border-b border-gray-200 p-4 dark:border-dark-700">
      <div class="flex items-center justify-between gap-3">
        <div>
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('supportChat.inbox') }}</h2>
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('supportChat.inboxHint') }}</p>
        </div>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="$emit('refresh')">
          {{ t('common.refresh') }}
        </button>
      </div>
      <div class="mt-3 space-y-2">
        <input
          :value="search"
          class="w-full rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm text-gray-900 outline-none transition-colors placeholder:text-gray-400 focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20 dark:border-dark-700 dark:bg-dark-800 dark:text-white dark:placeholder:text-dark-400"
          :placeholder="t('supportChat.searchUser')"
          @input="$emit('update:search', ($event.target as HTMLInputElement).value)"
        />
        <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-dark-300">
          <input
            type="checkbox"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            :checked="unreadOnly"
            @change="$emit('update:unreadOnly', ($event.target as HTMLInputElement).checked)"
          />
          <span>{{ t('supportChat.unreadOnly') }}</span>
        </label>
      </div>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto">
      <div v-if="loading && conversations.length === 0" class="p-6 text-center text-sm text-gray-500 dark:text-dark-400">
        {{ t('common.loading') }}
      </div>
      <div v-else-if="conversations.length === 0" class="p-6 text-center text-sm text-gray-500 dark:text-dark-400">
        {{ t('supportChat.noConversations') }}
      </div>
      <div
        v-for="conversation in conversations"
        :key="conversation.id"
        role="button"
        tabindex="0"
        class="block w-full border-b border-gray-100 px-4 py-3 text-left transition-colors hover:bg-gray-50 dark:border-dark-800 dark:hover:bg-dark-800/70"
        :class="conversation.id === selectedId ? 'bg-primary-50 dark:bg-primary-900/20' : ''"
        @click="$emit('select', conversation.id)"
        @keydown.enter.prevent="$emit('select', conversation.id)"
        @keydown.space.prevent="$emit('select', conversation.id)"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <button
              type="button"
              class="-mx-1 -my-0.5 max-w-full truncate rounded-md px-1 py-0.5 text-sm font-medium text-gray-900 transition-colors hover:bg-primary-50 hover:text-primary-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:text-white dark:hover:bg-primary-900/20 dark:hover:text-primary-200"
              :title="t('supportChat.userProfile.viewProfile')"
              @click.stop="$emit('viewUser', conversation)"
            >
              {{ displayUser(conversation) }}
            </button>
            <div class="truncate text-xs text-gray-500 dark:text-dark-400">
              #{{ conversation.user_id }} · {{ conversation.user_email || t('supportChat.noEmail') }}
            </div>
          </div>
          <span
            v-if="conversation.unread_by_admin > 0 || conversation.manually_unread_by_admin"
            class="rounded-full bg-red-100 px-2 py-0.5 text-xs font-semibold text-red-700 dark:bg-red-900/40 dark:text-red-200"
          >
            {{ conversation.unread_by_admin > 0 ? conversation.unread_by_admin : t('supportChat.unread') }}
          </span>
        </div>
        <div class="mt-2 text-xs text-gray-500 dark:text-dark-400">
          {{ lastActiveLabel(conversation.last_message_at || conversation.updated_at) }}
        </div>
      </div>
    </div>

    <div class="border-t border-gray-200 p-3 text-xs text-gray-500 dark:border-dark-700 dark:text-dark-400">
      {{ t('supportChat.totalConversations', { total }) }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { ChatConversation } from '@/features/support-chat/data/datasources/supportChatDatasource'

defineProps<{
  conversations: ChatConversation[]
  selectedId: number | null
  loading: boolean
  total: number
  search: string
  unreadOnly: boolean
}>()

defineEmits<{
  select: [conversationID: number]
  viewUser: [conversation: ChatConversation]
  refresh: []
  'update:search': [value: string]
  'update:unreadOnly': [value: boolean]
}>()

const { t, locale } = useI18n()

function displayUser(conversation: ChatConversation): string {
  return conversation.user_username || conversation.user_email || t('supportChat.unknownUser')
}

function lastActiveLabel(value: string | null): string {
  if (!value) return t('supportChat.noMessagesYet')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(locale.value, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}
</script>
