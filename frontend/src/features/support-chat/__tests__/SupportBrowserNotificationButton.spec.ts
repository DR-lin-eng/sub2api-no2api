import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useSupportChatNotificationStore } from '@/features/support-chat/presentation/stores/supportChatNotificationStore'
import SupportBrowserNotificationButton from '@/features/support-chat/presentation/widgets/SupportBrowserNotificationButton.vue'

class MockNotification {
  static permission: NotificationPermission = 'default'
  static requestPermission = vi.fn(async (): Promise<NotificationPermission> => {
    MockNotification.permission = 'granted'
    return MockNotification.permission
  })
}

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  messages: {
    en: {
      supportChat: {
        browserNotifications: {
          enable: () => 'Enable browser notifications',
          disable: () => 'Disable browser notifications',
          enabled: () => 'Browser notifications enabled',
          requesting: () => 'Requesting notification permission',
          permissionDenied: () => 'Notifications are blocked',
          unsupported: () => 'Notifications are unsupported',
          requestFailed: () => 'Notification permission request failed',
        },
      },
    },
  },
})

describe('SupportBrowserNotificationButton', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    window.localStorage.clear()
    MockNotification.permission = 'default'
    MockNotification.requestPermission.mockClear()
    Object.defineProperty(window, 'Notification', {
      configurable: true,
      value: MockNotification,
    })
    Object.defineProperty(window, 'isSecureContext', {
      configurable: true,
      value: true,
    })
  })

  it('requests permission from a click and persists the local opt-in', async () => {
    const wrapper = mount(SupportBrowserNotificationButton, {
      global: { plugins: [i18n] },
    })

    const button = wrapper.get('button')
    expect(button.attributes('aria-pressed')).toBe('false')
    await button.trigger('click')
    await flushPromises()

    expect(MockNotification.requestPermission).toHaveBeenCalledOnce()
    expect(button.attributes('aria-pressed')).toBe('true')
    expect(window.localStorage.getItem('support_chat_browser_notifications_enabled')).toBe('true')

    await button.trigger('click')
    expect(useSupportChatNotificationStore().enabled).toBe(false)
    expect(window.localStorage.getItem('support_chat_browser_notifications_enabled')).toBeNull()
  })

  it('disables the control when the browser has denied permission', () => {
    MockNotification.permission = 'denied'
    const wrapper = mount(SupportBrowserNotificationButton, {
      global: { plugins: [i18n] },
    })

    const button = wrapper.get('button')
    expect(button.attributes('disabled')).toBeDefined()
    expect(button.attributes('title')).toBe('Notifications are blocked')
  })

  it('disables the control outside a secure browser context', () => {
    Object.defineProperty(window, 'isSecureContext', {
      configurable: true,
      value: false,
    })
    const wrapper = mount(SupportBrowserNotificationButton, {
      global: { plugins: [i18n] },
    })

    const button = wrapper.get('button')
    expect(button.attributes('disabled')).toBeDefined()
    expect(button.attributes('title')).toBe('Notifications are unsupported')
  })
})
