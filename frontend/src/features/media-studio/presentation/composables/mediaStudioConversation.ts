import { safeMediaStudioReferenceSource } from '@/features/media-studio/data/datasources/mediaStudioBrowserMedia'
import { normalizeMediaStudioOwnerID } from '@/features/media-studio/data/datasources/mediaStudioImageStore'
import type {
  MediaStudioGeneratedImage,
  MediaStudioImageTask,
  MediaStudioModelLimits,
  MediaStudioVideoAspectRatio,
  MediaStudioVideoResolution,
  MediaStudioVideoTask,
  ModelAttributeField,
  ModelAttributeRenderMode,
  ModelAttributeValues,
} from '@/features/media-studio/data/datasources/mediaStudioDatasource'

export const MEDIA_STUDIO_STORAGE_KEY = 'sub2api.mediaStudio.v2'
export const MEDIA_STUDIO_LEGACY_STORAGE_KEY = 'sub2api.mediaStudio.v1'
const DEFAULT_VIDEO_RESOLUTION: MediaStudioVideoResolution = '720p'
const DEFAULT_VIDEO_ASPECT_RATIO: MediaStudioVideoAspectRatio = '16:9'
const DEFAULT_VIDEO_DURATION = 6

export interface MediaStudioParameterOption {
  value: string
  label: string
}

export interface MediaStudioModelOption {
  id: string
  fields: ModelAttributeField[]
  renderMode: ModelAttributeRenderMode
  hide: string[]
  limits?: MediaStudioModelLimits
}

export type MediaStudioImageResolution = '1K' | '2K' | '4K'
export type MediaStudioImageAspectRatio = '1:1' | '3:2' | '2:3' | '4:3' | '3:4' | '16:9' | '9:16' | `custom:${string}`

const IMAGE_RESOLUTION_DEFAULT: MediaStudioImageResolution = '1K'
const IMAGE_ASPECT_RATIO_DEFAULT: MediaStudioImageAspectRatio = '1:1'
export const IMAGE_QUALITY_OPTIONS: MediaStudioParameterOption[] = [
  { value: 'auto', label: 'auto' },
  { value: 'low', label: 'low' },
  { value: 'medium', label: 'medium' },
  { value: 'high', label: 'high' },
]
const IMAGE_RESOLUTION_MAX_EDGE: Record<MediaStudioImageResolution, number> = {
  '1K': 1024,
  '2K': 2048,
  '4K': 3840,
}
const IMAGE_SIZE_MULTIPLE = 16
const IMAGE_MAX_PIXELS = 8_294_400

export interface MediaStudioGeneratedImagePreview {
  id: string
  src: string
  url?: string
  b64Json?: string
  revisedPrompt?: string
  cacheKey?: string
}

export interface MediaStudioInputImagePreview {
  id: string
  src: string
  name: string
  mimeType: string
  cacheKey?: string
}

export interface MediaStudioGeneratedVideoPreview {
  src: string
  mimeType: string
  cacheKey?: string
}

// 用户消息里保留的参考素材快照：src 直接使用已上传的公网 URL，刷新后仍可渲染。
export interface MediaStudioInputReferencePreview {
  id: string
  src: string
  url: string
  name: string
  kind: 'image' | 'video' | 'audio'
  mimeType: string
  cacheKey?: string
  size?: number
}

export type MediaStudioMessageStatus = 'queued' | 'processing' | 'completed' | 'failed'

// 把后端错误收敛成稳定的分类，视图层据此展示可操作提示；原始错误仍保留在 error 字段。
export type MediaStudioJobErrorCode =
  | 'insufficient_quota'
  | 'unauthorized'
  | 'rate_limited'
  | 'content_policy'
  | 'timeout'
  | 'upstream'
  | 'network'
  | 'unknown'

