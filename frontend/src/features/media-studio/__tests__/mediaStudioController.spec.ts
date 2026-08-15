import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useMediaStudioController } from '@/features/media-studio/presentation/composables/useMediaStudioController'
import type { ApiKey, PaginatedResponse } from '@/types'

function key(): ApiKey {
  return { id: 1, key: 'sk-demo', name: 'demo', status: 'active' } as ApiKey
}

function page(items: ApiKey[]): PaginatedResponse<ApiKey> {
  return { items, total: items.length, page: 1, page_size: 100, pages: 1 }
}

async function settle() {
  await new Promise(resolve => setTimeout(resolve, 0))
  await nextTick()
}

describe('media studio controller', () => {
  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('loads image models and submits a completed image result', async () => {
    const submitImage = vi.fn().mockResolvedValue({
      id: 'image-1', task_id: 'image-1', object: 'image.generation.task', status: 'completed',
      result: { data: [{ url: 'https://cdn.example/image.png' }] }, created_at: 1, expires_at: 2,
    })
    const controller = useMediaStudioController({
      storage: localStorage,
      listKeys: vi.fn().mockResolvedValue(page([key()])),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }, { id: 'grok-imagine-video' }] }),
      submitImage,
      pollDelayMs: 0,
    })

    await controller.loadApiKeys()
    controller.prompt.value = 'a quiet cat'
    await controller.submitPrompt()

    expect(controller.modelOptions.value).toEqual(['gpt-image-2'])
    expect(submitImage).toHaveBeenCalledWith('sk-demo', expect.objectContaining({
      model: 'gpt-image-2', prompt: 'a quiet cat', response_format: 'b64_json',
    }), expect.stringMatching(/^media_idem_/))
    expect(controller.conversation.value.messages.at(-1)).toMatchObject({
      mode: 'image', status: 'completed', images: [{ src: 'https://cdn.example/image.png' }],
    })
  })

  it('runs video submit, polling, authenticated content loading, and URL cleanup', async () => {
    const createObjectURL = vi.fn().mockReturnValue('blob:video-preview')
    const revokeObjectURL = vi.fn()
    const getVideoContent = vi.fn().mockResolvedValue(new Blob(['video'], { type: 'video/mp4' }))
    const controller = useMediaStudioController({
      storage: localStorage,
      listKeys: vi.fn().mockResolvedValue(page([key()])),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }, { id: 'grok-imagine-video' }] }),
      submitVideo: vi.fn().mockResolvedValue({ id: 'video-1', status: 'processing', raw: {} }),
      getVideoTask: vi.fn().mockResolvedValue({ id: 'video-1', status: 'completed', raw: {} }),
      getVideoContent,
      createObjectURL,
      revokeObjectURL,
      pollDelayMs: 0,
    })

    await controller.loadApiKeys()
    controller.selectMode('video')
    await settle()
    controller.resolution.value = '1080p'
    controller.duration.value = 15
    controller.prompt.value = 'slow camera move'
    await controller.submitPrompt()
    await settle()

    expect(controller.modelOptions.value).toEqual(['grok-imagine-video'])
    expect(getVideoContent).toHaveBeenCalledWith('sk-demo', 'video-1')
    expect(controller.conversation.value.messages.at(-1)).toMatchObject({
      mode: 'video', status: 'completed', resolution: '1080p', duration: 15,
      video: { src: 'blob:video-preview', mimeType: 'video/mp4' },
    })

    controller.clearConversation()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:video-preview')
  })

  it('exposes a real batch handoff and persists only small configuration', async () => {
    const controller = useMediaStudioController({ storage: localStorage })
    expect(controller.modes.map(mode => [mode.id, mode.available])).toEqual([
      ['image', true], ['video', true], ['batch', true],
    ])
    controller.selectMode('batch')
    expect(controller.canSubmit.value).toBe(false)

    controller.prompt.value = 'secret prompt that must stay in memory'
    controller.count.value = 4
    await nextTick()
    const persisted = localStorage.getItem('sub2api.mediaStudio.v2') || ''
    expect(persisted).toContain('"count":4')
    expect(persisted).not.toContain('secret prompt')
    expect(persisted).not.toContain('data:image')
  })
})
