import { computed, ref, type Ref } from 'vue'
import {
  classifyMediaStudioError,
  extractImages,
  imageTaskErrorMessage,
  now,
  videoTaskErrorMessage,
  type MediaStudioConversation,
  type MediaStudioGeneratedImagePreview,
  type MediaStudioJobErrorCode,
  type MediaStudioMessage,
  type MediaStudioMessageStatus,
} from './mediaStudioConversation'
import {
  mediaStudioErrorMessage,
  type MediaStudioImageTask,
  type MediaStudioVideoTask,
} from '@/features/media-studio/data/datasources/mediaStudioDatasource'

// 任务条只消费纯视图数据，轮询细节留在 composable 内部。
export interface MediaStudioJobView {
  messageId: string
  mode: 'image' | 'video'
  prompt: string
  status: MediaStudioMessageStatus
  taskIds: string[]
  createdAt: number
  progress?: number
  cost?: number
  currency?: string
  error?: string
  errorCode?: MediaStudioJobErrorCode
  completedAt?: number
}

interface MediaStudioJobsData {
  getTask: (apiKey: string, taskId: string) => Promise<MediaStudioImageTask>
  getVideoTask: (apiKey: string, taskId: string) => Promise<MediaStudioVideoTask>
  getVideoContent: (apiKey: string, taskId: string) => Promise<Blob>
}

export interface MediaStudioJobsOptions {
  conversation: Ref<MediaStudioConversation>
  patchMessage: (messageId: string, patch: Partial<MediaStudioMessage>) => boolean
  getApiKey: () => string
  getActiveGroupId: () => number
  createObjectURL: (blob: Blob) => string
  revokeObjectURL: (url: string) => void
  videoObjectURLs: Set<string>
  isDisposed: () => boolean
  persist: () => void
  cacheImages: (
    messageId: string,
    images: MediaStudioGeneratedImagePreview[],
  ) => Promise<MediaStudioGeneratedImagePreview[]>
  data: MediaStudioJobsData
  pollDelayMs?: number
  maxPollCount?: number
}

const DEFAULT_POLL_DELAY_MS = 1_500
const DEFAULT_MAX_POLL_COUNT = 240
// 视频任务刚创建时可能短暂 404，容忍少量连续未命中再判定失败。
const VIDEO_NOT_FOUND_RETRIES = 5

function isNotFound(error: unknown): boolean {
  return Boolean(error && typeof error === 'object' && (error as { status?: unknown }).status === 404)
}

function jobFromMessage(message: MediaStudioMessage): MediaStudioJobView | null {
  if (message.role !== 'assistant' || !message.status) return null
  const taskIds = message.taskIds && message.taskIds.length > 0
    ? message.taskIds
    : message.taskId
      ? [message.taskId]
      : []
  const view: MediaStudioJobView = {
    messageId: message.id,
    mode: message.mode,
    prompt: message.prompt,
    status: message.status,
    taskIds,
    createdAt: message.createdAt,
  }
  if (message.progress !== undefined) view.progress = message.progress
  if (message.cost !== undefined) view.cost = message.cost
  if (message.currency) view.currency = message.currency
  if (message.error) view.error = message.error
  if (message.errorCode) view.errorCode = message.errorCode
  if (message.completedAt !== undefined) view.completedAt = message.completedAt
  return view
}

