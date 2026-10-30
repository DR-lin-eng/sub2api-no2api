import { computed, getCurrentInstance, onBeforeUnmount, ref, watch } from 'vue'
import {
  createMediaStudioSession,
  estimateMediaStudioCost,
  getAsyncImageTask,
  getMediaStudioConfig,
  getVideoGenerationContent,
  getVideoGenerationTask,
  listMediaStudioModels,
  mediaStudioErrorMessage,
  mediaStudioFileKind,
  mediaStudioKindEnabled,
  resolveMediaStudioModelLimits,
  submitAsyncImageEdit,
  submitAsyncImageGeneration,
  submitImageEdit,
  submitImageGeneration,
  submitVideoGeneration,
  uploadMediaStudioFile,
  MEDIA_STUDIO_PARAMETER_BINDINGS,
  MEDIA_STUDIO_MEDIA_KINDS,
  type MediaStudioConfig,
  type MediaStudioCostEstimate,
  type MediaStudioCostEstimateRequest,
  type MediaStudioGroupOption,
  type MediaStudioImageSubmitRequest,
  type MediaStudioImageTask,
  type MediaStudioMediaType,
  type MediaStudioParameterControl,
  type MediaStudioResolvedLimits,
  type MediaStudioVideoAspectRatio,
  type MediaStudioVideoResolution,
  type MediaStudioVideoSubmitRequest,
  type ModelAttributeField,
  type ModelAttributeRenderMode,
  type ModelAttributeValues,
} from '@/features/media-studio/data/datasources/mediaStudioDatasource'
import {
  deleteMediaStudioImages,
  deleteMediaStudioVideos,
  loadMediaStudioImage,
  loadMediaStudioVideo,
  mediaStudioImageStorageKey,
  storeMediaStudioImage,
} from '@/features/media-studio/data/datasources/mediaStudioImageStore'
import {
  addMediaStudioImageAttachments,
  addMediaStudioReferenceAttachments,
  revokeMediaStudioImageAttachments,
  revokeMediaStudioPreviewURLs,
  type MediaStudioAttachmentRejection,
  type MediaStudioImageAttachment,
  type MediaStudioImageAttachmentLimits,
  type MediaStudioReferenceAttachment,
  type MediaStudioReferenceLimits,
} from '@/features/media-studio/presentation/composables/useMediaStudioAttachments'
import {
  useMediaStudioPreview,
  type MediaStudioModeId,
} from '@/features/media-studio/presentation/composables/useMediaStudioPreview'
import {
  IMAGE_QUALITY_OPTIONS,
  MEDIA_STUDIO_LEGACY_STORAGE_KEY,
  MEDIA_STUDIO_MAX_SESSIONS,
  MEDIA_STUDIO_STORAGE_KEY,
  aspectRatioFromSize,
  classifyMediaStudioError,
  conversationCacheKeys,
  conversationForPersistence,
  conversationTitle,
  extractImages,
  imageSizeFor,
  imageTaskErrorMessage,
  makeId,
  modelId,
  normalizeCount,
  normalizeCustomImageAspectRatio,
  normalizeDuration,
  normalizeImageAspectRatio,
  normalizeImageQuality,
  normalizeImageResolution,
  normalizeResolution,
  normalizeVideoAspectRatio,
  now,
  persistedImageSource,
  readPersisted,
  restorePersistedConversation,
  restorePersistedSessions,
  safeRemoteImageURL,
  videoTaskErrorMessage,
  type MediaStudioConversation,
  type MediaStudioGeneratedImagePreview,
  type MediaStudioImageAspectRatio as MediaStudioImageAspectRatioType,
  type MediaStudioImageResolution as MediaStudioImageResolutionType,
  type MediaStudioInputImagePreview,
  type MediaStudioJobErrorCode,
  type MediaStudioMessage,
  type MediaStudioModelOption as MediaStudioModelOptionType,
  type PersistedMediaStudioState,
} from '@/features/media-studio/presentation/composables/mediaStudioConversation'
import { useMediaStudioJobs } from '@/features/media-studio/presentation/composables/useMediaStudioJobs'

// 视图层历史上从这里导入消息与模型类型，保持该入口不变，内部实现已迁到 mediaStudioConversation。
export type {
  MediaStudioConversation,
  MediaStudioGeneratedImagePreview,
  MediaStudioGeneratedVideoPreview,
  MediaStudioImageAspectRatio,
  MediaStudioImageResolution,
  MediaStudioInputImagePreview,
  MediaStudioInputReferencePreview,
  MediaStudioJobErrorCode,
  MediaStudioMessage,
  MediaStudioMessageStatus,
  MediaStudioModelOption,
  MediaStudioParameterOption,
} from '@/features/media-studio/presentation/composables/mediaStudioConversation'

const DEFAULT_IMAGE_MODEL = 'gpt-image-2'
const DEFAULT_VIDEO_MODEL = 'grok-imagine-video'
const DEFAULT_VIDEO_RESOLUTION: MediaStudioVideoResolution = '720p'
const DEFAULT_VIDEO_DURATION = 6
// 预估消耗防抖，避免拖动时长/切换分辨率时连续打后端。
const COST_ESTIMATE_DEBOUNCE_MS = 300

export interface MediaStudioSessionSummary {
  id: string
  title: string
  messageCount: number
  updatedAt: number
}

export interface MediaStudioControllerOptions {
  storage?: Storage
  pollDelayMs?: number
  maxPollCount?: number
  listModels?: typeof listMediaStudioModels
  submitImage?: typeof submitAsyncImageGeneration
  submitImageEdit?: typeof submitAsyncImageEdit
  // 异步端点不可用（未启用对象存储或分组平台不支持）时的同步回退实现。
  submitImageSync?: typeof submitImageGeneration
  submitImageEditSync?: typeof submitImageEdit
  getTask?: typeof getAsyncImageTask
  submitVideo?: typeof submitVideoGeneration
  getVideoTask?: typeof getVideoGenerationTask
  getVideoContent?: typeof getVideoGenerationContent
  getConfig?: typeof getMediaStudioConfig
  createSession?: typeof createMediaStudioSession
  estimateCost?: typeof estimateMediaStudioCost
  uploadFile?: typeof uploadMediaStudioFile
  createObjectURL?: (blob: Blob) => string
  revokeObjectURL?: (url: string) => void
}

