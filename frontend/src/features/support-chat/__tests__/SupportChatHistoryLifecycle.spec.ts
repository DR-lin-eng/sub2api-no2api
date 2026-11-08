import { flushPromises, shallowMount, type VueWrapper } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatConversation, ChatMessage } from '@/features/support-chat/data/datasources/supportChatDatasource'
import type { SupportChatSocketOptions } from '@/features/support-chat/presentation/composables/useSupportChatSocket'
import SupportChatPage from '@/features/support-chat/presentation/pages/SupportChatPage.vue'
import AdminSupportChatPage from '@/features/support-chat/presentation/pages/AdminSupportChatPage.vue'

const mocks = vi.hoisted(() => ({
  listUser: vi.fn(),
  listAdmin: vi.fn(),
  listConversations: vi.fn(),
  getConversation: vi.fn(),
  markRead: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  sockets: [] as SupportChatSocketOptions[],
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<object>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('vue-router', () => ({
  useRoute: () => ({ query: { conversation: '7' } }),
  useRouter: () => ({ replace: vi.fn() }),
}))
vi.mock('@/features/auth', () => ({ useAuthStore: () => ({ hasPermission: () => true }) }))
vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({
    showError: mocks.showError,
    showSuccess: mocks.showSuccess,
    setSupportInboxUnread: vi.fn(),
    setSupportUserUnread: vi.fn(),
  }),
}))
vi.mock('@/features/support-chat/presentation/stores/supportChatAdminStore', () => ({
  useSupportChatAdminStore: () => ({
    refreshUnreadIndicator: vi.fn(),
    markHasUnread: vi.fn(),
    markUserHasUnread: vi.fn(),
    markUserRead: vi.fn(),
    setUnreadConversationCount: vi.fn(),
  }),
}))
vi.mock('@/features/support-chat/presentation/composables/useSupportChatSocket', () => ({
  useSupportChatSocket: (options: SupportChatSocketOptions) => {
    mocks.sockets.push(options)
    return { connect: vi.fn() }
  },
}))
vi.mock('@/features/support-chat/data/datasources/supportChatDatasource', async (importOriginal) => ({
  ...await importOriginal<object>(),
  getUserChatConversation: mocks.getConversation,
  listUserChatMessages: mocks.listUser,
  listAdminChatMessages: mocks.listAdmin,
  listAdminChatConversations: mocks.listConversations,
  markUserChatRead: mocks.markRead,
  markAdminChatRead: mocks.markRead,
  listAdminChatQuickReplies: vi.fn(async () => []),
  listAdminChatCatalog: vi.fn(async () => []),
}))

const conversation: ChatConversation = {
  id: 7, user_id: 42, last_message_at: null, unread_by_user: 0, unread_by_admin: 0,
  manually_unread_by_admin: false, last_read_by_user_at: null, last_read_by_admin_at: null,
  created_at: '2026-11-06T00:00:00Z', updated_at: '2026-11-06T00:00:00Z',
}

function message(id: number, sender: 'admin' | 'user' = 'user'): ChatMessage {
  return {
    id, conversation_id: 7, sender_type: sender, sender_id: sender === 'user' ? 42 : 1,
    content: `message ${id}`, kind: 'text', reply_to_id: null, metadata: {}, assets: [], recalled_at: null,
    created_at: `2026-11-06T00:00:${String(id).padStart(2, '0')}Z`,
  }
}

function page(items: ChatMessage[]) {
  return { items, total: items.length, page: 1, page_size: 100, pages: 1 }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

const MessageList = defineComponent({
  name: 'SupportMessageList',
  props: ['messages'],
  template: '<div />',
})
const wrappers: VueWrapper[] = []

async function mountAdminInbox() {
  const wrapper = shallowMount(AdminSupportChatPage, {
    global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, SupportMessageList: MessageList } },
  })
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}

