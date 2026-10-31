import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useMediaStudioGallery as createMediaStudioGallery } from '../presentation/composables/useMediaStudioGallery'
import type { MediaStudioMessage } from '../presentation/composables/mediaStudioConversation'

const useMediaStudioGallery = () => createMediaStudioGallery({ ownerID: 1 })

const state = vi.hoisted(() => ({
  items: [] as Array<Record<string, unknown>>,
  media: new Map<string, string | Blob>(),
  archiveState: { excludedIDs: [] as string[], clearedBefore: 0 },
}))

vi.mock('@/features/media-studio/data/datasources/mediaStudioGalleryStore', () => ({
  readMediaStudioGalleryArchiveState: async () => state.archiveState,
  writeMediaStudioGalleryArchiveState: async (value: typeof state.archiveState) => { state.archiveState = value },
  mediaStudioGalleryMediaKey: (itemID: string) => `gallery:${itemID}`,
  listMediaStudioGalleryItems: async () => [...state.items]
    .sort((a, b) => Number(b.createdAt || 0) - Number(a.createdAt || 0)),
  putMediaStudioGalleryItem: async (item: Record<string, unknown>) => {
    state.items = state.items.filter(entry => entry.id !== item.id)
    state.items.push(item)
    return true
  },
  putMediaStudioGalleryMedia: async (key: string, value: string | Blob) => {
    state.media.set(key, value)
    return true
  },
  getMediaStudioGalleryMedia: async (key: string) => state.media.get(key) ?? null,
  deleteMediaStudioGalleryItems: async (items: Array<Record<string, unknown>>) => {
    const ids = new Set(items.map(item => item.id))
    state.items = state.items.filter(item => !ids.has(item.id))
    items.forEach(item => {
      if (typeof item.mediaKey === 'string') state.media.delete(item.mediaKey)
    })
  },
  clearMediaStudioGallery: async () => {
    state.items = []
    state.media.clear()
  },
}))

vi.mock('@/features/media-studio/data/datasources/mediaStudioImageStore', () => ({
  normalizeMediaStudioOwnerID: (ownerID?: number) => ownerID || 0,
  openMediaStudioDatabase: async () => ({ close: () => undefined }),
  loadMediaStudioImage: async () => '',
  loadMediaStudioVideo: vi.fn(async () => null),
}))

function imageMessage(overrides: Partial<MediaStudioMessage> = {}): MediaStudioMessage {
  return {
    id: 'msg-image',
    role: 'assistant',
    mode: 'image',
    prompt: 'a quiet cat',
    status: 'completed',
    createdAt: 1_000,
    completedAt: 2_000,
    images: [{ id: 'img-1', src: 'data:image/png;base64,AAAA', url: 'https://cdn.example.com/a.png' }],
    ...overrides,
  } as MediaStudioMessage
}

describe('media studio gallery archive', () => {
  beforeEach(() => {
    state.items = []
    state.media.clear()
    state.archiveState = { excludedIDs: [], clearedBefore: 0 }
  })

  it('archives completed remote images once and dedupes repeated saves', async () => {
    const gallery = useMediaStudioGallery()
    await gallery.saveFromMessages([imageMessage()])

    expect(state.items).toHaveLength(1)
    expect(state.items[0].remoteURL).toBe('https://cdn.example.com/a.png')
    expect(state.items[0].prompt).toBe('a quiet cat')

    await gallery.saveFromMessages([imageMessage()])
    expect(state.items).toHaveLength(1)
    expect(gallery.items.value).toHaveLength(1)
  })

  it('copies inline image data into the gallery media store', async () => {
    const gallery = useMediaStudioGallery()
    await gallery.saveFromMessages([
      imageMessage({
        id: 'msg-inline',
        images: [{ id: 'img-inline', src: 'data:image/png;base64,AAAA' }],
        completedAt: 3_000,
      }),
    ])

    expect(state.items).toHaveLength(1)
    expect(state.items[0].mediaKey).toBe('gallery:gallery_msg-inline_img-inline')
    expect(state.media.get('gallery:gallery_msg-inline_img-inline')).toBe('data:image/png;base64,AAAA')
    expect(gallery.sources['gallery_msg-inline_img-inline']).toBeUndefined()
    await gallery.loadSources(['gallery_msg-inline_img-inline'])
    expect(gallery.sources['gallery_msg-inline_img-inline']).toBe('data:image/png;base64,AAAA')
  })

  it('skips failed messages and pending images', async () => {
    const gallery = useMediaStudioGallery()
    await gallery.saveFromMessages([
      imageMessage({ id: 'msg-failed', status: 'failed' }),
      imageMessage({ id: 'msg-processing', status: 'processing' }),
    ])

    expect(state.items).toHaveLength(0)
  })

  it('toggles favorite and removes the archived work with its media', async () => {
    const gallery = useMediaStudioGallery()
    await gallery.saveFromMessages([imageMessage({ id: 'msg-inline', images: [{ id: 'img', src: 'data:image/png;base64,AAAA' }] })])
    const archived = gallery.items.value[0]

    await gallery.toggleFavorite(archived)
    expect(gallery.items.value[0].favorite).toBe(true)
    expect(state.items[0].favorite).toBe(true)

    await gallery.remove(gallery.items.value[0])
    expect(gallery.items.value).toHaveLength(0)
    expect(state.items).toHaveLength(0)
    expect(state.media.size).toBe(0)
    await gallery.saveFromMessages([imageMessage({ id: 'msg-inline', images: [{ id: 'img', src: 'data:image/png;base64,AAAA' }] })])
    expect(state.items).toHaveLength(0)
    const reopened = useMediaStudioGallery()
    await reopened.refresh()
    await reopened.saveFromMessages([imageMessage({ id: 'msg-inline', images: [{ id: 'img', src: 'data:image/png;base64,AAAA' }] })])
    expect(state.items).toHaveLength(0)
  })
})
