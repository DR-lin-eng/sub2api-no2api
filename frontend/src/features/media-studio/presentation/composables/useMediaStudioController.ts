import { computed, getCurrentInstance, onBeforeUnmount, ref, watch } from 'vue'
import {
  createMediaStudioSession,
  getAsyncImageTask,
  getMediaStudioConfig,
  getVideoGenerationContent,
  getVideoGenerationTask,
  listMediaStudioModels,
  mediaStudioErrorMessage,
  submitAsyncImageGeneration,
  submitImageEdit,
  submitImageGeneration,
  submitVideoGeneration,
  type MediaStudioConfig,
  type MediaStudioGroupOption,
  type MediaStudioMediaType,
  type MediaStudioGeneratedImage,
  type MediaStudioImageSubmitRequest,
  type MediaStudioImageTask,
  type MediaStudioModel,
  type MediaStudioVideoResolution,
  type MediaStudioVideoTask,
} from '@/features/media-studio/data/datasources/mediaStudioDatasource'
import {
  clearMediaStudioImages,
  deleteMediaStudioImages,
  loadMediaStudioImage,
  mediaStudioImageStorageKey,
  storeMediaStudioImage,
} from '@/features/media-studio/data/datasources/mediaStudioImageStore'
import {
  addMediaStudioImageAttachments,
  revokeMediaStudioImageAttachments,
  type MediaStudioImageAttachment,
} from '@/features/media-studio/presentation/composables/useMediaStudioAttachments'
import {
  useMediaStudioPreview,
  type MediaStudioModeId,
} from '@/features/media-studio/presentation/composables/useMediaStudioPreview'

const STORAGE_KEY = 'sub2api.mediaStudio.v2'
const LEGACY_STORAGE_KEY = 'sub2api.mediaStudio.v1'
const DEFAULT_IMAGE_MODEL = 'gpt-image-2'
const DEFAULT_VIDEO_MODEL = 'grok-imagine-video'
const DEFAULT_VIDEO_RESOLUTION: MediaStudioVideoResolution = '720p'
const DEFAULT_VIDEO_DURATION = 6
const MAX_POLL_COUNT = 240
const DEFAULT_POLL_DELAY_MS = 3000

export interface MediaStudioParameterOption {
  value: string
  label: string
}

export type MediaStudioImageResolution = '1K' | '2K' | '4K'
export type MediaStudioImageAspectRatio = '1:1' | '3:2' | '2:3' | '4:3' | '3:4' | '16:9' | '9:16' | `custom:${string}`

const IMAGE_RESOLUTION_DEFAULT: MediaStudioImageResolution = '1K'
const IMAGE_ASPECT_RATIO_DEFAULT: MediaStudioImageAspectRatio = '1:1'
const IMAGE_QUALITY_OPTIONS: MediaStudioParameterOption[] = [
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
}

export interface MediaStudioMessage {
  id: string
  role: 'user' | 'assistant'
  mode: 'image' | 'video'
  prompt: string
  status?: 'queued' | 'processing' | 'completed' | 'failed'
  taskId?: string
  model?: string
  size?: string
  imageResolution?: MediaStudioImageResolution
  imageAspectRatio?: MediaStudioImageAspectRatio
  quality?: string
  count?: number
  resolution?: MediaStudioVideoResolution
  duration?: number
  inputImages?: MediaStudioInputImagePreview[]
  images?: MediaStudioGeneratedImagePreview[]
  video?: MediaStudioGeneratedVideoPreview
  error?: string
  createdAt: number
  completedAt?: number
}

export interface MediaStudioConversation {
  id: string
  messages: MediaStudioMessage[]
  updatedAt: number
}

interface PersistedMediaStudioState {
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
  duration?: number
  conversation?: MediaStudioConversation
}

export interface MediaStudioControllerOptions {
  storage?: Storage
  pollDelayMs?: number
  maxPollCount?: number
  listModels?: typeof listMediaStudioModels
  submitImage?: typeof submitAsyncImageGeneration
  submitImageEdit?: typeof submitImageEdit
  getTask?: typeof getAsyncImageTask
  submitVideo?: typeof submitVideoGeneration
  getVideoTask?: typeof getVideoGenerationTask
  getVideoContent?: typeof getVideoGenerationContent
  getConfig?: typeof getMediaStudioConfig
  createSession?: typeof createMediaStudioSession
  createObjectURL?: (blob: Blob) => string
  revokeObjectURL?: (url: string) => void
}

function safeStorage(provided?: Storage): Storage | null {
  if (provided) return provided
  if (typeof window === 'undefined') return null
  return window.localStorage
}

function now(): number {
  return Date.now()
}

