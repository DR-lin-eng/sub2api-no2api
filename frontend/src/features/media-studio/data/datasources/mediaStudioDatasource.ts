import { isMediaStudioAudioModel, isMediaStudioImageModel, isMediaStudioVideoModel } from '@/features/custom-model-config/domain/services/modelCapabilityService'
import { mediaStudioBlobDataURL } from './mediaStudioBrowserMedia'
import { apiClient, buildGatewayUrl } from '@/core/networks/client'
import type { ModelAttributeBlock, ModelAttributeValues } from '@/common/widgets/model-attributes/types'

export type {
  ModelAttributeBlock,
  ModelAttributeField,
  ModelAttributeFieldType,
  ModelAttributeRenderMode,
  ModelAttributeValues,
} from '@/common/widgets/model-attributes/types'

export type MediaStudioMediaType = 'image' | 'video' | 'audio'

export interface MediaStudioGroupOption {
  group_id: number
  group_name: string
  platform: string
  media_type: MediaStudioMediaType
  models?: string[]
}

export interface MediaStudioConfig {
  groups: MediaStudioGroupOption[]
  storage_available?: boolean
}

export interface MediaStudioSession {
  api_key: string
  group_id: number
  media_type: MediaStudioMediaType
}

export interface MediaStudioModel {
  id: string
  object?: string
  display_name?: string
  displayName?: string
  owned_by?: string
  ownedBy?: string
  attributes?: ModelAttributeBlock
  limits?: MediaStudioModelLimits
  [key: string]: unknown
}

// 模型配置里的媒体限制原始结构；缺省字段表示「未配置」，由前端回退默认值。
export interface MediaStudioModelLimits {
  allow_upload?: boolean
  file_types?: MediaStudioMediaType[]
  max_files?: Partial<Record<MediaStudioMediaType, number>>
  max_file_size_mb?: Partial<Record<MediaStudioMediaType, number>>
  duration?: { min?: number; max?: number } | null
}

// 归一化后的限制：所有字段都有值，便于渲染与校验直接使用。
export interface MediaStudioResolvedLimits {
  configured: boolean
  allowUpload: boolean
  fileTypes: MediaStudioMediaType[]
  maxFiles: Record<MediaStudioMediaType, number>
  maxFileSizeMB: Record<MediaStudioMediaType, number>
  duration: { min: number; max: number }
}

export const MEDIA_STUDIO_MEDIA_KINDS: MediaStudioMediaType[] = ['image', 'video', 'audio']

// 未配置模型限制时沿用的默认值（与媒体工坊既有行为保持一致）。
export const MEDIA_STUDIO_DEFAULT_MAX_FILES: Record<MediaStudioMediaType, number> = {
  image: 9,
  video: 3,
  audio: 3,
}
export const MEDIA_STUDIO_DEFAULT_MAX_FILE_SIZE_MB: Record<MediaStudioMediaType, number> = {
  image: 20,
  video: 256,
  audio: 50,
}
export const MEDIA_STUDIO_DEFAULT_DURATION_RANGE = { min: 1, max: 15 }

function positiveInteger(value: unknown, fallback: number): number {
  const numeric = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numeric) || numeric <= 0) return fallback
  return Math.trunc(numeric)
}

