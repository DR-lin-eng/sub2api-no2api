import type { ChatConversation } from '@/features/support-chat/data/datasources/supportChatDatasource'

function isUnread(conversation: ChatConversation): boolean {
  return conversation.unread_by_admin > 0 || conversation.manually_unread_by_admin
}

function activityTime(conversation: ChatConversation): number {
  return Date.parse(conversation.last_message_at || conversation.updated_at) || 0
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
