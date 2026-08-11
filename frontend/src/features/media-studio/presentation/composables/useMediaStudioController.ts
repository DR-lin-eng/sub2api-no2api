import { computed, getCurrentInstance, onBeforeUnmount, ref, watch } from 'vue'
import { keysAPI } from '@/features/keys/data/datasources/keysDatasource'
import type { ApiKey, PaginatedResponse } from '@/types'
import {
  getAsyncImageTask,
  isMediaStudioImageModel,
  listMediaStudioModels,
  mediaStudioErrorMessage,
  submitAsyncImageGeneration,
  submitImageGeneration,
  type MediaStudioGeneratedImage,
  type MediaStudioImageSubmitRequest,
  type MediaStudioImageTask,
  type MediaStudioModel,
} from '@/features/media-studio/data/datasources/mediaStudioDatasource'
import { useMediaStudioPreview, type MediaStudioModeId } from '@/features/media-studio/presentation/composables/useMediaStudioPreview'

const STORAGE_KEY = 'sub2api.mediaStudio.v1'
const DEFAULT_MODEL = 'gpt-image-2'
const DEFAULT_SIZE = '1024x1024'
const MAX_POLL_COUNT = 240
const DEFAULT_POLL_DELAY_MS = 3000

export interface MediaStudioGeneratedImagePreview {
  id: string
  src: string
  url?: string
  b64Json?: string
  revisedPrompt?: string
}

export interface MediaStudioMessage {
  id: string
  role: 'user' | 'assistant'
  mode: 'image'
  prompt: string
  status?: 'queued' | 'processing' | 'completed' | 'failed'
  taskId?: string
  model?: string
  size?: string
  quality?: string
  count?: number
  images?: MediaStudioGeneratedImagePreview[]
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
  size?: string
  quality?: string
  count?: number
  conversation?: MediaStudioConversation
}

export interface MediaStudioControllerOptions {
  storage?: Storage
  pollDelayMs?: number
  maxPollCount?: number
  listModels?: typeof listMediaStudioModels
  submitImage?: typeof submitAsyncImageGeneration
  getTask?: typeof getAsyncImageTask
  listKeys?: typeof keysAPI.list
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
  const random =
    typeof crypto !== 'undefined' && 'randomUUID' in crypto
      ? crypto.randomUUID()
      : Math.random().toString(36).slice(2)
  return `${prefix}_${String(random).replace(/-/g, '')}`
}