export function resolveMediaStudioModelLimits(
  limits?: MediaStudioModelLimits | null,
): MediaStudioResolvedLimits {
  const resolved: MediaStudioResolvedLimits = {
    configured: false,
    allowUpload: true,
    fileTypes: [...MEDIA_STUDIO_MEDIA_KINDS],
    maxFiles: { ...MEDIA_STUDIO_DEFAULT_MAX_FILES },
    maxFileSizeMB: { ...MEDIA_STUDIO_DEFAULT_MAX_FILE_SIZE_MB },
    duration: { ...MEDIA_STUDIO_DEFAULT_DURATION_RANGE },
  }
  if (!limits || typeof limits !== 'object') return resolved

  resolved.configured = Object.keys(limits).length > 0
  resolved.allowUpload = limits.allow_upload !== false
  const declaredTypes = Array.isArray(limits.file_types)
    ? limits.file_types.filter(kind => MEDIA_STUDIO_MEDIA_KINDS.includes(kind))
    : []
  // 未声明 file_types 表示三类素材都允许，与后端校验语义一致。
  if (Array.isArray(limits.file_types)) resolved.fileTypes = declaredTypes

  for (const kind of MEDIA_STUDIO_MEDIA_KINDS) {
    const files = limits.max_files?.[kind]
    // 0 是合法配置，表示该类型禁用，不能按缺省值回退。
    if (typeof files === 'number' && Number.isFinite(files) && files >= 0) {
      resolved.maxFiles[kind] = Math.trunc(files)
    }
    resolved.maxFileSizeMB[kind] = positiveInteger(
      limits.max_file_size_mb?.[kind],
      resolved.maxFileSizeMB[kind],
    )
  }

  const min = Number(limits.duration?.min)
  const max = Number(limits.duration?.max)
  if (Number.isFinite(min) && Number.isFinite(max) && min >= 1 && max >= min) {
    resolved.duration = { min: Math.trunc(min), max: Math.trunc(max) }
  }
  return resolved
}

export function mediaStudioKindEnabled(
  limits: MediaStudioResolvedLimits,
  kind: MediaStudioMediaType,
): boolean {
  return limits.allowUpload && limits.fileTypes.includes(kind) && limits.maxFiles[kind] > 0
}

export function mediaStudioUploadAccept(limits: MediaStudioResolvedLimits): string {
  return MEDIA_STUDIO_MEDIA_KINDS
    .filter(kind => mediaStudioKindEnabled(limits, kind))
    .map(kind => `${kind}/*`)
    .join(',')
}

export function mediaStudioFileKind(file: File): MediaStudioMediaType | '' {
  const mimeType = (file.type || '').toLowerCase()
  if (mimeType.startsWith('image/')) return 'image'
  if (mimeType.startsWith('video/')) return 'video'
  if (mimeType.startsWith('audio/')) return 'audio'
  return ''
}

// 媒体工坊硬编码参数控件与请求字段的绑定：模板声明了同名字段时，局部覆盖模式下该控件被替换。
export type MediaStudioParameterControl =
  | 'image.size'
  | 'image.quality'
  | 'image.count'
  | 'video.resolution'
  | 'video.aspectRatio'
  | 'video.duration'

export const MEDIA_STUDIO_PARAMETER_BINDINGS: Record<MediaStudioParameterControl, string> = {
  'image.size': 'size',
  'image.quality': 'quality',
  'image.count': 'n',
  'video.resolution': 'resolution',
  'video.aspectRatio': 'aspect_ratio',
  'video.duration': 'duration',
}

// 请求模板里可屏蔽的内置参数：由绑定表按模态派生，避免参数名重复维护。
export const MEDIA_STUDIO_HIDE_CANDIDATES: Record<MediaStudioMediaType, string[]> = {
  image: [],
  video: [],
  audio: [],
}

for (const [control, key] of Object.entries(MEDIA_STUDIO_PARAMETER_BINDINGS)) {
  const modality = control.split('.', 1)[0] as MediaStudioMediaType
  MEDIA_STUDIO_HIDE_CANDIDATES[modality].push(key)
}

export interface MediaStudioModelsResponse {
  object: string
  data: MediaStudioModel[]
}

export interface MediaStudioImageSubmitRequest {
  model: string
  prompt: string
  n?: number
  size?: string
  quality?: string
  response_format?: 'b64_json' | 'url'
  attributes?: ModelAttributeValues
}

export async function getMediaStudioConfig(): Promise<MediaStudioConfig> {
  const { data } = await apiClient.get<MediaStudioConfig>('/media-studio/config')
  return {
    ...data,
    groups: data.groups.map(group => {
      if (group.media_type === 'image' || group.media_type === 'video' || group.media_type === 'audio') return group
      for (const model of group.models ?? []) {
        if (isMediaStudioVideoModel(model)) return { ...group, media_type: 'video' as const }
        if (isMediaStudioAudioModel(model)) return { ...group, media_type: 'audio' as const }
        if (isMediaStudioImageModel(model)) return { ...group, media_type: 'image' as const }
      }
      return { ...group, media_type: 'image' as const }
    }),
  }
}

