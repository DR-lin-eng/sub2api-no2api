import apiClient, { buildApiUrl } from '@/core/networks/client'
import { getAccessToken } from '@/core/networks/tokenStore'
import type { PaginatedResponse } from '@/types'

export type ChatSenderType = 'user' | 'admin'
export type ChatMessageKind = 'text' | 'image' | 'sticker' | 'balance_transfer'
export type ChatAssetScope = 'message' | 'library' | 'sticker'

export interface ChatConversation {
  id: number
  user_id: number
  last_message_at: string | null
  unread_by_user: number
  unread_by_admin: number
  manually_unread_by_admin: boolean
  last_read_by_user_at: string | null
  last_read_by_admin_at: string | null
  created_at: string
  updated_at: string
  user_email?: string
  user_username?: string
}

export interface ChatAsset {
  id: number
  scope: ChatAssetScope
  name: string
  mime_type: 'image/png' | 'image/jpeg'
  size: number
  collection?: string
  created_at: string
}

export interface ChatStickerMetadata {
  name: string
  emoji?: string
}

export interface ChatBalanceTransferMetadata {
  amount: number
  balance_after: number
  notes?: string
}

export interface ChatMessage {
  id: number
  conversation_id: number
  sender_type: ChatSenderType
  sender_id: number
  content: string
  kind: ChatMessageKind
  reply_to_id: number | null
  metadata: Record<string, unknown>
  assets: ChatAsset[]
  recalled_at: string | null
  created_at: string
}

export interface ChatQuickReply {
  id: number
  admin_id: number
  title: string
  content: string
  sort_order: number
  created_at: string
  updated_at: string
}

export interface ChatReadState {
  conversation_id: number
  reader: ChatSenderType
  read_at: string
}

export interface ChatSocketEvent {
  type: 'message' | 'message_recalled' | 'read_state' | string
  message?: ChatMessage
  read_state?: ChatReadState
}

export interface ChatSendMessageInput {
  content?: string
  kind?: ChatMessageKind
  reply_to_id?: number | null
  asset_ids?: number[]
  sticker?: ChatStickerMetadata
  idempotency_key?: string
}

export interface ChatConversationListParams {
  page?: number
  page_size?: number
  unread_only?: boolean
  search?: string
}

export interface ChatMessageListParams {
  page?: number
  page_size?: number
}

type RawRecord = Record<string, unknown>

const USER_CHAT_WS_PROTOCOL = 'sub2api-chat'
const ADMIN_CHAT_WS_PROTOCOL = 'sub2api-admin-chat'
const MAX_CHAT_ASSET_BYTES = 5 * 1024 * 1024
const ALLOWED_UPLOAD_TYPES = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp'])
const ALLOWED_ASSET_TYPES = new Set(['image/png', 'image/jpeg'])
const UPLOAD_TYPE_ALIASES = new Map([
  ['image/jpg', 'image/jpeg'],
  ['image/pjpeg', 'image/jpeg'],
])

function recordValue(value: unknown): RawRecord {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as RawRecord : {}
}

function pick(raw: RawRecord, snake: string, pascal: string): unknown {
  return raw[snake] ?? raw[pascal]
}

function numberValue(value: unknown, fallback = 0): number {
  const number = Number(value)
  return Number.isFinite(number) ? number : fallback
}

function positiveNumberOrNull(value: unknown): number | null {
  const number = numberValue(value)
  return number > 0 ? number : null
}

function stringValue(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : fallback
}

function nullableStringValue(value: unknown): string | null {
  return typeof value === 'string' && value.length > 0 ? value : null
}

function booleanValue(value: unknown): boolean {
  return value === true || value === 1 || value === 'true'
}

function normalizeMetadata(value: unknown): Record<string, unknown> {
  if (typeof value === 'string') {
    try {
      return recordValue(JSON.parse(value))
    } catch {
      return {}
    }
  }
  return recordValue(value)
}