function readPersisted(storage: Storage | null): PersistedMediaStudioState {
  if (!storage) return {}
  try {
    const raw = storage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw) as PersistedMediaStudioState
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

function taskErrorMessage(task: MediaStudioImageTask): string {
  const err = task.error
  if (!err) return 'image generation failed'
  if (typeof err === 'string') return err
  const message = (err as { message?: unknown }).message
  return typeof message === 'string' && message.trim() ? message : 'image generation failed'
}

function modelId(model: MediaStudioModel): string {
  return String(model.id || '').trim()
}

function extractImages(task: MediaStudioImageTask): MediaStudioGeneratedImagePreview[] {
  const images: MediaStudioGeneratedImagePreview[] = []
  if (task.image_url) {
    images.push({
      id: `${task.task_id || task.id}-image-url`,
      src: task.image_url,
      url: task.image_url,
    })
  }
  const resultImages = Array.isArray(task.result?.data) ? task.result?.data ?? [] : []
  resultImages.forEach((item: MediaStudioGeneratedImage, index: number) => {
    const url = String(item.url || '').trim()
    const b64Json = String(item.b64_json || '').trim()
    const src = url || (b64Json ? `data:image/png;base64,${b64Json}` : '')
    if (!src) return
    if (images.some(image => image.src === src)) return
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

function normalizeCount(count: number): number {
  if (!Number.isFinite(count)) return 1
  return Math.min(4, Math.max(1, Math.trunc(count)))
}

export function useMediaStudioController(options: MediaStudioControllerOptions = {}) {
  const storage = safeStorage(options.storage)
  const persisted = readPersisted(storage)
  const data = {
    listModels: options.listModels ?? listMediaStudioModels,
    // 默认走同步生图，避免本地未配置对象存储时异步接口直接 404；
    // 测试或后续开关可注入 submitAsyncImageGeneration 复用轮询路径。
    submitImage: options.submitImage ?? submitImageGeneration,
    getTask: options.getTask ?? getAsyncImageTask,
    listKeys: options.listKeys ?? keysAPI.list,
  }
  const pollDelayMs = options.pollDelayMs ?? DEFAULT_POLL_DELAY_MS
  const maxPollCount = options.maxPollCount ?? MAX_POLL_COUNT

  const { modes, getModeById } = useMediaStudioPreview()
  const selectedModeId = ref<MediaStudioModeId>('image')
  const prompt = ref('')
  const selectedApiKeyId = ref<number>(persisted.selectedApiKeyId || 0)
  const model = ref(persisted.model || DEFAULT_MODEL)
  const size = ref(persisted.size || DEFAULT_SIZE)
  const quality = ref(persisted.quality || 'auto')
  const count = ref(normalizeCount(persisted.count || 1))
  const apiKeys = ref<ApiKey[]>([])
  const loadingKeys = ref(false)
  const apiKeyLoadError = ref('')
  const modelOptions = ref<string[]>([])
  const loadingModels = ref(false)
  const modelLoadError = ref('')
  const submitting = ref(false)
  const submitError = ref('')
  const conversation = ref<MediaStudioConversation>(
    persisted.conversation ?? {
      id: makeId('media_conversation'),
      messages: [],
      updatedAt: now(),
    },
  )
  const pollingTaskIds = ref<string[]>([])
  let disposed = false

  const selectedMode = computed(() => getModeById(selectedModeId.value))
  const selectedApiKey = computed(() => apiKeys.value.find(key => key.id === selectedApiKeyId.value) ?? null)
  const hasMessages = computed(() => conversation.value.messages.length > 0)
  const canSubmit = computed(() => (
    selectedModeId.value === 'image' &&
    !!selectedApiKey.value &&
    !!prompt.value.trim() &&
    !submitting.value
  ))

  function persist() {
    if (!storage) return
    const payload: PersistedMediaStudioState = {
      selectedApiKeyId: selectedApiKeyId.value,
      model: model.value,
      size: size.value,
      quality: quality.value,
      count: count.value,
      conversation: conversation.value,
    }
    try {
      storage.setItem(STORAGE_KEY, JSON.stringify(payload))
    } catch {
      // localStorage may be full when a provider returns base64; keep runtime state.
    }
  }

  watch([selectedApiKeyId, model, size, quality, count, conversation], persist, { deep: true })

  async function loadApiKeys() {
    loadingKeys.value = true
    apiKeyLoadError.value = ''
    try {
      const firstPage = await data.listKeys(1, 100, { status: 'active' })
      const pages: PaginatedResponse<ApiKey>[] = [firstPage]
      for (let page = 2; page <= firstPage.pages; page += 1) {
        pages.push(await data.listKeys(page, 100, { status: 'active' }))
      }
      apiKeys.value = pages.flatMap(page => page.items)
      if (!selectedApiKey.value && apiKeys.value.length > 0) {
        selectedApiKeyId.value = apiKeys.value[0].id
      }
      await loadModels()
    } catch (error) {
      apiKeyLoadError.value = mediaStudioErrorMessage(error, 'Failed to load API keys')
    } finally {
      loadingKeys.value = false
    }
  }

  async function loadModels() {
    const key = selectedApiKey.value
    modelLoadError.value = ''
    modelOptions.value = []
    if (!key) return
    loadingModels.value = true
    try {
      const response = await data.listModels(key.key)
      const seen = new Set<string>()
      modelOptions.value = response.data
        .map(modelId)
        .filter(id => {
          if (!id || seen.has(id) || !isMediaStudioImageModel(id)) return false
          seen.add(id)
          return true
        })
      if (modelOptions.value.length > 0 && !modelOptions.value.includes(model.value)) {
        model.value = modelOptions.value[0]
      } else if (!model.value.trim()) {
        model.value = DEFAULT_MODEL
      }
    } catch (error) {
      modelLoadError.value = mediaStudioErrorMessage(error, 'Failed to load models')
      if (!model.value.trim()) model.value = DEFAULT_MODEL
    } finally {
      loadingModels.value = false
    }
  }

  function selectMode(id: MediaStudioModeId) {
    const nextMode = getModeById(id)
    if (!nextMode.available) return
    selectedModeId.value = id
  }

  function patchAssistantMessage(messageId: string, patch: Partial<MediaStudioMessage>) {
    const messages = conversation.value.messages
    const index = messages.findIndex(message => message.id === messageId)
    if (index < 0) return
    messages[index] = { ...messages[index], ...patch }
    conversation.value = {
      ...conversation.value,
      messages: [...messages],
      updatedAt: now(),
    }
  }

  async function pollTask(apiKey: string, taskId: string, messageId: string) {
    if (!pollingTaskIds.value.includes(taskId)) {
      pollingTaskIds.value = [...pollingTaskIds.value, taskId]
    }
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
            error: taskErrorMessage(task),
            completedAt: (task.completed_at || Math.floor(now() / 1000)) * 1000,
          })
          return
        }
        patchAssistantMessage(messageId, { status: 'processing' })
        if (pollDelayMs > 0) {
          await new Promise(resolve => setTimeout(resolve, pollDelayMs))
        }
      }
      if (!disposed) {
        patchAssistantMessage(messageId, {
          status: 'failed',
          error: 'image generation polling timed out',
          completedAt: now(),
        })
      }
    } catch (error) {
      if (!disposed) {
        patchAssistantMessage(messageId, {
          status: 'failed',
          error: mediaStudioErrorMessage(error, 'image generation failed'),
          completedAt: now(),
        })
      }
    } finally {
      pollingTaskIds.value = pollingTaskIds.value.filter(id => id !== taskId)
    }
  }

  async function submitPrompt() {
    const key = selectedApiKey.value
    const text = prompt.value.trim()
    submitError.value = ''
    if (!key || !text || selectedModeId.value !== 'image') return

    const imageModel = model.value.trim() || modelOptions.value[0] || DEFAULT_MODEL
    const imageCount = normalizeCount(count.value)
    const userMessage: MediaStudioMessage = {
      id: makeId('media_user'),
      role: 'user',
      mode: 'image',
      prompt: text,
      createdAt: now(),
    }
    const assistantMessage: MediaStudioMessage = {
      id: makeId('media_assistant'),
      role: 'assistant',
      mode: 'image',
      prompt: text,
      status: 'queued',
      model: imageModel,
      size: size.value,
      quality: quality.value,
      count: imageCount,
      createdAt: now(),
    }
    conversation.value = {
      ...conversation.value,
      messages: [...conversation.value.messages, userMessage, assistantMessage],
      updatedAt: now(),
    }
    prompt.value = ''
    submitting.value = true
    try {
      const payload: MediaStudioImageSubmitRequest = {
        model: imageModel,
        prompt: text,
        n: imageCount,
        size: size.value,
        response_format: 'b64_json',
      }
      if (quality.value && quality.value !== 'auto') {
        payload.quality = quality.value
      }
      const task = await data.submitImage(key.key, payload, makeId('media_idem'))
      patchAssistantMessage(assistantMessage.id, {
        taskId: task.task_id || task.id,
        status: task.status === 'completed' ? 'completed' : 'processing',
        images: task.status === 'completed' ? extractImages(task) : [],
      })
      if (task.status === 'completed' || task.status === 'failed') {
        if (task.status === 'failed') {
          patchAssistantMessage(assistantMessage.id, {
            status: 'failed',
            error: taskErrorMessage(task),
            completedAt: now(),
          })
        }
        return
      }
      void pollTask(key.key, task.task_id || task.id, assistantMessage.id)
    } catch (error) {
      const message = mediaStudioErrorMessage(error, 'image generation failed')
      submitError.value = message
      patchAssistantMessage(assistantMessage.id, {
        status: 'failed',
        error: message,
        completedAt: now(),
      })
    } finally {
      submitting.value = false
    }
  }

  function retryMessage(message: MediaStudioMessage) {
    if (!message.prompt.trim()) return
    prompt.value = message.prompt
    void submitPrompt()
  }

  function clearConversation() {
    conversation.value = {
      id: makeId('media_conversation'),
      messages: [],
      updatedAt: now(),
    }
  }

  watch(selectedApiKeyId, () => {
    void loadModels()
  })

  if (getCurrentInstance()) {
    onBeforeUnmount(() => {
      disposed = true
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
