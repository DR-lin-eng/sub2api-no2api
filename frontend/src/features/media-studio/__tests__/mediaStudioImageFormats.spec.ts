import { describe, expect, it } from 'vitest'
import { extractImages, persistedImageSource } from '../presentation/composables/mediaStudioConversation'
import type { MediaStudioImageTask } from '../data/datasources/mediaStudioDatasource'

describe('media studio image formats', () => {
  it.each([
    ['gif', 'image/gif'], ['svg', 'image/svg+xml'], ['webp', 'image/webp'], ['jpeg', 'image/jpeg'],
  ])('previews and persists %s results with their actual MIME type', (format, mimeType) => {
    const task: MediaStudioImageTask = {
      id: 'image-26', task_id: 'image-26', object: 'image.generation.task', status: 'completed', created_at: 1, expires_at: 2,
      result: { output_format: format, data: [{ b64_json: 'AAAA' }] },
    }
    expect(extractImages(task)[0].src).toBe(`data:${mimeType};base64,AAAA`)
    expect(persistedImageSource(extractImages(task)[0].src)).toBe(`data:${mimeType};base64,AAAA`)
    task.result = { data: [{ url: `data:${mimeType};base64,AAAA` }] }
    expect(extractImages(task)[0].src).toBe(`data:${mimeType};base64,AAAA`)
  })

  it('rejects active document URLs and malformed base64', () => {
    expect(persistedImageSource('data:text/html;base64,AAAA')).toBe('')
    expect(persistedImageSource('data:image/svg+xml;base64,<svg/>')).toBe('')
  })
})
