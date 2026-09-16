import i18n, { getLocale, loadLocaleMessages } from '@/core/i18n'
import type { ChatMessage } from '@/features/support-chat/data/datasources/supportChatDatasource'

export function buildSupportNotificationBody(message: ChatMessage): string {
  if (message.kind === 'image' || message.assets.length > 0) {
    return i18n.global.t('supportChat.browserNotifications.imageMessage')
  }
  if (message.kind === 'sticker') {
    return i18n.global.t('supportChat.browserNotifications.stickerMessage')
  }
  return i18n.global.t('supportChat.browserNotifications.newMessageBody')
}

export async function buildSupportNotificationContent(message: ChatMessage): Promise<{ title: string; body: string }> {
  await loadLocaleMessages(getLocale(), ['supportChat'])
  return {
    title: i18n.global.t('supportChat.browserNotifications.newMessageTitle'),
    body: buildSupportNotificationBody(message),
  }
}
