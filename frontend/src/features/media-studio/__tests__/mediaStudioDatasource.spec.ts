import { afterEach, describe, expect, it, vi } from 'vitest'
import {
	getVideoGenerationContent,
	getVideoGenerationTask,
	normalizeMediaStudioVideoTask,
  submitImageGeneration,
  submitVideoGeneration,
} from '@/features/media-studio/data/datasources/mediaStudioDatasource'
import {
	isMediaStudioImageModel,
	isMediaStudioVideoModel,
} from '@/features/custom-model-config/domain/services/modelCapabilityService'

function jsonResponse(body: unknown, init: ResponseInit = {}) {
  return new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: { 'Content-Type': 'application/json', ...(init.headers || {}) },
  })
}

describe('media studio datasource', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('uses the synchronous image endpoint without persisting output', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      created: 11,
      data: [{ url: 'https://cdn.example/sync.png' }],
    }))
    vi.stubGlobal('fetch', fetchMock)

    const task = await submitImageGeneration('sk-demo', {
      model: 'gpt-image-2', prompt: 'cat', response_format: 'b64_json',
    }, 'idem-sync')

    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/v1\/images\/generations$/)
    expect(fetchMock.mock.calls[0][1].headers.Authorization).toBe('Bearer sk-demo')
    expect(task.status).toBe('completed')
  })

  it('submits and polls normalized video tasks with bounded request ids', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ request_id: 'video-task_1', status: 'queued' }))
      .mockResolvedValueOnce(jsonResponse({ id: 'video-task_1', status: 'succeeded', video_url: 'https://untrusted.example/video.mp4' }))
    vi.stubGlobal('fetch', fetchMock)

    const submitted = await submitVideoGeneration('sk-demo', {
      model: 'grok-imagine-video', prompt: 'waves', resolution: '720p', duration: 6,
    }, 'video-idem')
    const completed = await getVideoGenerationTask('sk-demo', submitted.id)

    expect(submitted).toMatchObject({ id: 'video-task_1', status: 'processing' })
    expect(completed).toMatchObject({ id: 'video-task_1', status: 'completed' })
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/v1\/videos\/generations$/)
    expect(fetchMock.mock.calls[0][1].headers['Idempotency-Key']).toBe('video-idem')
    expect(String(fetchMock.mock.calls[1][0])).toMatch(/\/v1\/videos\/video-task_1$/)
    expect(() => normalizeMediaStudioVideoTask({ request_id: '../escape' })).toThrow(/invalid request id/)
  })

  it('fetches protected video content and rejects active content types', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(new Blob(['video'], { type: 'video/mp4' }), {
        headers: { 'Content-Type': 'video/mp4', 'Content-Length': '5' },
      }))
      .mockResolvedValueOnce(new Response('<script>alert(1)</script>', {
        headers: { 'Content-Type': 'text/html' },
      }))
    vi.stubGlobal('fetch', fetchMock)

    const blob = await getVideoGenerationContent('sk-demo', 'video-task_1')
    expect(blob.type).toBe('video/mp4')
    expect(fetchMock.mock.calls[0][1].headers.Authorization).toBe('Bearer sk-demo')
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/v1\/videos\/video-task_1\/content$/)

    await expect(getVideoGenerationContent('sk-demo', 'video-task_1')).rejects.toThrow(/unsafe content type/)
  })

  it('cancels unknown-length video streams as soon as they exceed the preview limit', async () => {
    const cancel = vi.fn().mockResolvedValue(undefined)
    const releaseLock = vi.fn()
    const read = vi.fn().mockResolvedValue({
      done: false,
      value: { byteLength: Number.MAX_SAFE_INTEGER },
    })
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      headers: new Headers({ 'Content-Type': 'video/mp4' }),
      body: { getReader: () => ({ read, cancel, releaseLock }) },
    } as unknown as Response)
    vi.stubGlobal('fetch', fetchMock)

    await expect(getVideoGenerationContent('sk-demo', 'video-task_1')).rejects.toThrow(/too large/)
    expect(read).toHaveBeenCalledTimes(1)
    expect(cancel).toHaveBeenCalled()
    expect(releaseLock).toHaveBeenCalled()
  })

  it('separates image and video model capabilities', () => {
    expect(isMediaStudioImageModel('gpt-image-2')).toBe(true)
    expect(isMediaStudioImageModel('grok-imagine-image')).toBe(true)
    expect(isMediaStudioVideoModel('grok-imagine-video-1.5')).toBe(true)
    expect(isMediaStudioVideoModel('gpt-image-2')).toBe(false)
  })
})