export async function createMediaStudioSession(
  mediaType: MediaStudioMediaType,
  groupId: number,
): Promise<MediaStudioSession> {
  const { data } = await apiClient.post<MediaStudioSession>('/media-studio/session', {
    media_type: mediaType,
    group_id: groupId,
  })
  return data
}

// 提交前预估消耗：金额由后端 sub2api 本地计价链路算出，前端不自行推算价格。
export interface MediaStudioCostEstimateRequest {
  media_type: MediaStudioMediaType
  group_id: number
  model: string
  resolution?: string
  duration?: number
  size?: string
  count?: number
}

export interface MediaStudioCostEstimate {
  amount: number
  currency: string
  billing_mode: string
  model: string
  resolution?: string
  duration_seconds?: number
  image_count?: number
}

export async function estimateMediaStudioCost(
  request: MediaStudioCostEstimateRequest,
): Promise<MediaStudioCostEstimate> {
  const { data } = await apiClient.post<MediaStudioCostEstimate>('/media-studio/estimate', request)
  return data
}

export interface MediaStudioGeneratedImage {
  url?: string
  b64_json?: string
  revised_prompt?: string
}

export interface MediaStudioImageResult {
  created?: number
  data?: MediaStudioGeneratedImage[]
  [key: string]: unknown
}

export type MediaStudioVideoResolution = '480p' | '720p' | '1080p'
export type MediaStudioVideoAspectRatio = '16:9' | '9:16' | '1:1' | '4:3' | '3:4'

export interface MediaStudioVideoSubmitRequest {
  model: string
  prompt: string
  resolution?: MediaStudioVideoResolution
  duration?: number
  aspect_ratio?: MediaStudioVideoAspectRatio
  images?: string[]
  videos?: string[]
  audios?: string[]
  attributes?: ModelAttributeValues
}

// 参考素材上传结果：url 可直接作为视频请求的 images/videos/audios 元素。
export interface MediaStudioUpload {
  url: string
  name: string
  kind: MediaStudioMediaType
  size: number
  content_type: string
}

export type MediaStudioVideoTaskStatus = 'processing' | 'completed' | 'failed'

export interface MediaStudioVideoTask {
  id: string
  status: MediaStudioVideoTaskStatus
  progress?: number
  error?: string
  // 本次视频的扣费金额与币种；处理中为冻结金额，完成后为实际结算金额。
  cost?: number
  currency?: string
  raw: Record<string, unknown>
}

export type MediaStudioImageTaskStatus = 'processing' | 'completed' | 'failed' | string

export interface MediaStudioImageTask {
  id: string
  task_id: string
  object: string
  status: MediaStudioImageTaskStatus
  http_status?: number
  image_url?: string
  result?: MediaStudioImageResult
  error?: {
    type?: string
    code?: string
    message?: string
    [key: string]: unknown
  } | Record<string, unknown> | string | null
  created_at: number
  completed_at?: number | null
  expires_at: number
  poll_url?: string
}

export interface MediaStudioDatasourceError extends Error {
  status?: number
  code?: string | number
  requestId?: string
}

async function parseMediaStudioError(response: Response): Promise<MediaStudioDatasourceError> {
  try {
    const body = await response.json()
    const message = body?.error?.message || body?.message || response.statusText || `HTTP ${response.status}`
    const error = new Error(message) as MediaStudioDatasourceError
    error.code = body?.error?.code || body?.error?.type || response.status
    error.status = response.status
    error.requestId = response.headers.get('X-Request-Id') || ''
    return error
  } catch {
    const error = new Error(response.statusText || `HTTP ${response.status}`) as MediaStudioDatasourceError
    error.code = response.status
    error.status = response.status
    error.requestId = response.headers.get('X-Request-Id') || ''
    return error
  }
}

function authHeaders(apiKey: string, extra?: HeadersInit): HeadersInit {
  return {
    Authorization: `Bearer ${apiKey}`,
    ...extra,
  }
}

// 模型属性按字段名平铺到请求体顶层，固定字段优先，避免属性覆盖 model/prompt 等。
function serializeSubmitPayload(payload: { attributes?: ModelAttributeValues }): string {
  const { attributes, ...rest } = payload
  return JSON.stringify({ ...(attributes ?? {}), ...rest })
}

