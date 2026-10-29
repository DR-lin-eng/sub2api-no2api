import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useMediaStudioController } from '@/features/media-studio/presentation/composables/useMediaStudioController'
import type {
  MediaStudioConfig,
  MediaStudioSession,
} from '@/features/media-studio/data/datasources/mediaStudioDatasource'

// 预估消耗只能由后端计价接口给出，测试里替换掉默认实现，避免防抖预估计时器发起真实请求。
const { estimateMediaStudioCostMock } = vi.hoisted(() => ({
  estimateMediaStudioCostMock: vi.fn(),
}))

vi.mock('@/features/media-studio/data/datasources/mediaStudioDatasource', async (importOriginal) => {
  const actual = await importOriginal<
    typeof import('@/features/media-studio/data/datasources/mediaStudioDatasource')
  >()
  return { ...actual, estimateMediaStudioCost: estimateMediaStudioCostMock }
})

const config: MediaStudioConfig = {
  groups: [
    { group_id: 1, group_name: 'Media', platform: 'openai', media_type: 'image', models: ['gpt-image-2'] },
    { group_id: 2, group_name: 'Video', platform: 'openai', media_type: 'video', models: ['grok-imagine-video'] },
  ],
}

function session(apiKey = 'sk-demo'): MediaStudioSession {
  return { api_key: apiKey, group_id: 1, media_type: 'image' }
}

async function settle() {
  await new Promise(resolve => setTimeout(resolve, 0))
  await nextTick()
}