export function normalizeChatConversation(value: unknown): ChatConversation {
  const raw = recordValue(value)
  return {
    id: numberValue(pick(raw, 'id', 'ID')),
    user_id: numberValue(pick(raw, 'user_id', 'UserID')),
    last_message_at: nullableStringValue(pick(raw, 'last_message_at', 'LastMessageAt')),
    unread_by_user: numberValue(pick(raw, 'unread_by_user', 'UnreadByUser')),
    unread_by_admin: numberValue(pick(raw, 'unread_by_admin', 'UnreadByAdmin')),
    manually_unread_by_admin: booleanValue(pick(raw, 'manually_unread_by_admin', 'ManuallyUnreadByAdmin')),
    last_read_by_user_at: nullableStringValue(pick(raw, 'last_read_by_user_at', 'LastReadByUserAt')),
    last_read_by_admin_at: nullableStringValue(pick(raw, 'last_read_by_admin_at', 'LastReadByAdminAt')),
    created_at: stringValue(pick(raw, 'created_at', 'CreatedAt')),
    updated_at: stringValue(pick(raw, 'updated_at', 'UpdatedAt')),
    user_email: stringValue(pick(raw, 'user_email', 'UserEmail')),
    user_username: stringValue(pick(raw, 'user_username', 'UserUsername')),
  }
}

export function normalizeChatAsset(value: unknown): ChatAsset {
  const raw = recordValue(value)
  const scope = stringValue(pick(raw, 'scope', 'Scope'))
  const mimeType = stringValue(pick(raw, 'mime_type', 'MIMEType'))
  return {
    id: numberValue(pick(raw, 'id', 'ID')),
    scope: scope === 'library' || scope === 'sticker' ? scope : 'message',
    name: stringValue(pick(raw, 'name', 'Name'), 'image'),
    mime_type: mimeType === 'image/jpeg' ? 'image/jpeg' : 'image/png',
    size: numberValue(pick(raw, 'size', 'Size')),
    collection: stringValue(pick(raw, 'collection', 'Collection')) || undefined,
    created_at: stringValue(pick(raw, 'created_at', 'CreatedAt')),
  }
}

export function normalizeChatMessage(value: unknown): ChatMessage {
  const raw = recordValue(value)
  const senderType = stringValue(pick(raw, 'sender_type', 'SenderType'))
  const kind = stringValue(pick(raw, 'kind', 'Kind'))
  const assets = pick(raw, 'assets', 'Assets')
  return {
    id: numberValue(pick(raw, 'id', 'ID')),
    conversation_id: numberValue(pick(raw, 'conversation_id', 'ConversationID')),
    sender_type: senderType === 'admin' ? 'admin' : 'user',
    sender_id: numberValue(pick(raw, 'sender_id', 'SenderID')),
    content: stringValue(pick(raw, 'content', 'Content')),
    kind: kind === 'image' || kind === 'sticker' || kind === 'balance_transfer' ? kind : 'text',
    reply_to_id: positiveNumberOrNull(pick(raw, 'reply_to_id', 'ReplyToID')),
    metadata: normalizeMetadata(pick(raw, 'metadata', 'Metadata')),
    assets: Array.isArray(assets) ? assets.map(normalizeChatAsset).filter(asset => asset.id > 0) : [],
    recalled_at: nullableStringValue(pick(raw, 'recalled_at', 'RecalledAt')),
    created_at: stringValue(pick(raw, 'created_at', 'CreatedAt')),
  }
}

export function normalizeChatQuickReply(value: unknown): ChatQuickReply {
  const raw = recordValue(value)
  return {
    id: numberValue(pick(raw, 'id', 'ID')),
    admin_id: numberValue(pick(raw, 'admin_id', 'AdminID')),
    title: stringValue(pick(raw, 'title', 'Title')),
    content: stringValue(pick(raw, 'content', 'Content')),
    sort_order: numberValue(pick(raw, 'sort_order', 'SortOrder')),
    created_at: stringValue(pick(raw, 'created_at', 'CreatedAt')),
    updated_at: stringValue(pick(raw, 'updated_at', 'UpdatedAt')),
  }
}

function normalizePaginated<T>(data: PaginatedResponse<unknown>, normalizer: (value: unknown) => T): PaginatedResponse<T> {
  return { ...data, items: Array.isArray(data.items) ? data.items.map(normalizer) : [] }
}

export function parseChatSocketEvent(raw: string): ChatSocketEvent | null {
  try {
    const value = recordValue(JSON.parse(raw))
    const type = stringValue(pick(value, 'type', 'Type'))
    if (!type) return null
    const message = pick(value, 'message', 'Message')
    const readStateValue = pick(value, 'read_state', 'ReadState')
    const readState = recordValue(readStateValue)
    return {
      type,
      message: message ? normalizeChatMessage(message) : undefined,
      read_state: Object.keys(readState).length > 0
        ? {
            conversation_id: numberValue(pick(readState, 'conversation_id', 'ConversationID')),
            reader: stringValue(pick(readState, 'reader', 'Reader')) === 'admin' ? 'admin' : 'user',
            read_at: stringValue(pick(readState, 'read_at', 'ReadAt')),
          }
        : undefined,
    }
  } catch {
    return null
  }
}