export function useMediaStudioJobs(options: MediaStudioJobsOptions) {
  const pollDelayMs = options.pollDelayMs ?? DEFAULT_POLL_DELAY_MS
  const maxPollCount = Math.max(1, options.maxPollCount ?? DEFAULT_MAX_POLL_COUNT)
  // 对外暴露正在轮询的 task id，供视图层判断任务是否仍有后台进度。
  const pollingTaskIds = ref<string[]>([])
  const inFlightImagePolls = new Map<string, Promise<MediaStudioImageTask>>()
  const inFlightVideoPolls = new Set<string>()
  const resumingMessages = new Set<string>()
  const inFlightVideoCompletions = new Map<string, Promise<void>>()

  const jobs = computed<MediaStudioJobView[]>(() => options.conversation.value.messages
    .map(jobFromMessage)
    .filter((job): job is MediaStudioJobView => job !== null)
    // 任务条只展示最近 5 条，按创建时间倒序保证 slice(0, 5) 取到最新任务。
    .sort((left, right) => right.createdAt - left.createdAt))

  function markPolling(taskId: string) {
    if (!taskId || pollingTaskIds.value.includes(taskId)) return
    pollingTaskIds.value = [...pollingTaskIds.value, taskId]
  }

  function unmarkPolling(taskId: string) {
    if (!taskId) return
    pollingTaskIds.value = pollingTaskIds.value.filter(id => id !== taskId)
  }

  function waitPollDelay(): Promise<void> {
    // pollDelayMs 为 0 时让轮询完全走微任务，测试与首帧恢复都不会被计时器拖慢。
    if (pollDelayMs <= 0) return Promise.resolve()
    return new Promise(resolve => {
      setTimeout(resolve, pollDelayMs)
    })
  }

  function failVideoTask(messageId: string, error: unknown) {
    const errorCode = classifyMediaStudioError(error)
    options.patchMessage(messageId, {
      status: 'failed',
      error: errorCode === 'network' ? '' : mediaStudioErrorMessage(error, 'video generation failed'),
      errorCode,
      completedAt: now(),
    })
    options.persist()
  }

  async function completeVideoTask(
    apiKey: string,
    taskId: string,
    messageId: string,
    cost?: number,
    currency?: string,
  ): Promise<void> {
    const running = inFlightVideoCompletions.get(taskId)
    if (running) return running
    const completion = (async () => {
      try {
        const blob = await options.data.getVideoContent(apiKey, taskId)
        if (options.isDisposed()) return
        const src = options.createObjectURL(blob)
        const patch: Partial<MediaStudioMessage> = {
          status: 'completed',
          progress: 100,
          error: '',
          errorCode: undefined,
          completedAt: now(),
        }
        if (src) {
          options.videoObjectURLs.add(src)
          patch.video = { src, mimeType: blob.type || 'video/mp4' }
        }
        if (cost !== undefined) patch.cost = cost
        if (currency) patch.currency = currency
        options.patchMessage(messageId, patch)
        options.persist()
      } catch (error) {
        if (options.isDisposed()) return
        failVideoTask(messageId, error)
      }
    })()
    inFlightVideoCompletions.set(taskId, completion)
    completion.then(
      () => inFlightVideoCompletions.delete(taskId),
      () => inFlightVideoCompletions.delete(taskId),
    )
    return completion
  }

  async function pollImageLoop(apiKey: string, taskId: string): Promise<MediaStudioImageTask> {
    markPolling(taskId)
    try {
      for (let attempt = 0; attempt < maxPollCount; attempt += 1) {
        if (attempt > 0) await waitPollDelay()
        if (options.isDisposed()) throw new Error('media studio disposed')
        const task = await options.data.getTask(apiKey, taskId)
        if (task.status === 'completed' || task.status === 'failed') return task
      }
      throw new Error('image generation timed out')
    } finally {
      unmarkPolling(taskId)
    }
  }

  function pollImageTask(apiKey: string, taskId: string): Promise<MediaStudioImageTask> {
    const existing = inFlightImagePolls.get(taskId)
    if (existing) return existing
    const promise = pollImageLoop(apiKey, taskId)
    inFlightImagePolls.set(taskId, promise)
    promise.then(
      () => inFlightImagePolls.delete(taskId),
      () => inFlightImagePolls.delete(taskId),
    )
    return promise
  }

  async function pollVideoTask(apiKey: string, taskId: string, messageId: string): Promise<void> {
    if (!taskId || inFlightVideoPolls.has(taskId)) return
    inFlightVideoPolls.add(taskId)
    markPolling(taskId)
    let notFoundStreak = 0
    try {
      for (let attempt = 0; attempt < maxPollCount; attempt += 1) {
        if (attempt > 0) await waitPollDelay()
        if (options.isDisposed()) return

        let task: MediaStudioVideoTask
        try {
          task = await options.data.getVideoTask(apiKey, taskId)
          notFoundStreak = 0
        } catch (error) {
          if (isNotFound(error)) {
            notFoundStreak += 1
            if (notFoundStreak < VIDEO_NOT_FOUND_RETRIES) continue
          }
          if (options.isDisposed()) return
          failVideoTask(messageId, error)
          return
        }

        if (options.isDisposed()) return
        if (task.status === 'completed') {
          await completeVideoTask(apiKey, taskId, messageId, task.cost, task.currency)
          return
        }
        if (task.status === 'failed') {
          failVideoTask(messageId, task.error ? new Error(task.error) : videoTaskErrorMessage(task))
          return
        }

        const patch: Partial<MediaStudioMessage> = { status: 'processing' }
        if (task.progress !== undefined) patch.progress = task.progress
        if (task.cost !== undefined) patch.cost = task.cost
        if (task.currency) patch.currency = task.currency
        options.patchMessage(messageId, patch)
        options.persist()
      }
      failVideoTask(messageId, new Error('video generation timed out'))
    } finally {
      unmarkPolling(taskId)
      inFlightVideoPolls.delete(taskId)
    }
  }

  async function resumeImageMessage(
    apiKey: string,
    message: MediaStudioMessage,
    taskIds: string[],
  ): Promise<void> {
    if (resumingMessages.has(message.id)) return
    resumingMessages.add(message.id)
    try {
      const outcomes = await Promise.all(taskIds.map(async taskId => {
        try {
          const task = await pollImageTask(apiKey, taskId)
          if (task.status === 'completed') {
            const images = await options.cacheImages(message.id, extractImages(task))
            return { images, error: '', errorCode: undefined as MediaStudioJobErrorCode | undefined }
          }
          const failure = imageTaskErrorMessage(task)
          return {
            images: [] as MediaStudioGeneratedImagePreview[],
            error: failure,
            errorCode: classifyMediaStudioError(failure),
          }
        } catch (error) {
          const errorCode = classifyMediaStudioError(error)
          return {
            images: [] as MediaStudioGeneratedImagePreview[],
            error: errorCode === 'network' ? '' : mediaStudioErrorMessage(error, 'image generation failed'),
            errorCode,
          }
        }
      }))
      if (options.isDisposed()) return

      const current = options.conversation.value.messages.find(item => item.id === message.id)
      const merged = [...(current?.images ?? [])]
      for (const outcome of outcomes) {
        for (const image of outcome.images) {
          if (!merged.some(item => item.id === image.id)) merged.push(image)
        }
      }
      const failure = outcomes.find(outcome => outcome.errorCode)
      const completed = merged.length > 0
      options.patchMessage(message.id, {
        images: merged,
        status: completed ? 'completed' : 'failed',
        error: completed ? '' : (failure?.error ?? ''),
        errorCode: completed ? undefined : failure?.errorCode,
        completedAt: now(),
      })
      options.persist()
    } finally {
      resumingMessages.delete(message.id)
    }
  }

  // 刷新或切换会话后，把仍在 queued/processing 的任务重新挂回轮询。
  function resumePendingJobs() {
    if (options.isDisposed()) return
    const apiKey = options.getApiKey()
    if (!apiKey) return

    for (const message of options.conversation.value.messages) {
      if (message.role !== 'assistant') continue
      if (message.status !== 'queued' && message.status !== 'processing') continue
      const taskIds = message.taskIds && message.taskIds.length > 0
        ? message.taskIds
        : message.taskId
          ? [message.taskId]
          : []
      if (taskIds.length === 0) continue

      if (message.mode === 'video') {
        const taskId = taskIds[0]
        if (taskId && !inFlightVideoPolls.has(taskId)) {
          void pollVideoTask(apiKey, taskId, message.id)
        }
        continue
      }
      if (!resumingMessages.has(message.id)) {
        void resumeImageMessage(apiKey, message, taskIds)
      }
    }
  }

  return {
    pollingTaskIds,
    jobs,
    pollImageTask,
    completeVideoTask,
    pollVideoTask,
    resumePendingJobs,
  }
}
