import { defineComponent, nextTick, reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAppStore } from '@/core/stores/appStore'
import type {
  ChatMessage,
} from '@/features/support-chat/data/datasources/supportChatDatasource'
import { useSupportBrowserNotifications } from '@/features/support-chat/presentation/composables/useSupportBrowserNotifications'
import { useSupportChatAdminStore } from '@/features/support-chat/presentation/stores/supportChatAdminStore'
import { useSupportChatNotificationStore } from '@/features/support-chat/presentation/stores/supportChatNotificationStore'
import { buildSupportNotificationBody } from '@/features/support-chat/presentation/utils/supportBrowserNotificationContent'

const socketHarness = vi.hoisted(() => ({
  options: null as { onMessage: (message: ChatMessage) => void | Promise<void> } | null,
  connect: vi.fn(),
  disconnect: vi.fn(),
}))

vi.mock('@/core/i18n', () => ({
  default: {
    global: {
      t: (key: string) => ({
        'supportChat.browserNotifications.newMessageTitle': 'New support message',
        'supportChat.browserNotifications.newMessageBody': 'A user sent a new message',
        'supportChat.browserNotifications.imageMessage': 'A user sent an image',
        'supportChat.browserNotifications.stickerMessage': 'A user sent a sticker',
      })[key] ?? key,
    },
  },
  getLocale: () => 'en',
  loadLocaleMessages: vi.fn(async () => undefined),
}))

vi.mock('@/features/support-chat/presentation/composables/useSupportChatSocket', () => ({
  useSupportChatSocket: (options: { onMessage: (message: ChatMessage) => void | Promise<void> }) => {
    socketHarness.options = options
    return {
      connect: socketHarness.connect,
      disconnect: socketHarness.disconnect,
    }
  },
}))

class MockNotification {
  static permission: NotificationPermission = 'granted'
  static requestPermission = vi.fn(async (): Promise<NotificationPermission> => MockNotification.permission)
  static instances: MockNotification[] = []

  onclick: ((event: Event) => void) | null = null
  close = vi.fn()

  constructor(
    readonly title: string,
    readonly options: NotificationOptions = {},
  ) {
    MockNotification.instances.push(this)
  }
}

const authState = reactive({
  isAuthenticated: true,
  canReadSupport: true,
  isSupportInboxActive: false,
})
const openConversation = vi.fn()

const Harness = defineComponent({
  setup() {
    useSupportBrowserNotifications({
      isAuthenticated: () => authState.isAuthenticated,
      canReadSupport: () => authState.canReadSupport,
      isSupportInboxActive: () => authState.isSupportInboxActive,
      openConversation,
    })
    return () => null
  },
})

function message(overrides: Partial<ChatMessage> = {}): ChatMessage {
  return {
    id: 10,
    conversation_id: 7,
    sender_type: 'user',
    sender_id: 2,
    content: 'Need help with my request',
    kind: 'text',
    reply_to_id: null,
    metadata: {},
    assets: [],
    recalled_at: null,
    created_at: '2026-09-15T01:02:03Z',
    ...overrides,
  }
}

describe('useSupportBrowserNotifications', () => {
  beforeEach(async () => {
    setActivePinia(createPinia())
    window.localStorage.clear()
    MockNotification.permission = 'granted'
    MockNotification.requestPermission.mockClear()
    MockNotification.instances = []
    socketHarness.options = null
    socketHarness.connect.mockReset()
    socketHarness.disconnect.mockReset()
    openConversation.mockReset()
    authState.isAuthenticated = true
    authState.canReadSupport = true
    authState.isSupportInboxActive = false
    Object.defineProperty(window, 'Notification', {
      configurable: true,
      value: MockNotification,
    })
    Object.defineProperty(window, 'isSecureContext', {
      configurable: true,
      value: true,
    })
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'hidden',
    })
    Object.defineProperty(document, 'hasFocus', {
      configurable: true,
      value: () => false,
    })
    Object.defineProperty(window, 'focus', {
      configurable: true,
      value: vi.fn(),
    })

    const appStore = useAppStore()
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = {
      support_chat_enabled: true,
    } as typeof appStore.cachedPublicSettings
    await useSupportChatNotificationStore().enable()
  })

  it('connects only while enabled and notifies for incoming user messages', async () => {
    const wrapper = mount(Harness)
    await flushPromises()

    expect(socketHarness.connect).toHaveBeenCalled()
    await socketHarness.options?.onMessage(message())

    expect(MockNotification.instances).toHaveLength(1)
    expect(MockNotification.instances[0]).toMatchObject({
      title: 'New support message',
      options: {
        body: 'A user sent a new message',
        tag: 'support-chat-message-10',
      },
    })
    expect(useAppStore().supportInboxHasUnread).toBe(true)
    expect(useSupportChatAdminStore().hasUnread).toBe(true)

    MockNotification.instances[0].onclick?.(new Event('click'))
    expect(MockNotification.instances[0].close).toHaveBeenCalled()
    expect(window.focus).toHaveBeenCalled()
    expect(openConversation).toHaveBeenCalledWith(7)

    await socketHarness.options?.onMessage(message({ id: 11, sender_type: 'admin' }))
    expect(MockNotification.instances).toHaveLength(1)

    useSupportChatNotificationStore().disable()
    await nextTick()
    expect(socketHarness.disconnect).toHaveBeenCalled()

    wrapper.unmount()
  })

  it('keeps foreground messages in the unread state without a system popup', async () => {
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'visible',
    })
    Object.defineProperty(document, 'hasFocus', {
      configurable: true,
      value: () => true,
    })

    const wrapper = mount(Harness)
    await flushPromises()
    await socketHarness.options?.onMessage(message())

    expect(MockNotification.instances).toHaveLength(0)
    expect(useAppStore().supportInboxHasUnread).toBe(true)
    wrapper.unmount()
  })

  it('leaves unread ownership to the inbox page while still notifying in the background', async () => {
    authState.isSupportInboxActive = true
    const wrapper = mount(Harness)
    await flushPromises()
    await socketHarness.options?.onMessage(message())

    expect(MockNotification.instances).toHaveLength(1)
    expect(useAppStore().supportInboxHasUnread).toBe(false)
    expect(useSupportChatAdminStore().hasUnread).toBe(false)
    wrapper.unmount()
  })

  it('uses generic notification bodies without exposing support message text', () => {
    const sensitiveText = 'sk-example-secret from an order request'
    expect(buildSupportNotificationBody(message({ content: sensitiveText }))).toBe('A user sent a new message')
    expect(buildSupportNotificationBody(message({ content: sensitiveText }))).not.toContain(sensitiveText)
    expect(buildSupportNotificationBody(message({ content: '', kind: 'image' }))).toBe('A user sent an image')
    expect(buildSupportNotificationBody(message({ content: '', kind: 'sticker' }))).toBe('A user sent a sticker')
  })
})