export function createChatIdempotencyKey(prefix = 'chat'): string {
  const random = typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID().replace(/-/g, '')
    : `${Date.now().toString(36)}${Math.random().toString(36).slice(2)}`
  return `${prefix}-${Date.now().toString(36)}-${random}`.slice(0, 128)
}

function chatHeaders(key: string): { 'Idempotency-Key': string } {
  return { 'Idempotency-Key': key }
}

export async function getUserChatConversation(): Promise<ChatConversation> {
  const { data } = await apiClient.get<unknown>('/chat/conversation')
  return normalizeChatConversation(data)
}

export async function listUserChatMessages(params: ChatMessageListParams): Promise<PaginatedResponse<ChatMessage>> {
  const { data } = await apiClient.get<PaginatedResponse<unknown>>('/chat/messages', { params })
  return normalizePaginated(data, normalizeChatMessage)
}

export async function sendUserChatMessage(input: ChatSendMessageInput): Promise<ChatMessage> {
  const idempotencyKey = input.idempotency_key || createChatIdempotencyKey('user-chat')
  const { data } = await apiClient.post<unknown>('/chat/messages', { ...input, idempotency_key: idempotencyKey }, {
    headers: chatHeaders(idempotencyKey),
  })
  return normalizeChatMessage(data)
}

export async function markUserChatRead(): Promise<void> {
  await apiClient.post('/chat/read')
}

export async function getUserChatUnreadCount(): Promise<number> {
  const { data } = await apiClient.get<{ unread_count?: number }>('/chat/unread-count')
  return numberValue(data?.unread_count)
}

export async function listAdminChatConversations(params: ChatConversationListParams): Promise<PaginatedResponse<ChatConversation>> {
  const { data } = await apiClient.get<PaginatedResponse<unknown>>('/admin/chat/conversations', { params })
  return normalizePaginated(data, normalizeChatConversation)
}

export async function getAdminChatUnreadCount(): Promise<number> {
  const { data } = await apiClient.get<{ unread_count?: number }>('/admin/chat/unread-count')
  return numberValue(data?.unread_count)
}

export async function listAdminChatMessages(conversationID: number, params: ChatMessageListParams): Promise<PaginatedResponse<ChatMessage>> {
  const { data } = await apiClient.get<PaginatedResponse<unknown>>(`/admin/chat/conversations/${conversationID}/messages`, { params })
  return normalizePaginated(data, normalizeChatMessage)
}

export async function sendAdminChatMessage(conversationID: number, input: ChatSendMessageInput): Promise<ChatMessage> {
  const idempotencyKey = input.idempotency_key || createChatIdempotencyKey('admin-chat')
  const { data } = await apiClient.post<unknown>(`/admin/chat/conversations/${conversationID}/messages`, {
    ...input,
    idempotency_key: idempotencyKey,
  }, { headers: chatHeaders(idempotencyKey) })
  return normalizeChatMessage(data)
}

export async function recallAdminChatMessage(conversationID: number, messageID: number): Promise<ChatMessage> {
  const { data } = await apiClient.post<unknown>(
    `/admin/chat/conversations/${conversationID}/messages/${messageID}/recall`,
  )
  return normalizeChatMessage(data)
}

export async function markAdminChatRead(conversationID: number): Promise<void> {
  await apiClient.post(`/admin/chat/conversations/${conversationID}/read`)
}

export async function markAdminChatUnread(conversationID: number): Promise<void> {
  await apiClient.post(`/admin/chat/conversations/${conversationID}/unread`)
}

function normalizeUploadType(value: string): string {
  const type = value.split(';', 1)[0].trim().toLowerCase()
  return UPLOAD_TYPE_ALIASES.get(type) || type
}

function canonicalUploadFilename(mimeType: string): string {
  return `image.${mimeType === 'image/jpeg' ? 'jpg' : mimeType.slice('image/'.length)}`
}

