import { buildGatewayUrl } from '@/core/networks/client'

export interface MediaStudioModel {
  id: string
  object?: string
  display_name?: string
  displayName?: string
  owned_by?: string
  ownedBy?: string
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

export async function listMediaStudioModels(apiKey: string): Promise<MediaStudioModelsResponse> {
  const response = await fetch(buildGatewayUrl('/v1/models'), {
    headers: authHeaders(apiKey),
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

export async function getAsyncImageTask(apiKey: string, taskId: string): Promise<MediaStudioImageTask> {
  const response = await fetch(buildGatewayUrl(`/v1/images/tasks/${encodeURIComponent(taskId)}`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseMediaStudioError(response)
  return response.json()
}

export function isMediaStudioImageModel(model: string): boolean {
  const normalized = model.trim().toLowerCase()
  return normalized.startsWith('gpt-image-') || normalized.startsWith('grok-imagine')
}

export function mediaStudioErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim()) return error.message
  if (typeof error === 'string' && error.trim()) return error
  return fallback
}