function makeId(prefix: string): string {
  const random = typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `${Date.now().toString(36)}${Math.random().toString(36).slice(2)}`
  return `${prefix}_${String(random).replace(/-/g, '')}`
}

function readPersisted(storage: Storage | null): PersistedMediaStudioState {
  if (!storage) return {}
  for (const key of [STORAGE_KEY, LEGACY_STORAGE_KEY]) {
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

function persistedImageSource(value: unknown): string {
  if (typeof value !== 'string' || !value.trim()) return ''
  const remoteURL = safeRemoteImageURL(value)
  if (remoteURL) return remoteURL

  const match = /^data:(image\/(?:png|jpeg|webp|gif));base64,(.+)$/i.exec(value.trim())
  if (!match || !safeBase64Image(match[2])) return ''
  return `data:${match[1].toLowerCase()};base64,${match[2]}`
}

function restorePersistedConversation(value: unknown): MediaStudioConversation | null {
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
      const hasPersistedImages = candidate.mode === 'image' &&
        Array.isArray(candidate.images) &&
        candidate.images.some((image) => {
          if (!image || typeof image !== 'object') return false
          const item = image as Partial<MediaStudioGeneratedImagePreview>
          return Boolean(persistedImageSource(item.src) || (typeof item.cacheKey === 'string' && item.cacheKey))
        })
      if (
        candidate.role === 'assistant' &&
        (candidate.status === 'queued' || candidate.status === 'processing') &&
        !hasPersistedImages
      ) {
        return []
      }

      const createdAt = typeof candidate.createdAt === 'number' && Number.isFinite(candidate.createdAt)
        ? candidate.createdAt
        : now()
      const restored: MediaStudioMessage = {
        id: candidate.id,
        role: candidate.role,
        mode: candidate.mode,
        prompt: candidate.prompt,
        status: candidate.status === 'queued' || candidate.status === 'processing'
          ? 'completed'
          : candidate.status,
        taskId: typeof candidate.taskId === 'string' ? candidate.taskId : undefined,
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
        duration: typeof candidate.duration === 'number' ? normalizeDuration(candidate.duration) : undefined,
        error: typeof candidate.error === 'string' ? candidate.error : undefined,
        createdAt,
        completedAt: typeof candidate.completedAt === 'number' ? candidate.completedAt : undefined,
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

function imageTaskErrorMessage(task: MediaStudioImageTask): string {
  const error = task.error
  if (!error) return 'image generation failed'
  if (typeof error === 'string') return error
  const message = (error as { message?: unknown }).message
  return typeof message === 'string' && message.trim() ? message : 'image generation failed'
}

function videoTaskErrorMessage(task: MediaStudioVideoTask): string {
  return task.error?.trim() || 'video generation failed'
}

function modelId(model: MediaStudioModel): string {
  return String(model.id || '').trim()
}

function safeRemoteImageURL(value: unknown): string {
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

function extractImages(task: MediaStudioImageTask): MediaStudioGeneratedImagePreview[] {
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

function mergeGeneratedImages(
  current: MediaStudioGeneratedImagePreview[],
  incoming: MediaStudioGeneratedImagePreview[],
): MediaStudioGeneratedImagePreview[] {
  return [...current, ...incoming]
}

function conversationForPersistence(value: MediaStudioConversation): MediaStudioConversation | null {
  const normalized = restorePersistedConversation(value)
  if (!normalized) return null

  return {
    ...normalized,
    messages: normalized.messages.map((message) => ({
      ...message,
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

async function cacheGeneratedImages(
  messageID: string,
  images: MediaStudioGeneratedImagePreview[],
): Promise<MediaStudioGeneratedImagePreview[]> {
  return Promise.all(images.map(async (image) => {
    if (safeRemoteImageURL(image.url) || safeRemoteImageURL(image.src)) return image
    const source = persistedImageSource(image.src)
    if (!source.startsWith('data:image/')) return image

    const cacheKey = image.cacheKey || mediaStudioImageStorageKey(messageID, image.id)
    const stored = await storeMediaStudioImage(cacheKey, source)
    return stored ? { ...image, cacheKey } : image
  }))
}

function readImageFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve) => {
    const reader = new FileReader()
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '')
    reader.onerror = () => resolve('')
    reader.readAsDataURL(file)
  })
}

async function imagePreviewToFile(image: MediaStudioGeneratedImagePreview): Promise<File> {
  const source = safeRemoteImageURL(image.url) || persistedImageSource(image.src)
  if (!source) throw new Error('image preview is unavailable')

  const response = await fetch(source)
  if (!response.ok) throw new Error(`failed to load image preview: ${response.status}`)
  const blob = await response.blob()
  const mimeType = blob.type.startsWith('image/') ? blob.type : 'image/png'
  const extension = mimeType.split('/')[1] || 'png'
  return new File([blob], `media-studio-edit.${extension}`, { type: mimeType })
}

function normalizeCount(value: number): number {
  if (!Number.isFinite(value)) return 1
  return Math.min(4, Math.max(1, Math.trunc(value)))
}

function normalizeImageQuality(value: unknown): string {
  return IMAGE_QUALITY_OPTIONS.some(option => option.value === value) ? String(value) : 'auto'
}

function normalizeImageResolution(value: unknown): MediaStudioImageResolution {
  return value === '2K' || value === '4K' ? value : IMAGE_RESOLUTION_DEFAULT
}

function normalizeImageAspectRatio(value: unknown): MediaStudioImageAspectRatio {
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

function aspectRatioFromSize(value: unknown): MediaStudioImageAspectRatio {
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

function normalizeCustomImageAspectRatio(value: unknown): string {
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

function imageSizeFor(
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

function normalizeDuration(value: number): number {
  if (!Number.isFinite(value)) return DEFAULT_VIDEO_DURATION
  return Math.min(15, Math.max(1, Math.trunc(value)))
}

function normalizeResolution(value: unknown): MediaStudioVideoResolution {
  return value === '480p' || value === '1080p' ? value : DEFAULT_VIDEO_RESOLUTION
}

function wait(delayMs: number): Promise<void> {
  return delayMs > 0 ? new Promise(resolve => setTimeout(resolve, delayMs)) : Promise.resolve()
}

export function useMediaStudioController(options: MediaStudioControllerOptions = {}) {
  const storage = safeStorage(options.storage)
  const persisted = readPersisted(storage)
  const data = {
    listModels: options.listModels ?? listMediaStudioModels,
    // Synchronous image generation works without object storage; tests can
    // inject the asynchronous endpoint to exercise the polling path.
    submitImage: options.submitImage ?? submitImageGeneration,
    submitImageEdit: options.submitImageEdit ?? submitImageEdit,
    getTask: options.getTask ?? getAsyncImageTask,
    submitVideo: options.submitVideo ?? submitVideoGeneration,
    getVideoTask: options.getVideoTask ?? getVideoGenerationTask,
    getVideoContent: options.getVideoContent ?? getVideoGenerationContent,
    getConfig: options.getConfig ?? getMediaStudioConfig,
    createSession: options.createSession ?? createMediaStudioSession,
  }
  const createObjectURL = options.createObjectURL ?? ((blob: Blob) => URL.createObjectURL(blob))
  const revokeObjectURL = options.revokeObjectURL ?? ((url: string) => URL.revokeObjectURL(url))
  const pollDelayMs = options.pollDelayMs ?? DEFAULT_POLL_DELAY_MS
  const maxPollCount = options.maxPollCount ?? MAX_POLL_COUNT

  const { modes: previewModes } = useMediaStudioPreview()
  const selectedModeId = ref<MediaStudioModeId>('image')
  const prompt = ref('')
  const selectedGroupId = ref<number>(persisted.selectedGroupId || 0)
  const imageModel = ref(persisted.imageModel || persisted.model || DEFAULT_IMAGE_MODEL)
  const videoModel = ref(persisted.videoModel || DEFAULT_VIDEO_MODEL)
  const model = ref(imageModel.value)
  const imageResolution = ref<MediaStudioImageResolution>(
    normalizeImageResolution(persisted.imageResolution),
  )
  const imageAspectRatio = ref<MediaStudioImageAspectRatio>(
    persisted.imageAspectRatio
      ? normalizeImageAspectRatio(persisted.imageAspectRatio)
      : aspectRatioFromSize(persisted.size),
  )
  const customImageAspectRatios = ref(
    Array.isArray(persisted.customImageAspectRatios)
      ? [...new Set(persisted.customImageAspectRatios.map(normalizeCustomImageAspectRatio).filter(Boolean))]
      : [],
  )
  const size = computed(() => imageSizeFor(imageResolution.value, imageAspectRatio.value))
  const quality = ref(normalizeImageQuality(persisted.quality))
  const count = ref(normalizeCount(persisted.count || 1))
  const resolution = ref<MediaStudioVideoResolution>(normalizeResolution(persisted.resolution))
  const duration = ref(normalizeDuration(persisted.duration || DEFAULT_VIDEO_DURATION))
  const groupConfig = ref<MediaStudioConfig>({ groups: [] })
  const loadingGroups = ref(false)
  const groupLoadError = ref('')
  const mediaStudioApiKey = ref('')
  const imageAttachments = ref<MediaStudioImageAttachment[]>([])
  const modelOptions = ref<string[]>([])
  const loadingModels = ref(false)
  const modelLoadError = ref('')
  const submitting = ref(false)
  const submitError = ref('')
  const conversation = ref<MediaStudioConversation>({
    ...(restorePersistedConversation(persisted.conversation) ?? {
      id: makeId('media_conversation'),
      messages: [],
      updatedAt: now(),
    }),
  })
  const pollingTaskIds = ref<string[]>([])
  const videoObjectURLs = new Set<string>()
  const imageInputFiles = new Map<string, File[]>()
  let disposed = false
  let modelRequestSequence = 0

  const modes = computed(() => previewModes.map((mode) => (
    mode.id === 'video'
      ? { ...mode, available: loadingGroups.value || groupConfig.value.groups.length > 0 }
      : mode
  )))
  const selectedMode = computed(() => modes.value.find(mode => mode.id === selectedModeId.value) ?? modes.value[0])
  const modelSelectionLocked = computed(() => (selectedGroup.value?.models?.length || 0) > 0)
  const imageQualityOptions = computed(() => IMAGE_QUALITY_OPTIONS)
  const groupOptions = computed<MediaStudioGroupOption[]>(() => {
    return groupConfig.value.groups
  })
  const selectedGroup = computed(() => groupOptions.value.find(group => group.group_id === selectedGroupId.value) ?? null)
  const hasMessages = computed(() => conversation.value.messages.length > 0)
  const canSubmit = computed(() => (
    !!selectedGroup.value &&
    !!mediaStudioApiKey.value &&
    !!prompt.value.trim() &&
    !submitting.value
  ))

  function updateImageAttachments(attachments: MediaStudioImageAttachment[]) {
    revokeMediaStudioImageAttachments(imageAttachments.value.filter(
      (current) => !attachments.some((next) => next.id === current.id),
    ))
    imageAttachments.value = attachments
  }

  async function editGeneratedImage(image: MediaStudioGeneratedImagePreview) {
    if (selectedModeId.value !== 'image') selectMode('image')
    submitError.value = ''
    try {
      const file = await imagePreviewToFile(image)
      const previousCount = imageAttachments.value.length
      const result = addMediaStudioImageAttachments(imageAttachments.value, [file])
      updateImageAttachments(result.attachments)
      if (result.attachments.length === previousCount) {
        throw new Error('reference image could not be added')
      }
    } catch (error) {
      submitError.value = mediaStudioErrorMessage(error, 'failed to prepare image edit')
    }
  }

  function addCustomImageAspectRatio(value: string) {
    const normalized = normalizeCustomImageAspectRatio(value)
    if (!normalized) return
    if (!customImageAspectRatios.value.includes(normalized)) {
      customImageAspectRatios.value = [...customImageAspectRatios.value, normalized]
    }
    imageAspectRatio.value = `custom:${normalized}`
  }

  function persist() {
    if (!storage) return
    const persistedConversation = conversationForPersistence(conversation.value)
    const payload: PersistedMediaStudioState = {
      selectedGroupId: selectedGroupId.value,
      imageModel: imageModel.value,
      videoModel: videoModel.value,
      size: size.value,
      imageResolution: imageResolution.value,
      imageAspectRatio: imageAspectRatio.value,
      customImageAspectRatios: customImageAspectRatios.value,
      quality: quality.value,
      count: count.value,
      resolution: resolution.value,
      duration: duration.value,
      conversation: persistedConversation || undefined,
    }
    try {
      storage.setItem(STORAGE_KEY, JSON.stringify(payload))
      storage.removeItem(LEGACY_STORAGE_KEY)
    } catch {
      // Ignore unavailable browser storage and keep the current session usable.
    }
  }

  async function hydratePersistedImages() {
    let changed = false
    const messages = await Promise.all(conversation.value.messages.map(async (message) => {
      const images = (await Promise.all((message.images || []).map(async (image) => {
        if (image.cacheKey) {
          const source = await loadMediaStudioImage(image.cacheKey)
          if (!persistedImageSource(source)) {
            changed = true
            return null
          }
          if (source !== image.src) changed = true
          return { ...image, src: source }
        }

        const source = persistedImageSource(image.src)
        if (!source.startsWith('data:image/')) return image
        const cacheKey = mediaStudioImageStorageKey(message.id, image.id)
        if (await storeMediaStudioImage(cacheKey, source)) {
          changed = true
          return { ...image, cacheKey }
        }
        return image
      }))).filter((image): image is MediaStudioGeneratedImagePreview => Boolean(image))

      const inputImages = (await Promise.all((message.inputImages || []).map(async (image) => {
        if (image.cacheKey) {
          const source = await loadMediaStudioImage(image.cacheKey)
          if (!persistedImageSource(source)) {
            changed = true
            return null
          }
          if (source !== image.src) changed = true
          return { ...image, src: source }
        }

        const source = persistedImageSource(image.src)
        if (!source.startsWith('data:image/')) return image
        const cacheKey = mediaStudioImageStorageKey(message.id, `input-${image.id}`)
        if (await storeMediaStudioImage(cacheKey, source)) {
          changed = true
          return { ...image, cacheKey }
        }
        return image
      }))).filter((image): image is MediaStudioInputImagePreview => Boolean(image))

      const imagesChanged = images.length !== (message.images || []).length ||
        images.some((image, index) => image !== message.images?.[index])
      const inputImagesChanged = inputImages.length !== (message.inputImages || []).length ||
        inputImages.some((image, index) => image !== message.inputImages?.[index])
      return !imagesChanged && !inputImagesChanged
        ? message
        : { ...message, images, inputImages }
    }))

    if (!disposed && changed) {
      conversation.value = { ...conversation.value, messages, updatedAt: now() }
    }
  }

  watch([selectedGroupId, imageModel, videoModel, size, imageResolution, imageAspectRatio, customImageAspectRatios, quality, count, resolution, duration], persist, { deep: true })
  watch(conversation, persist, { deep: true })
  watch(model, (value) => {
    if (selectedModeId.value === 'image') imageModel.value = value
    if (selectedModeId.value === 'video') videoModel.value = value
  })
  async function loadMediaGroups() {
    loadingGroups.value = true
    groupLoadError.value = ''
    try {
      groupConfig.value = await data.getConfig()
      const options = groupConfig.value.groups
      if (!options.some(group => group.group_id === selectedGroupId.value)) {
        selectedGroupId.value = options[0]?.group_id || 0
      }
      await establishMediaStudioSession()
    } catch (error) {
      groupLoadError.value = mediaStudioErrorMessage(error, 'Failed to load media studio groups')
    } finally {
      loadingGroups.value = false
    }
  }

  async function establishMediaStudioSession() {
    const mediaType = selectedModeId.value as MediaStudioMediaType
    const group = selectedGroup.value
    if (!group || (mediaType !== 'image' && mediaType !== 'video')) {
      mediaStudioApiKey.value = ''
      return
    }
    const session = await data.createSession(mediaType, group.group_id)
    mediaStudioApiKey.value = session.api_key
    await loadModels()
  }

  async function loadModels() {
    const requestedMode = selectedModeId.value
    const sequence = ++modelRequestSequence
    modelLoadError.value = ''
    modelOptions.value = []
    if (!mediaStudioApiKey.value) return
    loadingModels.value = true
    try {
      const response = await data.listModels(
        mediaStudioApiKey.value,
        requestedMode as MediaStudioMediaType,
        selectedGroupId.value,
      )
      if (sequence !== modelRequestSequence || requestedMode !== selectedModeId.value) return
      const seen = new Set<string>()
      modelOptions.value = response.data.map(modelId).filter((id) => {
        if (!id || seen.has(id)) return false
        seen.add(id)
        return true
      })
      const fallback = requestedMode === 'video' ? DEFAULT_VIDEO_MODEL : DEFAULT_IMAGE_MODEL
      if (modelOptions.value.length > 0 && !modelOptions.value.includes(model.value)) {
        model.value = modelOptions.value[0]
      } else if (!model.value.trim()) {
        model.value = fallback
      }
    } catch (error) {
      if (sequence !== modelRequestSequence || requestedMode !== selectedModeId.value) return
      modelLoadError.value = mediaStudioErrorMessage(error, 'Failed to load models')
      if (!model.value.trim()) model.value = requestedMode === 'video' ? DEFAULT_VIDEO_MODEL : DEFAULT_IMAGE_MODEL
    } finally {
      if (sequence === modelRequestSequence) loadingModels.value = false
    }
  }

  function selectMode(id: MediaStudioModeId) {
    const nextMode = modes.value.find(mode => mode.id === id) ?? modes.value[0]
    if (!nextMode.available || id === selectedModeId.value) return
    if (selectedModeId.value === 'image') imageModel.value = model.value
    if (selectedModeId.value === 'video') videoModel.value = model.value
    selectedModeId.value = id
    model.value = id === 'video' ? videoModel.value : imageModel.value
    submitError.value = ''
    modelLoadError.value = ''
    modelOptions.value = []
    mediaStudioApiKey.value = ''
    void loadMediaGroups()
  }

  function patchAssistantMessage(messageId: string, patch: Partial<MediaStudioMessage>): boolean {
    const messages = conversation.value.messages
    const index = messages.findIndex(message => message.id === messageId)
    if (index < 0) return false
    messages[index] = { ...messages[index], ...patch }
    conversation.value = { ...conversation.value, messages: [...messages], updatedAt: now() }
    return true
  }

  async function cacheInputImages(assistantMessageID: string, attachments: MediaStudioImageAttachment[]) {
    const assistantIndex = conversation.value.messages.findIndex(message => message.id === assistantMessageID)
    const userIndex = assistantIndex > 0 ? assistantIndex - 1 : -1
    const userMessage = userIndex >= 0 ? conversation.value.messages[userIndex] : undefined
    if (!userMessage || userMessage.role !== 'user' || !userMessage.inputImages?.length) return

    const cached = await Promise.all(userMessage.inputImages.map(async (image) => {
      const attachment = attachments.find(item => item.id === image.id)
      if (!attachment) return image
      const source = await readImageFileAsDataURL(attachment.file)
      if (!source) return image
      const cacheKey = mediaStudioImageStorageKey(userMessage.id, image.id)
      return await storeMediaStudioImage(cacheKey, source) ? { ...image, cacheKey } : image
    }))

    if (disposed) return
    const messages = [...conversation.value.messages]
    const currentUser = messages[userIndex]
    if (!currentUser || currentUser.id !== userMessage.id) return
    messages[userIndex] = { ...currentUser, inputImages: cached }
    conversation.value = { ...conversation.value, messages, updatedAt: now() }
  }

  function setPolling(taskId: string, active: boolean) {
    if (active && !pollingTaskIds.value.includes(taskId)) pollingTaskIds.value = [...pollingTaskIds.value, taskId]
    if (!active) pollingTaskIds.value = pollingTaskIds.value.filter(id => id !== taskId)
  }

  async function pollImageTask(apiKey: string, taskId: string): Promise<MediaStudioImageTask> {
    setPolling(taskId, true)
    try {
      for (let attempt = 0; attempt < maxPollCount && !disposed; attempt += 1) {
        const task = await data.getTask(apiKey, taskId)
        if (task.status === 'completed') {
          return task
        }
        if (task.status === 'failed') {
          return task
        }
        await wait(pollDelayMs)
      }
      throw new Error('image generation polling timed out')
    } finally {
      setPolling(taskId, false)
    }
  }

  async function completeVideoTask(apiKey: string, taskId: string, messageId: string) {
    const blob = await data.getVideoContent(apiKey, taskId)
    if (disposed) return
    const src = createObjectURL(blob)
    videoObjectURLs.add(src)
    if (!patchAssistantMessage(messageId, {
      status: 'completed',
      video: { src, mimeType: blob.type || 'video/mp4' },
      error: '',
      completedAt: now(),
    })) {
      videoObjectURLs.delete(src)
      revokeObjectURL(src)
    }
  }

  async function pollVideoTask(apiKey: string, taskId: string, messageId: string) {
    setPolling(taskId, true)
    try {
      for (let attempt = 0; attempt < maxPollCount && !disposed; attempt += 1) {
        const task = await data.getVideoTask(apiKey, taskId)
        if (task.status === 'completed') {
          await completeVideoTask(apiKey, task.id, messageId)
          return
        }
        if (task.status === 'failed') {
          patchAssistantMessage(messageId, { status: 'failed', error: videoTaskErrorMessage(task), completedAt: now() })
          return
        }
        patchAssistantMessage(messageId, { status: 'processing' })
        await wait(pollDelayMs)
      }
      if (!disposed) patchAssistantMessage(messageId, { status: 'failed', error: 'video generation polling timed out', completedAt: now() })
    } catch (error) {
      if (!disposed) patchAssistantMessage(messageId, { status: 'failed', error: mediaStudioErrorMessage(error, 'video generation failed'), completedAt: now() })
    } finally {
      setPolling(taskId, false)
    }
  }

  function appendPromptMessages(
    text: string,
    mode: 'image' | 'video',
    inputAttachments: MediaStudioImageAttachment[] = [],
  ): MediaStudioMessage {
    const userMessage: MediaStudioMessage = {
      id: makeId('media_user'),
      role: 'user',
      mode,
      prompt: text,
      inputImages: inputAttachments.map(attachment => ({
        id: attachment.id,
        src: attachment.previewUrl,
        name: attachment.name,
        mimeType: attachment.mimeType,
      })),
      createdAt: now(),
    }
    const assistantMessage: MediaStudioMessage = {
      id: makeId('media_assistant'),
      role: 'assistant',
      mode,
      prompt: text,
      status: 'queued',
      model: model.value.trim() || (mode === 'video' ? DEFAULT_VIDEO_MODEL : DEFAULT_IMAGE_MODEL),
      size: mode === 'image' ? size.value : undefined,
      imageResolution: mode === 'image' ? imageResolution.value : undefined,
      imageAspectRatio: mode === 'image' ? imageAspectRatio.value : undefined,
      quality: mode === 'image' ? quality.value : undefined,
      count: mode === 'image' ? normalizeCount(count.value) : 1,
      resolution: mode === 'video' ? normalizeResolution(resolution.value) : undefined,
      duration: mode === 'video' ? normalizeDuration(duration.value) : undefined,
      createdAt: now(),
    }
    conversation.value = {
      ...conversation.value,
      messages: [...conversation.value.messages, userMessage, assistantMessage],
      updatedAt: now(),
    }
    return assistantMessage
  }

  async function submitPrompt() {
    const text = prompt.value.trim().slice(0, 10_000)
    const mode = selectedModeId.value
    submitError.value = ''
    if (!mediaStudioApiKey.value || !selectedGroup.value || !text || (mode !== 'image' && mode !== 'video')) return

    const submittedAttachments = mode === 'image' ? [...imageAttachments.value] : []
    const assistantMessage = appendPromptMessages(text, mode, submittedAttachments)
    if (submittedAttachments.length > 0) {
      imageInputFiles.set(assistantMessage.id, submittedAttachments.map(attachment => attachment.file))
      void cacheInputImages(assistantMessage.id, submittedAttachments)
    }
    prompt.value = ''
    imageAttachments.value = []
    submitting.value = true
    try {
      if (mode === 'image') {
        const requestedCount = normalizeCount(assistantMessage.count || 1)
        const payload: MediaStudioImageSubmitRequest = {
          model: assistantMessage.model || DEFAULT_IMAGE_MODEL,
          prompt: text,
          n: requestedCount,
          size: imageSizeFor(
            normalizeImageResolution(assistantMessage.imageResolution),
            normalizeImageAspectRatio(assistantMessage.imageAspectRatio),
          ),
        }
        if (assistantMessage.quality) payload.quality = assistantMessage.quality

        const submitImageTask = (requestPayload: MediaStudioImageSubmitRequest) => (
          submittedAttachments.length > 0
            ? data.submitImageEdit(
              mediaStudioApiKey.value,
              submittedAttachments.map((attachment) => attachment.file),
              requestPayload,
              makeId('media_idem'),
            )
            : data.submitImage(mediaStudioApiKey.value, requestPayload, makeId('media_idem'))
        )

        patchAssistantMessage(assistantMessage.id, {
          status: 'processing',
          images: [],
        })

        let settledCount = 0
        let firstTaskID = ''
        let lastError = ''
        const submitSingleImage = async () => {
          try {
            let task = await submitImageTask({ ...payload, n: 1 })
            const taskID = task.task_id || task.id
            if (!firstTaskID) {
              firstTaskID = taskID
              patchAssistantMessage(assistantMessage.id, { taskId: taskID })
            }
            if (task.status !== 'completed' && task.status !== 'failed') {
              task = await pollImageTask(mediaStudioApiKey.value, taskID)
            }
            if (task.status === 'completed') {
              const current = conversation.value.messages.find(message => message.id === assistantMessage.id)?.images ?? []
              const generated = await cacheGeneratedImages(assistantMessage.id, extractImages(task))
              patchAssistantMessage(assistantMessage.id, {
                images: mergeGeneratedImages(current, generated),
              })
            } else {
              lastError = imageTaskErrorMessage(task)
            }
          } catch (error) {
            lastError = mediaStudioErrorMessage(error, 'image generation failed')
          } finally {
            settledCount += 1
            const currentImages = conversation.value.messages.find(message => message.id === assistantMessage.id)?.images ?? []
            if (settledCount === requestedCount) {
              patchAssistantMessage(assistantMessage.id, {
                status: currentImages.length > 0 ? 'completed' : 'failed',
                error: currentImages.length > 0 ? '' : lastError,
                completedAt: now(),
              })
            } else {
              patchAssistantMessage(assistantMessage.id, { status: 'processing' })
            }
          }
        }
        await Promise.all(Array.from({ length: requestedCount }, () => submitSingleImage()))
      } else {
        const task = await data.submitVideo(mediaStudioApiKey.value, {
          model: assistantMessage.model || DEFAULT_VIDEO_MODEL,
          prompt: text,
          resolution: assistantMessage.resolution || DEFAULT_VIDEO_RESOLUTION,
          duration: assistantMessage.duration || DEFAULT_VIDEO_DURATION,
        }, makeId('media_video_idem'))
        patchAssistantMessage(assistantMessage.id, { taskId: task.id, status: task.status === 'failed' ? 'failed' : 'processing' })
        if (task.status === 'failed') {
          patchAssistantMessage(assistantMessage.id, { error: videoTaskErrorMessage(task), completedAt: now() })
        } else if (task.status === 'completed') {
          await completeVideoTask(mediaStudioApiKey.value, task.id, assistantMessage.id)
        } else {
          void pollVideoTask(mediaStudioApiKey.value, task.id, assistantMessage.id)
        }
      }
    } catch (error) {
      const fallback = mode === 'video' ? 'video generation failed' : 'image generation failed'
      const message = mediaStudioErrorMessage(error, fallback)
      submitError.value = message
      patchAssistantMessage(assistantMessage.id, { status: 'failed', error: message, completedAt: now() })
    } finally {
      submitting.value = false
    }
  }

  async function retryMessage(message: MediaStudioMessage) {
    if (!message.prompt.trim()) return
    const modeChanged = selectedModeId.value !== message.mode
    selectMode(message.mode)
    if (modeChanged) await loadMediaGroups()
    model.value = message.model || (message.mode === 'video' ? DEFAULT_VIDEO_MODEL : DEFAULT_IMAGE_MODEL)
    if (message.mode === 'image') {
      imageResolution.value = normalizeImageResolution(message.imageResolution)
      imageAspectRatio.value = normalizeImageAspectRatio(message.imageAspectRatio || aspectRatioFromSize(message.size))
      quality.value = normalizeImageQuality(message.quality)
      count.value = normalizeCount(message.count || 1)
      const restored = addMediaStudioImageAttachments([], imageInputFiles.get(message.id) ?? []).attachments
      imageAttachments.value = restored
    } else {
      resolution.value = normalizeResolution(message.resolution)
      duration.value = normalizeDuration(message.duration || DEFAULT_VIDEO_DURATION)
    }
    prompt.value = message.prompt
    await submitPrompt()
  }

  function revokeAllVideoURLs() {
    for (const url of videoObjectURLs) revokeObjectURL(url)
    videoObjectURLs.clear()
  }

  function revokeInputImageURLs(messages: MediaStudioMessage[]) {
    for (const message of messages) {
      for (const image of message.inputImages || []) {
        if (image.src.startsWith('blob:')) URL.revokeObjectURL(image.src)
      }
    }
  }

  function clearConversation() {
    revokeAllVideoURLs()
    revokeInputImageURLs(conversation.value.messages)
    imageInputFiles.clear()
    void clearMediaStudioImages()
    conversation.value = { id: makeId('media_conversation'), messages: [], updatedAt: now() }
    persist()
  }

  async function deleteMessages(messageIDs: string[]) {
    const ids = new Set(messageIDs.filter(Boolean))
    if (ids.size === 0) return

    const imageKeys: string[] = []
    for (const message of conversation.value.messages) {
      if (!ids.has(message.id)) continue
      if (message.video?.src && videoObjectURLs.has(message.video.src)) {
        URL.revokeObjectURL(message.video.src)
        videoObjectURLs.delete(message.video.src)
      }
      imageKeys.push(...(message.images || []).flatMap(image => image.cacheKey ? [image.cacheKey] : []))
      imageKeys.push(...(message.inputImages || []).flatMap(image => image.cacheKey ? [image.cacheKey] : []))
      revokeInputImageURLs([message])
      imageInputFiles.delete(message.id)
    }

    conversation.value = {
      ...conversation.value,
      messages: conversation.value.messages.filter(message => !ids.has(message.id)),
      updatedAt: now(),
    }
    await deleteMediaStudioImages(imageKeys)
  }

  watch(selectedGroupId, () => {
    void establishMediaStudioSession()
  })

  void hydratePersistedImages()

  if (getCurrentInstance()) {
    onBeforeUnmount(() => {
      disposed = true
      revokeAllVideoURLs()
      revokeInputImageURLs(conversation.value.messages)
    })
  }

  return {
    modes,
    selectedMode,
    selectedModeId,
    prompt,
    selectedGroupId,
    selectedGroup,
    groupOptions,
    loadingGroups,
    groupLoadError,
    mediaStudioApiKey,
    imageAttachments,
    updateImageAttachments,
    customImageAspectRatios,
    addCustomImageAspectRatio,
    model,
    modelSelectionLocked,
    imageResolution,
    imageAspectRatio,
    size,
    quality,
    count,
    resolution,
    duration,
    imageQualityOptions,
    modelOptions,
    loadingModels,
    modelLoadError,
    submitting,
    submitError,
    conversation,
    hasMessages,
    canSubmit,
    pollingTaskIds,
    loadMediaGroups,
    loadModels,
    selectMode,
    submitPrompt,
    retryMessage,
    editGeneratedImage,
    clearConversation,
    deleteMessages,
  }
}

export type MediaStudioController = ReturnType<typeof useMediaStudioController>