function sniffImageType(bytes: Uint8Array): string | null {
  if (bytes.length >= 8 && bytes[0] === 0x89 && bytes[1] === 0x50 && bytes[2] === 0x4e && bytes[3] === 0x47 &&
      bytes[4] === 0x0d && bytes[5] === 0x0a && bytes[6] === 0x1a && bytes[7] === 0x0a) {
    return 'image/png'
  }
  if (bytes.length >= 3 && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff) return 'image/jpeg'
  if (bytes.length >= 6 && bytes[0] === 0x47 && bytes[1] === 0x49 && bytes[2] === 0x46 &&
      ((bytes[3] === 0x38 && bytes[4] === 0x37 && bytes[5] === 0x61) ||
       (bytes[3] === 0x38 && bytes[4] === 0x39 && bytes[5] === 0x61))) {
    return 'image/gif'
  }
  if (bytes.length >= 12 && bytes[0] === 0x52 && bytes[1] === 0x49 && bytes[2] === 0x46 && bytes[3] === 0x46 &&
      bytes[8] === 0x57 && bytes[9] === 0x45 && bytes[10] === 0x42 && bytes[11] === 0x50) {
    return 'image/webp'
  }
  return null
}

async function readUploadPrefix(file: File): Promise<Uint8Array> {
  const slice = file.slice(0, 512)
  if (typeof slice.arrayBuffer === 'function') return new Uint8Array(await slice.arrayBuffer())
  return await new Promise<Uint8Array>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(new Uint8Array(reader.result as ArrayBuffer))
    reader.onerror = () => reject(reader.error || new Error('Unable to inspect image file'))
    reader.readAsArrayBuffer(slice)
  })
}

async function prepareUploadFile(file: File): Promise<File> {
  if (!file || file.size <= 0 || file.size > MAX_CHAT_ASSET_BYTES) {
    throw new Error('Only PNG, JPEG, GIF, or WebP images up to 5 MiB are allowed')
  }

  const rawType = normalizeUploadType(file.type)
  const genericType = rawType === '' || rawType === 'application/octet-stream'
  if (!genericType && !ALLOWED_UPLOAD_TYPES.has(rawType)) {
    throw new Error('Only PNG, JPEG, GIF, or WebP images up to 5 MiB are allowed')
  }

  const mimeType = genericType
    ? sniffImageType(await readUploadPrefix(file))
    : rawType
  if (!mimeType || !ALLOWED_UPLOAD_TYPES.has(mimeType)) {
    throw new Error('Only PNG, JPEG, GIF, or WebP images up to 5 MiB are allowed')
  }

  if (file.type.toLowerCase() === mimeType && file.name) return file
  return new File([file], canonicalUploadFilename(mimeType), { type: mimeType, lastModified: file.lastModified })
}

async function uploadForm(file: File, collectionField?: string, collection?: string): Promise<FormData> {
  const uploadFile = await prepareUploadFile(file)
  const form = new FormData()
  form.append('file', uploadFile, canonicalUploadFilename(normalizeUploadType(uploadFile.type)))
  if (collectionField && collection?.trim()) form.append(collectionField, collection.trim().slice(0, 100))
  return form
}

const multipartRequestConfig = {
  // Remove the JSON default so XMLHttpRequest supplies multipart/form-data
  // together with the generated boundary.
  headers: { 'Content-Type': undefined },
}

export async function uploadUserChatAsset(file: File): Promise<ChatAsset> {
  const { data } = await apiClient.post<unknown>('/chat/assets', await uploadForm(file), multipartRequestConfig)
  return normalizeChatAsset(data)
}

export async function uploadAdminChatAsset(conversationID: number, file: File): Promise<ChatAsset> {
  const { data } = await apiClient.post<unknown>(
    `/admin/chat/conversations/${conversationID}/assets`,
    await uploadForm(file),
    multipartRequestConfig,
  )
  return normalizeChatAsset(data)
}

export async function listAdminChatCatalog(scope: 'library' | 'sticker', limit = 100): Promise<ChatAsset[]> {
  const path = scope === 'library' ? '/admin/chat/image-library' : '/admin/chat/stickers'
  const { data } = await apiClient.get<unknown[]>(path, { params: { limit: Math.min(100, Math.max(1, limit)) } })
  return Array.isArray(data) ? data.map(normalizeChatAsset).filter(asset => asset.id > 0) : []
}