function safeStorage(provided?: Storage): Storage | null {
  if (provided) return provided
  if (typeof window === 'undefined') return null
  return window.localStorage
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

// 后端在未启用对象存储或分组平台不支持时会对异步端点返回 404；
// 这种情况退回同步提交，其他 404 仍按真实错误上抛。
function isAsyncImageUnavailable(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false
  const status = (error as { status?: unknown }).status
  if (status !== 404) return false
  const message = error instanceof Error ? error.message : ''
  return /async|not enabled|not supported|unsupported/i.test(message)
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

export function useMediaStudioController(options: MediaStudioControllerOptions = {}) {
  const storage = safeStorage(options.storage)
  const persisted = readPersisted(storage)
  const data = {
    listModels: options.listModels ?? listMediaStudioModels,
    // 默认走异步端点：提交后立即拿到 task id 并落盘，刷新后仍能续轮询。
    submitImage: options.submitImage ?? submitAsyncImageGeneration,
    submitImageEdit: options.submitImageEdit ?? submitAsyncImageEdit,
    submitImageSync: options.submitImageSync ?? submitImageGeneration,
    submitImageEditSync: options.submitImageEditSync ?? submitImageEdit,
    getTask: options.getTask ?? getAsyncImageTask,
    submitVideo: options.submitVideo ?? submitVideoGeneration,
    getVideoTask: options.getVideoTask ?? getVideoGenerationTask,
    getVideoContent: options.getVideoContent ?? getVideoGenerationContent,
    getConfig: options.getConfig ?? getMediaStudioConfig,
    createSession: options.createSession ?? createMediaStudioSession,
    estimateCost: options.estimateCost ?? estimateMediaStudioCost,
    uploadFile: options.uploadFile ?? uploadMediaStudioFile,
  }
  const createObjectURL = options.createObjectURL ?? ((blob: Blob) => (
    typeof URL.createObjectURL === 'function' ? URL.createObjectURL(blob) : ''
  ))
  const revokeObjectURL = options.revokeObjectURL ?? ((url: string) => {
    if (url && typeof URL.revokeObjectURL === 'function') URL.revokeObjectURL(url)
  })

  const { modes: previewModes } = useMediaStudioPreview()
  const selectedModeId = ref<MediaStudioModeId>('image')
  const prompt = ref('')
  const selectedGroupId = ref<number>(persisted.selectedGroupId || 0)
  const imageModel = ref(persisted.imageModel || persisted.model || DEFAULT_IMAGE_MODEL)
  const videoModel = ref(persisted.videoModel || DEFAULT_VIDEO_MODEL)
  const model = ref(imageModel.value)
  const imageResolution = ref<MediaStudioImageResolutionType>(
    normalizeImageResolution(persisted.imageResolution),
  )
  const imageAspectRatio = ref<MediaStudioImageAspectRatioType>(
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
  const videoAspectRatio = ref<MediaStudioVideoAspectRatio>(
    normalizeVideoAspectRatio(persisted.videoAspectRatio),
  )
  const duration = ref(normalizeDuration(persisted.duration || DEFAULT_VIDEO_DURATION))
  const groupConfig = ref<MediaStudioConfig>({ groups: [] })
  const loadingGroups = ref(false)
  const groupLoadError = ref('')
  const mediaStudioApiKey = ref('')
  const imageAttachments = ref<MediaStudioImageAttachment[]>([])
  const referenceAttachments = ref<MediaStudioReferenceAttachment[]>([])
  // 最近一次被拒绝的附件：交给视图层翻译，数据层不产生用户文案。
  const attachmentRejection = ref<MediaStudioAttachmentRejection | null>(null)
  const modelOptions = ref<MediaStudioModelOptionType[]>([])
  const loadingModels = ref(false)
  const modelLoadError = ref('')
  const submitting = ref(false)
  const submitError = ref('')
  // 预估消耗只来自后端 sub2api 本地计价链路，前端不读取请求体或上游响应体里的价格。
  const estimatedCost = ref<number | null>(null)
  const estimatedCurrency = ref('')
  const estimateLoading = ref(false)
  const estimateError = ref('')
  const conversation = ref<MediaStudioConversation>({
    ...(restorePersistedConversation(persisted.conversation) ?? {
      id: makeId('media_conversation'),
      messages: [],
      updatedAt: now(),
    }),
  })
  // 历史会话与当前会话按 id 去重：当前会话不进历史列表，切换时再互换。
  const sessions = ref<MediaStudioConversation[]>(
    restorePersistedSessions(persisted.sessions).filter(item => item.id !== conversation.value.id),
  )
  const videoObjectURLs = new Set<string>()
  const imageInputFiles = new Map<string, File[]>()
  let disposed = false
  let modelRequestSequence = 0
  let estimateRequestSequence = 0
  let estimateTimer: ReturnType<typeof setTimeout> | null = null
  let groupLoadRequest: Promise<void> | null = null
  let sessionRequest: Promise<void> | null = null
  let sessionRequestGroupID = 0
  let activeSessionGroupID = 0

  // 轮询、刷新恢复与任务条数据统一由 jobs 承担，controller 只负责装配。
  const {
    pollingTaskIds,
    jobs,
    pollImageTask,
    completeVideoTask,
    pollVideoTask,
    resumePendingJobs,
  } = useMediaStudioJobs({
    conversation,
    patchMessage: patchAssistantMessage,
    getApiKey: () => mediaStudioApiKey.value,
    getActiveGroupId: () => selectedGroupId.value,
    createObjectURL,
    revokeObjectURL,
    videoObjectURLs,
    isDisposed: () => disposed,
    persist,
    cacheImages: cacheGeneratedImages,
    data: {
      getTask: data.getTask,
      getVideoTask: data.getVideoTask,
      getVideoContent: data.getVideoContent,
    },
    ...(options.pollDelayMs !== undefined ? { pollDelayMs: options.pollDelayMs } : {}),
    ...(options.maxPollCount !== undefined ? { maxPollCount: options.maxPollCount } : {}),
  })

  const modes = computed(() => previewModes.map((mode) => ({
    ...mode,
    available: loadingGroups.value || groupConfig.value.groups.some(group => group.media_type === mode.id),
  })))
  const selectedMode = computed(() => modes.value.find(mode => mode.id === selectedModeId.value) ?? modes.value[0])
  const modelSelectionLocked = computed(() => (selectedGroup.value?.models?.length || 0) > 0)
  const imageQualityOptions = computed(() => IMAGE_QUALITY_OPTIONS)
  const activeModelFields = computed<ModelAttributeField[]>(
    () => modelOptions.value.find(option => option.id === model.value)?.fields ?? [],
  )
  // 模板声明要隐藏的内置参数 key；仅用于屏蔽宿主控件，不参与属性值渲染。
  const activeModelHiddenKeys = computed<string[]>(
    () => modelOptions.value.find(option => option.id === model.value)?.hide ?? [],
  )
  const visibleAttributeFields = computed(
    () => activeModelFields.value.filter(field => field.ui !== 'hidden'),
  )
  // hardcoded：模板未声明字段，按硬编码参数渲染；其余两种模式由模板决定。
  const attributeRenderMode = computed<'hardcoded' | ModelAttributeRenderMode>(() => {
    if (activeModelFields.value.length === 0 && activeModelHiddenKeys.value.length === 0) return 'hardcoded'
    const option = modelOptions.value.find(item => item.id === model.value)
    return option?.renderMode ?? 'overlay'
  })
  // 不渲染的硬编码控件：完整替换全部隐藏，局部覆盖只隐藏被同名字段或 hide 接管的控件。
  const hiddenParameterControls = computed<MediaStudioParameterControl[]>(() => {
    const mode = attributeRenderMode.value
    const controls = Object.keys(MEDIA_STUDIO_PARAMETER_BINDINGS) as MediaStudioParameterControl[]
    if (mode === 'hardcoded') return []
    if (mode === 'replace') return controls
    const declared = new Set([
      ...activeModelFields.value.map(field => field.key),
      ...activeModelHiddenKeys.value,
    ])
    return controls.filter(control => declared.has(MEDIA_STUDIO_PARAMETER_BINDINGS[control]))
  })
  const activeModelOption = computed(
    () => modelOptions.value.find(option => option.id === model.value) ?? null,
  )
  // 模型级媒体限制：未配置时回退媒体工坊原有默认值，配置后按模型约束。
  const activeModelLimits = computed<MediaStudioResolvedLimits>(
    () => resolveMediaStudioModelLimits(activeModelOption.value?.limits),
  )
  const imageAttachmentLimits = computed<MediaStudioImageAttachmentLimits>(() => {
    const limits = activeModelLimits.value
    const maxFiles = limits.maxFiles.image
    const maxFileSizeBytes = limits.maxFileSizeMB.image * 1024 * 1024
    return { maxFiles, maxFileSizeBytes, maxTotalBytes: maxFiles * maxFileSizeBytes }
  })
  // 参考素材按类型分别校验：仅启用的类型可被加入，各自独立计数。
  const referenceLimits = computed<MediaStudioReferenceLimits>(() => ({
    enabled: MEDIA_STUDIO_MEDIA_KINDS.filter(kind => mediaStudioKindEnabled(activeModelLimits.value, kind)),
    maxFiles: { ...activeModelLimits.value.maxFiles },
    maxFileSizeMB: { ...activeModelLimits.value.maxFileSizeMB },
  }))
  // 未返回该字段的旧后端按可用处理，只有显式 false 才置灰上传入口。
  const storageAvailable = computed(() => groupConfig.value.storage_available !== false)
  const attributeValues = ref<ModelAttributeValues>({})

  function setAttributeValue(key: string, value: string | number | boolean | undefined) {
    const next: ModelAttributeValues = { ...attributeValues.value }
    if (value === undefined || value === '') {
      delete next[key]
    } else {
      next[key] = value
    }
    attributeValues.value = next
  }

  // 切换模型时属性值回到新模型声明的默认值；无属性模型清空。
  watch(activeModelFields, (fields) => {
    const next: ModelAttributeValues = {}
    for (const field of fields) {
      if (field.default !== undefined) next[field.key] = field.default
    }
    attributeValues.value = next
  }, { immediate: true, flush: 'sync' })
  // 模型限制加载或切换后，把已选时长收敛回该模型允许的区间。
  watch(activeModelLimits, (limits) => {
    duration.value = normalizeDuration(duration.value, limits.duration)
  })
  // 时长支持手输，任何写入都收敛到当前模型区间，避免越界值进入请求体。
  watch(duration, (value) => {
    const next = normalizeDuration(value, activeModelLimits.value.duration)
    if (next !== value) duration.value = next
  })
  // A group serves exactly one modality, so only expose groups of the active mode.
  const groupOptions = computed<MediaStudioGroupOption[]>(() => {
    return groupConfig.value.groups.filter(group => group.media_type === selectedModeId.value)
  })
  const selectedGroup = computed(() => groupOptions.value.find(group => group.group_id === selectedGroupId.value) ?? null)
  const hasMessages = computed(() => conversation.value.messages.length > 0)
  // 会话列表：当前会话永远置顶，其余按最近更新时间倒序，视图层据此做切换与删除。
  const sessionList = computed<MediaStudioSessionSummary[]>(() => {
    const activeId = conversation.value.id
    // 空白会话不进列表：新对话页不需要展示一条没有内容的历史记录。
    const source = conversation.value.messages.length > 0
      ? [conversation.value, ...sessions.value]
      : sessions.value
    const items = source.map(item => ({
      id: item.id,
      title: conversationTitle(item.messages),
      messageCount: item.messages.length,
      updatedAt: item.updatedAt,
    }))
    return items.sort((left, right) => {
      if (left.id === activeId) return -1
      if (right.id === activeId) return 1
      return right.updatedAt - left.updatedAt
    })
  })
  const canSubmit = computed(() => (
    !!selectedGroup.value &&
    !!mediaStudioApiKey.value &&
    !!prompt.value.trim() &&
    !submitting.value &&
    // 参考素材未上传完成（或失败未重试）时不提交，避免请求丢失素材。
    (selectedModeId.value !== 'video' || referenceAttachments.value.every(item => item.status === 'ready'))
  ))

  // 只按真正影响计价的输入变化触发预估，避免切换无关参数产生多余请求。
  const costEstimateSignature = computed(() => {
    const mode = selectedModeId.value
    if (mode !== 'image' && mode !== 'video') return ''
    const group = selectedGroup.value
    const modelID = model.value
    if (!group || !modelID) return ''
    const parts: Array<string | number> = [mode, group.group_id, modelID]
    if (mode === 'video') {
      parts.push(resolution.value, duration.value)
    } else {
      parts.push(size.value, normalizeCount(count.value))
    }
    return parts.join('|')
  })

  // 预估金额完全由后端 sub2api 通用计价链路算出，不接受请求体或上游响应体里的价格。
  async function refreshCostEstimate() {
    const signature = costEstimateSignature.value
    if (!signature) {
      estimateRequestSequence += 1
      estimatedCost.value = null
      estimatedCurrency.value = ''
      estimateError.value = ''
      estimateLoading.value = false
      return
    }
    const mode = selectedModeId.value
    const request: MediaStudioCostEstimateRequest = {
      media_type: mode as MediaStudioMediaType,
      group_id: selectedGroupId.value,
      model: model.value,
    }
    if (mode === 'video') {
      request.resolution = resolution.value
      request.duration = duration.value
    } else {
      request.size = size.value
      request.count = normalizeCount(count.value)
    }
    const sequence = ++estimateRequestSequence
    estimateLoading.value = true
    estimateError.value = ''
    try {
      const estimate: MediaStudioCostEstimate = await data.estimateCost(request)
      if (sequence !== estimateRequestSequence) return
      estimatedCost.value = estimate.amount
      estimatedCurrency.value = estimate.currency || ''
    } catch (error) {
      if (sequence !== estimateRequestSequence) return
      estimatedCost.value = null
      estimatedCurrency.value = ''
      estimateError.value = mediaStudioErrorMessage(error, 'cost estimate failed')
    } finally {
      if (sequence === estimateRequestSequence) estimateLoading.value = false
    }
  }

  function scheduleCostEstimate() {
    if (estimateTimer !== null) clearTimeout(estimateTimer)
    estimateTimer = setTimeout(() => {
      estimateTimer = null
      void refreshCostEstimate()
    }, COST_ESTIMATE_DEBOUNCE_MS)
  }

  watch(costEstimateSignature, () => {
    if (disposed) return
    scheduleCostEstimate()
  })

  function updateImageAttachments(attachments: MediaStudioImageAttachment[]) {
    revokeMediaStudioImageAttachments(imageAttachments.value.filter(
      (current) => !attachments.some((next) => next.id === current.id),
    ))
    imageAttachments.value = attachments
  }

  async function addImageAttachments(files: File[]) {
    if (files.length === 0) return
    const result = await addMediaStudioImageAttachments(
      imageAttachments.value,
      files,
      imageAttachmentLimits.value,
    )
    if (disposed) return
    attachmentRejection.value = result.rejected[0] ?? null
    updateImageAttachments(result.attachments)
  }

  function removeImageAttachment(id: string) {
    updateImageAttachments(imageAttachments.value.filter(attachment => attachment.id !== id))
  }

  function updateReferenceAttachments(attachments: MediaStudioReferenceAttachment[]) {
    revokeMediaStudioPreviewURLs(referenceAttachments.value.filter(
      (current) => !attachments.some((next) => next.id === current.id),
    ))
    referenceAttachments.value = attachments
  }

  function patchReferenceAttachment(id: string, patch: Partial<MediaStudioReferenceAttachment>) {
    referenceAttachments.value = referenceAttachments.value.map(item => (
      item.id === id ? { ...item, ...patch } : item
    ))
  }

  // 参考素材必须先落到对象存储，视频请求只能引用公网 URL。
  async function uploadReferenceAttachment(attachment: MediaStudioReferenceAttachment, modelName: string) {
    const file = attachment.file
    if (!file) return
    try {
      const uploaded = await data.uploadFile(modelName, attachment.kind, file)
      if (disposed) return
      patchReferenceAttachment(attachment.id, { url: uploaded.url, status: 'ready', error: '' })
    } catch (error) {
      if (disposed) return
      patchReferenceAttachment(attachment.id, {
        status: 'failed',
        error: mediaStudioErrorMessage(error, 'upload failed'),
      })
    }
  }

  async function addReferenceAttachments(files: File[]): Promise<MediaStudioAttachmentRejection[]> {
    const existing = new Set(referenceAttachments.value.map(attachment => attachment.id))
    const result = await addMediaStudioReferenceAttachments(
      referenceAttachments.value,
      files,
      referenceLimits.value,
      mediaStudioFileKind,
    )
    if (disposed) return result.rejected
    attachmentRejection.value = result.rejected[0] ?? null
    updateReferenceAttachments(result.attachments)
    const modelName = model.value.trim()
    for (const attachment of result.attachments) {
      if (existing.has(attachment.id) || attachment.status !== 'uploading') continue
      void uploadReferenceAttachment(attachment, modelName)
    }
    return result.rejected
  }

  function removeReferenceAttachment(id: string) {
    updateReferenceAttachments(referenceAttachments.value.filter(attachment => attachment.id !== id))
  }

  function retryReferenceUpload(id: string) {
    const attachment = referenceAttachments.value.find(item => item.id === id)
    if (!attachment || attachment.status === 'uploading') return
    const retried: MediaStudioReferenceAttachment = { ...attachment, status: 'uploading', error: '' }
    patchReferenceAttachment(id, { status: 'uploading', error: '' })
    void uploadReferenceAttachment(retried, model.value.trim())
  }

  async function editGeneratedImage(image: MediaStudioGeneratedImagePreview) {
    if (selectedModeId.value !== 'image') await selectMode('image')
    submitError.value = ''
    try {
      const file = await imagePreviewToFile(image)
      const previousCount = imageAttachments.value.length
      const result = await addMediaStudioImageAttachments(
        imageAttachments.value,
        [file],
        imageAttachmentLimits.value,
      )
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

  // 持久化只在关键状态变更处显式调用：轮询进度不再触发全量 JSON 序列化。
  function persist() {
    if (!storage) return
    const persistedConversation = conversationForPersistence(conversation.value)
    const persistedSessions = sessions.value
      .slice(0, MEDIA_STUDIO_MAX_SESSIONS)
      .flatMap((item) => {
        const value = conversationForPersistence(item)
        return value ? [value] : []
      })
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
      videoAspectRatio: videoAspectRatio.value,
      duration: duration.value,
      conversation: persistedConversation || undefined,
      ...(persistedSessions.length > 0 ? { sessions: persistedSessions } : {}),
    }
    try {
      storage.setItem(MEDIA_STUDIO_STORAGE_KEY, JSON.stringify(payload))
      storage.removeItem(MEDIA_STUDIO_LEGACY_STORAGE_KEY)
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

      let video = message.video
      if (video?.cacheKey) {
        const blob = await loadMediaStudioVideo(video.cacheKey)
        if (disposed) return message
        if (!blob) {
          changed = true
          video = undefined
        } else {
          const src = createObjectURL(blob)
          videoObjectURLs.add(src)
          if (src !== video.src) changed = true
          video = { src, mimeType: blob.type || video.mimeType, cacheKey: video.cacheKey }
        }
      }
      const videoChanged = video !== message.video

      return !imagesChanged && !inputImagesChanged && !videoChanged
        ? message
        : { ...message, images, inputImages, video }
    }))

    if (!disposed && changed) {
      conversation.value = { ...conversation.value, messages, updatedAt: now() }
      persist()
    }
  }

  // 配置项变更即时落盘，会话本身只在关键状态变更时写入。
  watch([selectedGroupId, imageModel, videoModel, size, imageResolution, imageAspectRatio, customImageAspectRatios, quality, count, resolution, videoAspectRatio, duration], persist, { deep: true })
  watch(model, (value) => {
    if (selectedModeId.value === 'image') imageModel.value = value
    if (selectedModeId.value === 'video') videoModel.value = value
  })
  async function loadMediaGroups() {
    if (groupLoadRequest) return groupLoadRequest
    const request = (async () => {
      loadingGroups.value = true
      groupLoadError.value = ''
      try {
        groupConfig.value = await data.getConfig()
        // A group serves exactly one modality; if the current mode has no group,
        // switch to the first mode that actually has one.
        if (!groupConfig.value.groups.some(group => group.media_type === selectedModeId.value)) {
          const fallbackMode = modes.value.find(mode =>
            mode.available && groupConfig.value.groups.some(group => group.media_type === mode.id),
          )
          if (fallbackMode) {
            selectedModeId.value = fallbackMode.id
            model.value = fallbackMode.id === 'video' ? videoModel.value : imageModel.value
          }
        }
        const options = groupOptions.value
        if (!options.some(group => group.group_id === selectedGroupId.value)) {
          selectedGroupId.value = options[0]?.group_id || 0
        }
        await establishMediaStudioSession()
      } catch (error) {
        groupLoadError.value = mediaStudioErrorMessage(error, 'Failed to load media studio groups')
      } finally {
        loadingGroups.value = false
      }
    })()
    groupLoadRequest = request
    try {
      await request
    } finally {
      if (groupLoadRequest === request) groupLoadRequest = null
    }
  }

  async function establishMediaStudioSession() {
    const mediaType = selectedModeId.value as MediaStudioMediaType
    const group = selectedGroup.value
    if (!group || (mediaType !== 'image' && mediaType !== 'video')) {
      mediaStudioApiKey.value = ''
      activeSessionGroupID = 0
      return
    }
    if (mediaStudioApiKey.value && activeSessionGroupID === group.group_id) {
      await loadModels()
      // 会话可用即可恢复未完成任务，不必等待模型列表变化。
      resumePendingJobs()
      return
    }
    if (sessionRequest && sessionRequestGroupID === group.group_id) {
      await sessionRequest
      return
    }
    const groupID = group.group_id
    const request = (async () => {
      const session = await data.createSession(mediaType, groupID)
      if (selectedGroupId.value !== groupID || disposed) return
      mediaStudioApiKey.value = session.api_key
      activeSessionGroupID = groupID
      await loadModels()
      resumePendingJobs()
    })()
    sessionRequest = request
    sessionRequestGroupID = groupID
    try {
      await request
    } finally {
      if (sessionRequest === request) {
        sessionRequest = null
        sessionRequestGroupID = 0
      }
    }
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
      modelOptions.value = response.data
        .map(item => ({
          id: modelId(item),
          fields: item.attributes?.fields ?? [],
          renderMode: item.attributes?.mode === 'replace' ? 'replace' as const : 'overlay' as const,
          hide: item.attributes?.hide ?? [],
          limits: item.limits,
        }))
        .filter((option) => {
          if (!option.id || seen.has(option.id)) return false
          seen.add(option.id)
          return true
        })
      const fallback = requestedMode === 'video' ? DEFAULT_VIDEO_MODEL : DEFAULT_IMAGE_MODEL
      if (modelOptions.value.length > 0 && !modelOptions.value.some(option => option.id === model.value)) {
        model.value = modelOptions.value[0].id
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

  async function selectMode(id: MediaStudioModeId) {
    const nextMode = modes.value.find(mode => mode.id === id) ?? modes.value[0]
    if (!nextMode.available || id === selectedModeId.value) return
    if (selectedModeId.value === 'image') imageModel.value = model.value
    if (selectedModeId.value === 'video') videoModel.value = model.value
    selectedModeId.value = id
    model.value = id === 'video' ? videoModel.value : imageModel.value
    submitError.value = ''
    modelLoadError.value = ''
    modelOptions.value = []
    if (groupConfig.value.groups.length > 0) {
      if (!groupOptions.value.some(group => group.group_id === selectedGroupId.value)) {
        selectedGroupId.value = groupOptions.value[0]?.group_id || 0
      }
      await establishMediaStudioSession()
    } else {
      await loadMediaGroups()
    }
  }

  function patchAssistantMessage(messageId: string, patch: Partial<MediaStudioMessage>): boolean {
    const messages = conversation.value.messages
    const index = messages.findIndex(message => message.id === messageId)
    if (index >= 0) {
      messages[index] = { ...messages[index], ...patch }
      conversation.value = { ...conversation.value, messages: [...messages], updatedAt: now() }
      return true
    }
    // 轮询可能跨会话返回：用户切走后把结果写回所属历史会话，而不是直接丢弃。
    const sessionIndex = sessions.value.findIndex(item => item.messages.some(message => message.id === messageId))
    if (sessionIndex < 0) return false
    const session = sessions.value[sessionIndex]
    const sessionMessages = [...session.messages]
    const sessionMessageIndex = sessionMessages.findIndex(message => message.id === messageId)
    sessionMessages[sessionMessageIndex] = { ...sessionMessages[sessionMessageIndex], ...patch }
    const nextSessions = [...sessions.value]
    nextSessions[sessionIndex] = { ...session, messages: sessionMessages, updatedAt: now() }
    sessions.value = nextSessions
    persist()
    return true
  }

  async function cacheInputImages(assistantMessageID: string, attachments: MediaStudioImageAttachment[]) {
    const assistantIndex = conversation.value.messages.findIndex(message => message.id === assistantMessageID)
    const userIndex = assistantIndex > 0 ? assistantIndex - 1 : -1
    const userMessage = userIndex >= 0 ? conversation.value.messages[userIndex] : undefined
    if (!userMessage || userMessage.role !== 'user' || !userMessage.inputImages?.length) return

    const cached: MediaStudioInputImagePreview[] = []
    for (const image of userMessage.inputImages) {
      const attachment = attachments.find(item => item.id === image.id)
      if (!attachment) {
        cached.push(image)
        continue
      }
      const source = await readImageFileAsDataURL(attachment.file)
      if (!source) {
        cached.push(image)
        continue
      }
      const cacheKey = mediaStudioImageStorageKey(userMessage.id, image.id)
      cached.push(await storeMediaStudioImage(cacheKey, source) ? { ...image, cacheKey } : image)
    }

    if (disposed) return
    const messages = [...conversation.value.messages]
    const currentUser = messages[userIndex]
    if (!currentUser || currentUser.id !== userMessage.id) return
    messages[userIndex] = { ...currentUser, inputImages: cached }
    conversation.value = { ...conversation.value, messages, updatedAt: now() }
    persist()
  }

  function appendPromptMessages(
    text: string,
    mode: 'image' | 'video',
    inputAttachments: MediaStudioImageAttachment[] = [],
    inputReferences: MediaStudioReferenceAttachment[] = [],
  ): MediaStudioMessage {
    // 参考素材已上传为公网 URL，直接写进消息快照，保证发送后仍能渲染并支持重试。
    const referencePreviews = inputReferences
      .filter(reference => reference.status === 'ready' && reference.url)
      .map(reference => ({
        id: reference.id,
        src: reference.url,
        url: reference.url,
        name: reference.name,
        kind: reference.kind,
        mimeType: reference.mimeType,
      }))
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
      inputReferences: referencePreviews.length > 0 ? referencePreviews : undefined,
      createdAt: now(),
    }
    const renderMode = attributeRenderMode.value
    const attributeMode = renderMode === 'hardcoded' ? undefined : renderMode
    const assistantMessage: MediaStudioMessage = {
      id: makeId('media_assistant'),
      role: 'assistant',
      mode,
      prompt: text,
      status: 'queued',
      // 记录提交时所在分组，刷新后只恢复当前分组里的任务。
      groupId: selectedGroupId.value,
      model: model.value.trim() || (mode === 'video' ? DEFAULT_VIDEO_MODEL : DEFAULT_IMAGE_MODEL),
      size: mode === 'image' ? size.value : undefined,
      imageResolution: mode === 'image' ? imageResolution.value : undefined,
      imageAspectRatio: mode === 'image' ? imageAspectRatio.value : undefined,
      quality: mode === 'image' ? quality.value : undefined,
      count: mode === 'image' ? normalizeCount(count.value) : 1,
      resolution: mode === 'video' ? normalizeResolution(resolution.value) : undefined,
      aspectRatio: mode === 'video' ? normalizeVideoAspectRatio(videoAspectRatio.value) : undefined,
      duration: mode === 'video' ? normalizeDuration(duration.value, activeModelLimits.value.duration) : undefined,
      attributes: attributeMode ? { ...attributeValues.value } : undefined,
      attributeMode,
      attributeKeys: attributeMode
        ? [...activeModelFields.value.map(field => field.key), ...activeModelHiddenKeys.value]
        : undefined,
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
    const submittedReferences = mode === 'video' ? [...referenceAttachments.value] : []
    const assistantMessage = appendPromptMessages(text, mode, submittedAttachments, submittedReferences)
    if (submittedAttachments.length > 0) {
      imageInputFiles.set(assistantMessage.id, submittedAttachments.map(attachment => attachment.file))
      void cacheInputImages(assistantMessage.id, submittedAttachments)
    }
    prompt.value = ''
    imageAttachments.value = []
    if (mode === 'video') updateReferenceAttachments([])
    submitting.value = true
    try {
      if (mode === 'image') {
        const attributeMode = assistantMessage.attributeMode
        const declaredKeys = new Set(assistantMessage.attributeKeys ?? [])
        // 局部覆盖模式下，模板声明或被 hide 的 key 都由模板接管，不再发送对应硬编码参数。
        const replacedByTemplate = (control: MediaStudioParameterControl) => (
          attributeMode === 'overlay' && declaredKeys.has(MEDIA_STUDIO_PARAMETER_BINDINGS[control])
        )
        const keepsCount = attributeMode !== 'replace' && !replacedByTemplate('image.count')
        const requestedCount = keepsCount ? normalizeCount(assistantMessage.count || 1) : 1
        const payload: MediaStudioImageSubmitRequest = {
          model: assistantMessage.model || DEFAULT_IMAGE_MODEL,
          prompt: text,
        }
        if (attributeMode) {
          payload.attributes = { ...(assistantMessage.attributes ?? {}) }
        }
        if (attributeMode !== 'replace') {
          if (!replacedByTemplate('image.size')) {
            payload.size = imageSizeFor(
              normalizeImageResolution(assistantMessage.imageResolution),
              normalizeImageAspectRatio(assistantMessage.imageAspectRatio),
            )
          }
          if (!replacedByTemplate('image.quality') && assistantMessage.quality) {
            payload.quality = assistantMessage.quality
          }
        }

        const imageFiles = submittedAttachments.map((attachment) => attachment.file)
        // 默认走异步端点；对象存储不可用或分组平台不支持时退回同步提交。
        const submitImageTask = async (
          requestPayload: MediaStudioImageSubmitRequest,
        ): Promise<MediaStudioImageTask> => {
          const idempotencyKey = makeId('media_idem')
          const submitSync = () => (
            imageFiles.length > 0
              ? data.submitImageEditSync(mediaStudioApiKey.value, imageFiles, requestPayload, idempotencyKey)
              : data.submitImageSync(mediaStudioApiKey.value, requestPayload, idempotencyKey)
          )
          if (!storageAvailable.value) return submitSync()
          try {
            return imageFiles.length > 0
              ? await data.submitImageEdit(mediaStudioApiKey.value, imageFiles, requestPayload, idempotencyKey)
              : await data.submitImage(mediaStudioApiKey.value, requestPayload, idempotencyKey)
          } catch (error) {
            if (!isAsyncImageUnavailable(error)) throw error
            return submitSync()
          }
        }

        patchAssistantMessage(assistantMessage.id, {
          status: 'processing',
          images: [],
        })

        interface ImageRequestResult {
          taskID: string
          images: MediaStudioGeneratedImagePreview[]
          error: string
          errorCode?: MediaStudioJobErrorCode
        }

        const requestResults: Array<ImageRequestResult | undefined> = Array(requestedCount)
        // 多图并发时把每个任务的 ID 都留在消息上，刷新后才能逐个续轮询。
        const publishProgress = () => {
          const completed = requestResults
            .flatMap(result => result?.images ?? [])
            .slice(0, requestedCount)
          const taskIDs = [...new Set(
            requestResults.flatMap(result => (result?.taskID ? [result.taskID] : [])),
          )]
          patchAssistantMessage(assistantMessage.id, {
            images: completed,
            status: 'processing',
            taskId: taskIDs[0],
            taskIds: taskIDs.length > 0 ? taskIDs : undefined,
          })
          persist()
        }

        const submitAndResolveImage = async (index: number): Promise<ImageRequestResult> => {
          let taskID = ''
          try {
            let task = await submitImageTask(keepsCount ? { ...payload, n: 1 } : payload)
            taskID = task.task_id || task.id
            if (taskID) {
              // 提交成功立即落盘任务 ID：随后刷新或网络中断都能从任务条续轮询。
              requestResults[index] = { taskID, images: [], error: '' }
              publishProgress()
            }
            if (task.status !== 'completed' && task.status !== 'failed') {
              task = await pollImageTask(mediaStudioApiKey.value, taskID)
            }
            if (task.status === 'completed') {
              return {
                taskID,
                images: await cacheGeneratedImages(assistantMessage.id, extractImages(task)),
                error: '',
              }
            }
            const failure = imageTaskErrorMessage(task)
            return { taskID, images: [], error: failure, errorCode: classifyMediaStudioError(failure) }
          } catch (error) {
            const errorCode = classifyMediaStudioError(error)
            return {
              taskID,
              images: [],
              // 浏览器网络错误原文对用户没有意义，交给 errorCode 展示本地化文案。
              error: errorCode === 'network' ? '' : mediaStudioErrorMessage(error, 'image generation failed'),
              errorCode,
            }
          }
        }

        await Promise.all(Array.from({ length: requestedCount }, async (_, index) => {
          requestResults[index] = await submitAndResolveImage(index)
          publishProgress()
        }))
        const completedImages = conversation.value.messages.find(
          message => message.id === assistantMessage.id,
        )?.images ?? []
        const failures = requestResults
          .filter((result): result is ImageRequestResult => Boolean(result?.errorCode))
        const lastFailure = failures.length > 0 ? failures[failures.length - 1] : undefined
        patchAssistantMessage(assistantMessage.id, {
          status: completedImages.length > 0 ? 'completed' : 'failed',
          error: completedImages.length > 0 ? '' : (lastFailure?.error ?? ''),
          errorCode: completedImages.length > 0 ? undefined : lastFailure?.errorCode,
          completedAt: now(),
        })
        persist()
      } else {
        const videoPayload: MediaStudioVideoSubmitRequest = {
          model: assistantMessage.model || DEFAULT_VIDEO_MODEL,
          prompt: text,
        }
        // 参考素材已上传为公网 URL，按类型填入对应字段。
        const referenceURLs = (kind: MediaStudioMediaType) => submittedReferences
          .filter(attachment => attachment.kind === kind && attachment.status === 'ready' && attachment.url)
          .map(attachment => attachment.url)
        const referenceImages = referenceURLs('image')
        const referenceVideos = referenceURLs('video')
        const referenceAudios = referenceURLs('audio')
        if (referenceImages.length > 0) videoPayload.images = referenceImages
        if (referenceVideos.length > 0) videoPayload.videos = referenceVideos
        if (referenceAudios.length > 0) videoPayload.audios = referenceAudios
        const videoAttributeMode = assistantMessage.attributeMode
        const videoDeclaredKeys = new Set(assistantMessage.attributeKeys ?? [])
        const replacedByTemplate = (control: MediaStudioParameterControl) => (
          videoAttributeMode === 'overlay' && videoDeclaredKeys.has(MEDIA_STUDIO_PARAMETER_BINDINGS[control])
        )
        if (videoAttributeMode) {
          videoPayload.attributes = { ...(assistantMessage.attributes ?? {}) }
        }
        if (videoAttributeMode !== 'replace') {
          if (!replacedByTemplate('video.resolution')) {
            videoPayload.resolution = assistantMessage.resolution || DEFAULT_VIDEO_RESOLUTION
          }
          if (!replacedByTemplate('video.duration')) {
            videoPayload.duration = assistantMessage.duration || DEFAULT_VIDEO_DURATION
          }
          if (!replacedByTemplate('video.aspectRatio')) {
            videoPayload.aspect_ratio = normalizeVideoAspectRatio(assistantMessage.aspectRatio)
          }
        }
        const task = await data.submitVideo(mediaStudioApiKey.value, videoPayload, makeId('media_video_idem'))
        const submitPatch: Partial<MediaStudioMessage> = {
          taskId: task.id,
          taskIds: [task.id],
          status: task.status === 'failed' ? 'failed' : 'processing',
        }
        if (task.cost !== undefined) submitPatch.cost = task.cost
        if (task.currency) submitPatch.currency = task.currency
        patchAssistantMessage(assistantMessage.id, submitPatch)
        persist()
        if (task.status === 'failed') {
          const failure = videoTaskErrorMessage(task)
          patchAssistantMessage(assistantMessage.id, {
            error: failure,
            errorCode: classifyMediaStudioError(failure),
            completedAt: now(),
          })
          persist()
        } else if (task.status === 'completed') {
          await completeVideoTask(mediaStudioApiKey.value, task.id, assistantMessage.id, task.cost, task.currency)
        } else {
          void pollVideoTask(mediaStudioApiKey.value, task.id, assistantMessage.id)
        }
      }
    } catch (error) {
      const fallback = mode === 'video' ? 'video generation failed' : 'image generation failed'
      const errorCode = classifyMediaStudioError(error)
      // 浏览器网络错误原文（如 Failed to fetch）不直接展示给用户，改由 errorCode 提供本地化文案。
      const message = errorCode === 'network' ? '' : mediaStudioErrorMessage(error, fallback)
      submitError.value = message
      patchAssistantMessage(assistantMessage.id, {
        status: 'failed',
        error: message,
        errorCode,
        completedAt: now(),
      })
      persist()
    } finally {
      submitting.value = false
    }
  }

  // 重试时从对应的用户消息里还原已上传的参考素材，请求不能丢附件。
  function restoreReferenceAttachments(message: MediaStudioMessage): MediaStudioReferenceAttachment[] {
    const messages = conversation.value.messages
    const index = messages.findIndex(item => item.id === message.id)
    const promptMessage = index > 0 ? messages[index - 1] : undefined
    if (!promptMessage || promptMessage.role !== 'user') return []
    return (promptMessage.inputReferences ?? [])
      .filter(reference => reference.url)
      .map(reference => ({
        id: reference.id,
        name: reference.name,
        kind: reference.kind,
        mimeType: reference.mimeType,
        size: 0,
        previewUrl: reference.src || reference.url,
        url: reference.url,
        status: 'ready',
        error: '',
      }))
  }

  // 图片重试直接复用提交时缓存的 File：无需重新计算哈希，也不会因为浏览器无法读取历史文件而失败。
  function restoreImageAttachments(message: MediaStudioMessage): MediaStudioImageAttachment[] {
    const files = imageInputFiles.get(message.id) ?? []
    if (files.length === 0) return []
    const messages = conversation.value.messages
    const index = messages.findIndex(item => item.id === message.id)
    const promptMessage = index > 0 ? messages[index - 1] : undefined
    const previews = promptMessage?.role === 'user' ? (promptMessage.inputImages ?? []) : []
    return files.map((file, position) => {
      const preview = previews[position]
      return {
        id: preview?.cacheKey || preview?.id || makeId('media_image'),
        file,
        name: preview?.name || file.name || 'pasted-image',
        mimeType: preview?.mimeType || file.type,
        size: file.size,
        previewUrl: preview?.src || '',
      }
    })
  }

  async function retryMessage(message: MediaStudioMessage) {
    if (!message.prompt.trim()) return
    await selectMode(message.mode)
    model.value = message.model || (message.mode === 'video' ? DEFAULT_VIDEO_MODEL : DEFAULT_IMAGE_MODEL)
    if (message.attributes) attributeValues.value = { ...message.attributes }
    if (message.mode === 'image') {
      imageResolution.value = normalizeImageResolution(message.imageResolution)
      imageAspectRatio.value = normalizeImageAspectRatio(message.imageAspectRatio || aspectRatioFromSize(message.size))
      quality.value = normalizeImageQuality(message.quality)
      count.value = normalizeCount(message.count || 1)
      imageAttachments.value = restoreImageAttachments(message)
    } else {
      resolution.value = normalizeResolution(message.resolution)
      videoAspectRatio.value = normalizeVideoAspectRatio(message.aspectRatio)
      duration.value = normalizeDuration(
        message.duration || DEFAULT_VIDEO_DURATION,
        activeModelLimits.value.duration,
      )
      updateReferenceAttachments(restoreReferenceAttachments(message))
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
        if (image.src.startsWith('blob:')) revokeObjectURL(image.src)
      }
    }
  }

  // 释放当前会话占用的临时 URL 与附件状态；素材缓存按会话维度单独回收。
  function releaseActiveSession() {
    revokeAllVideoURLs()
    revokeInputImageURLs(conversation.value.messages)
    // 草稿附件属于当前输入区，换会话/新建/清空时一并回收，避免预览 URL 泄漏。
    updateImageAttachments([])
    updateReferenceAttachments([])
    imageInputFiles.clear()
  }

  function dropConversationCache(messages: MediaStudioMessage[]) {
    const { imageKeys, videoKeys } = conversationCacheKeys(messages)
    void deleteMediaStudioImages(imageKeys)
    void deleteMediaStudioVideos(videoKeys)
  }

  function clearConversation() {
    const outgoing = conversation.value
    releaseActiveSession()
    dropConversationCache(outgoing.messages)
    conversation.value = { id: makeId('media_conversation'), messages: [], updatedAt: now() }
    prompt.value = ''
    submitError.value = ''
    persist()
  }

  // 新建对话：把当前会话归档进历史列表，再从空白会话开始。
  function newConversation() {
    const outgoing = conversation.value
    // 空白会话时“新建对话”等价于清空草稿，给用户一个干净的起点。
    if (outgoing.messages.length === 0) {
      releaseActiveSession()
      prompt.value = ''
      submitError.value = ''
      return
    }
    releaseActiveSession()
    sessions.value = [outgoing, ...sessions.value.filter(item => item.id !== outgoing.id)]
      .slice(0, MEDIA_STUDIO_MAX_SESSIONS)
    conversation.value = { id: makeId('media_conversation'), messages: [], updatedAt: now() }
    prompt.value = ''
    submitError.value = ''
    persist()
  }

  async function switchConversation(id: string) {
    if (id === conversation.value.id) return
    const target = sessions.value.find(item => item.id === id)
    if (!target) return
    const outgoing = conversation.value
    releaseActiveSession()
    const remaining = sessions.value.filter(item => item.id !== id)
    if (outgoing.messages.length > 0) remaining.unshift(outgoing)
    sessions.value = remaining.slice(0, MEDIA_STUDIO_MAX_SESSIONS)
    conversation.value = { ...target }
    // 每个会话维护自己的输入区，切换后不把上一个会话的草稿带过去。
    prompt.value = ''
    submitError.value = ''
    persist()
    // 历史会话的图片/视频只在切换时从 IndexedDB 回填。
    await hydratePersistedImages()
    resumePendingJobs()
  }

  function removeConversation(id: string) {
    if (id === conversation.value.id) {
      clearConversation()
      return
    }
    const target = sessions.value.find(item => item.id === id)
    if (!target) return
    sessions.value = sessions.value.filter(item => item.id !== id)
    dropConversationCache(target.messages)
    persist()
  }

  async function deleteMessages(messageIDs: string[]) {
    const ids = new Set(messageIDs.filter(Boolean))
    if (ids.size === 0) return

    const imageKeys: string[] = []
    const videoKeys: string[] = []
    for (const message of conversation.value.messages) {
      if (!ids.has(message.id)) continue
      if (message.video?.src && videoObjectURLs.has(message.video.src)) {
        revokeObjectURL(message.video.src)
        videoObjectURLs.delete(message.video.src)
      }
      if (message.video?.cacheKey) videoKeys.push(message.video.cacheKey)
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
    persist()
    await Promise.all([
      deleteMediaStudioImages(imageKeys),
      deleteMediaStudioVideos(videoKeys),
    ])
  }

  watch(selectedGroupId, () => {
    void establishMediaStudioSession()
  })

  void hydratePersistedImages()

  if (getCurrentInstance()) {
    onBeforeUnmount(() => {
      disposed = true
      if (estimateTimer !== null) {
        clearTimeout(estimateTimer)
        estimateTimer = null
      }
      revokeAllVideoURLs()
      revokeInputImageURLs(conversation.value.messages)
      revokeMediaStudioPreviewURLs(referenceAttachments.value)
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
    addImageAttachments,
    removeImageAttachment,
    attachmentRejection,
    referenceAttachments,
    updateReferenceAttachments,
    addReferenceAttachments,
    removeReferenceAttachment,
    retryReferenceUpload,
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
    videoAspectRatio,
    duration,
    imageQualityOptions,
    activeModelLimits,
    imageAttachmentLimits,
    referenceLimits,
    storageAvailable,
    modelOptions,
    activeModelFields,
    visibleAttributeFields,
    attributeRenderMode,
    hiddenParameterControls,
    attributeValues,
    setAttributeValue,
    loadingModels,
    modelLoadError,
    submitting,
    submitError,
    estimatedCost,
    estimatedCurrency,
    estimateLoading,
    estimateError,
    refreshCostEstimate,
    conversation,
    hasMessages,
    sessionList,
    canSubmit,
    pollingTaskIds,
    jobs,
    loadMediaGroups,
    loadModels,
    selectMode,
    submitPrompt,
    retryMessage,
    editGeneratedImage,
    clearConversation,
    newConversation,
    switchConversation,
    removeConversation,
    deleteMessages,
  }
}

export type MediaStudioController = ReturnType<typeof useMediaStudioController>
