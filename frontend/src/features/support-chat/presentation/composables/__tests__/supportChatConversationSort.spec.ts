import { describe, expect, it } from 'vitest'
import type { ChatConversation } from '@/features/support-chat/data/datasources/supportChatDatasource'
import { sortSupportChatConversations } from '@/features/support-chat/presentation/composables/supportChatConversationSort'

function conversation(id: number, overrides: Partial<ChatConversation> = {}): ChatConversation {
  return {
    id,
    user_id: id,
    last_message_at: `2026-11-07T00:00:${String(id).padStart(2, '0')}Z`,
    unread_by_user: 0,
    unread_by_admin: 0,
    manually_unread_by_admin: false,
    last_read_by_user_at: null,
    last_read_by_admin_at: null,
    created_at: '2026-11-07T00:00:00Z',
    updated_at: '2026-11-07T00:00:00Z',
    ...overrides,
  }
}

describe('support chat conversation ordering', () => {
  it('puts real unread conversations before newer read conversations', () => {
    const result = sortSupportChatConversations([
      conversation(3, { unread_by_admin: 0, last_message_at: '2026-11-07T03:00:00Z' }),
      conversation(1, { unread_by_admin: 2, last_message_at: '2026-11-07T01:00:00Z' }),
      conversation(2, { unread_by_admin: 1, last_message_at: '2026-11-07T02:00:00Z' }),
    ])

    expect(result.map(item => item.id)).toEqual([2, 1, 3])
  })

  it('treats manual unread reminders as unread and preserves activity ordering within a state', () => {
    const result = sortSupportChatConversations([
      conversation(1, { manually_unread_by_admin: true, last_message_at: '2026-11-07T01:00:00Z' }),
      conversation(3, { last_message_at: '2026-11-07T04:00:00Z' }),
      conversation(2, { unread_by_admin: 1, last_message_at: '2026-11-07T02:00:00Z' }),
    ])

    expect(result.map(item => item.id)).toEqual([2, 1, 3])
  })
})
