import type { ChatConversation } from '@/features/support-chat/data/datasources/supportChatDatasource'

function isUnread(conversation: ChatConversation): boolean {
  return conversation.unread_by_admin > 0 || conversation.manually_unread_by_admin
}

function activityTime(conversation: ChatConversation): number {
  return messageTime(conversation.last_message_at)
}

function messageTime(value: string | null): number {
  const timestamp = Date.parse(value || '')
  return Number.isFinite(timestamp) ? timestamp : Number.NEGATIVE_INFINITY
}

export function latestSupportChatMessageAt(current: string | null, incoming: string | null): string | null {
  return messageTime(incoming) > messageTime(current) ? incoming : current
}

// Keep activity received while a list request was pending. Other rows use the
// response, including server-side retention cleanup and search filtering.
export function reconcileSupportChatConversations(
  incoming: ChatConversation[],
  beforeLoad: ChatConversation[],
  current: ChatConversation[],
): ChatConversation[] {
  const previousByID = new Map(beforeLoad.map(conversation => [conversation.id, conversation.last_message_at]))
  const currentByID = new Map(current.map(conversation => [conversation.id, conversation]))
  return incoming.map(conversation => {
    const latest = currentByID.get(conversation.id)
    if (!latest || latest.last_message_at === previousByID.get(conversation.id)) return conversation
    return {
      ...conversation,
      last_message_at: latestSupportChatMessageAt(conversation.last_message_at, latest.last_message_at),
    }
  })
}

/** Keep actionable inbox conversations ahead of already-read conversations. */
export function sortSupportChatConversations(conversations: ChatConversation[]): ChatConversation[] {
  return [...conversations].sort((left, right) => {
    const leftUnread = isUnread(left)
    const rightUnread = isUnread(right)
    if (leftUnread !== rightUnread) return leftUnread ? -1 : 1

    const leftActivity = activityTime(left)
    const rightActivity = activityTime(right)
    if (leftActivity !== rightActivity) return rightActivity - leftActivity
    return right.id - left.id
  })
}
