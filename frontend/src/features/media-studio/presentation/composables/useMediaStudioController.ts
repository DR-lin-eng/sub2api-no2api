import { computed, getCurrentInstance, onBeforeUnmount, ref, watch } from 'vue'
import { keysAPI } from '@/features/keys/data/datasources/keysDatasource'
import type { ApiKey, PaginatedResponse } from '@/types'
import {
  getAsyncImageTask,
  getVideoGenerationContent,
  getVideoGenerationTask,
  isMediaStudioImageModel,
  isMediaStudioVideoModel,
  listMediaStudioModels,
  mediaStudioErrorMessage,
  submitAsyncImageGeneration,
  submitImageGeneration,
  submitVideoGeneration,
  type MediaStudioGeneratedImage,
  type MediaStudioImageSubmitRequest,
  type MediaStudioImageTask,
  type MediaStudioModel,
  type MediaStudioVideoResolution,
  type MediaStudioVideoTask,
} from '@/features/media-studio/data/datasources/mediaStudioDatasource'
import {
  useMediaStudioPreview,
  type MediaStudioModeId,
} from '@/features/media-studio/presentation/composables/useMediaStudioPreview'

const STORAGE_KEY = 'sub2api.mediaStudio.v2'
const LEGACY_STORAGE_KEY = 'sub2api.mediaStudio.v1'
const DEFAULT_IMAGE_MODEL = 'gpt-image-2'
const DEFAULT_VIDEO_MODEL = 'grok-imagine-video'
const DEFAULT_SIZE = '1024x1024'
const DEFAULT_VIDEO_RESOLUTION: MediaStudioVideoResolution = '720p'
const DEFAULT_VIDEO_DURATION = 6
const MAX_POLL_COUNT = 240
const DEFAULT_POLL_DELAY_MS = 3000

export interface MediaStudioGeneratedImagePreview {
  id: string
  src: string
  url?: string
  b64Json?: string
  revisedPrompt?: string
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
  quality?: string
  count?: number
  resolution?: MediaStudioVideoResolution
  duration?: number
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
  selectedApiKeyId?: number
  model?: string
  imageModel?: string
  videoModel?: string
  size?: string
  quality?: string
  count?: number
  resolution?: MediaStudioVideoResolution
  duration?: number
}

export interface MediaStudioControllerOptions {
  storage?: Storage
  pollDelayMs?: number
  maxPollCount?: number
  listModels?: typeof listMediaStudioModels
  submitImage?: typeof submitAsyncImageGeneration
  getTask?: typeof getAsyncImageTask
  submitVideo?: typeof submitVideoGeneration
  getVideoTask?: typeof getVideoGenerationTask
  getVideoContent?: typeof getVideoGenerationContent
  listKeys?: typeof keysAPI.list
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
  const taskImageURL = safeRemoteImageURL(task.image_url)
  if (taskImageURL) {
    images.push({
      id: `${task.task_id || task.id}-image-url`,
      src: taskImageURL,
      url: taskImageURL,
    })
  }
  const resultImages = Array.isArray(task.result?.data) ? task.result?.data ?? [] : []
  resultImages.forEach((item: MediaStudioGeneratedImage, index: number) => {
    const url = safeRemoteImageURL(item.url)
    const b64Json = safeBase64Image(item.b64_json)
    const src = url || (b64Json ? `data:image/png;base64,${b64Json}` : '')
    if (!src || images.some(image => image.src === src)) return
    images.push({
      id: `${task.task_id || task.id}-image-${index}`,
      src,
      url: url || undefined,
      b64Json: b64Json || undefined,
      revisedPrompt: item.revised_prompt,
    })
  })
  return images
}

