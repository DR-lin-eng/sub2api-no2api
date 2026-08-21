import apiClient, { buildApiUrl } from '@/core/networks/client'
import { getAccessToken } from '@/core/networks/tokenStore'
import type { PaginatedResponse } from '@/types'

export type ChatSenderType = 'user' | 'admin'
export type ChatMessageKind = 'text' | 'image' | 'sticker' | 'balance_transfer'

export interface ChatAsset {
  id: number
  scope: 'message' | 'library' | 'sticker'
  name: string
  mime_type: string
  size: number
  url?: string
  collection?: string
  created_at?: string
}

export interface ChatStickerMetadata {
  name: string
  emoji?: string
}

export interface ChatSendMessageInput {
  content?: string
  kind?: ChatMessageKind
  reply_to_id?: number | null
  asset_ids?: number[]
  sticker?: ChatStickerMetadata
  idempotency_key?: string
}

export interface ChatConversation {
  id: number
  user_id: number
  last_message_at: string | null
  unread_by_user: number
  unread_by_admin: number
  manually_unread_by_admin?: boolean
  created_at: string
  updated_at: string
  user_email?: string
  user_username?: string
}

export interface ChatMessage {
  id: number
  conversation_id: number
  sender_type: ChatSenderType
  sender_id: number
  content: string
  created_at: string
  reply_to?: number // 回复的消息 ID
  kind?: ChatMessageKind
  reply_to_id?: number | null
  metadata?: Record<string, unknown>
  assets?: ChatAsset[]
  recalled_at?: string | null
}

export interface ChatAssetUpload {
  id?: number
  url: string
  name: string
  size: number
  mime_type: string
}

interface RawChatAssetUpload {
  id?: number
  url?: string
  name?: string
  size?: number
  mime_type?: string
  MIMEType?: string
}

export interface ChatImageLibraryItem {
  id: string
  name: string
  category: string
  url: string
  size: number
  mime_type: string
  created_at: string
}

