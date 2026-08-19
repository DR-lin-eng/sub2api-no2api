import { apiClient, buildGatewayUrl } from '@/core/networks/client'

export type MediaStudioMediaType = 'image' | 'video' | 'audio'

export interface MediaStudioGroupOption {
  group_id: number
  group_name: string
  platform: string
  models?: string[]
}

export interface MediaStudioConfig {
  groups: MediaStudioGroupOption[]
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
  supported_parameters?: string[]
  supportedParameters?: string[]
  supported_sizes?: string[]
  supportedSizes?: string[]
  supported_qualities?: string[]
  supportedQualities?: string[]
  supports_quality?: boolean
  supportsQuality?: boolean
  [key: string]: unknown
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
}

export async function getMediaStudioConfig(): Promise<MediaStudioConfig> {
  const { data } = await apiClient.get<MediaStudioConfig>('/media-studio/config')
  return data
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

export interface MediaStudioVideoSubmitRequest {
  model: string
  prompt: string
  resolution: MediaStudioVideoResolution
  duration: number
}

export type MediaStudioVideoTaskStatus = 'processing' | 'completed' | 'failed'

export interface MediaStudioVideoTask {
  id: string
  status: MediaStudioVideoTaskStatus
  error?: string
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

function normalizeVideoStatus(value: unknown): MediaStudioVideoTaskStatus {
  const status = firstString(value).toLowerCase()
  if (['completed', 'complete', 'succeeded', 'success', 'done'].includes(status)) return 'completed'
  if (['failed', 'failure', 'error', 'cancelled', 'canceled', 'expired'].includes(status)) return 'failed'
  return 'processing'
}

export function normalizeMediaStudioVideoTask(value: unknown, fallbackID = ''): MediaStudioVideoTask {
  const raw = recordValue(value)
  const data = recordValue(raw.data)
  const result = recordValue(raw.result)
  const error = recordValue(raw.error)
  const id = normalizeVideoRequestID(
    raw.request_id ?? raw.requestId ?? raw.id ?? data.request_id ?? data.id ?? result.request_id ?? result.id ?? fallbackID,
  )
  return {
    id,
    status: normalizeVideoStatus(raw.status ?? data.status ?? result.status),
    error: firstString(error.message, raw.message, data.error, result.error) || undefined,
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
    body: JSON.stringify(payload),
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
): Promise<MediaStudioVideoTask> {
  const response = await fetch(buildGatewayUrl('/v1/videos/generations'), {
    method: 'POST',
    headers: authHeaders(apiKey, {
      'Content-Type': 'application/json',
      'Idempotency-Key': idempotencyKey,
    }),
    body: JSON.stringify(payload),
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

// Re-export from modelCapabilityService
export {
  isMediaStudioImageModel,
  isMediaStudioVideoModel,
  isMediaStudioAudioModel,
} from "@/features/custom-model-config/domain/services/modelCapabilityService";

export function mediaStudioErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim()) return error.message
  if (typeof error === 'string' && error.trim()) return error
  return fallback
}
