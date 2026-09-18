import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAnnouncementStore } from '@/features/announcements/presentation/stores/announcementsStore'

const markRead = vi.hoisted(() => vi.fn())
vi.mock('@/features/announcements/data/datasources/announcementsDatasource', () => ({
  default: { markRead }
}))

beforeEach(() => {
  setActivePinia(createPinia())
  markRead.mockReset()
})

describe('mark all announcements read', () => {
  it('preserves successful results and retries only failures', async () => {
    const store = useAnnouncementStore()
    store.announcements = [1, 2].map((id) => ({
      id, title: `Notice ${id}`, content: 'Body', notify_mode: 'silent',
      created_at: '2026-09-18T00:00:00Z', updated_at: '2026-09-18T00:00:00Z'
    } as any))
    markRead.mockImplementation((id: number) => id === 1 ? Promise.resolve() : Promise.reject(new Error('offline')))
    await expect(store.markAllAsRead()).rejects.toThrow('offline')
    expect(store.announcements[0].read_at).toBeTruthy()
    expect(store.announcements[1].read_at).toBeFalsy()
    markRead.mockClear().mockResolvedValue(undefined)
    await store.markAllAsRead()
    expect(markRead.mock.calls).toEqual([[2]])
  })
})