export interface MediaStudioMessage {
  id: string
  role: 'user' | 'assistant'
  mode: 'image' | 'video'
  prompt: string
  status?: MediaStudioMessageStatus
  // 单个任务 ID 沿用主线协议；视频恢复兼容 taskIds。
  taskId?: string
  taskIds?: string[]
  groupId?: number
  model?: string
  size?: string
  imageResolution?: MediaStudioImageResolution
  imageAspectRatio?: MediaStudioImageAspectRatio
  quality?: string
  count?: number
  resolution?: MediaStudioVideoResolution
  aspectRatio?: MediaStudioVideoAspectRatio
  duration?: number
  attributes?: ModelAttributeValues
  attributeMode?: ModelAttributeRenderMode
  attributeKeys?: string[]
  progress?: number
  inputImages?: MediaStudioInputImagePreview[]
  inputReferences?: MediaStudioInputReferencePreview[]
  images?: MediaStudioGeneratedImagePreview[]
  video?: MediaStudioGeneratedVideoPreview
  // 视频扣费金额与币种：生成中为冻结金额，完成后为实际结算金额。
  cost?: number
  currency?: string
  error?: string
  errorCode?: MediaStudioJobErrorCode
  createdAt: number
  completedAt?: number
}

export interface MediaStudioConversation {
  id: string
  messages: MediaStudioMessage[]
  updatedAt: number
}

// 会话历史只保留有限条数，避免 localStorage 被旧会话无限撑大。
export const MEDIA_STUDIO_MAX_SESSIONS = 20

export interface PersistedMediaStudioState {
  selectedGroupId?: number
  model?: string
  imageModel?: string
  videoModel?: string
  size?: string
  imageResolution?: MediaStudioImageResolution
  imageAspectRatio?: MediaStudioImageAspectRatio
  customImageAspectRatios?: string[]
  quality?: string
  count?: number
  resolution?: MediaStudioVideoResolution
  videoAspectRatio?: MediaStudioVideoAspectRatio
  duration?: number
  // 当前正在使用的会话；历史会话放在 sessions 中，两者按 id 去重。
  conversation?: MediaStudioConversation
  sessions?: MediaStudioConversation[]
}

export function now(): number {
  return Date.now()
}

export function makeId(prefix: string): string {
  const random = typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `${Date.now().toString(36)}${Math.random().toString(36).slice(2)}`
  return `${prefix}_${String(random).replace(/-/g, '')}`
}

export function mediaStudioUserStorageKey(ownerID?: number): string {
  const owner = normalizeMediaStudioOwnerID(ownerID)
  return owner ? `${MEDIA_STUDIO_STORAGE_KEY}.user.${owner}` : ''
}

export function readPersisted(storage: Storage | null, ownerID?: number): PersistedMediaStudioState {
  const scopedKey = mediaStudioUserStorageKey(ownerID)
  if (!storage || !scopedKey) return {}
  // Legacy keys have no owner proof. Do not expose their prompts/results to the next login.
  for (const key of [scopedKey]) {
    try {
      const raw = storage.getItem(key)
      if (!raw) continue
      const parsed = JSON.parse(raw) as PersistedMediaStudioState
      if (parsed && typeof parsed === 'object') return parsed
    } catch {
      // Ignore malformed or inaccessible browser storage.
    }
  }
  return {}
}

export function persistedImageSource(value: unknown): string {
  if (typeof value !== 'string' || !value.trim()) return ''
  const remoteURL = safeRemoteImageURL(value)
  if (remoteURL) return remoteURL

  const match = /^data:(image\/(?:png|jpeg|webp|gif));base64,(.+)$/i.exec(value.trim())
  if (!match || !safeBase64Image(match[2])) return ''
  return `data:${match[1].toLowerCase()};base64,${match[2]}`
}

function restoreModelAttributeValues(value: unknown): ModelAttributeValues | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const restored: ModelAttributeValues = {}
  for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
    if (!key) continue
    if (typeof item === 'string' || typeof item === 'boolean') {
      restored[key] = item
    } else if (typeof item === 'number' && Number.isFinite(item)) {
      restored[key] = item
    }
  }
  return restored
}

function restoreTaskIDs(candidate: Partial<MediaStudioMessage>): string[] {
  const fromArray = Array.isArray(candidate.taskIds)
    ? candidate.taskIds.filter((id): id is string => typeof id === 'string' && !!id.trim())
    : []
  if (fromArray.length > 0) return [...new Set(fromArray)]
  return typeof candidate.taskId === 'string' && candidate.taskId.trim() ? [candidate.taskId] : []
}

