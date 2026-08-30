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

  it('submits reference images through the image edits endpoint', async () => {
    const submitImage = vi.fn().mockResolvedValue({
      id: 'generation-1', task_id: 'generation-1', object: 'image.generation.task', status: 'completed',
      result: { data: [{ url: 'https://cdn.example/generated.png' }] }, created_at: 1, expires_at: 2,
    })
    const submitImageEdit = vi.fn().mockResolvedValue({
      id: 'edit-1', task_id: 'edit-1', object: 'image.generation.task', status: 'completed',
      result: { data: [{ url: 'https://cdn.example/edited.png' }] }, created_at: 1, expires_at: 2,
    })
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }] }),
      submitImage,
      submitImageEdit,
    })

    await controller.loadMediaGroups()
    controller.updateImageAttachments([{
      id: 'reference-1',
      file: new File(['reference'], 'reference.png', { type: 'image/png' }),
      name: 'reference.png',
      mimeType: 'image/png',
      size: 9,
      previewUrl: 'blob:reference-1',
    }])
    controller.prompt.value = 'edit the reference image'
    await controller.submitPrompt()

    expect(submitImageEdit).toHaveBeenCalledWith(
      'sk-demo',
      [expect.any(File)],
      expect.objectContaining({
        model: 'gpt-image-2',
        prompt: 'edit the reference image',
        n: 1,
      }),
      expect.stringMatching(/^media_idem_/),
    )
    expect(submitImage).not.toHaveBeenCalled()
  })

  it('retries a failed image edit with its original reference image', async () => {
    const submitImage = vi.fn()
    const submitImageEdit = vi.fn()
      .mockRejectedValueOnce(new Error('temporary edit failure'))
      .mockResolvedValueOnce({
        id: 'edit-retry-1', task_id: 'edit-retry-1', object: 'image.generation.task', status: 'completed',
        result: { data: [{ url: 'https://cdn.example/retry.png' }] }, created_at: 1, expires_at: 2,
      })
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }] }),
      submitImage,
      submitImageEdit,
    })

    await controller.loadMediaGroups()
    const file = new File(['reference'], 'reference.png', { type: 'image/png' })
    controller.updateImageAttachments([{
      id: 'reference-1',
      file,
      name: 'reference.png',
      mimeType: 'image/png',
      size: 9,
      previewUrl: 'blob:reference-1',
    }])
    controller.prompt.value = 'retry this edit'
    await controller.submitPrompt()

    const failedMessage = controller.conversation.value.messages.at(-1)
    expect(failedMessage).toMatchObject({ mode: 'image', status: 'failed' })

    await controller.retryMessage(failedMessage!)

    expect(submitImageEdit).toHaveBeenCalledTimes(2)
    expect(submitImageEdit.mock.calls[1][1]).toEqual([file])
    expect(submitImage).not.toHaveBeenCalled()
    expect(controller.conversation.value.messages.at(-1)).toMatchObject({
      mode: 'image', status: 'completed', images: [{ src: 'https://cdn.example/retry.png' }],
    })
  })

  it('renders each requested image as its independent task completes', async () => {
    let releaseSecond: ((value: unknown) => void) | undefined
    let releaseThird: ((value: unknown) => void) | undefined
    const second = new Promise(resolve => { releaseSecond = resolve })
    const third = new Promise(resolve => { releaseThird = resolve })
    const submitImage = vi.fn()
      .mockResolvedValueOnce({
        id: 'image-1', task_id: 'image-1', object: 'image.generation.task', status: 'completed',
        result: { data: [{ url: 'https://cdn.example/image-1.png' }] }, created_at: 1, expires_at: 2,
      })
      .mockReturnValueOnce(second)
      .mockReturnValueOnce(third)
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }] }),
      submitImage,
    })

    await controller.loadMediaGroups()
    controller.count.value = 3
    controller.prompt.value = 'three independent images'
    const submission = controller.submitPrompt()
    await settle()

    expect(submitImage).toHaveBeenCalledTimes(3)
    expect(submitImage.mock.calls.every(call => call[1].n === 1)).toBe(true)
    expect(controller.conversation.value.messages.at(-1)?.images).toHaveLength(1)
    expect(controller.conversation.value.messages.at(-1)?.status).toBe('processing')

    releaseSecond?.({
      id: 'image-2', task_id: 'image-2', object: 'image.generation.task', status: 'completed',
      result: { data: [{ url: 'https://cdn.example/image-2.png' }] }, created_at: 1, expires_at: 2,
    })
    releaseThird?.({
      id: 'image-3', task_id: 'image-3', object: 'image.generation.task', status: 'completed',
      result: { data: [{ url: 'https://cdn.example/image-3.png' }] }, created_at: 1, expires_at: 2,
    })
    await submission

    expect(controller.conversation.value.messages.at(-1)?.images).toHaveLength(3)
    expect(controller.conversation.value.messages.at(-1)?.status).toBe('completed')

    await nextTick()
    const restoredController = useMediaStudioController({ storage: localStorage })
    expect(restoredController.conversation.value.messages.at(-1)?.images).toHaveLength(3)
  })

  it('runs video submit, polling, authenticated content loading, and URL cleanup', async () => {
    const createObjectURL = vi.fn().mockReturnValue('blob:video-preview')
    const revokeObjectURL = vi.fn()
    const getVideoContent = vi.fn().mockResolvedValue(new Blob(['video'], { type: 'video/mp4' }))
    const createSession = vi.fn().mockResolvedValue(session())
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession,
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
    expect(createSession).toHaveBeenCalledTimes(1)
    expect(getVideoContent).toHaveBeenCalledWith('sk-demo', 'video-1')
    expect(controller.conversation.value.messages.at(-1)).toMatchObject({
      mode: 'video', status: 'completed', resolution: '1080p', duration: 15,
      video: { src: 'blob:video-preview', mimeType: 'video/mp4' },
    })

    controller.clearConversation()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:video-preview')
  })

  it('keeps successful concurrent image results when another request fails', async () => {
    const submitImage = vi.fn()
      .mockResolvedValueOnce({
        id: 'image-main', task_id: 'image-main', object: 'image.generation.task', status: 'completed',
        result: { data: [{ url: 'https://cdn.example/image-1.png' }] }, created_at: 1, expires_at: 2,
      })
      .mockRejectedValueOnce(new Error('one upstream request failed'))
      .mockResolvedValueOnce({
        id: 'image-3', task_id: 'image-3', object: 'image.generation.task', status: 'completed',
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
    expect(submitImage.mock.calls.every(call => call[1].n === 1)).toBe(true)
    expect(controller.conversation.value.messages.at(-1)).toMatchObject({
      status: 'completed',
      images: [
        { src: 'https://cdn.example/image-1.png' },
        { src: 'https://cdn.example/image-3.png' },
      ],
    })
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

    controller.imageResolution.value = '1K'
    controller.imageAspectRatio.value = '9:16'
    await nextTick()
    expect(controller.size.value).toBe('576x1024')

    controller.imageResolution.value = '2K'
    controller.imageAspectRatio.value = '16:9'
    await nextTick()
    expect(controller.size.value).toBe('2048x1152')

    controller.addCustomImageAspectRatio('5:4')
    await nextTick()

    expect(controller.imageAspectRatio.value).toBe('custom:5:4')
    expect(controller.size.value).toBe('2048x1632')
    expect(controller.customImageAspectRatios.value).toContain('5:4')
    expect(localStorage.getItem('sub2api.mediaStudio.v2')).toContain('"customImageAspectRatios":["5:4"]')
  })
})