export interface ChatStickerItem {
  id: string
  name: string
  group: string
  url: string
  size: number
  mime_type: string
  created_at: string
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

interface RawChatConversation {
  id?: number
  ID?: number
  user_id?: number
  UserID?: number
  last_message_at?: string | null
  LastMessageAt?: string | null
  unread_by_user?: number
  UnreadByUser?: number
  unread_by_admin?: number
  UnreadByAdmin?: number
  manually_unread_by_admin?: boolean
  ManuallyUnreadByAdmin?: boolean
  created_at?: string
  CreatedAt?: string
  updated_at?: string
  UpdatedAt?: string
  user_email?: string
  UserEmail?: string
  user_username?: string
  UserUsername?: string
}

interface RawChatMessage {
  id?: number
  ID?: number
  conversation_id?: number
  ConversationID?: number
  sender_type?: ChatSenderType
  SenderType?: ChatSenderType
  sender_id?: number
  SenderID?: number
  content?: string
  Content?: string
  created_at?: string
  CreatedAt?: string
  kind?: ChatMessageKind
  Kind?: ChatMessageKind
  reply_to_id?: number | null
  ReplyToID?: number | null
  metadata?: Record<string, unknown>
  Metadata?: Record<string, unknown>
  assets?: unknown[]
  Assets?: unknown[]
  recalled_at?: string | null
  RecalledAt?: string | null
}

interface RawSocketEvent {
  type?: string
  Type?: string
  message?: RawChatMessage
  Message?: RawChatMessage
  read_state?: {
    conversation_id?: number
    ConversationID?: number
    reader?: string
    Reader?: string
  }
  ReadState?: {
    conversation_id?: number
    ConversationID?: number
    reader?: string
    Reader?: string
  }
}

const USER_CHAT_WS_PROTOCOL = 'sub2api-chat'
const ADMIN_CHAT_WS_PROTOCOL = 'sub2api-admin-chat'
const MAX_CHAT_ASSET_BYTES = 5 * 1024 * 1024
const ALLOWED_CHAT_ASSET_TYPES = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp'])
const UPLOAD_TYPE_ALIASES = new Map([
  ['image/jpg', 'image/jpeg'],
  ['image/pjpeg', 'image/jpeg'],
])

function numberValue(value: unknown, fallback = 0): number {
  const n = Number(value)
  return Number.isFinite(n) ? n : fallback
}

function stringValue(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : fallback
}

/** Convert both old filename URLs and current numeric URLs to an API-client path. */
export function resolveChatAssetRequestPath(
  value: string,
  scope: 'user' | 'admin' = 'user',
): string | null {
  const raw = value.trim()
  if (!raw) return null
  const appOrigin = typeof window === 'undefined' ? 'http://localhost' : window.location.origin
  let parsed: URL
  try {
    parsed = new URL(raw, appOrigin)
    const apiBase = new URL(buildApiUrl('/'), appOrigin)
    if (parsed.origin !== apiBase.origin) return null
  } catch {
    return null
  }

  const apiBasePath = new URL(buildApiUrl('/'), appOrigin).pathname.replace(/\/+$/, '')
  let suffix = parsed.pathname
  if (apiBasePath && suffix.startsWith(`${apiBasePath}/`)) {
    suffix = suffix.slice(apiBasePath.length)
  } else if (suffix.startsWith('/api/v1/')) {
    suffix = suffix.slice('/api/v1'.length)
  }
  const match = suffix.match(/^\/(?:admin\/)?chat\/assets\/([^/]+)$/)
  if (!match) return null
  const reference = match[1]
  const route = scope === 'admin' ? '/admin/chat/assets/' : '/chat/assets/'
  return `${route}${reference}${parsed.search}`
}

export async function getChatAssetBlobByURL(
  value: string,
  scope: 'user' | 'admin' = 'user',
): Promise<Blob> {
  const path = resolveChatAssetRequestPath(value, scope)
  if (!path) throw new Error('External image URL')
  const { data, headers } = await apiClient.get<Blob>(path, { responseType: 'blob' })
  const headerType = stringValue(headers['content-type']).split(';', 1)[0].toLowerCase()
  const blobType = stringValue(data?.type).split(';', 1)[0].toLowerCase()
  const mimeType = headerType || blobType
  if (!ALLOWED_CHAT_ASSET_TYPES.has(mimeType) ||
      (headerType && blobType && headerType !== blobType) ||
      !data || data.size <= 0 || data.size > MAX_CHAT_ASSET_BYTES) {
    throw new Error('Chat image response failed security validation')
  }
  return blobType ? data : data.slice(0, data.size, mimeType)
}

function normalizeAssetUpload(raw: RawChatAssetUpload, scope: 'user' | 'admin'): ChatAssetUpload {
  const id = numberValue(raw.id)
  const fallbackPath = id > 0
    ? scope === 'admin' ? `/admin/chat/assets/${id}` : `/chat/assets/${id}`
    : ''
  return {
    id: id > 0 ? id : undefined,
    url: stringValue(raw.url, fallbackPath),
    name: stringValue(raw.name, 'image'),
    size: numberValue(raw.size),
    mime_type: stringValue(raw.mime_type ?? raw.MIMEType),
  }
}

function normalizeChatAsset(raw: unknown): ChatAsset {
  const value = raw && typeof raw === 'object' ? raw as Record<string, unknown> : {}
  const scope = stringValue(value.scope ?? value.Scope)
  const id = numberValue(value.id ?? value.ID)
  return {
    id,
    scope: scope === 'library' || scope === 'sticker' ? scope : 'message',
    name: stringValue(value.name ?? value.Name, 'image'),
    mime_type: stringValue(value.mime_type ?? value.MIMEType),
    size: numberValue(value.size ?? value.Size),
    url: stringValue(value.url ?? value.URL, id > 0 ? `/chat/assets/${id}` : '') || undefined,
    collection: stringValue(value.collection ?? value.Collection) || undefined,
    created_at: stringValue(value.created_at ?? value.CreatedAt) || undefined,
  }
}

function normalizeCatalogItem(raw: Record<string, unknown>, field: 'category'): ChatImageLibraryItem
function normalizeCatalogItem(raw: Record<string, unknown>, field: 'group'): ChatStickerItem
function normalizeCatalogItem(raw: Record<string, unknown>, field: 'category' | 'group'): ChatImageLibraryItem | ChatStickerItem {
  const id = String(raw.id ?? raw.ID ?? '')
  const collection = stringValue(raw.collection ?? raw[field], '默认')
  const base = {
    id,
    name: stringValue(raw.name ?? raw.Name, 'image'),
    url: stringValue(raw.url ?? raw.URL, id ? `/admin/chat/assets/${id}` : ''),
    size: numberValue(raw.size ?? raw.Size),
    mime_type: stringValue(raw.mime_type ?? raw.MIMEType),
    created_at: stringValue(raw.created_at ?? raw.CreatedAt),
  }
  return field === 'category'
    ? { ...base, category: collection }
    : { ...base, group: collection }
}

function nullableStringValue(value: unknown): string | null {
  return typeof value === 'string' && value.length > 0 ? value : null
}

export function normalizeChatConversation(raw: RawChatConversation): ChatConversation {
  const conversation: ChatConversation = {
    id: numberValue(raw.id ?? raw.ID),
    user_id: numberValue(raw.user_id ?? raw.UserID),
    last_message_at: nullableStringValue(raw.last_message_at ?? raw.LastMessageAt),
    unread_by_user: numberValue(raw.unread_by_user ?? raw.UnreadByUser),
    unread_by_admin: numberValue(raw.unread_by_admin ?? raw.UnreadByAdmin),
    created_at: stringValue(raw.created_at ?? raw.CreatedAt),
    updated_at: stringValue(raw.updated_at ?? raw.UpdatedAt),
    user_email: stringValue(raw.user_email ?? raw.UserEmail),
    user_username: stringValue(raw.user_username ?? raw.UserUsername),
  }
  if (raw.manually_unread_by_admin !== undefined || raw.ManuallyUnreadByAdmin !== undefined) {
    conversation.manually_unread_by_admin = Boolean(raw.manually_unread_by_admin ?? raw.ManuallyUnreadByAdmin)
  }
  return conversation
}

export function normalizeChatMessage(raw: RawChatMessage): ChatMessage {
  const senderType = raw.sender_type ?? raw.SenderType
  const kind = raw.kind ?? raw.Kind
  const assetsRaw = raw.assets ?? raw.Assets
  const message: ChatMessage = {
    id: numberValue(raw.id ?? raw.ID),
    conversation_id: numberValue(raw.conversation_id ?? raw.ConversationID),
    sender_type: senderType === 'admin' ? 'admin' : 'user',
    sender_id: numberValue(raw.sender_id ?? raw.SenderID),
    content: stringValue(raw.content ?? raw.Content),
    created_at: stringValue(raw.created_at ?? raw.CreatedAt),
  }
  if (kind === 'text' || kind === 'image' || kind === 'sticker' || kind === 'balance_transfer') message.kind = kind
  if (raw.reply_to_id !== undefined || raw.ReplyToID !== undefined) {
    message.reply_to_id = raw.reply_to_id ?? raw.ReplyToID ?? null
    message.reply_to = message.reply_to_id ?? undefined
  }
  const metadata = raw.metadata ?? raw.Metadata
  if (metadata && typeof metadata === 'object' && !Array.isArray(metadata)) message.metadata = metadata as Record<string, unknown>
  if (Array.isArray(assetsRaw)) message.assets = assetsRaw.map(normalizeChatAsset).filter((asset) => asset.id > 0)
  if (raw.recalled_at !== undefined || raw.RecalledAt !== undefined) message.recalled_at = raw.recalled_at ?? raw.RecalledAt ?? null
  return message
}

function normalizePaginatedMessages(data: PaginatedResponse<RawChatMessage>): PaginatedResponse<ChatMessage> {
  return {
    ...data,
    items: Array.isArray(data.items) ? data.items.map(normalizeChatMessage) : [],
  }
}

function normalizePaginatedConversations(data: PaginatedResponse<RawChatConversation>): PaginatedResponse<ChatConversation> {
  return {
    ...data,
    items: Array.isArray(data.items) ? data.items.map(normalizeChatConversation) : [],
  }
}

export function parseChatSocketEvent(raw: string): { type: string; message?: ChatMessage; read_state?: { conversation_id: number; reader: 'user' | 'admin' } } | null {
  try {
    const data = JSON.parse(raw) as RawSocketEvent
    const type = stringValue(data.type ?? data.Type)
    const message = data.message ?? data.Message
    const rawReadState = data.read_state ?? data.ReadState
    if (!type) return null

    let readState: { conversation_id: number; reader: 'user' | 'admin' } | undefined
    if (rawReadState) {
      const conversationID = numberValue(rawReadState.conversation_id ?? rawReadState.ConversationID)
      const reader = stringValue(rawReadState.reader ?? rawReadState.Reader)
      if (conversationID && (reader === 'user' || reader === 'admin')) {
        readState = { conversation_id: conversationID, reader }
      }
    }

    return { type, message: message ? normalizeChatMessage(message) : undefined, read_state: readState }
  } catch {
    return null
  }
}

export async function getUserChatConversation(): Promise<ChatConversation> {
  const { data } = await apiClient.get<RawChatConversation>('/chat/conversation')
  return normalizeChatConversation(data)
}

export async function listUserChatMessages(params: ChatMessageListParams): Promise<PaginatedResponse<ChatMessage>> {
  const { data } = await apiClient.get<PaginatedResponse<RawChatMessage>>('/chat/messages', { params })
  return normalizePaginatedMessages(data)
}

export async function sendUserChatMessage(content: string): Promise<ChatMessage> {
  const { data } = await apiClient.post<RawChatMessage>('/chat/messages', { content })
  return normalizeChatMessage(data)
}

export function createChatIdempotencyKey(prefix = 'chat'): string {
  const random = typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `${prefix}:${random}`.slice(0, 128)
}

export async function sendUserChatRichMessage(input: ChatSendMessageInput): Promise<ChatMessage> {
  const { data } = await apiClient.post<RawChatMessage>('/chat/messages', {
    ...input,
    idempotency_key: input.idempotency_key || createChatIdempotencyKey('user-chat'),
  })
  return normalizeChatMessage(data)
}

export async function uploadUserChatAsset(file: File): Promise<ChatAssetUpload> {
  const uploadFile = await prepareUploadFile(file)
  const form = new FormData()
  form.append('file', uploadFile, canonicalUploadFilename(normalizeUploadType(uploadFile.type)))
  const { data } = await apiClient.post<RawChatAssetUpload>('/chat/assets', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return normalizeAssetUpload(data, 'user')
}

export async function markUserChatRead(): Promise<void> {
  await apiClient.post('/chat/read')
}

export async function getUserChatUnreadCount(): Promise<number> {
  const { data } = await apiClient.get<{ unread_count?: number }>('/chat/unread-count')
  return numberValue(data?.unread_count)
}

export async function listAdminChatConversations(params: ChatConversationListParams): Promise<PaginatedResponse<ChatConversation>> {
  const { data } = await apiClient.get<PaginatedResponse<RawChatConversation>>('/admin/chat/conversations', { params })
  return normalizePaginatedConversations(data)
}

export async function getAdminChatUnreadCount(): Promise<number> {
  const { data } = await apiClient.get<{ unread_count?: number }>('/admin/chat/unread-count')
  return numberValue(data?.unread_count)
}

export async function listAdminChatMessages(conversationID: number, params: ChatMessageListParams): Promise<PaginatedResponse<ChatMessage>> {
  const { data } = await apiClient.get<PaginatedResponse<RawChatMessage>>(
    `/admin/chat/conversations/${conversationID}/messages`,
    { params },
  )
  return normalizePaginatedMessages(data)
}

export async function sendAdminChatMessage(conversationID: number, content: string): Promise<ChatMessage> {
  const { data } = await apiClient.post<RawChatMessage>(`/admin/chat/conversations/${conversationID}/messages`, { content })
  return normalizeChatMessage(data)
}

export async function sendAdminChatRichMessage(conversationID: number, input: ChatSendMessageInput): Promise<ChatMessage> {
  const { data } = await apiClient.post<RawChatMessage>(`/admin/chat/conversations/${conversationID}/messages`, {
    ...input,
    idempotency_key: input.idempotency_key || createChatIdempotencyKey('admin-chat'),
  })
  return normalizeChatMessage(data)
}

export async function recallAdminChatMessage(conversationID: number, messageID: number): Promise<ChatMessage> {
  const { data } = await apiClient.post<RawChatMessage>(
    `/admin/chat/conversations/${conversationID}/messages/${messageID}/recall`,
  )
  return normalizeChatMessage(data)
}

export async function uploadAdminChatAsset(file: File, conversationID?: number): Promise<ChatAssetUpload> {
  const uploadFile = await prepareUploadFile(file)
  const form = new FormData()
  form.append('file', uploadFile, canonicalUploadFilename(normalizeUploadType(uploadFile.type)))
  const path = conversationID
    ? `/admin/chat/conversations/${conversationID}/assets`
    : '/admin/chat/assets'
  const { data } = await apiClient.post<RawChatAssetUpload>(path, form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return normalizeAssetUpload(data, 'admin')
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
      bytes[4] === 0x0d && bytes[5] === 0x0a && bytes[6] === 0x1a && bytes[7] === 0x0a) return 'image/png'
  if (bytes.length >= 3 && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff) return 'image/jpeg'
  if (bytes.length >= 6 && bytes[0] === 0x47 && bytes[1] === 0x49 && bytes[2] === 0x46 &&
      bytes[3] === 0x38 && (bytes[4] === 0x37 || bytes[4] === 0x39) && bytes[5] === 0x61) return 'image/gif'
  if (bytes.length >= 12 && bytes[0] === 0x52 && bytes[1] === 0x49 && bytes[2] === 0x46 && bytes[3] === 0x46 &&
      bytes[8] === 0x57 && bytes[9] === 0x45 && bytes[10] === 0x42 && bytes[11] === 0x50) return 'image/webp'
  return null
}

async function prepareUploadFile(file: File): Promise<File> {
  if (!file || file.size <= 0 || file.size > MAX_CHAT_ASSET_BYTES) {
    throw new Error('Only PNG, JPEG, GIF, or WebP images up to 5 MiB are allowed')
  }
  const rawType = normalizeUploadType(file.type)
  const genericType = rawType === '' || rawType === 'application/octet-stream'
  if (!genericType && !ALLOWED_CHAT_ASSET_TYPES.has(rawType)) {
    throw new Error('Only PNG, JPEG, GIF, or WebP images up to 5 MiB are allowed')
  }
  let mimeType = rawType
  if (genericType) {
    const prefix = new Uint8Array(await file.slice(0, 512).arrayBuffer())
    mimeType = sniffImageType(prefix) || ''
  }
  if (!ALLOWED_CHAT_ASSET_TYPES.has(mimeType)) {
    throw new Error('Only PNG, JPEG, GIF, or WebP images up to 5 MiB are allowed')
  }
  if (file.type.toLowerCase() === mimeType && file.name) return file
  return new File([file], canonicalUploadFilename(mimeType), { type: mimeType, lastModified: file.lastModified })
}

export async function listAdminChatImageLibrary(): Promise<ChatImageLibraryItem[]> {
  const { data } = await apiClient.get<Array<Record<string, unknown>>>('/admin/chat/image-library')
  return Array.isArray(data) ? data.map((item) => normalizeCatalogItem(item, 'category')) : []
}

export async function createAdminChatImageLibraryItem(params: {
  file: File
  name?: string
  category?: string
}): Promise<ChatImageLibraryItem> {
  const form = new FormData()
  form.append('file', params.file)
  if (params.name) form.append('name', params.name)
  if (params.category) form.append('category', params.category)
  const { data } = await apiClient.post<Record<string, unknown>>('/admin/chat/image-library', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return normalizeCatalogItem(data, 'category')
}

export async function deleteAdminChatImageLibraryItem(id: string): Promise<void> {
  await apiClient.delete(`/admin/chat/image-library/${encodeURIComponent(id)}`)
}

export async function listAdminChatStickers(): Promise<ChatStickerItem[]> {
  const { data } = await apiClient.get<Array<Record<string, unknown>>>('/admin/chat/stickers')
  return Array.isArray(data) ? data.map((item) => normalizeCatalogItem(item, 'group')) : []
}

export async function createAdminChatSticker(params: {
  file: File
  name?: string
  group?: string
}): Promise<ChatStickerItem> {
  const form = new FormData()
  form.append('file', params.file)
  if (params.name) form.append('name', params.name)
  if (params.group) form.append('group', params.group)
  const { data } = await apiClient.post<Record<string, unknown>>('/admin/chat/stickers', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return normalizeCatalogItem(data, 'group')
}

export async function deleteAdminChatSticker(id: string): Promise<void> {
  await apiClient.delete(`/admin/chat/stickers/${encodeURIComponent(id)}`)
}

export async function markAdminChatRead(conversationID: number): Promise<void> {
  await apiClient.post(`/admin/chat/conversations/${conversationID}/read`)
}

export async function markAdminChatUnread(conversationID: number): Promise<void> {
  await apiClient.post(`/admin/chat/conversations/${conversationID}/unread`)
}

export function buildChatWebSocket(scope: 'user' | 'admin'): WebSocket | null {
  const token = getAccessToken()
  if (!token) return null

  const path = scope === 'admin' ? '/admin/chat/ws' : '/chat/ws'
  const httpURL = buildApiUrl(path)

  // 构建完整的 WebSocket URL
  let wsURL: string
  if (httpURL.startsWith('http://') || httpURL.startsWith('https://')) {
    // 绝对 URL：直接替换协议
    wsURL = httpURL.replace(/^https?:/, httpURL.startsWith('https:') ? 'wss:' : 'ws:')
  } else {
    // 相对 URL：基于当前页面构建
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = window.location.host
    wsURL = `${protocol}//${host}${httpURL}`
  }

  const protocol = scope === 'admin' ? ADMIN_CHAT_WS_PROTOCOL : USER_CHAT_WS_PROTOCOL
  return new WebSocket(wsURL, [protocol, `jwt.${token}`])
}

// ============== 快捷回复 ==============

export interface ChatQuickReply {
  id: number
  admin_id: number
  title: string
  content: string
  sort_order: number
  created_at: string
  updated_at: string
}

interface RawChatQuickReply {
  id?: number
  ID?: number
  admin_id?: number
  AdminID?: number
  title?: string
  Title?: string
  content?: string
  Content?: string
  sort_order?: number
  SortOrder?: number
  created_at?: string
  CreatedAt?: string
  updated_at?: string
  UpdatedAt?: string
}

function normalizeChatQuickReply(raw: RawChatQuickReply): ChatQuickReply {
  return {
    id: numberValue(raw.id ?? raw.ID),
    admin_id: numberValue(raw.admin_id ?? raw.AdminID),
    title: stringValue(raw.title ?? raw.Title),
    content: stringValue(raw.content ?? raw.Content),
    sort_order: numberValue(raw.sort_order ?? raw.SortOrder),
    created_at: stringValue(raw.created_at ?? raw.CreatedAt),
    updated_at: stringValue(raw.updated_at ?? raw.UpdatedAt),
  }
}

export async function listAdminChatQuickReplies(): Promise<ChatQuickReply[]> {
  const { data } = await apiClient.get<RawChatQuickReply[]>('/admin/chat/quick-replies')
  if (!Array.isArray(data)) return []
  return data.map(normalizeChatQuickReply)
}

export async function createAdminChatQuickReply(params: { title: string; content: string }): Promise<ChatQuickReply> {
  const { data } = await apiClient.post<RawChatQuickReply>('/admin/chat/quick-replies', params)
  return normalizeChatQuickReply(data)
}

export async function updateAdminChatQuickReply(id: number, params: { title: string; content: string }): Promise<ChatQuickReply> {
  const { data } = await apiClient.put<RawChatQuickReply>(`/admin/chat/quick-replies/${id}`, params)
  return normalizeChatQuickReply(data)
}

export async function deleteAdminChatQuickReply(id: number): Promise<void> {
  await apiClient.delete(`/admin/chat/quick-replies/${id}`)
}

export async function reorderAdminChatQuickReplies(ids: number[]): Promise<void> {
  await apiClient.post('/admin/chat/quick-replies/reorder', { ids })
}

export async function importAdminChatQuickReplies(
  items: Array<{ title: string; content: string }>,
): Promise<ChatQuickReply[]> {
  const { data } = await apiClient.post<RawChatQuickReply[]>('/admin/chat/quick-replies/import', { items })
  return Array.isArray(data) ? data.map(normalizeChatQuickReply) : []
}

export interface ChatBalanceTransferResult {
  balance: number
  message?: ChatMessage
}

export async function transferAdminChatBalance(
  conversationID: number,
  amount: number,
  notes = '',
): Promise<ChatBalanceTransferResult> {
  const { data } = await apiClient.post<ChatBalanceTransferResult>(
    `/admin/chat/conversations/${conversationID}/balance-transfers`,
    { amount, notes },
    { headers: { 'Idempotency-Key': createChatIdempotencyKey('balance-transfer') } },
  )
  return {
    balance: numberValue(data?.balance),
    message: data?.message ? normalizeChatMessage(data.message as unknown as RawChatMessage) : undefined,
  }
}