// 视频的未完成任务保留 ID 以便恢复；图片沿用主线原有的恢复行为。
export function restorePersistedConversation(value: unknown): MediaStudioConversation | null {
  if (!value || typeof value !== 'object') return null
  const raw = value as Partial<MediaStudioConversation>
  if (!Array.isArray(raw.messages)) return null

  const messages = raw.messages
    .slice(-100)
    .flatMap((message) => {
      if (!message || typeof message !== 'object') return []
      const candidate = message as Partial<MediaStudioMessage>
      if (
        typeof candidate.id !== 'string' ||
        (candidate.role !== 'user' && candidate.role !== 'assistant') ||
        (candidate.mode !== 'image' && candidate.mode !== 'video') ||
        typeof candidate.prompt !== 'string' ||
        !Number.isFinite(candidate.createdAt)
      ) {
        return []
      }

      const taskIDs = candidate.role === 'assistant' && candidate.mode === 'video' ? restoreTaskIDs(candidate) : []
      const hasPersistedImages = candidate.mode === 'image' &&
        Array.isArray(candidate.images) &&
        candidate.images.some((image) => {
          if (!image || typeof image !== 'object') return false
          const item = image as Partial<MediaStudioGeneratedImagePreview>
          return Boolean(persistedImageSource(item.src) || (typeof item.cacheKey === 'string' && item.cacheKey))
        })
      // 只接受当前支持的状态，历史持久化数据里的未知状态直接丢弃，避免渲染出空消息卡。
      const rawStatus = (candidate as { status?: unknown }).status
      const status = rawStatus === 'queued' || rawStatus === 'processing' ||
        rawStatus === 'completed' || rawStatus === 'failed'
        ? rawStatus
        : undefined
      const wasPending = status === 'queued' || status === 'processing'
      // 既没有任务 ID 也没有结果时无法续轮询（例如提交前就被中断），
      // 保留消息并标记失败，让用户看到发生了什么，而不是静默丢弃。
      if (candidate.role === 'assistant' && candidate.mode === 'image' && wasPending && !hasPersistedImages) return []
      const unrecoverablePending = candidate.role === 'assistant' && candidate.mode === 'video' && wasPending &&
        taskIDs.length === 0

      const createdAt = typeof candidate.createdAt === 'number' && Number.isFinite(candidate.createdAt)
        ? candidate.createdAt
        : now()
      const restored: MediaStudioMessage = {
        id: candidate.id,
        role: candidate.role,
        mode: candidate.mode,
        prompt: candidate.prompt,
        // 有任务 ID 时保留 pending 状态，由启动恢复流程继续轮询；无任务 ID 但有结果视为已完成。
        status: unrecoverablePending
          ? 'failed'
          : wasPending && taskIDs.length === 0 ? 'completed' : status,
        taskId: candidate.mode === 'video' ? taskIDs[0] : typeof candidate.taskId === 'string' ? candidate.taskId : undefined,
        taskIds: taskIDs.length > 0 ? taskIDs : undefined,
        groupId: typeof candidate.groupId === 'number' && Number.isFinite(candidate.groupId)
          ? candidate.groupId
          : undefined,
        model: typeof candidate.model === 'string' ? candidate.model : undefined,
        size: typeof candidate.size === 'string' ? candidate.size : undefined,
        imageResolution: normalizeImageResolution(candidate.imageResolution),
        imageAspectRatio: candidate.imageAspectRatio
          ? normalizeImageAspectRatio(candidate.imageAspectRatio)
          : aspectRatioFromSize(candidate.size),
        quality: normalizeImageQuality(candidate.quality),
        count: typeof candidate.count === 'number' ? normalizeCount(candidate.count) : undefined,
        resolution: candidate.resolution === '480p' || candidate.resolution === '720p' || candidate.resolution === '1080p'
          ? candidate.resolution
          : undefined,
        aspectRatio: candidate.aspectRatio
          ? normalizeVideoAspectRatio(candidate.aspectRatio)
          : undefined,
        duration: typeof candidate.duration === 'number' ? normalizeDuration(candidate.duration) : undefined,
        attributes: restoreModelAttributeValues(candidate.attributes),
        progress: typeof candidate.progress === 'number' && Number.isFinite(candidate.progress)
          ? candidate.progress
          : undefined,
        error: typeof candidate.error === 'string' ? candidate.error : undefined,
        errorCode: unrecoverablePending
          ? 'unknown'
          : typeof candidate.errorCode === 'string'
            ? candidate.errorCode as MediaStudioJobErrorCode
            : undefined,
        cost: typeof candidate.cost === 'number' && Number.isFinite(candidate.cost) ? candidate.cost : undefined,
        currency: typeof candidate.currency === 'string' ? candidate.currency : undefined,
        createdAt,
        completedAt: unrecoverablePending
          ? now()
          : typeof candidate.completedAt === 'number' ? candidate.completedAt : undefined,
      }

      if (candidate.mode === 'video' && candidate.video && typeof candidate.video === 'object') {
        const video = candidate.video as Partial<MediaStudioGeneratedVideoPreview>
        const cacheKey = typeof video.cacheKey === 'string' && video.cacheKey.length <= 512
          ? video.cacheKey
          : ''
        if (cacheKey) {
          restored.video = {
            src: '',
            mimeType: typeof video.mimeType === 'string' ? video.mimeType : 'video/mp4',
            cacheKey,
          }
        }
      }

      if (candidate.mode === 'image' && Array.isArray(candidate.inputImages)) {
        restored.inputImages = candidate.inputImages
          .slice(0, 9)
          .flatMap((image) => {
            if (!image || typeof image !== 'object') return []
            const item = image as Partial<MediaStudioInputImagePreview>
            const source = persistedImageSource(item.src)
            const cacheKey = typeof item.cacheKey === 'string' && item.cacheKey.length <= 512
              ? item.cacheKey
              : ''
            if (!source && !cacheKey) return []
            return [{
              id: typeof item.id === 'string' ? item.id : makeId('media_input_image'),
              src: source,
              name: typeof item.name === 'string' ? item.name : 'reference-image',
              mimeType: typeof item.mimeType === 'string' ? item.mimeType : 'image/*',
              cacheKey: cacheKey || undefined,
            }]
          })
      }

      if (candidate.mode === 'video' && Array.isArray(candidate.inputReferences)) {
        restored.inputReferences = candidate.inputReferences
          .slice(0, 20)
          .flatMap((reference) => {
            if (!reference || typeof reference !== 'object') return []
            const item = reference as Partial<MediaStudioInputReferencePreview>
            const url = safeMediaStudioReferenceSource(item.url)
            const cacheKey = typeof item.cacheKey === 'string' && item.cacheKey.length <= 512 ? item.cacheKey : ''
            if (!url && !cacheKey) return []
            const kind = item.kind === 'image' || item.kind === 'video' || item.kind === 'audio'
              ? item.kind
              : ''
            if (!kind) return []
            return [{
              id: typeof item.id === 'string' ? item.id : makeId('media_input_reference'),
              src: safeRemoteImageURL(item.src) || url,
              url,
              name: typeof item.name === 'string' ? item.name : kind,
              kind,
              mimeType: typeof item.mimeType === 'string' ? item.mimeType : '',
              cacheKey: cacheKey || undefined,
              size: typeof item.size === 'number' && Number.isFinite(item.size) ? item.size : undefined,
            }]
          })
      }

      if (candidate.mode === 'image' && Array.isArray(candidate.images)) {
        restored.images = candidate.images
          .slice(0, 9)
          .flatMap((image) => {
            if (!image || typeof image !== 'object') return []
            const item = image as MediaStudioGeneratedImagePreview
            const source = persistedImageSource(item.src)
            const cacheKey = typeof item.cacheKey === 'string' && item.cacheKey.length <= 512
              ? item.cacheKey
              : ''
            if (!source && !cacheKey) return []
            return [{
              id: typeof item.id === 'string' ? item.id : makeId('media_image'),
              src: source,
              url: persistedImageSource(item.url) || undefined,
              revisedPrompt: typeof item.revisedPrompt === 'string' ? item.revisedPrompt : undefined,
              cacheKey: cacheKey || undefined,
            }]
          })
      }

      return [restored]
    })

  return {
    id: typeof raw.id === 'string' ? raw.id : makeId('media_conversation'),
    messages,
    updatedAt: typeof raw.updatedAt === 'number' ? raw.updatedAt : now(),
  }
}

