import {
  MAX_MEDIA_STUDIO_REFERENCE_BYTES,
  MAX_MEDIA_STUDIO_REFERENCE_TOTAL_BYTES,
  mediaStudioReferenceTypeAllowed,
} from '@/features/media-studio/data/datasources/mediaStudioBrowserMedia'

import type { MediaStudioMediaType } from '@/features/media-studio/data/datasources/mediaStudioDatasource'

export interface MediaStudioImageAttachment {
  id: string
  file: File
  name: string
  mimeType: string
  size: number
  previewUrl: string
}

// Reference files stay in browser memory/IndexedDB until the generation request.
export interface MediaStudioReferenceAttachment {
  id: string
  file?: File
  name: string
  kind: MediaStudioMediaType
  mimeType: string
  size: number
  previewUrl: string
  url: string
  cacheKey?: string
  status: 'uploading' | 'ready' | 'failed'
  error: string
}

export type MediaStudioAttachmentRejectionReason = 'type' | 'size' | 'count' | 'total'

// 拒绝原因交给调用方翻译，避免在数据层硬编码用户可见文案。
export interface MediaStudioAttachmentRejection {
  name: string
  reason: MediaStudioAttachmentRejectionReason
  limit: number
}

export const MEDIA_STUDIO_MAX_IMAGE_ATTACHMENTS = 9
export const MEDIA_STUDIO_MAX_IMAGE_FILE_BYTES = 20 * 1024 * 1024
export const MEDIA_STUDIO_MAX_IMAGE_TOTAL_BYTES = 80 * 1024 * 1024

export interface MediaStudioImageAttachmentLimits {
  maxFiles: number
  maxFileSizeBytes: number
  maxTotalBytes: number
}

export const MEDIA_STUDIO_DEFAULT_IMAGE_ATTACHMENT_LIMITS: MediaStudioImageAttachmentLimits = {
  maxFiles: MEDIA_STUDIO_MAX_IMAGE_ATTACHMENTS,
  maxFileSizeBytes: MEDIA_STUDIO_MAX_IMAGE_FILE_BYTES,
  maxTotalBytes: MEDIA_STUDIO_MAX_IMAGE_TOTAL_BYTES,
}

export interface MediaStudioReferenceLimits {
  enabled: MediaStudioMediaType[]
  maxFiles: Record<MediaStudioMediaType, number>
  maxFileSizeMB: Record<MediaStudioMediaType, number>
}

function attachmentID(file: File): string {
  return `${file.name}:${file.size}:${file.lastModified}:${file.type}`
}

function attachmentPreviewURL(file: File): string {
  const createObjectURL = globalThis.URL?.createObjectURL
  return typeof createObjectURL === 'function' ? createObjectURL.call(globalThis.URL, file) : ''
}

export async function addMediaStudioImageAttachments(
  current: MediaStudioImageAttachment[],
  files: File[],
  limits: MediaStudioImageAttachmentLimits = MEDIA_STUDIO_DEFAULT_IMAGE_ATTACHMENT_LIMITS,
): Promise<{ attachments: MediaStudioImageAttachment[]; rejected: MediaStudioAttachmentRejection[] }> {
  const next = [...current]
  const rejected: MediaStudioAttachmentRejection[] = []
  const seen = new Set(next.map(attachment => attachment.id))
  let totalBytes = next.reduce((total, attachment) => total + attachment.size, 0)
  for (const file of files) {
    const id = attachmentID(file)
    if (seen.has(id)) continue
    if (!file.type.startsWith('image/') || !mediaStudioReferenceTypeAllowed(file.type)) {
      rejected.push({ name: file.name, reason: 'type', limit: 0 }); continue
    }
    const maxSize = Math.min(limits.maxFileSizeBytes, MEDIA_STUDIO_MAX_IMAGE_FILE_BYTES)
    if (file.size > maxSize) {
      rejected.push({ name: file.name, reason: 'size', limit: Math.round(maxSize / (1024 * 1024)) }); continue
    }
    const maxFiles = Math.min(limits.maxFiles, MEDIA_STUDIO_MAX_IMAGE_ATTACHMENTS)
    if (next.length >= maxFiles) {
      rejected.push({ name: file.name, reason: 'count', limit: maxFiles }); continue
    }
    const maxTotal = Math.min(limits.maxTotalBytes, MEDIA_STUDIO_MAX_IMAGE_TOTAL_BYTES)
    if (totalBytes + file.size > maxTotal) {
      rejected.push({ name: file.name, reason: 'total', limit: Math.round(maxTotal / (1024 * 1024)) }); continue
    }
    seen.add(id)
    totalBytes += file.size
    next.push({ id, file, name: file.name || 'pasted-image', mimeType: file.type, size: file.size, previewUrl: attachmentPreviewURL(file) })
  }
  return { attachments: next, rejected }
}

export async function addMediaStudioReferenceAttachments(
  current: MediaStudioReferenceAttachment[],
  files: File[],
  limits: MediaStudioReferenceLimits,
  kindOf: (file: File) => MediaStudioMediaType | '',
): Promise<{ attachments: MediaStudioReferenceAttachment[]; rejected: MediaStudioAttachmentRejection[] }> {
  const next = [...current]
  const rejected: MediaStudioAttachmentRejection[] = []
  const seen = new Set(next.map(attachment => attachment.id))
  let totalBytes = next.reduce((total, attachment) => total + attachment.size, 0)
  for (const file of files) {
    const id = attachmentID(file)
    if (seen.has(id)) continue
    const kind = kindOf(file)
    if (!kind || !limits.enabled.includes(kind) || !mediaStudioReferenceTypeAllowed(file.type)) {
      rejected.push({ name: file.name, reason: 'type', limit: 0 }); continue
    }
    const maxSize = Math.min(limits.maxFileSizeMB[kind] * 1024 * 1024, MAX_MEDIA_STUDIO_REFERENCE_BYTES)
    if (file.size > maxSize) {
      rejected.push({ name: file.name, reason: 'size', limit: Math.round(maxSize / (1024 * 1024)) }); continue
    }
    const kindCount = next.filter(attachment => attachment.kind === kind).length
    const maxFiles = Math.min(limits.maxFiles[kind], 9)
    if (kindCount >= maxFiles) {
      rejected.push({ name: file.name, reason: 'count', limit: maxFiles }); continue
    }
    if (totalBytes + file.size > MAX_MEDIA_STUDIO_REFERENCE_TOTAL_BYTES) {
      rejected.push({ name: file.name, reason: 'total', limit: 32 }); continue
    }
    seen.add(id)
    totalBytes += file.size
    next.push({ id, file, name: file.name || kind, kind, mimeType: file.type, size: file.size, previewUrl: attachmentPreviewURL(file), url: '', status: 'uploading', error: '' })
  }
  return { attachments: next, rejected }
}

export function revokeMediaStudioPreviewURLs(attachments: Array<{ previewUrl: string }>): void {
  const revokeObjectURL = globalThis.URL?.revokeObjectURL
  if (typeof revokeObjectURL !== 'function') return
  for (const attachment of attachments) {
    if (attachment.previewUrl.startsWith('blob:')) {
      revokeObjectURL.call(globalThis.URL, attachment.previewUrl)
    }
  }
}

export function revokeMediaStudioImageAttachments(attachments: MediaStudioImageAttachment[]): void {
  revokeMediaStudioPreviewURLs(attachments)
}
