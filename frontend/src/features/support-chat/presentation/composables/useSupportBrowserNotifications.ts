import { onBeforeUnmount, onMounted, watch } from 'vue'
import { useAppStore } from '@/core/stores/appStore'
import type { ChatMessage } from '@/features/support-chat/data/datasources/supportChatDatasource'
import { useSupportChatSocket } from '@/features/support-chat/presentation/composables/useSupportChatSocket'
import { useSupportChatAdminStore } from '@/features/support-chat/presentation/stores/supportChatAdminStore'
import {
  getBrowserNotificationAPI,
  SUPPORT_BROWSER_NOTIFICATION_STORAGE_KEY,
  useSupportChatNotificationStore,
} from '@/features/support-chat/presentation/stores/supportChatNotificationStore'

interface SupportBrowserNotificationOptions {
  isAuthenticated: () => boolean
  canReadSupport: () => boolean
  isSupportInboxActive: () => boolean
  openConversation: (conversationID: number) => void | Promise<void>
}

export function useSupportBrowserNotifications(options: SupportBrowserNotificationOptions): void {
  const appStore = useAppStore()
  const unreadStore = useSupportChatAdminStore()
  const notificationStore = useSupportChatNotificationStore()

  function runtimeEnabled(): boolean {
    return options.isAuthenticated()
      && options.canReadSupport()
      && appStore.publicSettingsLoaded
      && appStore.cachedPublicSettings?.support_chat_enabled === true
      && notificationStore.enabled
  }

  function shouldShowSystemNotification(): boolean {
    if (document.visibilityState !== 'visible') return true
    return typeof document.hasFocus === 'function' && !document.hasFocus()
  }

  async function showIncomingMessage(message: ChatMessage): Promise<void> {
    if (message.sender_type !== 'user') return

    if (!options.isSupportInboxActive()) {
      appStore.setSupportInboxUnread(true)
      unreadStore.markHasUnread()
    }
    if (!runtimeEnabled() || !shouldShowSystemNotification()) return

    const notificationAPI = getBrowserNotificationAPI()
    if (!notificationAPI) {
      notificationStore.syncBrowserState()
      return
    }

    try {
      const { buildSupportNotificationContent } = await import('../utils/supportBrowserNotificationContent')
      const content = await buildSupportNotificationContent(message)
      const notification = new notificationAPI(
        content.title,
        {
          body: content.body,
          tag: `support-chat-message-${message.id}`,
        },
      )
      notification.onclick = () => {
        notification.close()
        window.focus()
        void options.openConversation(message.conversation_id)
      }
    } catch (error) {
      console.error('[supportChatNotifications] Failed to show browser notification:', error)
      notificationStore.syncBrowserState()
    }
  }

  const socket = useSupportChatSocket({
    scope: 'admin',
    onMessage: showIncomingMessage,
  })

  function syncRuntime(): void {
    notificationStore.syncBrowserState()
    if (runtimeEnabled()) socket.connect()
    else socket.disconnect()
  }

  const stopWatch = watch(
    [
      options.isAuthenticated,
      options.canReadSupport,
      () => appStore.publicSettingsLoaded,
      () => appStore.cachedPublicSettings?.support_chat_enabled,
      () => notificationStore.enabled,
    ],
    syncRuntime,
    { immediate: true },
  )

  function onVisibilityChange(): void {
    if (document.visibilityState === 'visible') syncRuntime()
  }

  function onStorage(event: StorageEvent): void {
    if (event.key === null || event.key === SUPPORT_BROWSER_NOTIFICATION_STORAGE_KEY) syncRuntime()
  }

  onMounted(() => {
    document.addEventListener('visibilitychange', onVisibilityChange)
    window.addEventListener('focus', syncRuntime)
    window.addEventListener('storage', onStorage)
  })

  onBeforeUnmount(() => {
    stopWatch()
    socket.disconnect()
    document.removeEventListener('visibilitychange', onVisibilityChange)
    window.removeEventListener('focus', syncRuntime)
    window.removeEventListener('storage', onStorage)
  })
}
