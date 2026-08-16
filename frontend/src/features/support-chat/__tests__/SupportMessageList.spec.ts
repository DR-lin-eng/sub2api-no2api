import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { ChatMessage } from '@/features/support-chat/data/datasources/supportChatDatasource'
import SupportMessageList from '@/features/support-chat/presentation/widgets/SupportMessageList.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'en-US' },
    }),
  }
})

function message(overrides: Partial<ChatMessage> = {}): ChatMessage {
  return {
    id: 10,
    conversation_id: 7,
    sender_type: 'admin',
    sender_id: 3,
    content: 'hello',
    kind: 'text',
    reply_to_id: null,
    metadata: {},
    assets: [],
    recalled_at: null,
    created_at: '2026-01-02T03:04:05Z',
    ...overrides,
  }
}

describe('SupportMessageList recall presentation', () => {
  it('never renders a recalled payload or reply action', () => {
    const wrapper = mount(SupportMessageList, {
      props: {
        messages: [message({ content: 'must stay hidden', recalled_at: '2026-01-02T03:05:00Z' })],
        ownSender: 'user',
        assetScope: 'user',
      },
    })

    expect(wrapper.text()).toContain('supportChat.recall.placeholder')
    expect(wrapper.text()).not.toContain('must stay hidden')
    expect(wrapper.text()).not.toContain('supportChat.reply.action')
  })

  it('offers recall only for ordinary administrator messages', async () => {
    const ordinary = message()
    const receipt = message({ id: 11, kind: 'balance_transfer' })
    const wrapper = mount(SupportMessageList, {
      props: {
        messages: [ordinary, receipt],
        ownSender: 'admin',
        assetScope: 'admin',
        allowRecall: true,
      },
    })

    const recallButtons = wrapper.findAll('button').filter(button => button.text() === 'supportChat.recall.action')
    expect(recallButtons).toHaveLength(1)
    await recallButtons[0].trigger('click')
    expect(wrapper.emitted('recall')).toEqual([[ordinary]])
  })
})
