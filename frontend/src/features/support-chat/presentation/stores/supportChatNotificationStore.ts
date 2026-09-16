import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { safeLocalStorage } from '@/core/utils/safeStorage'

export const SUPPORT_BROWSER_NOTIFICATION_STORAGE_KEY = 'support_chat_browser_notifications_enabled'

export type SupportBrowserNotificationPermission = NotificationPermission | 'unsupported'

export function getBrowserNotificationAPI(): typeof Notification | null {
  if (typeof window === 'undefined' || window.isSecureContext === false || !('Notification' in window)) return null
  return window.Notification
}

function readPermission(): SupportBrowserNotificationPermission {
  return getBrowserNotificationAPI()?.permission ?? 'unsupported'
}

function readPreference(): boolean {
  return safeLocalStorage.getItem(SUPPORT_BROWSER_NOTIFICATION_STORAGE_KEY) === 'true'
}

export const useSupportChatNotificationStore = defineStore('supportChatNotification', () => {
  const permission = ref<SupportBrowserNotificationPermission>(readPermission())
  const preferenceEnabled = ref(readPreference())
  const requestingPermission = ref(false)

  const supported = computed(() => permission.value !== 'unsupported')
  const enabled = computed(() => preferenceEnabled.value && permission.value === 'granted')

  function syncBrowserState(): void {
    permission.value = readPermission()
    preferenceEnabled.value = readPreference()
  }

  async function enable(): Promise<SupportBrowserNotificationPermission> {
    const notificationAPI = getBrowserNotificationAPI()
    if (!notificationAPI) {
      permission.value = 'unsupported'
      preferenceEnabled.value = false
      return permission.value
    }

    requestingPermission.value = true
    try {
      permission.value = notificationAPI.permission === 'default'
        ? await notificationAPI.requestPermission()
        : notificationAPI.permission
      preferenceEnabled.value = permission.value === 'granted'
      if (preferenceEnabled.value) {
        safeLocalStorage.setItem(SUPPORT_BROWSER_NOTIFICATION_STORAGE_KEY, 'true')
      } else {
        safeLocalStorage.removeItem(SUPPORT_BROWSER_NOTIFICATION_STORAGE_KEY)
      }
      return permission.value
    } finally {
      requestingPermission.value = false
    }
  }

  function disable(): void {
    preferenceEnabled.value = false
    safeLocalStorage.removeItem(SUPPORT_BROWSER_NOTIFICATION_STORAGE_KEY)
  }

  return {
    permission,
    preferenceEnabled,
    requestingPermission,
    supported,
    enabled,
    syncBrowserState,
    enable,
    disable,
  }
})
