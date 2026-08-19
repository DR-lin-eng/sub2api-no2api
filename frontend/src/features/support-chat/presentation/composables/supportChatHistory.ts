import type { PaginatedResponse } from '@/types'

export const SUPPORT_CHAT_MESSAGE_PAGE_SIZE = 100

export function chatMessagePageCount(page: Pick<PaginatedResponse<unknown>, 'pages' | 'total' | 'page_size'>): number {
  const declared = Number(page.pages)
  if (Number.isFinite(declared) && declared > 0) return declared
  const pageSize = Number(page.page_size) > 0 ? Number(page.page_size) : SUPPORT_CHAT_MESSAGE_PAGE_SIZE
  return Math.max(1, Math.ceil(Number(page.total || 0) / pageSize))
}

export function mergeChatMessages<T extends { id: number }>(existing: T[], incoming: T[]): T[] {
  const byID = new Map(existing.map((message) => [message.id, message]))
  for (const message of incoming) byID.set(message.id, message)
  return [...byID.values()]
}

// The API returns newest-first pages. Keep the loaded page boundary in the
// caller while merging older pages by ID, allowing refreshes and socket updates
// to keep already visible history.
export async function loadChatMessagePages<T extends { id: number }>(
  fetchPage: (page: number) => Promise<PaginatedResponse<T>>,
  throughPage = 1,
): Promise<{ items: T[]; page: number; pages: number }> {
  const firstPage = await fetchPage(1)
  const pages = chatMessagePageCount(firstPage)
  const targetPage = Math.min(Math.max(1, throughPage), pages)
  let items = firstPage.items

  for (let page = 2; page <= targetPage; page += 1) {
    const nextPage = await fetchPage(page)
    items = mergeChatMessages(items, nextPage.items)
  }

  return { items, page: targetPage, pages }
}
