import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useMediaStudioController } from '@/features/media-studio/presentation/composables/useMediaStudioController'
import type {
  MediaStudioConfig,
  MediaStudioSession,
} from '@/features/media-studio/data/datasources/mediaStudioDatasource'

const config: MediaStudioConfig = {
  groups: [{ group_id: 1, group_name: 'Media', platform: 'openai', models: ['gpt-image-2', 'grok-imagine-video'] }],
}

function session(apiKey = 'sk-demo'): MediaStudioSession {
  return { api_key: apiKey, group_id: 1, media_type: 'image' }
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
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }] }),
      submitImage,
      pollDelayMs: 0,
    })

    await controller.loadMediaGroups()
    controller.prompt.value = 'a quiet cat'
    await controller.submitPrompt()

    expect(controller.modelOptions.value).toEqual(['gpt-image-2'])
    expect(controller.imageQualityOptions.value.map(option => option.value)).toEqual(['auto', 'low', 'medium', 'high'])
    expect(submitImage).toHaveBeenCalledWith('sk-demo', expect.objectContaining({
      model: 'gpt-image-2', prompt: 'a quiet cat',
    }), expect.stringMatching(/^media_idem_/))
    expect(submitImage.mock.calls[0][1]).not.toHaveProperty('response_format')
    expect(submitImage.mock.calls[0][1]).toHaveProperty('quality', 'auto')
    expect(controller.conversation.value.messages.at(-1)).toMatchObject({
      mode: 'image', status: 'completed', images: [{ src: 'https://cdn.example/image.png' }],
    })

    await nextTick()
    const restoredController = useMediaStudioController({ storage: localStorage })
    expect(restoredController.conversation.value.messages.at(-1)).toMatchObject({
      mode: 'image',
      status: 'completed',
      images: [{ src: 'https://cdn.example/image.png', url: 'https://cdn.example/image.png' }],
    })
  })

  it('runs video submit, polling, authenticated content loading, and URL cleanup', async () => {
    const createObjectURL = vi.fn().mockReturnValue('blob:video-preview')
    const revokeObjectURL = vi.fn()
    const getVideoContent = vi.fn().mockResolvedValue(new Blob(['video'], { type: 'video/mp4' }))
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'grok-imagine-video' }] }),
      submitVideo: vi.fn().mockResolvedValue({ id: 'video-1', status: 'processing', raw: {} }),
      getVideoTask: vi.fn().mockResolvedValue({ id: 'video-1', status: 'completed', raw: {} }),
      getVideoContent,
      createObjectURL,
      revokeObjectURL,
      pollDelayMs: 0,
    })

    await controller.loadMediaGroups()
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

  it('fills missing images when an upstream ignores n', async () => {
    const submitImage = vi.fn()
      .mockResolvedValueOnce({
        id: 'image-main', task_id: 'image-main', object: 'image.generation.task', status: 'completed',
        result: { data: [{ url: 'https://cdn.example/image-1.png' }] }, created_at: 1, expires_at: 2,
      })
      .mockResolvedValueOnce({
        id: 'image-fallback-2', task_id: 'image-fallback-2', object: 'image.generation.task', status: 'completed',
        result: { data: [{ url: 'https://cdn.example/image-2.png' }] }, created_at: 1, expires_at: 2,
      })
      .mockResolvedValueOnce({
        id: 'image-fallback-3', task_id: 'image-fallback-3', object: 'image.generation.task', status: 'completed',
        result: { data: [{ url: 'https://cdn.example/image-3.png' }] }, created_at: 1, expires_at: 2,
      })
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }] }),
      submitImage,
    })

    await controller.loadMediaGroups()
    controller.count.value = 3
    controller.prompt.value = 'three cats'
    await controller.submitPrompt()

    expect(submitImage).toHaveBeenCalledTimes(3)
    expect(submitImage.mock.calls[0][1].n).toBe(3)
    expect(submitImage.mock.calls[1][1].n).toBe(1)
    expect(controller.conversation.value.messages.at(-1)?.images).toHaveLength(3)
  })

  it('persists configuration without prompt input', async () => {
    const controller = useMediaStudioController({ storage: localStorage })
    expect(controller.modes.value.map(mode => [mode.id, mode.available])).toEqual([
      ['image', true], ['video', false],
    ])
    expect(controller.canSubmit.value).toBe(false)

    controller.prompt.value = 'secret prompt that must stay in memory'
    controller.count.value = 4
    await nextTick()
    const persisted = localStorage.getItem('sub2api.mediaStudio.v2') || ''
    expect(persisted).toContain('"count":4')
    expect(persisted).not.toContain('secret prompt')
    expect(persisted).not.toContain('data:image')
  })

  it('calculates image size from resolution and aspect ratio and persists custom sizes', async () => {
    const controller = useMediaStudioController({ storage: localStorage })

    controller.imageResolution.value = '2K'
    controller.imageAspectRatio.value = '16:9'
    controller.addCustomImageAspectRatio('5:4')
    await nextTick()

    expect(controller.imageAspectRatio.value).toBe('custom:5:4')
    expect(controller.customImageAspectRatios.value).toContain('5:4')
    expect(localStorage.getItem('sub2api.mediaStudio.v2')).toContain('"customImageAspectRatios":["5:4"]')
  })
})
