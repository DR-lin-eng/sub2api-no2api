import { describe, expect, it } from 'vitest'
import {
  normalizeChatConversation,
  normalizeChatMessage,
  parseChatSocketEvent,
} from '@/features/support-chat/data/datasources/supportChatDatasource'

describe('support chat datasource normalization', () => {
  it('normalizes backend PascalCase conversation fields', () => {
    const conversation = normalizeChatConversation({
      ID: 7,
      UserID: 42,
      LastMessageAt: '2026-01-02T03:04:05Z',
      UnreadByUser: 1,
      UnreadByAdmin: 2,
      CreatedAt: '2026-01-01T00:00:00Z',
      UpdatedAt: '2026-01-02T00:00:00Z',
      UserEmail: 'user@example.test',
      UserUsername: 'tester',
    })

    expect(conversation).toMatchObject({
      id: 7,
      user_id: 42,
      last_message_at: '2026-01-02T03:04:05Z',
      unread_by_user: 1,
      unread_by_admin: 2,
      user_email: 'user@example.test',
      user_username: 'tester',
    })
  })

  it('normalizes snake_case message fields', () => {
    const message = normalizeChatMessage({
      id: 9,
      conversation_id: 7,
      sender_type: 'admin',
      sender_id: 1,
      content: 'hello',
      kind: 'text',
      reply_to_id: null,
      metadata: {},
      assets: [],
      created_at: '2026-01-02T03:04:05Z',
    })

    expect(message).toEqual({
      id: 9,
      conversation_id: 7,
      sender_type: 'admin',
      sender_id: 1,
      content: 'hello',
      kind: 'text',
      reply_to_id: null,
      metadata: {},
      assets: [],
      created_at: '2026-01-02T03:04:05Z',
    })
  })

  it('normalizes structured messages, protected asset metadata, and read-state events', () => {
    const message = normalizeChatMessage({
      ID: 11,
      ConversationID: 8,
      SenderType: 'admin',
      SenderID: 3,
      Content: '[image]',
      Kind: 'image',
      ReplyToID: 10,
      Metadata: { caption: '<script>not rendered</script>' },
      Assets: [{ id: 5, scope: 'library', name: 'asset.png', mime_type: 'image/png', size: 42 }],
      CreatedAt: '2026-01-02T03:04:05Z',
    })

    expect(message).toMatchObject({
      id: 11,
      kind: 'image',
      reply_to_id: 10,
      metadata: { caption: '<script>not rendered</script>' },
      assets: [{ id: 5, scope: 'library', mime_type: 'image/png' }],
    })

    const event = parseChatSocketEvent(JSON.stringify({
      type: 'read_state',
      read_state: { conversation_id: 8, reader: 'user', read_at: '2026-01-02T04:00:00Z' },
    }))
    expect(event?.read_state).toEqual({
      conversation_id: 8,
      reader: 'user',
      read_at: '2026-01-02T04:00:00Z',
    })
  })

  it('parses websocket events from the chat hub', () => {
    const event = parseChatSocketEvent(JSON.stringify({
      Type: 'message',
      Message: {
        ID: 10,
        ConversationID: 8,
        SenderType: 'user',
        SenderID: 42,
        Content: 'need help',
        CreatedAt: '2026-01-02T03:04:05Z',
      },
    }))

    expect(event?.type).toBe('message')
    expect(event?.message).toMatchObject({
      id: 10,
      conversation_id: 8,
      sender_type: 'user',
      content: 'need help',
    })
  })

  it('ignores invalid websocket payloads', () => {
    expect(parseChatSocketEvent('not-json')).toBeNull()
  })
})
