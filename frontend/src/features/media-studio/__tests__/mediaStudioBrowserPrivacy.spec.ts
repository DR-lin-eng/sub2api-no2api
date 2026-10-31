import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import { mount } from '@vue/test-utils'
import {
  openMediaStudioDatabase,
} from '../data/datasources/mediaStudioImageStore'
import {
  mediaStudioBlobDataURL,
  MAX_MEDIA_STUDIO_REFERENCE_BYTES,
} from '../data/datasources/mediaStudioBrowserMedia'
import { uploadMediaStudioFile, resolveMediaStudioModelLimits } from '../data/datasources/mediaStudioDatasource'
import {
  conversationForPersistence,
  readPersisted,
  type MediaStudioConversation,
} from '../presentation/composables/mediaStudioConversation'
import { useMediaStudioController } from '../presentation/composables/useMediaStudioController'
import { useMediaStudioJobs } from '../presentation/composables/useMediaStudioJobs'

const media = vi.hoisted(() => ({ videos: new Map<string, Blob>() }))
vi.mock('../data/datasources/mediaStudioImageStore', async (original) => {
  const actual = await original<typeof import('../data/datasources/mediaStudioImageStore')>()
  return { ...actual, storeMediaStudioVideo: async (key: string, blob: Blob, ownerID: number) => {
    media.videos.set(`${ownerID}:${key}`, blob)
    return true
  } }
})

afterEach(() => { localStorage.clear(); media.videos.clear(); vi.unstubAllGlobals() })

