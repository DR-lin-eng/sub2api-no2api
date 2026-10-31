export const MAX_MEDIA_STUDIO_REFERENCE_BYTES = 20 * 1024 * 1024
export const MAX_MEDIA_STUDIO_REFERENCE_TOTAL_BYTES = 32 * 1024 * 1024
const MEDIA_TYPES = /^(?:image\/(?:png|jpeg|webp|gif|avif)|video\/(?:mp4|webm|quicktime|ogg)|audio\/(?:mpeg|mp3|wav|x-wav|mp4|ogg|webm|aac|flac))$/i

export function mediaStudioReferenceTypeAllowed(mimeType: string): boolean {
  return MEDIA_TYPES.test(mimeType)
}

// Conversion happens only in the browser; references are sent with the generation request.
export function mediaStudioBlobDataURL(blob: Blob): Promise<string> {
  if (!blob.size || blob.size > MAX_MEDIA_STUDIO_REFERENCE_BYTES || !mediaStudioReferenceTypeAllowed(blob.type)) {
    return Promise.reject(new Error('unsupported reference file or reference file exceeds 20 MB'))
  }
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => typeof reader.result === 'string' ? resolve(reader.result) : reject(new Error('failed to read reference file'))
    reader.onerror = () => reject(new Error('failed to read reference file'))
    reader.onabort = () => reject(new Error('reference file read cancelled'))
    reader.readAsDataURL(blob)
  })
}

export function safeMediaStudioReferenceSource(value: unknown): string {
  if (typeof value !== 'string' || !value) return ''
  if (value.startsWith('data:')) {
    const separator = value.indexOf(',')
    if (separator < 0 || separator > 128) return ''
    const prefix = value.slice(5, separator)
    if (!prefix.endsWith(';base64') || !mediaStudioReferenceTypeAllowed(prefix.slice(0, -7))) return ''
    const body = value.slice(separator + 1)
    if (!body || body.length > Math.ceil(MAX_MEDIA_STUDIO_REFERENCE_BYTES / 3) * 4 || !/^[A-Za-z0-9+/]+={0,2}$/.test(body)) return ''
    return value
  }
  if (value.length > 8_192) return ''
  try {
    const parsed = new URL(value, typeof window === 'undefined' ? 'https://localhost/' : window.location.origin)
    return parsed.protocol === 'https:' || parsed.protocol === 'http:' ? parsed.toString() : ''
  } catch { return '' }
}
