import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SelectOption } from '@/common/widgets/forms/Select.vue'
import { useClipboard } from '@/common/composables/useClipboard'
import { getPersistedPageSize, setPersistedPageSize } from '@/common/composables/usePersistedPageSize'
import { useAppStore } from '@/core/stores/appStore'
import { keysAPI } from '@/api'
import {
  cancelBatchImageJob,
  deleteBatchImageJobRecord,
  downloadBatchImageZip,
  getBatchImageItemContent,
  getBatchImageJob,
  listBatchImageJobs,
  listBatchImageItems,
  listBatchImageModels,
  saveBlob,
  submitBatchImageJob,
  type BatchImageItem,
  type BatchImageJob,
  type BatchImageJobsListOptions,
  type BatchImageReferenceImage,
  type BatchImageStatus,
  type BatchImageSubmitItem,
} from '@/features/batch-image/data/datasources/batchImageDatasource'
import { createBatchImagePreviewCache } from '@/features/batch-image/presentation/preview/batchImagePreviewCache'
import { buildBatchImageAgentInstruction } from '@/features/batch-image/presentation/resolvers/batchImageAgentInstruction'
import { createBatchImageMessages } from '@/features/batch-image/presentation/resolvers/batchImageMessages'
import {
  createBatchImageGenerationScope,
  createBatchImageLatestSingleFlight,
  replaceBatchImageObjectURL,
  revokeBatchImageObjectURLs,
  type BatchImageGenerationSnapshot,
} from '@/features/batch-image/presentation/composables/batchImageAsyncLifecycle'
import { useBatchImagePromptPopover } from '@/features/batch-image/presentation/composables/useBatchImagePromptPopover'
import {
  batchImageItemStatusLabel,
  batchImageJobStatusLabel,
} from '@/features/batch-image/presentation/batchImageLocale'
import type { ApiKey } from '@/types'
import type { Column } from '@/common/types/uiTypes'

