import { describe, expect, it } from 'vitest'
import {
  normalizeChatConversation,
  normalizeChatMessage,
  parseChatSocketEvent,
  resolveChatAssetRequestPath,
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
      created_at: '2026-01-02T03:04:05Z',
    })

    expect(message).toEqual({
      id: 9,
      conversation_id: 7,
      sender_type: 'admin',
      sender_id: 1,
      content: 'hello',
      created_at: '2026-01-02T03:04:05Z',
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

  it('normalizes structured message fields used by rich chat actions', () => {
    const message = normalizeChatMessage({
      ID: 11,
      ConversationID: 8,
      SenderType: 'admin',
      SenderID: 3,
      Content: '[image]',
      Kind: 'image',
      ReplyToID: 10,
      Assets: [{ ID: 5, Name: 'asset.png', MIMEType: 'image/png', Size: 42 }],
      RecalledAt: null,
      CreatedAt: '2026-01-02T03:04:05Z',
    })

    expect(message).toMatchObject({
      id: 11,
      kind: 'image',
      reply_to_id: 10,
      assets: [{ id: 5, name: 'asset.png', mime_type: 'image/png' }],
      recalled_at: null,
    })
  })

  it('maps both legacy filenames and current ids to authenticated asset paths', () => {
    expect(resolveChatAssetRequestPath('/api/v1/chat/assets/muxue_coin.png', 'user'))
      .toBe('/chat/assets/muxue_coin.png')
    expect(resolveChatAssetRequestPath('/api/v1/chat/assets/42', 'admin'))
      .toBe('/admin/chat/assets/42')
    expect(resolveChatAssetRequestPath('https://other.example/image.png', 'user')).toBeNull()
  })
})
