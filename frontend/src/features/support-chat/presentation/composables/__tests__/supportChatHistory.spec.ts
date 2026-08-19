import { describe, expect, it } from 'vitest'
import {
  chatMessagePageCount,
  loadChatMessagePages,
  mergeChatMessages,
} from '@/features/support-chat/presentation/composables/supportChatHistory'

function page(items: Array<{ id: number }>, pageNumber: number, pages: number): {
  items: Array<{ id: number }>
  total: number
  page: number
  page_size: number
  pages: number
} {
  return { items, total: 250, page: pageNumber, page_size: 100, pages }
}

describe('support chat history pagination', () => {
  it('loads every requested older page and de-duplicates overlapping rows', async () => {
    const calls: number[] = []
    const result = await loadChatMessagePages(async (pageNumber) => {
      calls.push(pageNumber)
      if (pageNumber === 1) return page([{ id: 250 }, { id: 249 }], 1, 3)
      if (pageNumber === 2) return page([{ id: 150 }, { id: 249 }], 2, 3)
      return page([{ id: 50 }, { id: 49 }], 3, 3)
    }, 3)

    expect(calls).toEqual([1, 2, 3])
    expect(result.page).toBe(3)
    expect(result.pages).toBe(3)
    expect(result.items.map((item) => item.id)).toEqual([250, 249, 150, 50, 49])
  })

  it('derives page count when an older server omits pages', () => {
    expect(chatMessagePageCount({ pages: 0, total: 201, page_size: 100 })).toBe(3)
  })

  it('replaces a stale duplicate with the refreshed message payload', () => {
    expect(mergeChatMessages([{ id: 1, value: 'old' }], [{ id: 1, value: 'new' }, { id: 2, value: 'two' }]))
      .toEqual([{ id: 1, value: 'new' }, { id: 2, value: 'two' }])
  })
})
