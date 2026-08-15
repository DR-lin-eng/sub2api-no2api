import apiClient, { buildApiUrl } from '@/core/networks/client'
import { getAccessToken } from '@/core/networks/tokenStore'
import type { PaginatedResponse } from '@/types'

export type ChatSenderType = 'user' | 'admin'

export interface ChatConversation {
  id: number
  user_id: number
  last_message_at: string | null
  unread_by_user: number
  unread_by_admin: number
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
}

export interface ChatAssetUpload {
  url: string
  name: string
  size: number
  mime_type: string
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

function numberValue(value: unknown, fallback = 0): number {
  const n = Number(value)
  return Number.isFinite(n) ? n : fallback
}

function stringValue(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : fallback
}

function nullableStringValue(value: unknown): string | null {
  return typeof value === 'string' && value.length > 0 ? value : null
}

export function normalizeChatConversation(raw: RawChatConversation): ChatConversation {
  return {
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
}

export function normalizeChatMessage(raw: RawChatMessage): ChatMessage {
  const senderType = raw.sender_type ?? raw.SenderType
  return {
    id: numberValue(raw.id ?? raw.ID),
    conversation_id: numberValue(raw.conversation_id ?? raw.ConversationID),
    sender_type: senderType === 'admin' ? 'admin' : 'user',
    sender_id: numberValue(raw.sender_id ?? raw.SenderID),
    content: stringValue(raw.content ?? raw.Content),
    created_at: stringValue(raw.created_at ?? raw.CreatedAt),
  }
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

export async function uploadUserChatAsset(file: File): Promise<ChatAssetUpload> {
  const form = new FormData()
  form.append('file', file)
  const { data } = await apiClient.post<ChatAssetUpload>('/chat/assets', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return data
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

export async function uploadAdminChatAsset(file: File): Promise<ChatAssetUpload> {
  const form = new FormData()
  form.append('file', file)
  const { data } = await apiClient.post<ChatAssetUpload>('/admin/chat/assets', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return data
}

export async function listAdminChatImageLibrary(): Promise<ChatImageLibraryItem[]> {
  const { data } = await apiClient.get<ChatImageLibraryItem[]>('/admin/chat/image-library')
  return Array.isArray(data) ? data : []
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
  const { data } = await apiClient.post<ChatImageLibraryItem>('/admin/chat/image-library', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return data
}

export async function deleteAdminChatImageLibraryItem(id: string): Promise<void> {
  await apiClient.delete(`/admin/chat/image-library/${encodeURIComponent(id)}`)
}

export async function listAdminChatStickers(): Promise<ChatStickerItem[]> {
  const { data } = await apiClient.get<ChatStickerItem[]>('/admin/chat/stickers')
  return Array.isArray(data) ? data : []
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
  const { data } = await apiClient.post<ChatStickerItem>('/admin/chat/stickers', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return data
}

export async function deleteAdminChatSticker(id: string): Promise<void> {
  await apiClient.delete(`/admin/chat/stickers/${encodeURIComponent(id)}`)
}

export async function markAdminChatRead(conversationID: number): Promise<void> {
  await apiClient.post(`/admin/chat/conversations/${conversationID}/read`)
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
