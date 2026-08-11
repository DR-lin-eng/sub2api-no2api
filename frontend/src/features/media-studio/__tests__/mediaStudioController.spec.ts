import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useMediaStudioController } from '@/features/media-studio/presentation/composables/useMediaStudioController'
import type { ApiKey, PaginatedResponse } from '@/types'

function page(items: ApiKey[]): PaginatedResponse<ApiKey> {
  return {
    items,
    total: items.length,
    page: 1,
    page_size: 100,
    pages: 1,
  }
}

function key(overrides: Partial<ApiKey> = {}): ApiKey {
  return {
    id: 1,
    user_id: 1,
    key: 'sk-demo',
    name: 'demo',
    group_id: null,
    status: 'active',
    ip_whitelist: [],
    ip_blacklist: [],
    last_used_at: null,
    last_used_ip: null,
    quota: 0,
    quota_used: 0,
    expires_at: null,
    created_at: '',
    updated_at: '',
    concurrency_limit: 0,
    current_concurrency: 0,
    rate_limit_5h: 0,
    rate_limit_1d: 0,
    rate_limit_7d: 0,
    usage_5h: 0,
    usage_1d: 0,
    usage_7d: 0,
    window_5h_start: null,
    window_1d_start: null,
    window_7d_start: null,
    reset_5h_at: null,
    reset_1d_at: null,
    reset_7d_at: null,
    ...overrides,
  }
}

describe('media studio controller', () => {
  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('loads keys, filters image models, and submits a completed image message', async () => {
    const submitImage = vi.fn().mockResolvedValue({
      id: 'imgsync_1',
      task_id: 'imgsync_1',
      object: 'image.generation.task',
      status: 'completed',
      image_url: 'https://cdn.example/cat.png',
      result: { data: [{ url: 'https://cdn.example/cat.png' }] },
      created_at: 1,
      completed_at: 2,
      expires_at: 3,
    })
    const controller = useMediaStudioController({
      storage: localStorage,
      listKeys: vi.fn().mockResolvedValue(page([key()])),
      listModels: vi.fn().mockResolvedValue({
        object: 'list',
        data: [{ id: 'claude-sonnet' }, { id: 'gpt-image-2' }, { id: 'grok-imagine-image' }],
      }),
      submitImage,
      pollDelayMs: 0,
    })

    await controller.loadApiKeys()
    controller.prompt.value = 'a quiet cat'
    await controller.submitPrompt()
    await nextTick()

    expect(controller.selectedApiKeyId.value).toBe(1)
    expect(controller.modelOptions.value).toEqual(['gpt-image-2', 'grok-imagine-image'])
    expect(submitImage).toHaveBeenCalledWith(
      'sk-demo',
      expect.objectContaining({ model: 'gpt-image-2', prompt: 'a quiet cat', response_format: 'b64_json' }),
      expect.stringMatching(/^media_idem_/),
    )
    expect(controller.conversation.value.messages).toHaveLength(2)
    expect(controller.conversation.value.messages[1]).toMatchObject({
      role: 'assistant',
      status: 'completed',
      images: [{ src: 'https://cdn.example/cat.png' }],
    })
  })

  it('persists local conversation state', async () => {
    const controller = useMediaStudioController({
      storage: localStorage,
      listKeys: vi.fn().mockResolvedValue(page([key()])),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }] }),
      submitImage: vi.fn().mockResolvedValue({
        id: 'imgsync_2',
        task_id: 'imgsync_2',
        object: 'image.generation.task',
        status: 'completed',
        result: { data: [{ url: 'https://cdn.example/dog.png' }] },
        created_at: 1,
        completed_at: 2,
        expires_at: 3,
      }),
      pollDelayMs: 0,
    })

    await controller.loadApiKeys()
    controller.prompt.value = 'a dog'
    await controller.submitPrompt()
    await nextTick()

    const restored = useMediaStudioController({ storage: localStorage })
    expect(restored.conversation.value.messages.at(-1)?.images?.[0]?.src).toBe('https://cdn.example/dog.png')
  })

  it('polls async processing tasks into completed state when injected', async () => {
    const controller = useMediaStudioController({
      storage: localStorage,
      listKeys: vi.fn().mockResolvedValue(page([key()])),
      listModels: vi.fn().mockResolvedValue({ object: 'list', data: [{ id: 'gpt-image-2' }] }),
      submitImage: vi.fn().mockResolvedValue({
        id: 'imgtask_1',
        task_id: 'imgtask_1',
        object: 'image.generation.task',
        status: 'processing',
        created_at: 1,
        expires_at: 3,
      }),
      getTask: vi.fn().mockResolvedValue({
        id: 'imgtask_1',
        task_id: 'imgtask_1',
        object: 'image.generation.task',
        status: 'completed',
        result: { data: [{ url: 'https://cdn.example/async.png' }] },
        created_at: 1,
        completed_at: 2,
        expires_at: 3,
      }),
      pollDelayMs: 0,
    })

    await controller.loadApiKeys()
    controller.prompt.value = 'async cat'
    await controller.submitPrompt()
    await new Promise(resolve => setTimeout(resolve, 0))
    await nextTick()

    expect(controller.conversation.value.messages.at(-1)).toMatchObject({
      status: 'completed',
      images: [{ src: 'https://cdn.example/async.png' }],
    })
  })
})