describe('admin inbox activity timestamps', () => {
  const oldActivity = '2026-11-06T01:00:00Z'
  const latestActivity = '2026-11-09T10:00:00Z'
  const otherActivity = '2026-11-08T10:00:00Z'

  function inbox() {
    return [
      { ...conversation, id: 8, last_message_at: otherActivity },
      { ...conversation, last_message_at: oldActivity },
    ]
  }

  function listProps(wrapper: VueWrapper): ChatConversation[] {
    return wrapper.findComponent({ name: 'AdminConversationList' }).props('conversations')
  }

  it('keeps a read thread at the top when an old message is replayed after its latest message', async () => {
    mocks.listConversations.mockResolvedValue({ ...page([]), items: inbox() })
    const wrapper = await mountAdminInbox()
    await mocks.sockets[0].onMessage({ ...message(20), created_at: latestActivity })
    await mocks.sockets[0].onMessage({ ...message(1), created_at: oldActivity })
    await flushPromises()

    expect(listProps(wrapper).map(item => item.id)).toEqual([7, 8])
    expect(listProps(wrapper)[0]).toMatchObject({ unread_by_admin: 0, last_message_at: latestActivity })
  })

  it('keeps the latest activity when an inbox refresh returns a snapshot taken before a new message', async () => {
    mocks.listConversations.mockResolvedValue({ ...page([]), items: inbox() })
    const wrapper = await mountAdminInbox()
    const pending = deferred<{ items: ChatConversation[]; total: number; page: number; page_size: number; pages: number }>()
    mocks.listConversations.mockReturnValueOnce(pending.promise)
    wrapper.findComponent({ name: 'AdminConversationList' }).vm.$emit('refresh')
    await flushPromises()
    await mocks.sockets[0].onMessage({ ...message(20), created_at: latestActivity })
    pending.resolve({ items: inbox(), total: 2, page: 1, page_size: 100, pages: 1 })
    await flushPromises()

    expect(listProps(wrapper).map(item => item.id)).toEqual([7, 8])
    expect(listProps(wrapper)[0]).toMatchObject({ unread_by_admin: 0, last_message_at: latestActivity })
  })
})

beforeEach(() => {
  vi.clearAllMocks()
  mocks.sockets.length = 0
  mocks.getConversation.mockResolvedValue({ ...conversation })
  mocks.listConversations.mockResolvedValue({ ...page([]), items: [{ ...conversation }] })
  mocks.listUser.mockResolvedValue(page([message(1)]))
  mocks.listAdmin.mockResolvedValue(page([message(1)]))
  mocks.markRead.mockResolvedValue(undefined)
})

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
})

for (const scope of ['user', 'admin'] as const) {
  describe(`${scope} chat history lifecycle`, () => {
    async function mountPage() {
      const wrapper = shallowMount(scope === 'user' ? SupportChatPage : AdminSupportChatPage, {
        global: {
          stubs: {
            AppLayout: { template: '<div><slot /></div>' },
            SupportMessageList: MessageList,
          },
        },
      })
      wrappers.push(wrapper)
      await flushPromises()
      return wrapper
    }

    it('preserves a realtime message received while a history refresh is pending', async () => {
      const wrapper = await mountPage()
      const list = scope === 'user' ? mocks.listUser : mocks.listAdmin
      const pending = deferred<ReturnType<typeof page>>()
      list.mockReturnValueOnce(pending.promise)
      const refresh = wrapper.findAll('button').find(button => button.text() === 'common.refresh' || button.text().includes('common.refresh'))
      expect(refresh).toBeDefined()
      await refresh!.trigger('click')

      await mocks.sockets[0].onMessage(message(2, scope === 'user' ? 'admin' : 'user'))
      await flushPromises()
      pending.resolve(page([message(1)]))
      await flushPromises()

      expect(wrapper.findComponent(MessageList).props('messages').map((item: ChatMessage) => item.id).sort()).toEqual([1, 2])
    })

    it('fetches history immediately when a socket reconnects', async () => {
      const wrapper = await mountPage()
      const list = scope === 'user' ? mocks.listUser : mocks.listAdmin
      const previousCalls = list.mock.calls.length
      list.mockResolvedValue(page([message(2), message(1)]))

      mocks.sockets[0].onStatusChange?.(true)
      await flushPromises()

      expect(list.mock.calls.length).toBeGreaterThan(previousCalls)
      expect(wrapper.findComponent(MessageList).props('messages')).toHaveLength(2)
    })
  })
}
