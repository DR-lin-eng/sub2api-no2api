import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  getAsyncImageTask,
  isMediaStudioImageModel,
  listMediaStudioModels,
  submitAsyncImageGeneration,
  submitImageGeneration,
} from '@/features/media-studio/data/datasources/mediaStudioDatasource'

function jsonResponse(body: unknown, init: ResponseInit = {}) {
  return new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: {
      'Content-Type': 'application/json',
      ...(init.headers || {}),
    },
  })
}

describe('media studio datasource', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('calls model and async task endpoints with the selected API key', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ object: 'list', data: [{ id: 'gpt-image-2' }] }))
      .mockResolvedValueOnce(jsonResponse({ id: 'imgtask_1', task_id: 'imgtask_1', object: 'image.generation.task', status: 'processing', created_at: 1, expires_at: 2 }))
      .mockResolvedValueOnce(jsonResponse({ id: 'imgtask_1', task_id: 'imgtask_1', object: 'image.generation.task', status: 'completed', created_at: 1, expires_at: 2, result: { data: [{ url: 'https://cdn.example/image.png' }] } }))
    vi.stubGlobal('fetch', fetchMock)

    await listMediaStudioModels('sk-demo')
    await submitAsyncImageGeneration('sk-demo', { model: 'gpt-image-2', prompt: 'cat' }, 'idem-1')
    await getAsyncImageTask('sk-demo', 'imgtask_1')

    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/v1\/models$/)
    expect(fetchMock.mock.calls[0][1].headers.Authorization).toBe('Bearer sk-demo')
    expect(String(fetchMock.mock.calls[1][0])).toMatch(/\/v1\/images\/generations\/async$/)
    expect(fetchMock.mock.calls[1][1].headers['Idempotency-Key']).toBe('idem-1')
    expect(String(fetchMock.mock.calls[2][0])).toMatch(/\/v1\/images\/tasks\/imgtask_1$/)
  })

  it('uses synchronous generation as the local usable path', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      created: 11,
      data: [{ url: 'https://cdn.example/sync.png' }],
    }))
    vi.stubGlobal('fetch', fetchMock)

    const task = await submitImageGeneration('sk-demo', { model: 'gpt-image-2', prompt: 'cat', response_format: 'b64_json' }, 'idem-sync')

    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/v1\/images\/generations$/)
    expect(task.status).toBe('completed')
    expect(task.image_url).toBe('https://cdn.example/sync.png')
    expect(task.result?.data?.[0]?.url).toBe('https://cdn.example/sync.png')
  })

  it('parses OpenAI-style errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({
      error: { type: 'not_found_error', code: 'not_found_error', message: 'async image tasks are not enabled' },
    }, { status: 404, headers: { 'X-Request-Id': 'req_1' } })))

    await expect(submitAsyncImageGeneration('sk-demo', { model: 'gpt-image-2', prompt: 'cat' }, 'idem-1'))
      .rejects
      .toMatchObject({ status: 404, code: 'not_found_error', message: 'async image tasks are not enabled', requestId: 'req_1' })
  })

  it('filters image-capable model names', () => {
    expect(isMediaStudioImageModel('gpt-image-2')).toBe(true)
    expect(isMediaStudioImageModel('grok-imagine-image-quality')).toBe(true)
    expect(isMediaStudioImageModel('claude-sonnet')).toBe(false)
  })
})