const MAX_VIDEO_BYTES = 256 * 1024 * 1024
const MAX_VIDEO_CHUNKS = 65_536

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}

function firstString(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return ''
}

function normalizeVideoRequestID(value: unknown): string {
  const id = firstString(value)
  if (!id || id.length > 256 || !/^[A-Za-z0-9._:-]+$/.test(id)) {
    throw new Error('video generation returned an invalid request id')
  }
  return id
}

function extractVideoTaskID(raw: Record<string, unknown>): string {
  const data = recordValue(raw.data)
  const result = recordValue(raw.result)
  const video = recordValue(raw.video)

  // 支持多个可能的位置
  const taskID = raw.request_id ?? raw.requestId ?? raw.id ??
    data.request_id ?? data.id ??
    video.request_id ?? video.id ??
    result.request_id ?? result.id ??
    raw.task_id ?? data.task_id ?? video.task_id

  // 轮询响应可能只带 status/progress，不带任务 ID；此时交给调用方的 fallbackID 兜底。
  const normalized = firstString(taskID)
  if (!normalized) return ''
  return normalizeVideoRequestID(normalized)
}

function normalizeVideoStatus(value: unknown): MediaStudioVideoTaskStatus {
  const status = firstString(value).toLowerCase()
  if (['completed', 'complete', 'succeeded', 'success', 'done'].includes(status)) return 'completed'
  if (['failed', 'failure', 'error', 'cancelled', 'canceled', 'expired'].includes(status)) return 'failed'
  return 'processing'
}

function extractVideoError(raw: Record<string, unknown>): string | undefined {
  const error = recordValue(raw.error)
  const data = recordValue(raw.data)
  const result = recordValue(raw.result)

  // 检查多个可能的错误位置
  const errorMessage = firstString(
    error.message,
    raw.message,
    raw.error_message,
    data.error,
    result.error
  )

  return errorMessage || undefined
}

function normalizeVideoProgress(value: unknown): number | undefined {
  const numeric = typeof value === 'number'
    ? value
    : typeof value === 'string' && value.trim()
      ? Number(value)
      : Number.NaN
  if (!Number.isFinite(numeric)) return undefined
  return Math.min(100, Math.max(0, Math.round(numeric)))
}

function normalizeVideoCost(value: unknown): number | undefined {
  const numeric = typeof value === 'number'
    ? value
    : typeof value === 'string' && value.trim()
      ? Number(value)
      : Number.NaN
  if (!Number.isFinite(numeric) || numeric < 0) return undefined
  return numeric
}

export function normalizeMediaStudioVideoTask(value: unknown, fallbackID = ''): MediaStudioVideoTask {
  const raw = recordValue(value)
  const data = recordValue(raw.data)
  const result = recordValue(raw.result)
  const video = recordValue(raw.video)

  const id = extractVideoTaskID(raw) || fallbackID

  return {
    id,
    status: normalizeVideoStatus(raw.status ?? data.status ?? result.status),
    progress: normalizeVideoProgress(raw.progress ?? data.progress ?? result.progress ?? video.progress),
    error: extractVideoError(raw),
    cost: normalizeVideoCost(raw.cost ?? data.cost ?? result.cost ?? video.cost),
    currency: firstString(raw.currency, data.currency, result.currency, video.currency) || undefined,
    raw,
  }
}

export async function listMediaStudioModels(
  _apiKey: string,
  mediaType?: MediaStudioMediaType,
  groupId?: number,
): Promise<MediaStudioModelsResponse> {
  if (mediaType && groupId) {
    const { data } = await apiClient.get<MediaStudioModelsResponse>('/media-studio/models', {
      params: { media_type: mediaType, group_id: groupId },
    })
    return data
  }
  const response = await fetch(buildGatewayUrl('/v1/models'), {
    headers: authHeaders(_apiKey),
  })
  if (!response.ok) throw await parseMediaStudioError(response)
  return response.json()
}