describe('media studio controller', () => {
  beforeEach(() => {
    estimateMediaStudioCostMock.mockClear()
    estimateMediaStudioCostMock.mockResolvedValue({
      amount: 0.12,
      currency: 'USD',
      billing_mode: 'image',
      model: 'gpt-image-2',
    })
  })

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

    expect(controller.modelOptions.value.map(option => option.id)).toEqual(['gpt-image-2'])
    expect(controller.modelOptions.value[0].fields).toEqual([])
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
    const submitVideo = vi.fn().mockResolvedValue({ id: 'video-1', status: 'processing', raw: {} })
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession,
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'grok-imagine-video' }] }),
      submitVideo,
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
    controller.videoAspectRatio.value = '9:16'
    controller.duration.value = 15
    controller.prompt.value = 'slow camera move'
    await controller.submitPrompt()
    await settle()

    expect(controller.modelOptions.value.map(option => option.id)).toEqual(['grok-imagine-video'])
    expect(createSession).toHaveBeenLastCalledWith('video', 2)
    expect(submitVideo).toHaveBeenCalledWith('sk-demo', expect.objectContaining({ aspect_ratio: '9:16' }), expect.any(String))
    expect(getVideoContent).toHaveBeenCalledWith('sk-demo', 'video-1')
    expect(controller.conversation.value.messages.at(-1)).toMatchObject({
      mode: 'video', status: 'completed', resolution: '1080p', aspectRatio: '9:16', duration: 15,
      video: { src: 'blob:video-preview', mimeType: 'video/mp4' },
    })

    controller.clearConversation()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:video-preview')
  })

  it('renders video reference attachments on the sent message and submits them', async () => {
    const submitVideo = vi.fn().mockResolvedValue({ id: 'video-1', status: 'processing', raw: {} })
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'grok-imagine-video' }] }),
      submitVideo,
      getVideoTask: vi.fn().mockResolvedValue({ id: 'video-1', status: 'completed', raw: {} }),
      getVideoContent: vi.fn().mockResolvedValue(new Blob(['video'], { type: 'video/mp4' })),
      createObjectURL: vi.fn().mockReturnValue('blob:video-preview'),
      revokeObjectURL: vi.fn(),
      pollDelayMs: 0,
    })

    await controller.loadMediaGroups()
    controller.selectMode('video')
    await settle()
    controller.updateReferenceAttachments([
      {
        id: 'reference-image',
        file: new File(['image'], 'reference.png', { type: 'image/png' }),
        name: 'reference.png',
        kind: 'image',
        mimeType: 'image/png',
        size: 5,
        previewUrl: 'blob:reference-image',
        url: 'https://cdn.example/reference.png',
        status: 'ready',
        error: '',
      },
      {
        id: 'reference-video',
        file: new File(['video'], 'reference.mp4', { type: 'video/mp4' }),
        name: 'reference.mp4',
        kind: 'video',
        mimeType: 'video/mp4',
        size: 5,
        previewUrl: 'blob:reference-video',
        url: 'https://cdn.example/reference.mp4',
        status: 'ready',
        error: '',
      },
    ])
    controller.prompt.value = 'animate the reference'
    await controller.submitPrompt()
    await settle()

    expect(submitVideo).toHaveBeenCalledWith('sk-demo', expect.objectContaining({
      images: ['https://cdn.example/reference.png'],
      videos: ['https://cdn.example/reference.mp4'],
    }), expect.any(String))
    expect(controller.conversation.value.messages.at(-2)).toMatchObject({
      role: 'user',
      mode: 'video',
      inputReferences: [
        { url: 'https://cdn.example/reference.png', kind: 'image' },
        { url: 'https://cdn.example/reference.mp4', kind: 'video' },
      ],
    })
    expect(controller.referenceAttachments.value).toHaveLength(0)
  })

  it('retries a failed video task with its original reference attachments', async () => {
    const submitVideo = vi.fn()
      .mockRejectedValueOnce(new Error('temporary video failure'))
      .mockResolvedValueOnce({ id: 'video-2', status: 'processing', raw: {} })
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'grok-imagine-video' }] }),
      submitVideo,
      getVideoTask: vi.fn().mockResolvedValue({ id: 'video-2', status: 'completed', raw: {} }),
      getVideoContent: vi.fn().mockResolvedValue(new Blob(['video'], { type: 'video/mp4' })),
      createObjectURL: vi.fn().mockReturnValue('blob:video-preview'),
      revokeObjectURL: vi.fn(),
      pollDelayMs: 0,
    })

    await controller.loadMediaGroups()
    controller.selectMode('video')
    await settle()
    controller.updateReferenceAttachments([{
      id: 'reference-image',
      file: new File(['image'], 'reference.png', { type: 'image/png' }),
      name: 'reference.png',
      kind: 'image',
      mimeType: 'image/png',
      size: 5,
      previewUrl: 'blob:reference-image',
      url: 'https://cdn.example/reference.png',
      status: 'ready',
      error: '',
    }])
    controller.prompt.value = 'animate the reference'
    await controller.submitPrompt()

    const failedMessage = controller.conversation.value.messages.at(-1)
    expect(failedMessage).toMatchObject({ role: 'assistant', mode: 'video', status: 'failed' })

    await controller.retryMessage(failedMessage!)
    await settle()

    expect(submitVideo).toHaveBeenCalledTimes(2)
    expect(submitVideo.mock.calls[1][1]).toMatchObject({
      images: ['https://cdn.example/reference.png'],
    })
    expect(controller.conversation.value.messages.at(-2)).toMatchObject({
      role: 'user',
      inputReferences: [{ url: 'https://cdn.example/reference.png' }],
    })
  })

  it('tolerates transient 404 responses while polling a video task', async () => {
    const notFound = () => Object.assign(new Error('task not found'), { status: 404 })
    const getVideoTask = vi.fn()
      .mockRejectedValueOnce(notFound())
      .mockRejectedValueOnce(notFound())
      .mockRejectedValueOnce(notFound())
      .mockRejectedValueOnce(notFound())
      .mockResolvedValue({ id: 'video-1', status: 'completed', raw: {} })
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'grok-imagine-video' }] }),
      submitVideo: vi.fn().mockResolvedValue({ id: 'video-1', status: 'processing', raw: {} }),
      getVideoTask,
      getVideoContent: vi.fn().mockResolvedValue(new Blob(['video'], { type: 'video/mp4' })),
      createObjectURL: vi.fn().mockReturnValue('blob:video-preview'),
      revokeObjectURL: vi.fn(),
      pollDelayMs: 0,
    })

    await controller.loadMediaGroups()
    controller.selectMode('video')
    await settle()
    controller.prompt.value = 'slow camera move'
    await controller.submitPrompt()
    await settle()

    expect(getVideoTask).toHaveBeenCalledTimes(5)
    expect(controller.conversation.value.messages.at(-1)).toMatchObject({ status: 'completed' })
  })

  it('fails the video task after five consecutive 404 responses', async () => {
    const getVideoTask = vi.fn().mockRejectedValue(Object.assign(new Error('task not found'), { status: 404 }))
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'grok-imagine-video' }] }),
      submitVideo: vi.fn().mockResolvedValue({ id: 'video-1', status: 'processing', raw: {} }),
      getVideoTask,
      pollDelayMs: 0,
    })

    await controller.loadMediaGroups()
    controller.selectMode('video')
    await settle()
    controller.prompt.value = 'slow camera move'
    await controller.submitPrompt()
    await settle()

    expect(getVideoTask).toHaveBeenCalledTimes(5)
    expect(controller.conversation.value.messages.at(-1)).toMatchObject({
      status: 'failed',
      error: 'task not found',
    })
  })

  it('records video progress while polling', async () => {
    let resolveSecond: ((task: { id: string; status: 'completed'; raw: Record<string, unknown> }) => void) | undefined
    const secondPoll = new Promise<{ id: string; status: 'completed'; raw: Record<string, unknown> }>((resolve) => {
      resolveSecond = resolve
    })
    const getVideoTask = vi.fn()
      .mockResolvedValueOnce({ id: 'video-1', status: 'processing', progress: 42, raw: {} })
      .mockImplementationOnce(() => secondPoll)
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'grok-imagine-video' }] }),
      submitVideo: vi.fn().mockResolvedValue({ id: 'video-1', status: 'processing', raw: {} }),
      getVideoTask,
      getVideoContent: vi.fn().mockResolvedValue(new Blob(['video'], { type: 'video/mp4' })),
      createObjectURL: vi.fn().mockReturnValue('blob:video-preview'),
      revokeObjectURL: vi.fn(),
      pollDelayMs: 0,
    })

    await controller.loadMediaGroups()
    controller.selectMode('video')
    await settle()
    controller.prompt.value = 'slow camera move'
    await controller.submitPrompt()

    await vi.waitFor(() => {
      expect(controller.conversation.value.messages.at(-1)?.progress).toBe(42)
    })

    resolveSecond?.({ id: 'video-1', status: 'completed', raw: {} })
    await vi.waitFor(() => {
      expect(controller.conversation.value.messages.at(-1)?.status).toBe('completed')
    })
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

  it('reads the estimated cost from the backend billing endpoint', async () => {
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }] }),
    })

    await controller.loadMediaGroups()
    await controller.refreshCostEstimate()

    // 预估请求只携带计价上下文，价格本身由后端通用计价链路给出。
    expect(estimateMediaStudioCostMock).toHaveBeenCalledWith({
      media_type: 'image',
      group_id: 1,
      model: 'gpt-image-2',
      size: '1024x1024',
      count: 1,
    })
    expect(controller.estimatedCost.value).toBe(0.12)
    expect(controller.estimatedCurrency.value).toBe('USD')
    expect(controller.estimateError.value).toBe('')
  })

  it('surfaces estimate failures instead of inventing a local price', async () => {
    estimateMediaStudioCostMock.mockRejectedValueOnce(new Error('video price is not configured'))
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }] }),
    })

    await controller.loadMediaGroups()
    await controller.refreshCostEstimate()

    expect(controller.estimatedCost.value).toBeNull()
    expect(controller.estimateError.value).toContain('video price is not configured')
  })

  it('persists configuration without prompt input', async () => {
    const controller = useMediaStudioController({ storage: localStorage })
    // Modes stay unavailable until group config loads, because each group serves one modality.
    expect(controller.modes.value.map(mode => [mode.id, mode.available])).toEqual([
      ['image', false], ['video', false],
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

  function seedPendingImageTask(status: 'processing' | 'queued' = 'processing') {
    localStorage.setItem('sub2api.mediaStudio.v2', JSON.stringify({
      selectedGroupId: 1,
      conversation: {
        id: 'conversation-resume',
        updatedAt: Date.now(),
        messages: [{
          id: 'assistant-resume',
          role: 'assistant',
          mode: 'image',
          prompt: 'a resumed cat',
          status,
          groupId: 1,
          model: 'gpt-image-2',
          count: 1,
          taskId: 'task-resume',
          taskIds: ['task-resume'],
          createdAt: Date.now() - 5_000,
        }],
      },
    }))
  }

  function resumedImageTask() {
    return {
      id: 'task-resume', task_id: 'task-resume', object: 'image.generation.task', status: 'completed',
      result: { data: [{ url: 'https://cdn.example/resumed.png' }] }, created_at: 1, expires_at: 2,
    }
  }

  it('resumes a pending image task after reload and polls it to completion', async () => {
    seedPendingImageTask()
    const getTask = vi.fn().mockResolvedValue(resumedImageTask())
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }] }),
      getTask,
      pollDelayMs: 0,
    })

    await controller.loadMediaGroups()
    await vi.waitFor(() => {
      expect(controller.conversation.value.messages.at(-1)?.status).toBe('completed')
    })

    expect(getTask).toHaveBeenCalledWith('sk-demo', 'task-resume')
    expect(controller.conversation.value.messages.at(-1)?.images).toEqual([
      expect.objectContaining({ src: 'https://cdn.example/resumed.png' }),
    ])
    expect(controller.jobs.value.at(-1)).toMatchObject({
      messageId: 'assistant-resume',
      status: 'completed',
    })
  })

  it('persists the async image task id before polling so a reload can resume', async () => {
    let resolvePoll: ((value: unknown) => void) | undefined
    const submitImage = vi.fn().mockResolvedValue({
      id: 'task-async-1', task_id: 'task-async-1', object: 'image.generation.task', status: 'processing',
      created_at: 1, expires_at: 2,
    })
    const getTask = vi.fn().mockImplementation(() => new Promise(resolve => { resolvePoll = resolve }))
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }] }),
      submitImage,
      getTask,
      pollDelayMs: 0,
    })

    await controller.loadMediaGroups()
    controller.prompt.value = 'async cat'
    const submission = controller.submitPrompt()
    await settle()

    // 提交成功即落盘任务 ID，随后刷新页面才能从 localStorage 续轮询。
    expect(getTask).toHaveBeenCalledWith('sk-demo', 'task-async-1')
    const persisted = JSON.parse(localStorage.getItem('sub2api.mediaStudio.v2') || '{}')
    expect(persisted.conversation?.messages?.at(-1)).toMatchObject({
      status: 'processing',
      taskIds: ['task-async-1'],
    })

    resolvePoll?.({
      id: 'task-async-1', task_id: 'task-async-1', object: 'image.generation.task', status: 'completed',
      result: { data: [{ url: 'https://cdn.example/async.png' }] }, created_at: 1, expires_at: 2,
    })
    await submission

    expect(controller.conversation.value.messages.at(-1)).toMatchObject({
      status: 'completed',
      images: [{ src: 'https://cdn.example/async.png' }],
    })
  })

  it('falls back to the synchronous image endpoint when async tasks are disabled', async () => {
    const submitImage = vi.fn().mockRejectedValue(
      Object.assign(new Error('async image tasks are not enabled'), { status: 404 }),
    )
    const submitImageSync = vi.fn().mockResolvedValue({
      id: 'imgsync_demo', task_id: 'imgsync_demo', object: 'image.generation.task', status: 'completed',
      result: { data: [{ url: 'https://cdn.example/sync.png' }] }, created_at: 1, expires_at: 2,
    })
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }] }),
      submitImage,
      submitImageSync,
      pollDelayMs: 0,
    })

    await controller.loadMediaGroups()
    controller.prompt.value = 'sync cat'
    await controller.submitPrompt()

    expect(submitImageSync).toHaveBeenCalledWith(
      'sk-demo',
      expect.objectContaining({ model: 'gpt-image-2', prompt: 'sync cat' }),
      expect.stringMatching(/^media_idem_/),
    )
    expect(controller.conversation.value.messages.at(-1)).toMatchObject({
      status: 'completed',
      images: [{ src: 'https://cdn.example/sync.png' }],
    })
  })

  it('falls back to the synchronous edit endpoint when async edits are disabled', async () => {
    const submitImageEdit = vi.fn().mockRejectedValue(
      Object.assign(new Error('async image tasks are not enabled'), { status: 404 }),
    )
    const submitImageEditSync = vi.fn().mockResolvedValue({
      id: 'imgedit_demo', task_id: 'imgedit_demo', object: 'image.generation.task', status: 'completed',
      result: { data: [{ url: 'https://cdn.example/sync-edit.png' }] }, created_at: 1, expires_at: 2,
    })
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }] }),
      submitImageEdit,
      submitImageEditSync,
      pollDelayMs: 0,
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
    controller.prompt.value = 'sync edit'
    await controller.submitPrompt()

    expect(submitImageEditSync).toHaveBeenCalledWith(
      'sk-demo',
      [file],
      expect.objectContaining({ model: 'gpt-image-2', prompt: 'sync edit', n: 1 }),
      expect.stringMatching(/^media_idem_/),
    )
    expect(controller.conversation.value.messages.at(-1)).toMatchObject({
      status: 'completed',
      images: [{ src: 'https://cdn.example/sync-edit.png' }],
    })
  })

  it('classifies browser network failures without leaking the raw message', async () => {
    const submitImage = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'))
    const controller = useMediaStudioController({
      storage: localStorage,
      getConfig: vi.fn().mockResolvedValue(config),
      createSession: vi.fn().mockResolvedValue(session()),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }] }),
      submitImage,
      pollDelayMs: 0,
    })

    await controller.loadMediaGroups()
    controller.prompt.value = 'offline cat'
    await controller.submitPrompt()

    expect(controller.conversation.value.messages.at(-1)).toMatchObject({
      status: 'failed',
      error: '',
      errorCode: 'network',
    })
    expect(controller.submitError.value).toBe('')
  })
})