export async function createAdminChatCatalogAsset(
  scope: 'library' | 'sticker',
  file: File,
  collection = '',
): Promise<ChatAsset> {
  const path = scope === 'library' ? '/admin/chat/image-library' : '/admin/chat/stickers'
  const field = scope === 'library' ? 'category' : 'group'
  const { data } = await apiClient.post<unknown>(path, await uploadForm(file, field, collection), multipartRequestConfig)
  return normalizeChatAsset(data)
}

export async function deleteAdminChatCatalogAsset(scope: 'library' | 'sticker', id: number): Promise<void> {
  const path = scope === 'library' ? '/admin/chat/image-library' : '/admin/chat/stickers'
  await apiClient.delete(`${path}/${id}`)
}

export async function getChatAssetBlob(scope: 'user' | 'admin', id: number): Promise<Blob> {
  const path = scope === 'admin' ? `/admin/chat/assets/${id}` : `/chat/assets/${id}`
  const { data, headers } = await apiClient.get<Blob>(path, { responseType: 'blob' })
  const headerType = stringValue(headers['content-type']).split(';', 1)[0].toLowerCase()
  const blobType = data.type.split(';', 1)[0].toLowerCase()
  const responseTypes = [headerType, blobType].filter(Boolean)
  const mimeType = headerType || blobType
  if (responseTypes.length === 0 || responseTypes.some(type => !ALLOWED_ASSET_TYPES.has(type)) ||
      (headerType && blobType && headerType !== blobType) || data.size <= 0 || data.size > MAX_CHAT_ASSET_BYTES) {
    throw new Error('Chat image response failed security validation')
  }
  return blobType ? data : data.slice(0, data.size, mimeType)
}

export async function listAdminChatQuickReplies(): Promise<ChatQuickReply[]> {
  const { data } = await apiClient.get<unknown[]>('/admin/chat/quick-replies')
  return Array.isArray(data) ? data.map(normalizeChatQuickReply) : []
}

export async function createAdminChatQuickReply(title: string, content: string): Promise<ChatQuickReply> {
  const { data } = await apiClient.post<unknown>('/admin/chat/quick-replies', { title, content })
  return normalizeChatQuickReply(data)
}

export async function updateAdminChatQuickReply(id: number, title: string, content: string): Promise<ChatQuickReply> {
  const { data } = await apiClient.put<unknown>(`/admin/chat/quick-replies/${id}`, { title, content })
  return normalizeChatQuickReply(data)
}

export async function deleteAdminChatQuickReply(id: number): Promise<void> {
  await apiClient.delete(`/admin/chat/quick-replies/${id}`)
}

export async function reorderAdminChatQuickReplies(ids: number[]): Promise<void> {
  await apiClient.post('/admin/chat/quick-replies/reorder', { ids })
}

export async function importAdminChatQuickReplies(items: Array<{ title: string; content: string }>): Promise<ChatQuickReply[]> {
  const { data } = await apiClient.post<unknown[]>('/admin/chat/quick-replies/import', { items })
  return Array.isArray(data) ? data.map(normalizeChatQuickReply) : []
}

export interface ChatBalanceTransferResult {
  message: ChatMessage
  user_id: number
  balance: number
  replayed: boolean
}

export async function transferAdminChatBalance(
  conversationID: number,
  amount: number,
  notes: string,
  idempotencyKey = createChatIdempotencyKey('chat-transfer'),
): Promise<ChatBalanceTransferResult> {
  const { data } = await apiClient.post<unknown>(`/admin/chat/conversations/${conversationID}/balance-transfers`, {
    amount,
    notes,
  }, { headers: chatHeaders(idempotencyKey) })
  const raw = recordValue(data)
  return {
    message: normalizeChatMessage(pick(raw, 'message', 'Message')),
    user_id: numberValue(pick(raw, 'user_id', 'UserID')),
    balance: numberValue(pick(raw, 'balance', 'Balance')),
    replayed: Boolean(pick(raw, 'replayed', 'Replayed')),
  }
}

export function buildChatWebSocket(scope: 'user' | 'admin'): WebSocket | null {
  const token = getAccessToken()
  if (!token) return null

  const path = scope === 'admin' ? '/admin/chat/ws' : '/chat/ws'
  const httpURL = buildApiUrl(path)
  const url = new URL(httpURL, window.location.origin)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'

  const protocol = scope === 'admin' ? ADMIN_CHAT_WS_PROTOCOL : USER_CHAT_WS_PROTOCOL
  return new WebSocket(url.toString(), [protocol, `jwt.${token}`])
}