describe('browser-only media privacy', () => {
  it('never reads unscoped history or another authenticated account cache', () => {
    localStorage.setItem('sub2api.mediaStudio.v2', JSON.stringify({ conversation: { prompt: 'unscoped secret' } }))
    localStorage.setItem('sub2api.mediaStudio.v2.user.11', JSON.stringify({ imageModel: 'user-11-model' }))
    expect(readPersisted(localStorage)).toEqual({})
    expect(readPersisted(localStorage, 12)).toEqual({})
    expect(readPersisted(localStorage, 11)).toMatchObject({ imageModel: 'user-11-model' })
    expect(useMediaStudioController({ storage: localStorage }).canSubmit.value).toBe(false)
  })

  it('captures each database owner before asynchronous opens complete', async () => {
    const pending: Array<{ name: string; request: { result?: unknown; onsuccess?: () => void } }> = []
    const open = vi.fn((name: string) => {
      const request = {}
      pending.push({ name, request })
      return request
    })
    vi.stubGlobal('indexedDB', { open })
    expect(await openMediaStudioDatabase()).toBeNull()
    const first = openMediaStudioDatabase(11)
    const second = openMediaStudioDatabase(12)
    expect(pending.map(item => item.name)).toEqual(['sub2api-media-studio-user-11', 'sub2api-media-studio-user-12'])
    const db11 = { close: vi.fn() }
    const db12 = { close: vi.fn() }
    pending[1].request.result = db12; pending[1].request.onsuccess?.()
    pending[0].request.result = db11; pending[0].request.onsuccess?.()
    expect(await first).toBe(db11)
    expect(await second).toBe(db12)
  })

  it('prepares reference bytes locally without an upload request', async () => {
    const fetch = vi.fn(); vi.stubGlobal('fetch', fetch)
    const result = await uploadMediaStudioFile('video-model', 'image', new File(['reference'], 'ref.png', { type: 'image/png' }))
    expect(result.url).toMatch(/^data:image\/png;base64,/)
    expect(fetch).not.toHaveBeenCalled()
    const oversized = { size: MAX_MEDIA_STUDIO_REFERENCE_BYTES + 1, type: 'image/png' } as Blob
    await expect(mediaStudioBlobDataURL(oversized)).rejects.toThrow(/20 MB/)
    expect(resolveMediaStudioModelLimits({}).allowUpload).toBe(true)
    expect(resolveMediaStudioModelLimits({ file_types: [] }).fileTypes).toEqual([])
  })

  it('keeps cached reference and result bytes out of localStorage snapshots', () => {
    const conversation: MediaStudioConversation = { id: 'chat', updatedAt: 1, messages: [{
      id: 'user', role: 'user', mode: 'video', prompt: 'scene', createdAt: 1,
      inputReferences: [{ id: 'ref', name: 'ref.mp4', kind: 'video', mimeType: 'video/mp4', src: 'blob:preview', url: 'data:video/mp4;base64,AAAA', cacheKey: 'local-reference' }],
    }] }
    const snapshot = conversationForPersistence(conversation)
    expect(snapshot?.messages[0].inputReferences).toMatchObject([{ cacheKey: 'local-reference', src: '', url: '' }])
    expect(JSON.stringify(snapshot)).not.toContain('base64')
    expect(JSON.stringify(snapshot)).not.toContain('blob:')
  })

  it('resumes only the tasks belonging to the active group and caches completed video bytes', async () => {
    const conversation = ref<MediaStudioConversation>({ id: 'chat', updatedAt: 1, messages: [
      { id: 'wrong', role: 'assistant', mode: 'video', prompt: 'other group', groupId: 2, status: 'processing', taskId: 'other-task', createdAt: 1 },
      { id: 'right', role: 'assistant', mode: 'video', prompt: 'same group', groupId: 1, status: 'processing', taskId: 'own-task', createdAt: 1 },
    ] })
    const getVideoTask = vi.fn().mockResolvedValue({ id: 'own-task', status: 'completed', raw: {} })
    const content = new Blob(['video'], { type: 'video/mp4' })
    const jobs = useMediaStudioJobs({
      conversation, ownerID: 11,
      patchMessage: (id, patch) => { const message = conversation.value.messages.find(item => item.id === id); if (message) Object.assign(message, patch); return !!message },
      getApiKey: () => 'group-one-key', getActiveGroupId: () => 1,
      createObjectURL: () => 'blob:completed', revokeObjectURL: vi.fn(), videoObjectURLs: new Set(), isDisposed: () => false, persist: vi.fn(),
      data: { getTask: vi.fn(), getVideoTask, getVideoContent: vi.fn().mockResolvedValue(content) }, pollDelayMs: 0,
    })
    jobs.resumePendingJobs()
    await vi.waitFor(() => expect(conversation.value.messages[1].status).toBe('completed'))
    expect(getVideoTask).toHaveBeenCalledExactlyOnceWith('group-one-key', 'own-task')
    expect(conversation.value.messages[0].status).toBe('processing')
    expect(conversation.value.messages[1].video).toMatchObject({ cacheKey: 'right:video' })
    expect(media.videos.get('11:right:video')).toBe(content)
  })
  it('aborts the old workspace request when its authenticated owner is unmounted', async () => {
    let controller: ReturnType<typeof useMediaStudioController>
    let signal: AbortSignal | undefined
    let complete: ((value: unknown) => void) | undefined
    const pending = new Promise(resolve => { complete = resolve })
    const wrapper = mount(defineComponent({ setup() {
      controller = useMediaStudioController({ ownerID: 11,
        getConfig: vi.fn().mockResolvedValue({ groups: [{ group_id: 1, group_name: 'videos', platform: 'openai', media_type: 'video' }] }),
        createSession: vi.fn().mockResolvedValue({ api_key: 'owner-11-key', group_id: 1, media_type: 'video' }),
        listModels: vi.fn().mockResolvedValue({ object: 'list', data: [] }),
        estimateCost: vi.fn().mockResolvedValue({ amount: 1, currency: 'USD', billing_mode: 'video', model: 'test' }),
        submitVideo: vi.fn((_key, _payload, _id, requestSignal) => { signal = requestSignal; return pending as never }),
      })
      return () => h('div')
    } }))
    await controller!.loadMediaGroups()
    controller!.prompt.value = 'private pending video'
    const submission = controller!.submitPrompt()
    expect(signal?.aborted).toBe(false)
    wrapper.unmount()
    expect(signal?.aborted).toBe(true)
    complete?.({ id: 'done', status: 'failed', raw: {} })
    await submission
    expect(controller!.conversation.value.messages.at(-1)?.status).toBe('queued')
  })

})
