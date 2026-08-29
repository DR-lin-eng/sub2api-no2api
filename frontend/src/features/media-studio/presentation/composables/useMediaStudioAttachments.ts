export interface MediaStudioImageAttachment {
  id: string
  file: File
  name: string
  mimeType: string
  size: number
  previewUrl: string
}

export const MEDIA_STUDIO_MAX_IMAGE_ATTACHMENTS = 9
export const MEDIA_STUDIO_MAX_IMAGE_FILE_BYTES = 20 * 1024 * 1024
export const MEDIA_STUDIO_MAX_IMAGE_TOTAL_BYTES = 80 * 1024 * 1024

function attachmentID(file: File): string {
  return `${file.name}:${file.size}:${file.lastModified}:${file.type}`
}

function attachmentPreviewURL(file: File): string {
  const createObjectURL = globalThis.URL?.createObjectURL
  return typeof createObjectURL === 'function' ? createObjectURL.call(globalThis.URL, file) : ''
}

export function addMediaStudioImageAttachments(
  current: MediaStudioImageAttachment[],
  files: File[],
): { attachments: MediaStudioImageAttachment[]; rejected: string[] } {
  const next = [...current]
  const rejected: string[] = []
  const seen = new Set(next.map((attachment) => attachment.id))

  for (const file of files) {
    const id = attachmentID(file)
    if (seen.has(id)) continue
    if (!file.type.startsWith('image/')) {
      rejected.push(`${file.name}: unsupported image type`)
      continue
    }
    if (file.size > MEDIA_STUDIO_MAX_IMAGE_FILE_BYTES) {
      rejected.push(`${file.name}: file is too large`)
      continue
    }
    if (next.length >= MEDIA_STUDIO_MAX_IMAGE_ATTACHMENTS) {
      rejected.push(`${file.name}: maximum ${MEDIA_STUDIO_MAX_IMAGE_ATTACHMENTS} images`)
      continue
    }
    const currentBytes = next.reduce((total, attachment) => total + attachment.size, 0)
    if (currentBytes + file.size > MEDIA_STUDIO_MAX_IMAGE_TOTAL_BYTES) {
      rejected.push(`${file.name}: total image size limit exceeded`)
      continue
    }
    seen.add(id)
    next.push({
      id,
      file,
      name: file.name || 'pasted-image',
      mimeType: file.type,
      size: file.size,
      previewUrl: attachmentPreviewURL(file),
    })
  }

  return { attachments: next, rejected }
}

export function revokeMediaStudioImageAttachments(attachments: MediaStudioImageAttachment[]): void {
  const revokeObjectURL = globalThis.URL?.revokeObjectURL
  if (typeof revokeObjectURL !== 'function') return
  for (const attachment of attachments) {
    if (attachment.previewUrl.startsWith('blob:')) {
      revokeObjectURL.call(globalThis.URL, attachment.previewUrl)
    }
  }
}