function normalizeCount(value: number): number {
  if (!Number.isFinite(value)) return 1
  return Math.min(4, Math.max(1, Math.trunc(value)))
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
    getTask: options.getTask ?? getAsyncImageTask,
    submitVideo: options.submitVideo ?? submitVideoGeneration,
    getVideoTask: options.getVideoTask ?? getVideoGenerationTask,
    getVideoContent: options.getVideoContent ?? getVideoGenerationContent,
    listKeys: options.listKeys ?? keysAPI.list,
  }
  const createObjectURL = options.createObjectURL ?? ((blob: Blob) => URL.createObjectURL(blob))
  const revokeObjectURL = options.revokeObjectURL ?? ((url: string) => URL.revokeObjectURL(url))
  const pollDelayMs = options.pollDelayMs ?? DEFAULT_POLL_DELAY_MS
  const maxPollCount = options.maxPollCount ?? MAX_POLL_COUNT

  const { modes, getModeById } = useMediaStudioPreview()
  const selectedModeId = ref<MediaStudioModeId>('image')
  const prompt = ref('')
  const selectedApiKeyId = ref<number>(persisted.selectedApiKeyId || 0)
  const imageModel = ref(persisted.imageModel || persisted.model || DEFAULT_IMAGE_MODEL)
  const videoModel = ref(persisted.videoModel || DEFAULT_VIDEO_MODEL)
  const model = ref(imageModel.value)
  const size = ref(persisted.size || DEFAULT_SIZE)
  const quality = ref(persisted.quality || 'auto')
  const count = ref(normalizeCount(persisted.count || 1))
  const resolution = ref<MediaStudioVideoResolution>(normalizeResolution(persisted.resolution))
  const duration = ref(normalizeDuration(persisted.duration || DEFAULT_VIDEO_DURATION))
  const apiKeys = ref<ApiKey[]>([])
  const loadingKeys = ref(false)
  const apiKeyLoadError = ref('')
  const modelOptions = ref<string[]>([])
  const loadingModels = ref(false)
  const modelLoadError = ref('')
  const submitting = ref(false)
  const submitError = ref('')
  const conversation = ref<MediaStudioConversation>({
    id: makeId('media_conversation'),
    messages: [],
    updatedAt: now(),
  })
  const pollingTaskIds = ref<string[]>([])
  const videoObjectURLs = new Set<string>()
  let disposed = false
  let modelRequestSequence = 0

  const selectedMode = computed(() => getModeById(selectedModeId.value))
  const selectedApiKey = computed(() => apiKeys.value.find(key => key.id === selectedApiKeyId.value) ?? null)
  const hasMessages = computed(() => conversation.value.messages.length > 0)
  const canSubmit = computed(() => (
    (selectedModeId.value === 'image' || selectedModeId.value === 'video') &&
    !!selectedApiKey.value &&
    !!prompt.value.trim() &&
    !submitting.value
  ))

  function persist() {
    if (!storage) return
    const payload: PersistedMediaStudioState = {
      selectedApiKeyId: selectedApiKeyId.value,
      imageModel: imageModel.value,
      videoModel: videoModel.value,
      size: size.value,
      quality: quality.value,
      count: count.value,
      resolution: resolution.value,
      duration: duration.value,
    }
    try {
      storage.setItem(STORAGE_KEY, JSON.stringify(payload))
      storage.removeItem(LEGACY_STORAGE_KEY)
    } catch {
      // Generated media is never persisted; ignore unavailable small storage.
    }
  }

  watch([selectedApiKeyId, imageModel, videoModel, size, quality, count, resolution, duration], persist)
  watch(model, (value) => {
    if (selectedModeId.value === 'image') imageModel.value = value
    if (selectedModeId.value === 'video') videoModel.value = value
  })

  async function loadApiKeys() {
    loadingKeys.value = true
    apiKeyLoadError.value = ''
    try {
      const firstPage = await data.listKeys(1, 100, { status: 'active' })
      const pages: PaginatedResponse<ApiKey>[] = [firstPage]
      const pendingPages = Array.from({ length: Math.max(0, firstPage.pages - 1) }, (_, index) => index + 2)
      for (let index = 0; index < pendingPages.length; index += 4) {
        const batch = pendingPages.slice(index, index + 4)
        pages.push(...await Promise.all(batch.map(page => data.listKeys(page, 100, { status: 'active' }))))
      }
      apiKeys.value = pages.flatMap(page => page.items)
      if (!selectedApiKey.value && apiKeys.value.length > 0) selectedApiKeyId.value = apiKeys.value[0].id
      await loadModels()
    } catch (error) {
      apiKeyLoadError.value = mediaStudioErrorMessage(error, 'Failed to load API keys')
    } finally {
      loadingKeys.value = false
    }
  }

  async function loadModels() {
    const key = selectedApiKey.value
    const requestedMode = selectedModeId.value
    const sequence = ++modelRequestSequence
    modelLoadError.value = ''
    modelOptions.value = []
    if (!key || requestedMode === 'batch') return
    loadingModels.value = true
    try {
      const response = await data.listModels(key.key)
      if (sequence !== modelRequestSequence || requestedMode !== selectedModeId.value) return
      const predicate = requestedMode === 'video' ? isMediaStudioVideoModel : isMediaStudioImageModel
      const seen = new Set<string>()
      modelOptions.value = response.data.map(modelId).filter((id) => {
        if (!id || seen.has(id) || !predicate(id)) return false
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
    const nextMode = getModeById(id)
    if (!nextMode.available || id === selectedModeId.value) return
    if (selectedModeId.value === 'image') imageModel.value = model.value
    if (selectedModeId.value === 'video') videoModel.value = model.value
    selectedModeId.value = id
    model.value = id === 'video' ? videoModel.value : imageModel.value
    submitError.value = ''
    modelLoadError.value = ''
    modelOptions.value = []
    if (id !== 'batch') void loadModels()
  }

  function patchAssistantMessage(messageId: string, patch: Partial<MediaStudioMessage>): boolean {
    const messages = conversation.value.messages
    const index = messages.findIndex(message => message.id === messageId)
    if (index < 0) return false
    messages[index] = { ...messages[index], ...patch }
    conversation.value = { ...conversation.value, messages: [...messages], updatedAt: now() }
    return true
  }

  function setPolling(taskId: string, active: boolean) {
    if (active && !pollingTaskIds.value.includes(taskId)) pollingTaskIds.value = [...pollingTaskIds.value, taskId]
    if (!active) pollingTaskIds.value = pollingTaskIds.value.filter(id => id !== taskId)
  }

  async function pollImageTask(apiKey: string, taskId: string, messageId: string) {
    setPolling(taskId, true)
    try {
      for (let attempt = 0; attempt < maxPollCount && !disposed; attempt += 1) {
        const task = await data.getTask(apiKey, taskId)
        if (task.status === 'completed') {
          patchAssistantMessage(messageId, {
            status: 'completed',
            images: extractImages(task),
            completedAt: (task.completed_at || Math.floor(now() / 1000)) * 1000,
            error: '',
          })
          return
        }
        if (task.status === 'failed') {
          patchAssistantMessage(messageId, {
            status: 'failed',
            error: imageTaskErrorMessage(task),
            completedAt: (task.completed_at || Math.floor(now() / 1000)) * 1000,
          })
          return
        }
        patchAssistantMessage(messageId, { status: 'processing' })
        await wait(pollDelayMs)
      }
      if (!disposed) patchAssistantMessage(messageId, { status: 'failed', error: 'image generation polling timed out', completedAt: now() })
    } catch (error) {
      if (!disposed) patchAssistantMessage(messageId, { status: 'failed', error: mediaStudioErrorMessage(error, 'image generation failed'), completedAt: now() })
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

  function appendPromptMessages(text: string, mode: 'image' | 'video'): MediaStudioMessage {
    const userMessage: MediaStudioMessage = { id: makeId('media_user'), role: 'user', mode, prompt: text, createdAt: now() }
    const assistantMessage: MediaStudioMessage = {
      id: makeId('media_assistant'),
      role: 'assistant',
      mode,
      prompt: text,
      status: 'queued',
      model: model.value.trim() || (mode === 'video' ? DEFAULT_VIDEO_MODEL : DEFAULT_IMAGE_MODEL),
      size: mode === 'image' ? size.value : undefined,
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
    const key = selectedApiKey.value
    const text = prompt.value.trim().slice(0, 10_000)
    const mode = selectedModeId.value
    submitError.value = ''
    if (!key || !text || (mode !== 'image' && mode !== 'video')) return

    const assistantMessage = appendPromptMessages(text, mode)
    prompt.value = ''
    submitting.value = true
    try {
      if (mode === 'image') {
        const payload: MediaStudioImageSubmitRequest = {
          model: assistantMessage.model || DEFAULT_IMAGE_MODEL,
          prompt: text,
          n: assistantMessage.count,
          size: assistantMessage.size,
          response_format: 'b64_json',
        }
        if (assistantMessage.quality && assistantMessage.quality !== 'auto') payload.quality = assistantMessage.quality
        const task = await data.submitImage(key.key, payload, makeId('media_idem'))
        const taskID = task.task_id || task.id
        patchAssistantMessage(assistantMessage.id, {
          taskId: taskID,
          status: task.status === 'completed' ? 'completed' : 'processing',
          images: task.status === 'completed' ? extractImages(task) : [],
          completedAt: task.status === 'completed' ? now() : undefined,
        })
        if (task.status === 'failed') {
          patchAssistantMessage(assistantMessage.id, { status: 'failed', error: imageTaskErrorMessage(task), completedAt: now() })
        } else if (task.status !== 'completed') {
          void pollImageTask(key.key, taskID, assistantMessage.id)
        }
      } else {
        const task = await data.submitVideo(key.key, {
          model: assistantMessage.model || DEFAULT_VIDEO_MODEL,
          prompt: text,
          resolution: assistantMessage.resolution || DEFAULT_VIDEO_RESOLUTION,
          duration: assistantMessage.duration || DEFAULT_VIDEO_DURATION,
        }, makeId('media_video_idem'))
        patchAssistantMessage(assistantMessage.id, { taskId: task.id, status: task.status === 'failed' ? 'failed' : 'processing' })
        if (task.status === 'failed') {
          patchAssistantMessage(assistantMessage.id, { error: videoTaskErrorMessage(task), completedAt: now() })
        } else if (task.status === 'completed') {
          await completeVideoTask(key.key, task.id, assistantMessage.id)
        } else {
          void pollVideoTask(key.key, task.id, assistantMessage.id)
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

  function retryMessage(message: MediaStudioMessage) {
    if (!message.prompt.trim()) return
    selectMode(message.mode)
    model.value = message.model || (message.mode === 'video' ? DEFAULT_VIDEO_MODEL : DEFAULT_IMAGE_MODEL)
    if (message.mode === 'image') {
      size.value = message.size || DEFAULT_SIZE
      quality.value = message.quality || 'auto'
      count.value = normalizeCount(message.count || 1)
    } else {
      resolution.value = normalizeResolution(message.resolution)
      duration.value = normalizeDuration(message.duration || DEFAULT_VIDEO_DURATION)
    }
    prompt.value = message.prompt
    void submitPrompt()
  }

  function revokeAllVideoURLs() {
    for (const url of videoObjectURLs) revokeObjectURL(url)
    videoObjectURLs.clear()
  }

  function clearConversation() {
    revokeAllVideoURLs()
    conversation.value = { id: makeId('media_conversation'), messages: [], updatedAt: now() }
  }

  watch(selectedApiKeyId, () => {
    void loadModels()
  })

  if (getCurrentInstance()) {
    onBeforeUnmount(() => {
      disposed = true
      revokeAllVideoURLs()
    })
  }

  return {
    modes,
    selectedMode,
    selectedModeId,
    prompt,
    selectedApiKeyId,
    selectedApiKey,
    model,
    size,
    quality,
    count,
    resolution,
    duration,
    apiKeys,
    loadingKeys,
    apiKeyLoadError,
    modelOptions,
    loadingModels,
    modelLoadError,
    submitting,
    submitError,
    conversation,
    hasMessages,
    canSubmit,
    pollingTaskIds,
    loadApiKeys,
    loadModels,
    selectMode,
    submitPrompt,
    retryMessage,
    clearConversation,
  }
}

export type MediaStudioController = ReturnType<typeof useMediaStudioController>