// Kept as a compatibility entry point: preparing a reference performs no HTTP upload.
export async function uploadMediaStudioFile(
  _model: string,
  kind: MediaStudioMediaType,
  file: File,
): Promise<MediaStudioUpload> {
  return { url: await mediaStudioBlobDataURL(file), name: file.name, kind, size: file.size, content_type: file.type }
}

export async function submitAsyncImageGeneration(
  apiKey: string,
  payload: MediaStudioImageSubmitRequest,
  idempotencyKey: string,
): Promise<MediaStudioImageTask> {
  const response = await fetch(buildGatewayUrl('/v1/images/generations/async'), {
    method: 'POST',
    headers: authHeaders(apiKey, {
      'Content-Type': 'application/json',
      'Idempotency-Key': idempotencyKey,
    }),
    body: JSON.stringify(payload),
  })
  if (!response.ok) throw await parseMediaStudioError(response)
  return response.json()
}

export async function submitImageGeneration(
  apiKey: string,
  payload: MediaStudioImageSubmitRequest,
  idempotencyKey: string,
): Promise<MediaStudioImageTask> {
  const response = await fetch(buildGatewayUrl('/v1/images/generations'), {
    method: 'POST',
    headers: authHeaders(apiKey, {
      'Content-Type': 'application/json',
      'Idempotency-Key': idempotencyKey,
    }),
    body: serializeSubmitPayload(payload),
  })
  if (!response.ok) throw await parseMediaStudioError(response)
  const result = await response.json() as MediaStudioImageResult
  const createdAt = typeof result.created === 'number' ? result.created : Math.floor(Date.now() / 1000)
  const imageURL = result.data?.find(item => typeof item.url === 'string' && item.url.trim())?.url || ''
  const id = `imgsync_${idempotencyKey.replace(/[^a-zA-Z0-9_]/g, '').slice(-32) || Date.now()}`
  return {
    id,
    task_id: id,
    object: 'image.generation.task',
    status: 'completed',
    http_status: response.status,
    image_url: imageURL,
    result,
    created_at: createdAt,
    completed_at: Math.floor(Date.now() / 1000),
    expires_at: Math.floor(Date.now() / 1000) + 86400,
  }
}

export async function submitImageEdit(
  apiKey: string,
  files: File[],
  payload: Omit<MediaStudioImageSubmitRequest, 'response_format'>,
  idempotencyKey: string,
): Promise<MediaStudioImageTask> {
  const form = new FormData()
  form.append('model', payload.model)
  form.append('prompt', payload.prompt)
  if (payload.n !== undefined) form.append('n', String(payload.n))
  if (payload.size) form.append('size', payload.size)
  if (payload.quality) form.append('quality', payload.quality)
  for (const [key, value] of Object.entries(payload.attributes ?? {})) form.append(key, String(value))
  for (const file of files) form.append('image', file, file.name)

  const response = await fetch(buildGatewayUrl('/v1/images/edits'), {
    method: 'POST',
    headers: authHeaders(apiKey, {
      'Idempotency-Key': idempotencyKey,
    }),
    body: form,
  })
  if (!response.ok) throw await parseMediaStudioError(response)
  const result = await response.json() as MediaStudioImageResult
  const createdAt = typeof result.created === 'number' ? result.created : Math.floor(Date.now() / 1000)
  const imageURL = result.data?.find(item => typeof item.url === 'string' && item.url.trim())?.url || ''
  const id = `imgedit_${idempotencyKey.replace(/[^a-zA-Z0-9_]/g, '').slice(-32) || Date.now()}`
  return {
    id,
    task_id: id,
    object: 'image.generation.task',
    status: 'completed',
    http_status: response.status,
    image_url: imageURL,
    result,
    created_at: createdAt,
    completed_at: Math.floor(Date.now() / 1000),
    expires_at: Math.floor(Date.now() / 1000) + 86400,
  }
}

