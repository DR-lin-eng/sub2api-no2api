import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, describe, expect, it, vi } from 'vitest'
import apiClient from '@/core/networks/client'
import {
  getChatAssetBlob,
  markAdminChatUnread,
  recallAdminChatMessage,
  sendAdminChatMessage,
  transferAdminChatBalance,
  uploadUserChatAsset,
} from '@/features/support-chat/data/datasources/supportChatDatasource'

const currentDir = dirname(fileURLToPath(import.meta.url))

describe('support chat frontend security contract', () => {
  afterEach(() => vi.restoreAllMocks())

  it('renders all chat content as Vue text instead of untrusted HTML', () => {
    const messageList = readFileSync(resolve(currentDir, '../presentation/widgets/SupportMessageList.vue'), 'utf8')
    const composer = readFileSync(resolve(currentDir, '../presentation/widgets/SupportMessageComposer.vue'), 'utf8')
    expect(messageList).not.toContain('v-html')
    expect(composer).not.toContain('v-html')
    expect(messageList).toContain('{{ message.content }}')
  })

  it('uses one durable idempotency key in both header and message payload', async () => {
    const post = vi.spyOn(apiClient, 'post').mockResolvedValue({
      data: { ID: 1, ConversationID: 7, SenderType: 'admin', SenderID: 3, Content: 'hello', Kind: 'text' },
    })
    await sendAdminChatMessage(7, { content: 'hello', kind: 'text', idempotency_key: 'chat-key-123' })

    expect(post).toHaveBeenCalledWith('/admin/chat/conversations/7/messages', expect.objectContaining({
      content: 'hello', idempotency_key: 'chat-key-123',
    }), { headers: { 'Idempotency-Key': 'chat-key-123' } })
  })

  it('uses the atomic transfer endpoint rather than a separate balance update', async () => {
    const post = vi.spyOn(apiClient, 'post').mockResolvedValue({
      data: {
        message: { ID: 2, ConversationID: 7, SenderType: 'admin', SenderID: 3, Content: 'credit', Kind: 'balance_transfer' },
        user_id: 42,
        balance: 12.5,
      },
    })
    const result = await transferAdminChatBalance(7, 2.5, 'goodwill', 'transfer-key-123')

    expect(post).toHaveBeenCalledOnce()
    expect(post.mock.calls[0][0]).toBe('/admin/chat/conversations/7/balance-transfers')
    expect(post.mock.calls[0][2]).toEqual({ headers: { 'Idempotency-Key': 'transfer-key-123' } })
    expect(result).toMatchObject({ user_id: 42, balance: 12.5, message: { kind: 'balance_transfer' } })
  })

  it('uses persistent admin endpoints for recall and manual unread reminders', async () => {
    const post = vi.spyOn(apiClient, 'post')
      .mockResolvedValueOnce({
        data: {
          ID: 9,
          ConversationID: 7,
          SenderType: 'admin',
          SenderID: 3,
          Content: '',
          Kind: 'text',
          RecalledAt: '2026-01-02T03:05:00Z',
        },
      })
      .mockResolvedValueOnce({ data: { message: 'ok' } })

    const recalled = await recallAdminChatMessage(7, 9)
    await markAdminChatUnread(7)

    expect(post).toHaveBeenNthCalledWith(1, '/admin/chat/conversations/7/messages/9/recall')
    expect(post).toHaveBeenNthCalledWith(2, '/admin/chat/conversations/7/unread')
    expect(recalled).toMatchObject({ id: 9, content: '', recalled_at: '2026-01-02T03:05:00Z' })
  })

  it('loads images through authenticated blobs and rejects unsafe response types', async () => {
    const get = vi.spyOn(apiClient, 'get')
      .mockResolvedValueOnce({ data: new Blob(['png'], { type: 'image/png' }), headers: { 'content-type': 'image/png' } })
      .mockResolvedValueOnce({ data: new Blob(['html'], { type: 'text/html' }), headers: { 'content-type': 'text/html' } })
      .mockResolvedValueOnce({ data: new Blob(['html'], { type: 'image/png' }), headers: { 'content-type': 'text/html' } })

    const blob = await getChatAssetBlob('user', 9)
    expect(blob.type).toBe('image/png')
    expect(get).toHaveBeenNthCalledWith(1, '/chat/assets/9', { responseType: 'blob' })
    await expect(getChatAssetBlob('admin', 9)).rejects.toThrow(/security validation/)
    await expect(getChatAssetBlob('admin', 9)).rejects.toThrow(/security validation/)
  })

  it('accepts only bounded image files before upload while leaving decoding to the server', async () => {
    const post = vi.spyOn(apiClient, 'post').mockResolvedValue({
      data: { id: 4, scope: 'message', name: 'image.png', mime_type: 'image/png', size: 3 },
    })
    await uploadUserChatAsset(new File(['png'], 'payload.html', { type: 'image/png' }))
    expect(post.mock.calls[0][0]).toBe('/chat/assets')
    expect(post.mock.calls[0][1]).toBeInstanceOf(FormData)

    await expect(uploadUserChatAsset(new File(['<svg/>'], 'payload.svg', { type: 'image/svg+xml' })))
      .rejects.toThrow(/Only PNG/)
  })

  it('normalizes non-standard JPEG MIME and sniffs generic clipboard image files', async () => {
    const post = vi.spyOn(apiClient, 'post').mockResolvedValue({
      data: { id: 4, scope: 'message', name: 'image.jpg', mime_type: 'image/jpeg', size: 3 },
    })

    await uploadUserChatAsset(new File(['jpeg'], 'photo.jpg', { type: 'image/jpg' }))
    const jpegForm = post.mock.calls[0]?.[1] as FormData
    expect((jpegForm.get('file') as File).type).toBe('image/jpeg')
    expect((jpegForm.get('file') as File).name).toBe('image.jpg')

    await uploadUserChatAsset(new File([
      new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    ], 'clipboard-image', { type: 'application/octet-stream' }))
    const sniffedForm = post.mock.calls[1]?.[1] as FormData
    expect((sniffedForm.get('file') as File).type).toBe('image/png')
    expect((sniffedForm.get('file') as File).name).toBe('image.png')
  })
})