export function imageTaskErrorMessage(task: MediaStudioImageTask): string {
  const error = task.error
  if (!error) return 'image generation failed'
  if (typeof error === 'string') return error
  const message = (error as { message?: unknown }).message
  return typeof message === 'string' && message.trim() ? message : 'image generation failed'
}

export function videoTaskErrorMessage(task: MediaStudioVideoTask): string {
  return task.error?.trim() || 'video generation failed'
}

// 错误分类用于任务条与消息卡展示可操作提示，分类本身不产生用户可见文案。
export function classifyMediaStudioError(error: unknown): MediaStudioJobErrorCode {
  const status = (error && typeof error === 'object')
    ? (error as { status?: unknown }).status
    : undefined
  const code = (error && typeof error === 'object')
    ? String((error as { code?: unknown }).code ?? '')
    : ''
  const message = error instanceof Error
    ? error.message
    : typeof error === 'string'
      ? error
      : ''
  const text = `${code} ${message}`.toLowerCase()
  if (/insufficient|quota|balance|credit|余额|额度/.test(text)) return 'insufficient_quota'
  if (status === 401 || status === 403 || /unauthor|forbidden|api key|invalid_token/.test(text)) return 'unauthorized'
  if (status === 429 || /rate.?limit|too many requests/.test(text)) return 'rate_limited'
  if (/failed to fetch|networkerror|network error|network request failed|load failed|econnreset|econnrefused|err_internet_disconnected|offline/.test(text)) {
    return 'network'
  }
  if (/content.?policy|moderation|safety|审核|敏感/.test(text)) return 'content_policy'
  if (/timeout|timed out|超时/.test(text)) return 'timeout'
  if ((typeof status === 'number' && status >= 500) || /upstream|bad gateway|service unavailable/.test(text)) {
    return 'upstream'
  }
  return 'unknown'
}

