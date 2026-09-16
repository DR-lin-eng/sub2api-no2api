<template>
  <button
    type="button"
    class="relative inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 disabled:cursor-not-allowed disabled:opacity-50"
    :class="notificationStore.enabled
      ? 'border-primary-200 bg-primary-50 text-primary-700 hover:bg-primary-100 dark:border-primary-800 dark:bg-primary-900/30 dark:text-primary-200 dark:hover:bg-primary-900/45'
      : 'border-gray-200 text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:text-dark-300 dark:hover:bg-dark-800'"
    :disabled="buttonDisabled"
    :title="buttonTitle"
    :aria-label="buttonTitle"
    :aria-pressed="notificationStore.enabled"
    @click="toggleNotifications"
  >
    <Icon name="bell" size="sm" />
    <span
      v-if="notificationStore.enabled"
      class="absolute right-1 top-1 h-1.5 w-1.5 rounded-full bg-emerald-500 ring-1 ring-white dark:ring-dark-900"
      aria-hidden="true"
    ></span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import { useAppStore } from '@/core/stores/appStore'
import { useSupportChatNotificationStore } from '@/features/support-chat/presentation/stores/supportChatNotificationStore'

const { t } = useI18n()
const appStore = useAppStore()
const notificationStore = useSupportChatNotificationStore()

const buttonDisabled = computed(() => notificationStore.requestingPermission
  || !notificationStore.supported
  || notificationStore.permission === 'denied')

const buttonTitle = computed(() => {
  if (!notificationStore.supported) return t('supportChat.browserNotifications.unsupported')
  if (notificationStore.permission === 'denied') return t('supportChat.browserNotifications.permissionDenied')
  if (notificationStore.requestingPermission) return t('supportChat.browserNotifications.requesting')
  return notificationStore.enabled
    ? t('supportChat.browserNotifications.disable')
    : t('supportChat.browserNotifications.enable')
})

async function toggleNotifications(): Promise<void> {
  if (notificationStore.enabled) {
    notificationStore.disable()
    return
  }

  try {
    const permission = await notificationStore.enable()
    if (permission === 'granted') {
      appStore.showSuccess(t('supportChat.browserNotifications.enabled'))
    } else if (permission === 'denied') {
      appStore.showError(t('supportChat.browserNotifications.permissionDenied'))
    }
  } catch {
    appStore.showError(t('supportChat.browserNotifications.requestFailed'))
  }
}
</script>