export async function getAsyncImageTask(apiKey: string, taskId: string): Promise<MediaStudioImageTask> {
  const response = await fetch(buildGatewayUrl(`/v1/images/tasks/${encodeURIComponent(taskId)}`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseMediaStudioError(response)
  return response.json()
}

export async function submitVideoGeneration(
  apiKey: string,
  payload: MediaStudioVideoSubmitRequest,
  idempotencyKey: string,
  signal?: AbortSignal,
): Promise<MediaStudioVideoTask> {
  const response = await fetch(buildGatewayUrl('/v1/videos/generations'), {
    method: 'POST',
    signal,
    headers: authHeaders(apiKey, {
      'Content-Type': 'application/json',
      'Idempotency-Key': idempotencyKey,
    }),
    body: serializeSubmitPayload(payload),
  })
  if (!response.ok) throw await parseMediaStudioError(response)
  return normalizeMediaStudioVideoTask(await response.json())
}

export async function getVideoGenerationTask(apiKey: string, taskId: string): Promise<MediaStudioVideoTask> {
  const normalizedID = normalizeVideoRequestID(taskId)
  const response = await fetch(buildGatewayUrl(`/v1/videos/${encodeURIComponent(normalizedID)}`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseMediaStudioError(response)
  return normalizeMediaStudioVideoTask(await response.json(), normalizedID)
}

async function cancelVideoResponse(response: Response): Promise<void> {
  try {
    await response.body?.cancel()
  } catch {
    // The response may already be cancelled or errored.
  }
}

async function readBoundedVideoBlob(response: Response, mimeType: string): Promise<Blob> {
  if (!response.body) {
    throw new Error('generated video has an invalid size')
  }

  const reader = response.body.getReader()
  const chunks: Uint8Array[] = []
  let totalBytes = 0
  let cancelled = false
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      if (!value?.byteLength) continue
      if (value.byteLength > MAX_VIDEO_BYTES - totalBytes || chunks.length >= MAX_VIDEO_CHUNKS) {
        cancelled = true
        await reader.cancel('generated video exceeded the preview size limit')
        throw new Error('generated video is too large to preview safely')
      }
      chunks.push(value)
      totalBytes += value.byteLength
    }
  } catch (error) {
    if (!cancelled) {
      try {
        await reader.cancel(error)
      } catch {
        // The stream may already be cancelled or errored.
      }
    }
    throw error
  } finally {
    reader.releaseLock()
  }

  if (totalBytes <= 0) {
    throw new Error('generated video has an invalid size')
  }
  return new Blob(chunks, { type: mimeType })
}

// Video content is protected by the same API key as the task. Fetch it into a
// bounded, incrementally-read Blob instead of putting the key in a query string
// or trusting an upstream-provided URL in a <video> element.
export async function getVideoGenerationContent(apiKey: string, taskId: string): Promise<Blob> {
  const normalizedID = normalizeVideoRequestID(taskId)
  const response = await fetch(buildGatewayUrl(`/v1/videos/${encodeURIComponent(normalizedID)}/content`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseMediaStudioError(response)

  const declaredSize = Number(response.headers.get('Content-Length') || 0)
  if (Number.isFinite(declaredSize) && declaredSize > MAX_VIDEO_BYTES) {
    await cancelVideoResponse(response)
    throw new Error('generated video is too large to preview safely')
  }
  const mimeType = (response.headers.get('Content-Type') || '').split(';', 1)[0].trim().toLowerCase()
  if (mimeType && !mimeType.startsWith('video/') && mimeType !== 'application/octet-stream') {
    await cancelVideoResponse(response)
    throw new Error('generated video returned an unsafe content type')
  }
  const blob = await readBoundedVideoBlob(response, mimeType)
  if (blob.type && !blob.type.toLowerCase().startsWith('video/') && blob.type !== 'application/octet-stream') {
    throw new Error('generated video returned an unsafe content type')
  }
  return blob
}

export function mediaStudioErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim()) return error.message
  if (typeof error === 'string' && error.trim()) return error
  // apiClient 拒绝的是普通对象而不是 Error，需要显式读取 message/reason。
  if (error && typeof error === 'object') {
    const candidate = error as { message?: unknown; reason?: unknown }
    if (typeof candidate.message === 'string' && candidate.message.trim()) return candidate.message
    if (typeof candidate.reason === 'string' && candidate.reason.trim()) return candidate.reason
  }
  return fallback
}

export function mediaStudioErrorStatus(error: unknown): number | undefined {
  if (error && typeof error === 'object') {
    const status = (error as { status?: unknown }).status
    if (typeof status === 'number') return status
  }
  return undefined
}
