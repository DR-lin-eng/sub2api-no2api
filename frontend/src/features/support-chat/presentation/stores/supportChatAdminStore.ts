import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  getAdminChatUnreadCount,
  getUserChatUnreadCount,
  type ChatConversation,
} from '@/features/support-chat/data/datasources/supportChatDatasource'

const UNREAD_REFRESH_THROTTLE_MS = 15000

export const useSupportChatAdminStore = defineStore('supportChatAdmin', () => {
  const unreadConversationCount = ref(0)
  const userUnreadCount = ref(0)
  const loadingUnread = ref(false)
  const loadingUserUnread = ref(false)
  const lastUnreadFetchAt = ref(0)
  const lastUserUnreadFetchAt = ref(0)

  const hasUnread = computed(() => unreadConversationCount.value > 0)
  const userHasUnread = computed(() => userUnreadCount.value > 0)

  async function refreshUnreadIndicator(force = false): Promise<void> {
    const now = Date.now()
    if (!force && lastUnreadFetchAt.value > 0 && now - lastUnreadFetchAt.value < UNREAD_REFRESH_THROTTLE_MS) return
    if (loadingUnread.value) return

    loadingUnread.value = true
    lastUnreadFetchAt.value = now
    try {
      unreadConversationCount.value = await getAdminChatUnreadCount()
    } catch (error) {
      lastUnreadFetchAt.value = 0
      console.error('[supportChatAdmin] Failed to refresh unread indicator:', error)
    } finally {
      loadingUnread.value = false
    }
  }

  async function refreshUserUnreadIndicator(force = false): Promise<void> {
    const now = Date.now()
    if (!force && lastUserUnreadFetchAt.value > 0 && now - lastUserUnreadFetchAt.value < UNREAD_REFRESH_THROTTLE_MS) return
    if (loadingUserUnread.value) return

    loadingUserUnread.value = true
    lastUserUnreadFetchAt.value = now
    try {
      userUnreadCount.value = await getUserChatUnreadCount()
    } catch (error) {
      lastUserUnreadFetchAt.value = 0
      console.error('[supportChatAdmin] Failed to refresh user unread indicator:', error)
    } finally {
      loadingUserUnread.value = false
    }
  }

  function syncFromConversations(conversations: ChatConversation[]): void {
    unreadConversationCount.value = conversations.filter(
      (conversation) => conversation.unread_by_admin > 0 || conversation.manually_unread_by_admin,
    ).length
    lastUnreadFetchAt.value = Date.now()
  }

  function setUnreadConversationCount(count: number): void {
    unreadConversationCount.value = Math.max(0, count)
    lastUnreadFetchAt.value = Date.now()
  }

  function markHasUnread(): void {
    if (unreadConversationCount.value === 0) unreadConversationCount.value = 1
  }

  function markUserHasUnread(): void {
    if (userUnreadCount.value === 0) userUnreadCount.value = 1
  }

  function markUserRead(): void {
    userUnreadCount.value = 0
    lastUserUnreadFetchAt.value = Date.now()
  }

  function reset(): void {
    unreadConversationCount.value = 0
    userUnreadCount.value = 0
    loadingUnread.value = false
    loadingUserUnread.value = false
    lastUnreadFetchAt.value = 0
    lastUserUnreadFetchAt.value = 0
  }

  return {
    unreadConversationCount,
    userUnreadCount,
    loadingUnread,
    loadingUserUnread,
    hasUnread,
    userHasUnread,
    refreshUnreadIndicator,
    refreshUserUnreadIndicator,
    syncFromConversations,
    setUnreadConversationCount,
    markHasUnread,
    markUserHasUnread,
    markUserRead,
    reset,
  }
})