export function modelId(model: { id?: unknown }): string {
  return String(model.id || '').trim()
}

export function safeRemoteImageURL(value: unknown): string {
  if (typeof value !== 'string' || !value.trim()) return ''
  try {
    const base = typeof window === 'undefined' ? 'https://localhost/' : window.location.origin
    const parsed = new URL(value, base)
    if (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') return ''
    return parsed.toString()
  } catch {
    return ''
  }
}

function safeBase64Image(value: unknown): string {
  if (typeof value !== 'string') return ''
  const normalized = value.trim()
  if (!normalized || normalized.length > 32 * 1024 * 1024 || !/^[A-Za-z0-9+/]+={0,2}$/.test(normalized)) return ''
  return normalized
}

export function extractImages(task: MediaStudioImageTask): MediaStudioGeneratedImagePreview[] {
  const images: MediaStudioGeneratedImagePreview[] = []
  const resultImages = Array.isArray(task.result?.data) ? task.result?.data ?? [] : []
  resultImages.forEach((item: MediaStudioGeneratedImage, index: number) => {
    const url = safeRemoteImageURL(item.url)
    const b64Json = safeBase64Image(item.b64_json)
    const src = url || (b64Json ? `data:image/png;base64,${b64Json}` : '')
    if (!src) return
    images.push({
      id: `${task.task_id || task.id}-image-${index}`,
      src,
      url: url || undefined,
      b64Json: b64Json || undefined,
      revisedPrompt: item.revised_prompt,
    })
  })
  if (images.length === 0) {
    const taskImageURL = safeRemoteImageURL(task.image_url)
    if (taskImageURL) {
      images.push({
        id: `${task.task_id || task.id}-image-url`,
        src: taskImageURL,
        url: taskImageURL,
      })
    }
  }
  return images
}

export function conversationForPersistence(value: MediaStudioConversation): MediaStudioConversation | null {
  const normalized = restorePersistedConversation({
    ...value,
    messages: value.messages.map(message => ({
      ...message,
      inputReferences: message.inputReferences?.flatMap(reference => {
        if (reference.cacheKey) return [{ ...reference, src: '', url: '' }]
        const remoteURL = safeRemoteImageURL(reference.url)
        return remoteURL ? [{ ...reference, src: remoteURL, url: remoteURL }] : []
      }),
      images: message.images?.map(image => image.cacheKey ? { ...image, src: '', b64Json: undefined } : image),
      inputImages: message.inputImages?.map(image => image.cacheKey ? { ...image, src: '' } : image),
    })),
  })
  if (!normalized) return null

  return {
    ...normalized,
    messages: normalized.messages.map((message) => ({
      ...message,
      inputReferences: message.inputReferences?.flatMap((reference) => {
        if (reference.cacheKey) return [{ ...reference, src: '', url: '' }]
        const remoteURL = safeRemoteImageURL(reference.url)
        return remoteURL ? [{ ...reference, src: remoteURL, url: remoteURL }] : []
      }),
      inputImages: message.inputImages?.flatMap((image): MediaStudioInputImagePreview[] => {
        const remoteURL = safeRemoteImageURL(image.src)
        if (remoteURL) {
          return [{ ...image, src: remoteURL }]
        }
        if (!image.cacheKey) return []
        return [{ ...image, src: '' }]
      }),
      images: message.images?.flatMap((image): MediaStudioGeneratedImagePreview[] => {
        const remoteURL = safeRemoteImageURL(image.url) || safeRemoteImageURL(image.src)
        if (remoteURL) {
          return [{
            id: image.id,
            src: remoteURL,
            url: remoteURL,
            revisedPrompt: image.revisedPrompt,
          }]
        }
        if (!image.cacheKey) return []
        return [{
          id: image.id,
          src: '',
          revisedPrompt: image.revisedPrompt,
          cacheKey: image.cacheKey,
        }]
      }),
    })),
  }
}

// 会话标题取自第一条用户提示词，视图层在为空时回退到本地化的“新对话”。
export function conversationTitle(messages: MediaStudioMessage[]): string {
  const prompt = messages.find(message => message.role === 'user' && message.prompt.trim())?.prompt ?? ''
  const normalized = prompt.replace(/\s+/g, ' ').trim()
  return normalized.length > 32 ? `${normalized.slice(0, 32)}…` : normalized
}

// 删除会话或清空对话时按消息维度回收 IndexedDB 素材，避免误删其它会话的缓存。
export function conversationCacheKeys(messages: MediaStudioMessage[]): { imageKeys: string[]; videoKeys: string[]; referenceKeys: string[] } {
  const imageKeys: string[] = []
  const videoKeys: string[] = []
  const referenceKeys: string[] = []
  for (const message of messages) {
    for (const image of message.images ?? []) {
      if (image.cacheKey) imageKeys.push(image.cacheKey)
    }
    for (const image of message.inputImages ?? []) {
      if (image.cacheKey) imageKeys.push(image.cacheKey)
    }
    if (message.video?.cacheKey) videoKeys.push(message.video.cacheKey)
    for (const reference of message.inputReferences ?? []) if (reference.cacheKey) referenceKeys.push(reference.cacheKey)
  }
  return { imageKeys, videoKeys, referenceKeys }
}

export function restorePersistedSessions(value: unknown): MediaStudioConversation[] {
  if (!Array.isArray(value)) return []
  return value
    .flatMap((item) => {
      const conversation = restorePersistedConversation(item)
      return conversation ? [conversation] : []
    })
    .slice(0, MEDIA_STUDIO_MAX_SESSIONS)
}

export function normalizeCount(value: number): number {
  if (!Number.isFinite(value)) return 1
  return Math.min(4, Math.max(1, Math.trunc(value)))
}

export function normalizeImageQuality(value: unknown): string {
  return IMAGE_QUALITY_OPTIONS.some(option => option.value === value) ? String(value) : 'auto'
}

export function normalizeImageResolution(value: unknown): MediaStudioImageResolution {
  return value === '2K' || value === '4K' ? value : IMAGE_RESOLUTION_DEFAULT
}

export function normalizeImageAspectRatio(value: unknown): MediaStudioImageAspectRatio {
  if (
    value === '3:2' ||
    value === '2:3' ||
    value === '4:3' ||
    value === '3:4' ||
    value === '16:9' ||
    value === '9:16'
  ) {
    return value
  }
  const customRatio = normalizeCustomImageAspectRatio(value)
  if (customRatio) return `custom:${customRatio}`
  return IMAGE_ASPECT_RATIO_DEFAULT
}

export function aspectRatioFromSize(value: unknown): MediaStudioImageAspectRatio {
  const match = typeof value === 'string' ? /^(\d+)x(\d+)$/.exec(value.trim()) : null
  if (!match) return IMAGE_ASPECT_RATIO_DEFAULT
  const width = Number(match[1])
  const height = Number(match[2])
  if (!width || !height) return IMAGE_ASPECT_RATIO_DEFAULT
  const ratio = width / height
  const candidates: Array<[MediaStudioImageAspectRatio, number]> = [
    ['1:1', 1],
    ['3:2', 1.5],
    ['2:3', 2 / 3],
    ['4:3', 4 / 3],
    ['3:4', 3 / 4],
    ['16:9', 16 / 9],
    ['9:16', 9 / 16],
  ]
  return candidates.reduce((best, candidate) => (
    Math.abs(ratio - candidate[1]) < Math.abs(ratio - best[1]) ? candidate : best
  ))[0]
}

function greatestCommonDivisor(left: number, right: number): number {
  let a = Math.abs(left)
  let b = Math.abs(right)
  while (b) {
    const remainder = a % b
    a = b
    b = remainder
  }
  return a || 1
}

export function normalizeCustomImageAspectRatio(value: unknown): string {
  const raw = typeof value === 'string' ? value.replace(/^custom:/i, '').trim() : ''
  const match = /^(\d{1,4}):(\d{1,4})$/.exec(raw)
  if (!match) return ''
  const ratioWidth = Number(match[1])
  const ratioHeight = Number(match[2])
  if (!ratioWidth || !ratioHeight || ratioWidth / ratioHeight > 3 || ratioHeight / ratioWidth > 3) return ''
  const divisor = greatestCommonDivisor(ratioWidth, ratioHeight)
  return `${ratioWidth / divisor}:${ratioHeight / divisor}`
}

function roundToImageMultiple(value: number): number {
  return Math.max(IMAGE_SIZE_MULTIPLE, Math.round(value / IMAGE_SIZE_MULTIPLE) * IMAGE_SIZE_MULTIPLE)
}

export function imageSizeFor(
  imageResolution: MediaStudioImageResolution,
  imageAspectRatio: MediaStudioImageAspectRatio,
): string {
  const ratio = imageAspectRatio.replace(/^custom:/, '')
  const [ratioWidth, ratioHeight] = ratio.split(':').map(Number)
  const maxEdge = IMAGE_RESOLUTION_MAX_EDGE[imageResolution]
  let width = ratioWidth >= ratioHeight
    ? maxEdge
    : roundToImageMultiple(maxEdge * ratioWidth / ratioHeight)
  let height = ratioHeight >= ratioWidth
    ? maxEdge
    : roundToImageMultiple(maxEdge * ratioHeight / ratioWidth)
  const scale = Math.min(
    1,
    Math.sqrt(IMAGE_MAX_PIXELS / (width * height)),
  )
  width = Math.max(IMAGE_SIZE_MULTIPLE, Math.floor((width * scale) / IMAGE_SIZE_MULTIPLE) * IMAGE_SIZE_MULTIPLE)
  height = Math.max(IMAGE_SIZE_MULTIPLE, Math.floor((height * scale) / IMAGE_SIZE_MULTIPLE) * IMAGE_SIZE_MULTIPLE)
  return `${width}x${height}`
}

export function normalizeDuration(
  value: number,
  range: { min: number; max: number } = { min: 1, max: 15 },
): number {
  const fallback = Math.min(range.max, Math.max(range.min, DEFAULT_VIDEO_DURATION))
  if (!Number.isFinite(value)) return fallback
  return Math.min(range.max, Math.max(range.min, Math.trunc(value)))
}

export function normalizeResolution(value: unknown): MediaStudioVideoResolution {
  return value === '480p' || value === '1080p' ? value : DEFAULT_VIDEO_RESOLUTION
}

export function normalizeVideoAspectRatio(value: unknown): MediaStudioVideoAspectRatio {
  return value === '9:16' || value === '1:1' || value === '4:3' || value === '3:4'
    ? value
    : DEFAULT_VIDEO_ASPECT_RATIO
}
