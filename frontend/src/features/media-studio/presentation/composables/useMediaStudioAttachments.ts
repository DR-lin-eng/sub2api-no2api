import SparkMD5 from 'spark-md5'

import type { MediaStudioMediaType } from '@/features/media-studio/data/datasources/mediaStudioDatasource'

export interface MediaStudioImageAttachment {
  id: string
  file: File
  name: string
  mimeType: string
  size: number
  previewUrl: string
}

// 视频/音频参考素材：上传到对象存储后才能作为请求里的公网 URL。
export interface MediaStudioReferenceAttachment {
  id: string
  file: File
  name: string
  kind: MediaStudioMediaType
  mimeType: string
  size: number
  previewUrl: string
  url: string
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

// 分块读取，避免把大体积参考视频一次性读入内存。
const MEDIA_STUDIO_HASH_CHUNK_BYTES = 2 * 1024 * 1024

// 内容哈希作为附件 ID：同一用户重复选择同一文件时前端直接去重，后端也按同一 key 秒传。
export async function mediaStudioFileMD5(file: File): Promise<string> {
  const spark = new SparkMD5.ArrayBuffer()
  for (let offset = 0; offset < file.size; offset += MEDIA_STUDIO_HASH_CHUNK_BYTES) {
    const chunk = await file.slice(offset, offset + MEDIA_STUDIO_HASH_CHUNK_BYTES).arrayBuffer()
    spark.append(chunk)
  }
  return spark.end()
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
  const seen = new Set(next.map((attachment) => attachment.id))

  // 先做低成本校验，只对可能入库的文件计算内容哈希。
  const candidates: File[] = []
  for (const file of files) {
    if (!file.type.startsWith('image/')) {
      rejected.push({ name: file.name, reason: 'type', limit: 0 })
      continue
    }
    if (file.size > limits.maxFileSizeBytes) {
      rejected.push({ name: file.name, reason: 'size', limit: Math.round(limits.maxFileSizeBytes / (1024 * 1024)) })
      continue
    }
    candidates.push(file)
  }
  const hashes = await Promise.all(candidates.map((file) => mediaStudioFileMD5(file)))

  candidates.forEach((file, index) => {
    const id = hashes[index]
    if (seen.has(id)) return
    if (next.length >= limits.maxFiles) {
      rejected.push({ name: file.name, reason: 'count', limit: limits.maxFiles })
      return
    }
    const currentBytes = next.reduce((total, attachment) => total + attachment.size, 0)
    if (currentBytes + file.size > limits.maxTotalBytes) {
      rejected.push({ name: file.name, reason: 'total', limit: Math.round(limits.maxTotalBytes / (1024 * 1024)) })
      return
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
  })

  return { attachments: next, rejected }
}

// 参考素材按类型分别计数，任一类型达到上限或未启用都会被拒绝。
export async function addMediaStudioReferenceAttachments(
  current: MediaStudioReferenceAttachment[],
  files: File[],
  limits: MediaStudioReferenceLimits,
  kindOf: (file: File) => MediaStudioMediaType | '',
): Promise<{ attachments: MediaStudioReferenceAttachment[]; rejected: MediaStudioAttachmentRejection[] }> {
  const next = [...current]
  const rejected: MediaStudioAttachmentRejection[] = []
  const seen = new Set(next.map((attachment) => attachment.id))

  // 先做低成本校验，只对可能入库的文件计算内容哈希。
  const candidates: Array<{ file: File; kind: MediaStudioMediaType }> = []
  for (const file of files) {
    const kind = kindOf(file)
    if (!kind || !limits.enabled.includes(kind)) {
      rejected.push({ name: file.name, reason: 'type', limit: 0 })
      continue
    }
    const maxSizeMB = limits.maxFileSizeMB[kind]
    if (file.size > maxSizeMB * 1024 * 1024) {
      rejected.push({ name: file.name, reason: 'size', limit: maxSizeMB })
      continue
    }
    candidates.push({ file, kind })
  }
  const hashes = await Promise.all(candidates.map(({ file }) => mediaStudioFileMD5(file)))

  candidates.forEach(({ file, kind }, index) => {
    const id = hashes[index]
    if (seen.has(id)) return
    const kindCount = next.filter((attachment) => attachment.kind === kind).length
    if (kindCount >= limits.maxFiles[kind]) {
      rejected.push({ name: file.name, reason: 'count', limit: limits.maxFiles[kind] })
      return
    }
    seen.add(id)
    next.push({
      id,
      file,
      name: file.name || kind,
      kind,
      mimeType: file.type,
      size: file.size,
      previewUrl: attachmentPreviewURL(file),
      url: '',
      status: 'uploading',
      error: '',
    })
  })

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