export function useBatchImageGuideController() {
  type BatchImageJobRow = Pick<BatchImageJob, 'id' | 'task_name' | 'parent_batch_id' | 'status' | 'model' | 'provider' | 'item_count' | 'success_count' | 'fail_count' | 'estimated_cost' | 'hold_amount' | 'actual_cost' | 'created_at' | 'downloaded_at'> & {
    api_key_id: number
    api_key_name: string
    child_count: number
    is_child?: boolean
  }

  type BatchImageDetailItem = BatchImageItem & {
    batch_id: string
    source_task_name: string
  }

  type PromptRow = {
    localId: string
    custom_id: string
    prompt: string
    output_count: number
    reference_images: BatchImageReferenceImage[]
  }

  type ReferenceImageDraft = BatchImageReferenceImage & {
    name: string
    size: number
  }

  type DetailRefreshRequest = {
    snapshot: BatchImageGenerationSnapshot
    batchId: string
    apiKey: string
  }

  type PreviewRequest = {
    detailSnapshot: BatchImageGenerationSnapshot
    previewSnapshot: BatchImageGenerationSnapshot
    ownerBatchId: string
  }

  const TERMINAL_STATUSES = new Set(['completed', 'failed', 'cancelled', 'output_deleted'])
  const BATCH_IMAGE_MAX_OUTPUTS_PER_ITEM = 4
  const BATCH_IMAGE_MAX_OUTPUTS_PER_JOB = 200
  const outputCountOptions = Array.from({ length: BATCH_IMAGE_MAX_OUTPUTS_PER_ITEM }, (_, index) => index + 1)
  const batchPageSizeOptions: SelectOption[] = [20, 50, 100].map(size => ({ value: size, label: String(size) }))

  const appStore = useAppStore()
  const { copyToClipboard } = useClipboard()
  const { t, locale } = useI18n()
  const { batchImageText, batchImageErrorMessage } = createBatchImageMessages({
    text: key => t(key),
    interpolate: (key, params) => t(key, params),
    locale: () => String(locale.value || ''),
  })
  const {
    promptPopover,
    cancelPromptPopoverClose,
    closePromptPopover,
    schedulePromptPopoverClose,
    schedulePromptPopoverOpen,
    showPromptPopover,
    copyPromptPopover,
  } = useBatchImagePromptPopover({
    copy: text => {
      void copyToClipboard(text, t('batchImage.promptPopover.copied'))
    },
  })
  const {
    isSupported: previewCacheSupported,
    key: previewCacheKey,
    get: getCachedPreviewBlob,
    put: putCachedPreviewBlob,
    cleanup: cleanupPreviewCache,
    createThumbnail: createThumbnailBlob,
  } = createBatchImagePreviewCache()

  const columns = computed<Column[]>(() => [
    { key: 'select', label: '', sortable: false, class: 'w-12 text-center' },
    { key: 'id', label: t('batchImage.columns.taskName'), sortable: false, class: 'w-[240px] max-w-[240px]' },
    { key: 'model', label: t('batchImage.columns.model'), sortable: false, class: 'w-[180px] max-w-[180px] text-center' },
    { key: 'api_key_name', label: t('batchImage.columns.apiKey'), sortable: false, class: 'w-40 max-w-40 text-center' },
    { key: 'status', label: t('common.status'), sortable: false, class: 'w-28 text-center' },
    { key: 'counts', label: t('batchImage.columns.result'), sortable: false, class: 'w-32 text-center' },
    { key: 'cost', label: t('batchImage.columns.cost'), sortable: false, class: 'w-36 text-center' },
    { key: 'downloaded', label: t('batchImage.columns.downloadStatus'), sortable: false, class: 'w-40 text-center' },
    { key: 'actions', label: t('common.actions'), sortable: false, class: 'w-40 text-center' },
  ])

  const statusFilterOptions = computed<SelectOption[]>(() => [
    { value: '', label: t('batchImage.filters.allStatuses') },
    { value: 'queued', label: t('batchImage.status.queued') },
    { value: 'running', label: t('batchImage.status.running') },
    { value: 'processing_results', label: t('batchImage.status.processingResults') },
    { value: 'settling', label: t('batchImage.status.settling') },
    { value: 'completed', label: t('batchImage.status.completed') },
    { value: 'failed', label: t('batchImage.status.failed') },
    { value: 'cancelled', label: t('batchImage.status.cancelled') },
    { value: 'output_deleted', label: t('batchImage.status.outputDeleted') },
  ])

  const downloadFilterOptions = computed<SelectOption[]>(() => [
    { value: '', label: t('batchImage.filters.allDownloadStates') },
    { value: 'true', label: t('batchImage.filters.downloaded') },
    { value: 'false', label: t('batchImage.filters.notDownloaded') },
  ])

  const form = reactive({
    apiKeyId: 0,
    taskName: '',
    model: '',
    responseMimeType: 'image/png',
  })

  const filters = reactive({
    taskName: '',
    apiKeyId: '',
    status: '',
    downloaded: '',
  })

  const pagination = reactive({
    page: 1,
    page_size: Math.min(getPersistedPageSize(20), 100),
    has_more: false,
  })

  const apiKeys = ref<ApiKey[]>([])
  const loadingKeys = ref(false)
  const loadingJobs = ref(false)
  const submitting = ref(false)
  const refreshing = ref(false)
  const cancelling = ref(false)
  const downloading = ref(false)
  const downloadingBatchId = ref('')
  const retryingBatchId = ref('')
  const bulkDownloading = ref(false)
  const bulkDeleting = ref(false)
  const deletingBatchId = ref('')
  const loadingItems = ref(false)
  const loadingModels = ref(false)
  const showCreateModal = ref(false)
  const showGuideModal = ref(false)
  const currentJob = ref<BatchImageJob | null>(null)
  const selectedBatchId = ref('')
  const selectedBatchApiKeyId = ref(0)
  const items = ref<BatchImageDetailItem[]>([])
  const batchJobs = ref<BatchImageJobRow[]>([])
  const selectedJobIds = ref(new Set<string>())
  const expandedParentIds = ref(new Set<string>())
  const promptRows = ref<PromptRow[]>([])
  const promptDraft = ref('')
  const customIdDraft = ref('')
  const outputCountDraft = ref(1)
  const referenceImageDrafts = ref<ReferenceImageDraft[]>([])
  const itemPreviewUrls = reactive<Record<string, string>>({})
  const previewLoadingIds = ref(new Set<string>())
  const previewErrorIds = ref(new Set<string>())
  const previewImageItem = ref<BatchImageItem | null>(null)
  const previewLoadingGenerations = new Map<string, number>()
  const availableBatchImageModels = ref<Array<{ value: string; label: string }>>([])
  const modelLoadError = ref('')
  const openMoreJobId = ref('')
  const moreMenuStyle = ref<Record<string, string>>({})
  let modelRequestSeq = 0
  let pollTimer: ReturnType<typeof setInterval> | null = null
  let previewCacheCleanupTimer: ReturnType<typeof setInterval> | null = null
  const detailGeneration = createBatchImageGenerationScope()
  const previewGeneration = createBatchImageGenerationScope()
  const refreshSingleFlight = createBatchImageLatestSingleFlight<DetailRefreshRequest>({
    key: request => `${request.snapshot.generation}:${request.batchId}`,
    isCurrent: isDetailRefreshCurrent,
    run: performDetailRefresh,
    onBusyChange: (busy) => {
      refreshing.value = busy
    },
  })

  const geminiApiKeys = computed(() =>
    apiKeys.value.filter((key) =>
      key.status === 'active' &&
      key.group?.platform === 'gemini' &&
      key.group?.allow_batch_image_generation === true,
    ),
  )

  const selectedApiKey = computed(() =>
    geminiApiKeys.value.find((key) => key.id === Number(form.apiKeyId)) || null,
  )

  const filteredApiKeys = computed(() => {
    const selectedFilterID = Number(filters.apiKeyId || 0)
    if (!selectedFilterID) return geminiApiKeys.value
    return geminiApiKeys.value.filter(key => key.id === selectedFilterID)
  })

  const apiKeyFilterOptions = computed<SelectOption[]>(() => [
    { value: '', label: t('batchImage.filters.allApiKeys') },
    ...geminiApiKeys.value.map(key => ({
      value: String(key.id),
      label: key.name || `API Key #${key.id}`,
    })),
  ])

  const selectedRows = computed(() =>
    batchJobs.value.filter(job => selectedJobIds.value.has(job.id)),
  )

  const childrenByParent = computed(() => {
    const groups = new Map<string, BatchImageJobRow[]>()
    for (const job of batchJobs.value) {
      if (!job.parent_batch_id) continue
      const rows = groups.get(job.parent_batch_id) || []
      rows.push(job)
      groups.set(job.parent_batch_id, rows)
    }
    for (const rows of groups.values()) {
      rows.sort((a, b) => a.created_at - b.created_at)
    }
    return groups
  })

  const visibleBatchJobs = computed(() => {
    const rows: BatchImageJobRow[] = []
    for (const job of batchJobs.value.filter(item => !item.parent_batch_id)) {
      rows.push(job)
      if (expandedParentIds.value.has(job.id)) {
        rows.push(...(childrenByParent.value.get(job.id) || []).map(child => ({ ...child, is_child: true })))
      }
    }
    return rows
  })

  const selectedDownloadableRows = computed(() =>
    selectedRows.value.filter(job => canDownload(job)),
  )

  const allVisibleSelected = computed(() =>
    visibleBatchJobs.value.length > 0 && visibleBatchJobs.value.every(job => selectedJobIds.value.has(job.id)),
  )

  const someVisibleSelected = computed(() =>
    visibleBatchJobs.value.some(job => selectedJobIds.value.has(job.id)) && !allVisibleSelected.value,
  )

  const previewImageUrl = computed(() => {
    const item = previewImageItem.value
    if (!item) return ''
    return itemPreviewUrls[itemPreviewKey(item)] || ''
  })

  const recoveredOriginalCustomIds = computed(() => {
    const rootBatchId = detailRootBatchId()
    if (!rootBatchId) return new Set<string>()
    const ids = new Set<string>()
    for (const item of items.value) {
      if (!isChildDetailItem(item) || !isSuccessfulImageItem(item)) continue
      const sourceCustomID = retrySourceCustomID(item.custom_id)
      if (sourceCustomID) ids.add(sourceCustomID)
    }
    return ids
  })

  const currentDisplayJob = computed(() => {
    if (!currentJob.value) return null
    return displayJob(currentJob.value)
  })

  const endpointBase = computed(() => {
    const configured = appStore.apiBaseUrl?.trim()
    if (configured) return configured.replace(/\/+$/, '')
    if (typeof window !== 'undefined') return window.location.origin.replace(/\/+$/, '')
    return '<你的 Sub2API API 端点>'
  })

  const selectedModelReferenceLimit = computed(() => referenceImageLimitForModel(form.model))

  const estimatedOutputCount = computed(() =>
    promptRows.value.reduce((sum, row) => sum + normalizeOutputCount(row.output_count), 0),
  )

  const parsedItems = computed<BatchImageSubmitItem[]>(() => {
    const used = new Set<string>()
    return promptRows.value
      .map((row, index) => {
        const customID = uniqueCustomID(row.custom_id || `img_${String(index + 1).padStart(3, '0')}`, used, index)
        const item: BatchImageSubmitItem = { custom_id: customID, prompt: row.prompt.trim() }
        const outputCount = normalizeOutputCount(row.output_count)
        if (outputCount > 1) {
          item.output_count = outputCount
        }
        if (row.reference_images.length) {
          item.reference_images = row.reference_images
        }
        return item
      })
      .filter(item => item.prompt)
  })

  function referenceImageLimitForModel(model: string) {
    const normalized = String(model || '').toLowerCase()
    if (normalized.includes('pro-image')) return 14
    if (normalized.includes('flash-image')) return 3
    return 0
  }

  const agentInstruction = computed(() => buildBatchImageAgentInstruction(endpointBase.value))

  function uniqueCustomID(raw: string, used: Set<string>, index: number): string {
    const base = raw.replace(/[^\w.-]+/g, '_').replace(/^_+|_+$/g, '') || `img_${String(index + 1).padStart(3, '0')}`
    let candidate = base
    let suffix = 2
    while (used.has(candidate)) {
      candidate = `${base}_${suffix}`
      suffix += 1
    }
    used.add(candidate)
    return candidate
  }

  function normalizeOutputCount(value: unknown): number {
    const parsed = Math.floor(Number(value || 1))
    if (!Number.isFinite(parsed)) return 1
    return Math.min(BATCH_IMAGE_MAX_OUTPUTS_PER_ITEM, Math.max(1, parsed))
  }

  function addPromptRow() {
    const prompt = promptDraft.value.trim()
    if (!prompt) return
    const outputCount = normalizeOutputCount(outputCountDraft.value)
    const used = new Set(promptRows.value.map(row => row.custom_id))
    const customID = uniqueCustomID(customIdDraft.value || `img_${String(promptRows.value.length + 1).padStart(3, '0')}`, used, promptRows.value.length)
    promptRows.value = [
      ...promptRows.value,
      {
        localId: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        custom_id: customID,
        prompt,
        output_count: outputCount,
        reference_images: referenceImageDrafts.value.map(({ name: _name, size: _size, ...ref }) => ref),
      },
    ]
    promptDraft.value = ''
    customIdDraft.value = ''
    outputCountDraft.value = 1
    referenceImageDrafts.value = []
  }

  function removePromptRow(index: number) {
    promptRows.value = promptRows.value.filter((_, currentIndex) => currentIndex !== index)
  }

  function removeReferenceImageDraft(index: number) {
    referenceImageDrafts.value = referenceImageDrafts.value.filter((_, currentIndex) => currentIndex !== index)
  }

  async function handleReferenceImageFiles(event: Event) {
    const input = event.target as HTMLInputElement
    const files = Array.from(input.files || [])
    input.value = ''
    if (files.length === 0) return
    const limit = selectedModelReferenceLimit.value
    if (limit <= 0) {
      appStore.showError(t('batchImage.create.modelNoReferenceImages'))
      return
    }
    const slots = Math.max(0, limit - referenceImageDrafts.value.length)
    if (slots <= 0) {
      appStore.showError(t('batchImage.create.refLimitReached', { limit }))
      return
    }
    const accepted = files.slice(0, slots)
    if (accepted.length < files.length) {
      appStore.showError(t('batchImage.create.refLimitExceededIgnored', { limit }))
    }
    const next: ReferenceImageDraft[] = []
    for (const file of accepted) {
      if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) {
        appStore.showError(t('batchImage.create.refFormatUnsupported'))
        continue
      }
      if (file.size > 10 * 1024 * 1024) {
        appStore.showError(t('batchImage.create.refFileTooLarge', { name: file.name }))
        continue
      }
      const data = await readFileAsBase64(file)
      next.push({
        id: file.name,
        type: 'reference',
        mime_type: file.type,
        data,
        name: file.name,
        size: file.size,
      })
    }
    referenceImageDrafts.value = [...referenceImageDrafts.value, ...next]
  }

  function readFileAsBase64(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onerror = () => reject(reader.error || new Error('Failed to read file'))
      reader.onload = () => {
        const result = String(reader.result || '')
        resolve(result.includes(',') ? result.slice(result.indexOf(',') + 1) : result)
      }
      reader.readAsDataURL(file)
    })
  }

  async function loadApiKeys() {
    loadingKeys.value = true
    try {
      const response = await keysAPI.list(1, 100, { status: 'active', sort_by: 'created_at', sort_order: 'desc' })
      apiKeys.value = response.items || []
      if (!selectedApiKey.value && geminiApiKeys.value.length > 0) {
        form.apiKeyId = geminiApiKeys.value[0].id
      }
      if (filters.apiKeyId && !geminiApiKeys.value.some(key => String(key.id) === filters.apiKeyId)) {
        filters.apiKeyId = ''
      }
      if (!selectedApiKey.value) {
        availableBatchImageModels.value = []
        form.model = ''
      }
    } catch (error: any) {
      appStore.showError(batchImageErrorMessage(error, batchImageText('loadKeysFailed')))
    } finally {
      loadingKeys.value = false
    }
  }

  async function loadAvailableModels() {
    const key = selectedApiKey.value
    const requestID = ++modelRequestSeq
    modelLoadError.value = ''
    availableBatchImageModels.value = []
    form.model = ''
    if (!key) return

    loadingModels.value = true
    try {
      const result = await listBatchImageModels(key.key)
      if (requestID !== modelRequestSeq) return
      const seen = new Set<string>()
      availableBatchImageModels.value = (result.data || [])
        .map(model => String(model.id || '').trim())
        .filter((model) => {
          if (!model || seen.has(model)) return false
          seen.add(model)
          return true
        })
        .map(model => ({ value: model, label: model }))
      form.model = availableBatchImageModels.value[0]?.value || ''
    } catch (error: any) {
      if (requestID !== modelRequestSeq) return
      modelLoadError.value = batchImageErrorMessage(error, batchImageText('loadModelsFailed'))
    } finally {
      if (requestID === modelRequestSeq) {
        loadingModels.value = false
      }
    }
  }

  async function refreshPage() {
    await loadApiKeys()
    await loadBatchJobs()
  }

  function applyFilters() {
    pagination.page = 1
    selectedJobIds.value = new Set()
    void loadBatchJobs()
  }

  function resetFilters() {
    filters.taskName = ''
    filters.apiKeyId = ''
    filters.status = ''
    filters.downloaded = ''
    applyFilters()
  }

  function listOptions(): BatchImageJobsListOptions {
    const options: BatchImageJobsListOptions = {
      limit: pagination.page_size,
      cursor: String((pagination.page - 1) * pagination.page_size),
    }
    if (filters.taskName.trim()) options.taskName = filters.taskName.trim()
    if (filters.status) options.status = filters.status
    if (filters.downloaded) options.downloaded = filters.downloaded
    return options
  }

  function toJobRow(job: BatchImageJob, key = selectedApiKey.value): BatchImageJobRow {
    return {
      id: job.id,
      task_name: job.task_name || defaultTaskName(job.created_at),
      parent_batch_id: job.parent_batch_id || null,
      status: job.status,
      model: job.model,
      provider: job.provider,
      item_count: job.item_count,
      success_count: job.success_count,
      fail_count: job.fail_count,
      estimated_cost: job.estimated_cost,
      hold_amount: job.hold_amount,
      actual_cost: job.actual_cost,
      created_at: job.created_at,
      downloaded_at: job.downloaded_at,
      api_key_id: key?.id || 0,
      api_key_name: key?.name || '',
      child_count: 0,
    }
  }

  function applyChildCounts(rows: BatchImageJobRow[]) {
    const counts = new Map<string, number>()
    for (const row of rows) {
      if (!row.parent_batch_id) continue
      counts.set(row.parent_batch_id, (counts.get(row.parent_batch_id) || 0) + 1)
    }
    return rows.map(row => ({ ...row, child_count: counts.get(row.id) || 0 }))
  }

  function displayJob<T extends Pick<BatchImageJob, 'id' | 'parent_batch_id' | 'status' | 'item_count' | 'success_count' | 'fail_count' | 'estimated_cost' | 'hold_amount' | 'actual_cost'>>(job: T): T {
    if (job.parent_batch_id) return job
    const children = childrenByParent.value.get(job.id) || []
    if (!children.length) return job

    const childSuccess = children.reduce((sum, child) => sum + child.success_count, 0)
    const childEstimated = children.reduce((sum, child) => sum + child.estimated_cost, 0)
    const childHold = children.reduce((sum, child) => sum + child.hold_amount, 0)
    const childActual = children.reduce((sum, child) => sum + (child.actual_cost || 0), 0)
    const childActualReady = children.every(child => child.actual_cost !== null)
    const successCount = Math.min(job.item_count, job.success_count + childSuccess)
    const failCount = Math.max(0, job.item_count - successCount)
    const actualCost = job.actual_cost === null
      ? (childActualReady ? childActual : null)
      : job.actual_cost + childActual

    return {
      ...job,
      success_count: successCount,
      fail_count: failCount,
      status: failCount === 0 && TERMINAL_STATUSES.has(job.status) ? 'completed' : job.status,
      estimated_cost: job.estimated_cost + childEstimated,
      hold_amount: job.hold_amount + childHold,
      actual_cost: actualCost,
    }
  }

  function hasChildJobs(batchId: string) {
    return (childrenByParent.value.get(batchId) || []).length > 0
  }

  function toggleChildRows(batchId: string) {
    const next = new Set(expandedParentIds.value)
    if (next.has(batchId)) next.delete(batchId)
    else next.add(batchId)
    expandedParentIds.value = next
  }

  function closeMoreMenu() {
    openMoreJobId.value = ''
  }

  function toggleMoreMenu(job: BatchImageJobRow, event: MouseEvent) {
    if (openMoreJobId.value === job.id) {
      closeMoreMenu()
      return
    }
    const trigger = event.currentTarget as HTMLElement | null
    const rect = trigger?.getBoundingClientRect()
    if (!rect) return
    const menuWidth = 176
    const margin = 8
    const left = Math.max(margin, Math.min(rect.right - menuWidth, window.innerWidth - menuWidth - margin))
    const top = Math.min(rect.bottom + margin, window.innerHeight - 96)
    moreMenuStyle.value = {
      left: `${left}px`,
      top: `${Math.max(margin, top)}px`,
    }
    openMoreJobId.value = job.id
  }

  async function loadBatchJobs() {
    const keys = filteredApiKeys.value
    if (!keys.length) {
      batchJobs.value = []
      pagination.has_more = false
      return
    }
    loadingJobs.value = true
    closeMoreMenu()
    try {
      const options = listOptions()
      const results = await Promise.all(keys.map(async (key) => {
        const result = await listBatchImageJobs(key.key, options)
        return {
          hasMore: Boolean(result.has_more),
          rows: (result.data || []).map(job => toJobRow(job, key)),
        }
      }))
      batchJobs.value = applyChildCounts(results
        .flatMap(result => result.rows)
        .sort((a, b) => b.created_at - a.created_at)
        .slice(0, pagination.page_size))
      pagination.has_more = results.some(result => result.hasMore)
      selectedJobIds.value = new Set([...selectedJobIds.value].filter(id => visibleBatchJobs.value.some(job => job.id === id)))
    } catch (error: any) {
      appStore.showError(batchImageErrorMessage(error, batchImageText('loadJobsFailed')))
    } finally {
      loadingJobs.value = false
    }
  }

  function upsertJob(job: BatchImageJob) {
    const next = toJobRow(job)
    const index = batchJobs.value.findIndex(item => item.id === job.id)
    if (index >= 0) {
      const rows = [...batchJobs.value]
      rows[index] = { ...next, is_child: rows[index].is_child }
      batchJobs.value = applyChildCounts(rows)
      return
    }
    batchJobs.value = applyChildCounts([next, ...batchJobs.value].slice(0, pagination.page_size))
  }

  function handlePageChange(page: number) {
    if (page < 1 || page === pagination.page) return
    pagination.page = page
    selectedJobIds.value = new Set()
    void loadBatchJobs()
  }

  function handlePageSizeChange(value: string | number | boolean | null) {
    if (value === null || typeof value === 'boolean') return
    const nextSize = Math.min(Math.max(Number(value) || 20, 1), 100)
    pagination.page_size = nextSize
    pagination.page = 1
    setPersistedPageSize(nextSize)
    selectedJobIds.value = new Set()
    void loadBatchJobs()
  }

  function openCreateModal() {
    showCreateModal.value = true
    if (!apiKeys.value.length) {
      void loadApiKeys()
    }
  }

  function closeCreateModal() {
    if (submitting.value) return
    showCreateModal.value = false
    resetCreateDraft()
  }

  function resetCreateDraft() {
    form.taskName = ''
    form.responseMimeType = 'image/png'
    promptRows.value = []
    promptDraft.value = ''
    customIdDraft.value = ''
    outputCountDraft.value = 1
    referenceImageDrafts.value = []
  }

  function closeDetail() {
    invalidateDetailGeneration()
    closePromptPopover()
    currentJob.value = null
    selectedBatchId.value = ''
    selectedBatchApiKeyId.value = 0
    items.value = []
    loadingItems.value = false
    clearItemPreviews()
  }

  function keyForSelectedBatch(): ApiKey | null {
    if (selectedBatchApiKeyId.value) {
      const key = geminiApiKeys.value.find(item => item.id === selectedBatchApiKeyId.value)
      if (key) return key
    }
    return selectedApiKey.value
  }

  function requireApiKey(): ApiKey | null {
    if (!selectedApiKey.value) {
      appStore.showError(batchImageText('selectApiKey'))
      return null
    }
    return selectedApiKey.value
  }

  function validateForm(): boolean {
    if (!requireApiKey()) return false
    if (!form.model) {
      appStore.showError(availableBatchImageModels.value.length === 0 ? batchImageText('noModelsForKey') : batchImageText('selectModel'))
      return false
    }
    if (parsedItems.value.length === 0) {
      appStore.showError(batchImageText('promptRequired'))
      return false
    }
    if (estimatedOutputCount.value > BATCH_IMAGE_MAX_OUTPUTS_PER_JOB) {
      appStore.showError(batchImageText('tooManyOutputImages'))
      return false
    }
    const refLimit = selectedModelReferenceLimit.value
    if (promptRows.value.some(row => row.reference_images.length > refLimit)) {
      appStore.showError(batchImageText('tooManyReferenceImages'))
      return false
    }
    return true
  }

  async function submitJob() {
    if (submitting.value) return
    if (promptDraft.value.trim()) addPromptRow()
    if (!validateForm()) return
    const key = requireApiKey()
    if (!key) return
    submitting.value = true
    try {
      const job = await submitBatchImageJob(
        key.key,
        {
          model: form.model,
          task_name: form.taskName.trim() || defaultTaskName(),
          image_size: '1K',
          response_mime_type: form.responseMimeType,
          items: parsedItems.value,
        },
        `sub2api-ui-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`,
      )
      invalidateDetailGeneration()
      currentJob.value = job
      selectedBatchId.value = job.id
      selectedBatchApiKeyId.value = key.id
      items.value = []
      upsertJob(job)
      showCreateModal.value = false
      resetCreateDraft()
      appStore.showSuccess(batchImageText('submitted'))
      void loadItems()
      startPolling()
    } catch (error: any) {
      appStore.showError(batchImageErrorMessage(error, batchImageText('submitFailed')))
    } finally {
      submitting.value = false
    }
  }

  function invalidateDetailGeneration() {
    detailGeneration.invalidate()
    refreshSingleFlight.clearPending()
    loadingItems.value = false
  }

  function isDetailSnapshotCurrent(
    snapshot: BatchImageGenerationSnapshot,
    batchId: string,
  ) {
    return detailGeneration.isCurrent(snapshot, selectedBatchId.value)
      && selectedBatchId.value === batchId
  }

  function isDetailRefreshCurrent(request: DetailRefreshRequest) {
    return isDetailSnapshotCurrent(request.snapshot, request.batchId)
  }

  function captureDetailRefreshRequest(): DetailRefreshRequest | null {
    const batchId = selectedBatchId.value
    if (!batchId) return null
    const key = keyForSelectedBatch() || requireApiKey()
    if (!key) return null
    return {
      snapshot: detailGeneration.capture(batchId),
      batchId,
      apiKey: key.key,
    }
  }

  async function performDetailRefresh(request: DetailRefreshRequest) {
    try {
      const job = await getBatchImageJob(request.apiKey, request.batchId)
      if (!isDetailRefreshCurrent(request)) return
      currentJob.value = job
      upsertJob(job)
      if (TERMINAL_STATUSES.has(job.status)) stopPolling()
    } catch (error: any) {
      if (isDetailRefreshCurrent(request)) {
        appStore.showError(batchImageErrorMessage(error, batchImageText('refreshFailed')))
      }
    }
  }

  function refreshSelected() {
    const request = captureDetailRefreshRequest()
    if (!request) return Promise.resolve()
    return refreshSingleFlight.request(request)
  }

  async function refreshDetail() {
    await Promise.all([
      refreshSelected(),
      loadItems(),
    ])
  }

  function selectJob(batchId: string) {
    invalidateDetailGeneration()
    const row = batchJobs.value.find(job => job.id === batchId)
    if (row?.api_key_id && geminiApiKeys.value.some(key => key.id === row.api_key_id)) {
      form.apiKeyId = row.api_key_id
      selectedBatchApiKeyId.value = row.api_key_id
    } else {
      selectedBatchApiKeyId.value = 0
    }
    selectedBatchId.value = batchId
    currentJob.value = null
    items.value = []
    void refreshSelected()
    void loadItems()
  }

  function startPolling() {
    stopPolling()
    pollTimer = setInterval(() => {
      if (!currentJob.value || TERMINAL_STATUSES.has(currentJob.value.status)) {
        stopPolling()
        return
      }
      void refreshSelected()
    }, 8000)
  }

  function stopPolling() {
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  }

  function canCancel(job: Pick<BatchImageJob, 'status'>) {
    return !TERMINAL_STATUSES.has(job.status)
  }

  function canDownload(job: Pick<BatchImageJob, 'status' | 'success_count'>) {
    return job.status === 'completed' && job.success_count > 0
  }

  function canRetry(job: Pick<BatchImageJob, 'status' | 'fail_count'>) {
    const display = 'id' in job ? displayJob(job as BatchImageJob) : job
    return TERMINAL_STATUSES.has(display.status) && display.fail_count > 0
  }

  function isDownloadingJob(batchId: string) {
    return downloading.value && downloadingBatchId.value === batchId
  }

  function applyJobApiKey(job: BatchImageJobRow | Pick<BatchImageJob, 'id'>) {
    if ('api_key_id' in job && job.api_key_id && geminiApiKeys.value.some(key => key.id === job.api_key_id)) {
      form.apiKeyId = job.api_key_id
    }
  }

  function apiKeyForJob(job: BatchImageJobRow | Pick<BatchImageJob, 'id'>): ApiKey | null {
    if ('api_key_id' in job && job.api_key_id) {
      return geminiApiKeys.value.find(key => key.id === job.api_key_id) || null
    }
    return selectedApiKey.value
  }

  function toggleJobSelection(batchId: string, checked: boolean) {
    const next = new Set(selectedJobIds.value)
    if (checked) next.add(batchId)
    else next.delete(batchId)
    selectedJobIds.value = next
  }

  function toggleAllVisible(checked: boolean) {
    const next = new Set(selectedJobIds.value)
    for (const job of visibleBatchJobs.value) {
      if (checked) next.add(job.id)
      else next.delete(job.id)
    }
    selectedJobIds.value = next
  }

  function canDeleteRecord(job: Pick<BatchImageJob, 'status'>) {
    return TERMINAL_STATUSES.has(job.status)
  }

  async function cancelSelected() {
    if (!currentJob.value) return
    const key = keyForSelectedBatch() || requireApiKey()
    if (!key) return
    if (!window.confirm(batchImageText('cancelConfirm'))) return
    cancelling.value = true
    try {
      const job = await cancelBatchImageJob(key.key, currentJob.value.id)
      currentJob.value = job
      upsertJob(job)
      appStore.showSuccess(batchImageText('cancelled'))
    } catch (error: any) {
      appStore.showError(batchImageErrorMessage(error, batchImageText('cancelFailed')))
    } finally {
      cancelling.value = false
    }
  }

  async function downloadSelected() {
    if (!currentJob.value) return
    await downloadJob(currentJob.value)
  }

  async function retrySelected() {
    if (!currentJob.value) return
    await retryFailedJob(currentJob.value)
  }

  async function retryFailedJob(job: BatchImageJobRow | BatchImageJob) {
    if (!canRetry(job) || retryingBatchId.value) return
    closeMoreMenu()
    const key = apiKeyForJob(job) || keyForSelectedBatch() || requireApiKey()
    if (!key) return
    retryingBatchId.value = job.id
    try {
      const sourceItems = await ensureItemsForRetry(key.key, job.id)
      const failedItems = sourceItems
        .filter(item => item.status === 'failed')
        .map(item => ({ custom_id: retryCustomID(item.custom_id), prompt: String(item.prompt_preview || '').trim() }))
        .filter(item => item.prompt)
      if (failedItems.length === 0) {
        appStore.showError(batchImageText('retryMissingPrompts'))
        return
      }
      const retryJob = await submitBatchImageJob(
        key.key,
        {
          model: job.model,
          task_name: `${job.task_name || defaultTaskName()} ${t('batchImage.messages.retryTaskNameSuffix')}`,
          parent_batch_id: rootBatchIdForRetry(job),
          provider: job.provider,
          image_size: '1K',
          response_mime_type: form.responseMimeType,
          items: failedItems,
        },
        `sub2api-ui-retry-${job.id}-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`,
      )
      invalidateDetailGeneration()
      currentJob.value = retryJob
      selectedBatchId.value = retryJob.id
      selectedBatchApiKeyId.value = key.id
      items.value = []
      upsertJob(retryJob)
      if (retryJob.parent_batch_id) {
        expandedParentIds.value = new Set([...expandedParentIds.value, retryJob.parent_batch_id])
      }
      appStore.showSuccess(batchImageText('retrySubmitted'))
      void loadItems()
      startPolling()
    } catch (error: any) {
      appStore.showError(batchImageErrorMessage(error, batchImageText('retryFailed')))
    } finally {
      retryingBatchId.value = ''
    }
  }

  async function ensureItemsForRetry(apiKey: string, batchId: string) {
    if (selectedBatchId.value === batchId && items.value.length > 0) {
      return items.value
    }
    const result = await listBatchImageItems(apiKey, batchId)
    return result.data || []
  }

  function retryCustomID(customID: string) {
    const base = String(customID || 'item').replace(/[^\w.-]+/g, '_').replace(/^_+|_+$/g, '') || 'item'
    return `${base}_retry_${Date.now().toString(36)}`
  }

  function rootBatchIdForRetry(job: BatchImageJobRow | BatchImageJob) {
    return job.parent_batch_id || job.id
  }

  async function downloadJob(job: (BatchImageJobRow | Pick<BatchImageJob, 'id'>)) {
    if (downloading.value) return
    closeMoreMenu()
    applyJobApiKey(job)
    const key = apiKeyForJob(job) || requireApiKey()
    if (!key) return
    downloading.value = true
    downloadingBatchId.value = job.id
    try {
      const blob = await downloadBatchImageZip(key.key, job.id)
      saveBlob(blob, `${job.id}.zip`)
      markJobDownloaded(job.id)
    } catch (error: any) {
      appStore.showError(batchImageErrorMessage(error, batchImageText('downloadFailed')))
    } finally {
      downloading.value = false
      downloadingBatchId.value = ''
    }
  }

  async function downloadSelectedJobs() {
    if (bulkDownloading.value || selectedDownloadableRows.value.length === 0) return
    bulkDownloading.value = true
    try {
      for (const row of selectedDownloadableRows.value) {
        const key = apiKeyForJob(row)
        if (!key) continue
        downloading.value = true
        downloadingBatchId.value = row.id
        const blob = await downloadBatchImageZip(key.key, row.id)
        saveBlob(blob, `${row.id}.zip`)
        markJobDownloaded(row.id)
      }
      appStore.showSuccess(batchImageText('batchDownloadStarted'))
    } catch (error: any) {
      appStore.showError(batchImageErrorMessage(error, batchImageText('downloadFailed')))
    } finally {
      bulkDownloading.value = false
      downloading.value = false
      downloadingBatchId.value = ''
    }
  }

  async function deleteJob(job: BatchImageJobRow) {
    if (!canDeleteRecord(job) || deletingBatchId.value) return
    closeMoreMenu()
    const key = apiKeyForJob(job)
    if (!key) return
    if (!window.confirm(batchImageText('deleteConfirm'))) return
    deletingBatchId.value = job.id
    try {
      await deleteBatchImageJobRecord(key.key, job.id)
      removeJobFromList(job.id)
      appStore.showSuccess(batchImageText('deleted'))
    } catch (error: any) {
      appStore.showError(batchImageErrorMessage(error, batchImageText('deleteFailed')))
    } finally {
      deletingBatchId.value = ''
    }
  }

  async function deleteSelectedJobs() {
    const rows = selectedRows.value.filter(job => canDeleteRecord(job))
    if (bulkDeleting.value || rows.length === 0) return
    if (!window.confirm(batchImageText('deleteSelectedConfirm'))) return
    bulkDeleting.value = true
    try {
      for (const row of rows) {
        const key = apiKeyForJob(row)
        if (!key) continue
        deletingBatchId.value = row.id
        await deleteBatchImageJobRecord(key.key, row.id)
        removeJobFromList(row.id)
      }
      appStore.showSuccess(batchImageText('deleted'))
    } catch (error: any) {
      appStore.showError(batchImageErrorMessage(error, batchImageText('deleteFailed')))
    } finally {
      bulkDeleting.value = false
      deletingBatchId.value = ''
    }
  }

  function markJobDownloaded(batchId: string) {
    const downloadedAt = Math.floor(Date.now() / 1000)
    batchJobs.value = batchJobs.value.map(job => job.id === batchId ? { ...job, downloaded_at: job.downloaded_at || downloadedAt } : job)
    if (currentJob.value?.id === batchId && !currentJob.value.downloaded_at) {
      currentJob.value = { ...currentJob.value, downloaded_at: downloadedAt }
    }
  }

  function removeJobFromList(batchId: string) {
    batchJobs.value = batchJobs.value.filter(job => job.id !== batchId)
    toggleJobSelection(batchId, false)
    if (currentJob.value?.id === batchId) closeDetail()
  }

  function canLoadItemPreview(item: BatchImageItem) {
    return (item.status === 'succeeded' || item.status === 'success') && item.image_count > 0
  }

  function isSuccessfulImageItem(item: Pick<BatchImageItem, 'status' | 'image_count'>) {
    return (item.status === 'succeeded' || item.status === 'success') && item.image_count > 0
  }

  function detailRootBatchId() {
    return currentJob.value?.parent_batch_id || selectedBatchId.value || currentJob.value?.id || ''
  }

  function isChildDetailItem(item: Pick<BatchImageDetailItem, 'batch_id'>) {
    const rootBatchId = detailRootBatchId()
    return Boolean(rootBatchId && item.batch_id && item.batch_id !== rootBatchId)
  }

  function retrySourceCustomID(customID: string) {
    return String(customID || '').replace(/(?:_retry_[a-z0-9]+)+$/i, '')
  }

  function isRecoveredOriginalFailure(item: BatchImageDetailItem) {
    const rootBatchId = detailRootBatchId()
    return Boolean(
      rootBatchId
      && item.batch_id === rootBatchId
      && item.status === 'failed'
      && recoveredOriginalCustomIds.value.has(item.custom_id),
    )
  }

  function detailItemRowClass(item: BatchImageDetailItem) {
    if (isRecoveredOriginalFailure(item)) {
      return 'bg-gray-50/80 text-gray-400 hover:bg-gray-100/80 dark:bg-dark-900/60 dark:text-gray-500 dark:hover:bg-dark-800/70'
    }
    return 'hover:bg-gray-50/70 dark:hover:bg-dark-800/60'
  }

  function itemPreviewKey(item: Pick<BatchImageItem, 'batch_id' | 'custom_id'>) {
    return previewCacheKey(
      item.batch_id || selectedBatchId.value || currentJob.value?.id || '',
      item.custom_id,
      0,
    )
  }

  function capturePreviewRequest(ownerBatchId: string): PreviewRequest {
    return {
      detailSnapshot: detailGeneration.capture(ownerBatchId),
      previewSnapshot: previewGeneration.capture(ownerBatchId),
      ownerBatchId,
    }
  }

  function isPreviewRequestCurrent(request: PreviewRequest) {
    return isDetailSnapshotCurrent(request.detailSnapshot, request.ownerBatchId)
      && previewGeneration.isCurrent(request.previewSnapshot, selectedBatchId.value)
  }

  function commitPreviewBlob(
    request: PreviewRequest,
    previewKey: string,
    blob: Blob,
  ) {
    return replaceBatchImageObjectURL(
      itemPreviewUrls,
      previewKey,
      blob,
      () => isPreviewRequestCurrent(request),
    )
  }

  async function hydrateCachedItemPreviews(
    detailItems: BatchImageDetailItem[],
    request: PreviewRequest,
  ) {
    const previewableItems = detailItems.filter(item => canLoadItemPreview(item))
    if (!previewableItems.length || !previewCacheSupported() || !isPreviewRequestCurrent(request)) return

    await Promise.all(previewableItems.map(async (item) => {
      if (!isPreviewRequestCurrent(request)) return
      const batchId = item.batch_id || request.ownerBatchId
      const previewKey = previewCacheKey(batchId, item.custom_id, 0)
      if (!batchId || itemPreviewUrls[previewKey] || previewErrorIds.value.has(previewKey)) return
      const cached = await getCachedPreviewBlob(
        previewCacheKey(batchId, item.custom_id, 0),
      ).catch(() => null)
      if (!cached || itemPreviewUrls[previewKey]) return
      commitPreviewBlob(request, previewKey, cached)
    }))
  }

  async function loadItems() {
    const batchId = selectedBatchId.value || currentJob.value?.id || ''
    if (!batchId) return
    const key = keyForSelectedBatch() || requireApiKey()
    if (!key) return
    const detailSnapshot = detailGeneration.capture(batchId)
    loadingItems.value = true
    clearItemPreviews()
    const previewRequest = capturePreviewRequest(batchId)
    try {
      const jobs = detailJobsForBatch(batchId)
      const results = await Promise.all(jobs.map(async (job) => {
        const result = await listBatchImageItems(key.key, job.id)
        return (result.data || []).map(item => ({
          ...item,
          batch_id: job.id,
          source_task_name: detailSourceName(job, batchId),
        }))
      }))
      if (!isDetailSnapshotCurrent(detailSnapshot, batchId)) return
      const detailItems = results.flat()
      items.value = detailItems
      void hydrateCachedItemPreviews(detailItems, previewRequest)
    } catch (error: any) {
      if (isDetailSnapshotCurrent(detailSnapshot, batchId)) {
        appStore.showError(batchImageErrorMessage(error, batchImageText('loadItemsFailed')))
      }
    } finally {
      if (isDetailSnapshotCurrent(detailSnapshot, batchId)) {
        loadingItems.value = false
      }
    }
  }

  function detailJobsForBatch(batchId: string): BatchImageJobRow[] {
    const row = batchJobs.value.find(job => job.id === batchId)
    const base = row || (currentJob.value && currentJob.value.id === batchId ? toJobRow(currentJob.value, keyForSelectedBatch() || selectedApiKey.value) : null)
    if (!base) return []
    if (base.parent_batch_id) return [base]
    return [base, ...(childrenByParent.value.get(base.id) || [])]
  }

  function detailSourceName(job: Pick<BatchImageJobRow, 'id' | 'task_name' | 'parent_batch_id'>, rootBatchId: string) {
    const name = job.task_name || job.id
    if (job.id === rootBatchId) return t('batchImage.detail.mainTask', { name })
    return t('batchImage.detail.childTask', { name })
  }

  async function loadItemPreview(item: BatchImageItem) {
    const ownerBatchId = selectedBatchId.value || currentJob.value?.id || ''
    const batchId = item.batch_id || selectedBatchId.value || currentJob.value?.id || ''
    const previewKey = itemPreviewKey(item)
    const request = capturePreviewRequest(ownerBatchId)
    if (
      !batchId
      || !ownerBatchId
      || !isPreviewRequestCurrent(request)
      || previewLoadingGenerations.has(previewKey)
      || !canLoadItemPreview(item)
      || (itemPreviewUrls[previewKey] && !previewErrorIds.value.has(previewKey))
    ) return
    const key = keyForSelectedBatch() || requireApiKey()
    if (!key) return
    const cacheKey = previewCacheKey(batchId, item.custom_id, 0)
    previewLoadingGenerations.set(previewKey, request.previewSnapshot.generation)
    previewLoadingIds.value = new Set([...previewLoadingIds.value, previewKey])
    try {
      previewErrorIds.value = new Set([...previewErrorIds.value].filter(id => id !== previewKey))
      if (itemPreviewUrls[previewKey]) {
        URL.revokeObjectURL(itemPreviewUrls[previewKey])
        delete itemPreviewUrls[previewKey]
      }
      const cached = await getCachedPreviewBlob(cacheKey)
      if (cached) {
        commitPreviewBlob(request, previewKey, cached)
        return
      }
      const blob = await getBatchImageItemContent(key.key, batchId, item.custom_id, 0)
      const thumbnail = await createThumbnailBlob(blob).catch(() => blob)
      commitPreviewBlob(request, previewKey, thumbnail)
      if (thumbnail !== blob || thumbnail.size <= 1024 * 1024) {
        void putCachedPreviewBlob(cacheKey, thumbnail)
      }
    } catch (error: any) {
      if (isPreviewRequestCurrent(request)) {
        previewErrorIds.value = new Set([...previewErrorIds.value, previewKey])
        appStore.showError(batchImageErrorMessage(error, batchImageText('loadPreviewFailed')))
      }
    } finally {
      if (previewLoadingGenerations.get(previewKey) === request.previewSnapshot.generation) {
        previewLoadingGenerations.delete(previewKey)
        const next = new Set(previewLoadingIds.value)
        next.delete(previewKey)
        previewLoadingIds.value = next
      }
    }
  }

  function openImagePreview(item: BatchImageItem) {
    const previewKey = itemPreviewKey(item)
    if (!itemPreviewUrls[previewKey] || previewErrorIds.value.has(previewKey)) return
    previewImageItem.value = item
  }

  function closeImagePreview() {
    previewImageItem.value = null
  }

  function handlePreviewError(customID: string) {
    if (itemPreviewUrls[customID]) {
      URL.revokeObjectURL(itemPreviewUrls[customID])
      delete itemPreviewUrls[customID]
    }
    previewErrorIds.value = new Set([...previewErrorIds.value, customID])
  }

  function clearItemPreviews() {
    previewGeneration.invalidate()
    closePromptPopover()
    revokeBatchImageObjectURLs(itemPreviewUrls)
    previewLoadingGenerations.clear()
    previewLoadingIds.value = new Set()
    previewErrorIds.value = new Set()
    previewImageItem.value = null
  }

  function copyInstruction() {
    void copyToClipboard(agentInstruction.value, batchImageText('copiedInstruction'))
  }

  function statusLabel(jobOrStatus: BatchImageStatus | Pick<BatchImageJob, 'status' | 'success_count' | 'fail_count'>) {
    const status = typeof jobOrStatus === 'string' ? jobOrStatus : jobOrStatus.status
    if (typeof jobOrStatus !== 'string' && status === 'completed' && jobOrStatus.fail_count > 0) {
      if (jobOrStatus.success_count > 0) return t('batchImage.status.partialSuccess')
      return t('batchImage.status.allFailed')
    }
    return batchImageJobStatusLabel(t, status)
  }

  function statusBadgeClass(jobOrStatus: BatchImageStatus | Pick<BatchImageJob, 'status' | 'success_count' | 'fail_count'>) {
    const status = typeof jobOrStatus === 'string' ? jobOrStatus : jobOrStatus.status
    if (typeof jobOrStatus !== 'string' && status === 'completed' && jobOrStatus.fail_count > 0) {
      if (jobOrStatus.success_count > 0) return 'badge-warning'
      return 'badge-danger'
    }
    if (status === 'completed') return 'badge-success'
    if (status === 'failed' || status === 'cancelled') return 'badge-danger'
    if (status === 'output_deleted') return 'badge-gray'
    return 'badge-primary'
  }

  function itemStatusLabel(status: string) {
    return batchImageItemStatusLabel(t, status)
  }

  function itemDisplayStatusLabel(item: BatchImageDetailItem) {
    if (isRecoveredOriginalFailure(item)) return t('batchImage.itemStatus.recovered')
    return itemStatusLabel(item.status)
  }

  function itemStatusBadgeClass(status: string) {
    if (status === 'succeeded' || status === 'success') return 'badge-success'
    if (status === 'failed' || status === 'cancelled') return 'badge-danger'
    return 'badge-primary'
  }

  function itemDisplayStatusBadgeClass(item: BatchImageDetailItem) {
    if (isRecoveredOriginalFailure(item)) return 'badge-gray'
    return itemStatusBadgeClass(item.status)
  }

  function itemResultLabel(item: BatchImageDetailItem) {
    if (isRecoveredOriginalFailure(item)) return t('batchImage.itemResult.recoveredByRetry')
    if (item.error) return friendlyItemError(item.error)
    if (item.status === 'succeeded' || item.status === 'success') {
      return itemPreviewUrls[itemPreviewKey(item)] ? t('batchImage.itemResult.readyPreview') : t('batchImage.itemResult.readyDownload')
    }
    if (item.status === 'failed') return t('batchImage.itemResult.noUsableImage')
    if (item.status === 'cancelled') return t('batchImage.itemResult.cancelled')
    return t('batchImage.itemResult.waiting')
  }

  function itemResultClass(item: BatchImageDetailItem) {
    if (isRecoveredOriginalFailure(item)) return 'bg-gray-100 text-gray-500 ring-gray-200 dark:bg-dark-800 dark:text-gray-400 dark:ring-dark-700'
    if (item.error || item.status === 'failed' || item.status === 'cancelled') return 'bg-red-50 text-red-700 ring-red-100 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/50'
    if (item.status === 'succeeded' || item.status === 'success') return 'bg-emerald-50 text-emerald-700 ring-emerald-100 dark:bg-emerald-950/30 dark:text-emerald-300 dark:ring-emerald-900/50'
    return 'bg-gray-50 text-gray-500 ring-gray-200 dark:bg-dark-800 dark:text-gray-400 dark:ring-dark-700'
  }

  function friendlyItemError(error: BatchImageItem['error']) {
    if (!error) return '-'
    if (error.code === 'EMPTY_IMAGE_OUTPUT') return t('batchImage.itemResult.emptyImageOutput')
    if (error.code === 'PROVIDER_ITEM_FAILED') return t('batchImage.itemResult.providerItemFailed')
    return error.message || error.code || '-'
  }

  function formatMoney(value: number | null | undefined) {
    if (value === null || value === undefined || Number.isNaN(Number(value))) return '$0.00'
    return `$${Number(value).toFixed(2)}`
  }

  function terminalZeroCost(job: Pick<BatchImageJob, 'status' | 'actual_cost'>) {
    return job.actual_cost === null && (job.status === 'failed' || job.status === 'cancelled')
  }

  function costLabel(job: Pick<BatchImageJob, 'status' | 'hold_amount' | 'actual_cost'>) {
    if (job.actual_cost !== null) return formatMoney(job.actual_cost)
    if (terminalZeroCost(job)) return formatMoney(0)
    return t('batchImage.detail.holdCost', { amount: formatMoney(job.hold_amount) })
  }

  function formatDate(timestamp: number) {
    if (!timestamp) return ''
    return new Date(timestamp * 1000).toLocaleString()
  }

  function defaultTaskName(timestamp?: number) {
    const date = timestamp ? new Date(timestamp * 1000) : new Date()
    return date.toLocaleString()
  }

  onMounted(() => {
    void appStore.fetchPublicSettings()
    void refreshPage()
    void cleanupPreviewCache()
    previewCacheCleanupTimer = setInterval(() => {
      void cleanupPreviewCache()
    }, 60 * 60 * 1000)
    document.addEventListener('click', closeMoreMenu)
    window.addEventListener('resize', closeMoreMenu)
    window.addEventListener('scroll', closeMoreMenu, true)
    window.addEventListener('resize', closePromptPopover)
    window.addEventListener('scroll', closePromptPopover, true)
  })

  watch(
    () => form.apiKeyId,
    () => {
      void loadAvailableModels()
    },
  )

  watch(
    () => form.model,
    () => {
      const limit = selectedModelReferenceLimit.value
      if (limit <= 0) {
        referenceImageDrafts.value = []
        return
      }
      if (referenceImageDrafts.value.length > limit) {
        referenceImageDrafts.value = referenceImageDrafts.value.slice(0, limit)
      }
    },
  )

  onBeforeUnmount(() => {
    detailGeneration.dispose()
    refreshSingleFlight.clearPending()
    stopPolling()
    if (previewCacheCleanupTimer) {
      clearInterval(previewCacheCleanupTimer)
      previewCacheCleanupTimer = null
    }
    clearItemPreviews()
    previewGeneration.dispose()
    document.removeEventListener('click', closeMoreMenu)
    window.removeEventListener('resize', closeMoreMenu)
    window.removeEventListener('scroll', closeMoreMenu, true)
    window.removeEventListener('resize', closePromptPopover)
    window.removeEventListener('scroll', closePromptPopover, true)
  })

  return {
    BATCH_IMAGE_MAX_OUTPUTS_PER_ITEM,
    BATCH_IMAGE_MAX_OUTPUTS_PER_JOB,
    addPromptRow,
    agentInstruction,
    allVisibleSelected,
    apiKeyFilterOptions,
    applyFilters,
    availableBatchImageModels,
    batchImageText,
    batchJobs,
    batchPageSizeOptions,
    bulkDeleting,
    bulkDownloading,
    canCancel,
    canDeleteRecord,
    canDownload,
    canLoadItemPreview,
    canRetry,
    cancelPromptPopoverClose,
    cancelSelected,
    cancelling,
    closeCreateModal,
    closeDetail,
    closeImagePreview,
    columns,
    copyInstruction,
    copyPromptPopover,
    costLabel,
    currentDisplayJob,
    currentJob,
    customIdDraft,
    defaultTaskName,
    deleteJob,
    deleteSelectedJobs,
    deletingBatchId,
    detailItemRowClass,
    displayJob,
    downloadFilterOptions,
    downloadJob,
    downloadSelected,
    downloadSelectedJobs,
    downloading,
    estimatedOutputCount,
    expandedParentIds,
    filters,
    form,
    formatDate,
    geminiApiKeys,
    handlePageChange,
    handlePageSizeChange,
    handlePreviewError,
    handleReferenceImageFiles,
    hasChildJobs,
    isDownloadingJob,
    isRecoveredOriginalFailure,
    itemDisplayStatusBadgeClass,
    itemDisplayStatusLabel,
    itemPreviewKey,
    itemPreviewUrls,
    itemResultClass,
    itemResultLabel,
    items,
    loadItemPreview,
    loadingItems,
    loadingJobs,
    loadingKeys,
    loadingModels,
    modelLoadError,
    moreMenuStyle,
    openCreateModal,
    openImagePreview,
    openMoreJobId,
    outputCountDraft,
    outputCountOptions,
    pagination,
    parsedItems,
    previewErrorIds,
    previewImageItem,
    previewImageUrl,
    previewLoadingIds,
    promptDraft,
    promptPopover,
    promptRows,
    referenceImageDrafts,
    refreshDetail,
    refreshPage,
    refreshing,
    removePromptRow,
    removeReferenceImageDraft,
    resetFilters,
    retryFailedJob,
    retrySelected,
    retryingBatchId,
    schedulePromptPopoverClose,
    schedulePromptPopoverOpen,
    selectJob,
    selectedApiKey,
    selectedDownloadableRows,
    selectedJobIds,
    selectedModelReferenceLimit,
    showCreateModal,
    showGuideModal,
    showPromptPopover,
    someVisibleSelected,
    statusBadgeClass,
    statusFilterOptions,
    statusLabel,
    submitJob,
    submitting,
    t,
    toggleAllVisible,
    toggleChildRows,
    toggleJobSelection,
    toggleMoreMenu,
    visibleBatchJobs,
  }
}

export type BatchImageGuideContext = ReturnType<typeof useBatchImageGuideController>
