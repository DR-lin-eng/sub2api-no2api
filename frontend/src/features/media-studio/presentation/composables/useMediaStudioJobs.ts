import { computed, ref, type Ref } from 'vue'
import type {
  MediaStudioImageTask,
  MediaStudioVideoTask,
} from '@/features/media-studio/data/datasources/mediaStudioDatasource'
import {
  deleteMediaStudioVideos,
  mediaStudioVideoStorageKey,
  storeMediaStudioVideo,
} from '@/features/media-studio/data/datasources/mediaStudioImageStore'
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
} from '@/features/media-studio/presentation/composables/mediaStudioConversation'

// 视频轮询间隔按上游文档要求不低于 6 秒。
const DEFAULT_POLL_DELAY_MS = 6000
const DEFAULT_MAX_POLL_COUNT = 240
// 任务刚创建时上游可能短暂返回 404，连续多次才判定失败。
const MAX_VIDEO_POLL_NOT_FOUND = 5
// 完成后保留在任务条里的时间窗口，方便用户回看刚结束的任务。
const RECENT_COMPLETED_WINDOW_MS = 5 * 60_000

export interface MediaStudioJobView {
  messageId: string
  mode: 'image' | 'video'
  prompt: string
  status: 'queued' | 'processing' | 'completed' | 'failed'
  progress?: number
  taskIds: string[]
  cost?: number
  currency?: string
  error?: string
  errorCode?: MediaStudioJobErrorCode
  createdAt: number
  completedAt?: number
}

export interface MediaStudioJobDeps {
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
  data: {
    getTask: (apiKey: string, taskId: string) => Promise<MediaStudioImageTask>
    getVideoTask: (apiKey: string, taskId: string) => Promise<MediaStudioVideoTask>
    getVideoContent: (apiKey: string, taskId: string) => Promise<Blob>
  }
  pollDelayMs?: number
  maxPollCount?: number
}

function wait(delayMs: number): Promise<void> {
  return delayMs > 0 ? new Promise(resolve => setTimeout(resolve, delayMs)) : Promise.resolve()
}

function uniqueImages(images: MediaStudioGeneratedImagePreview[]): MediaStudioGeneratedImagePreview[] {
  const seen = new Set<string>()
  return images.filter((image) => {
    if (seen.has(image.id)) return false
    seen.add(image.id)
    return true
  })
}

export function useMediaStudioJobs(deps: MediaStudioJobDeps) {
  const pollDelayMs = deps.pollDelayMs ?? DEFAULT_POLL_DELAY_MS
  const maxPollCount = deps.maxPollCount ?? DEFAULT_MAX_POLL_COUNT
  const pollingTaskIds = ref<string[]>([])
  const resumedMessageIds = new Set<string>()

  function messageById(messageId: string): MediaStudioMessage | undefined {
    return deps.conversation.value.messages.find(message => message.id === messageId)
  }

  function setPolling(taskId: string, active: boolean) {
    if (active && !pollingTaskIds.value.includes(taskId)) {
      pollingTaskIds.value = [...pollingTaskIds.value, taskId]
    }
    if (!active) pollingTaskIds.value = pollingTaskIds.value.filter(id => id !== taskId)
  }

  function taskIDsOf(message: MediaStudioMessage): string[] {
    if (message.taskIds?.length) return message.taskIds
    return message.taskId ? [message.taskId] : []
  }

  async function pollImageTask(apiKey: string, taskId: string): Promise<MediaStudioImageTask> {
    setPolling(taskId, true)
    try {
      for (let attempt = 0; attempt < maxPollCount && !deps.isDisposed(); attempt += 1) {
        const task = await deps.data.getTask(apiKey, taskId)
        if (task.status === 'completed' || task.status === 'failed') {
          return task
        }
        await wait(pollDelayMs)
      }
      throw new Error('image generation polling timed out')
    } finally {
      setPolling(taskId, false)
    }
  }

  async function completeVideoTask(
    apiKey: string,
    taskId: string,
    messageId: string,
    cost?: number,
    currency?: string,
  ) {
    const blob = await deps.data.getVideoContent(apiKey, taskId)
    if (deps.isDisposed()) return
    const cacheKey = mediaStudioVideoStorageKey(messageId)
    const stored = await storeMediaStudioVideo(cacheKey, blob)
    if (deps.isDisposed()) return
    const src = deps.createObjectURL(blob)
    deps.videoObjectURLs.add(src)
    const patch: Partial<MediaStudioMessage> = {
      status: 'completed',
      video: { src, mimeType: blob.type || 'video/mp4', cacheKey: stored ? cacheKey : undefined },
      error: '',
      errorCode: undefined,
      progress: 100,
      completedAt: now(),
    }
    if (cost !== undefined) patch.cost = cost
    if (currency) patch.currency = currency
    if (!deps.patchMessage(messageId, patch)) {
      deps.videoObjectURLs.delete(src)
      deps.revokeObjectURL(src)
      if (stored) void deleteMediaStudioVideos([cacheKey])
      return
    }
    deps.persist()
  }

  async function pollVideoTask(apiKey: string, taskId: string, messageId: string) {
    setPolling(taskId, true)
    let notFoundCount = 0
    try {
      for (let attempt = 0; attempt < maxPollCount && !deps.isDisposed(); attempt += 1) {
        let task: MediaStudioVideoTask
        try {
          task = await deps.data.getVideoTask(apiKey, taskId)
          notFoundCount = 0
        } catch (error) {
          if ((error as { status?: number })?.status !== 404) throw error
          notFoundCount += 1
          if (notFoundCount >= MAX_VIDEO_POLL_NOT_FOUND) throw error
          await wait(pollDelayMs)
          continue
        }
        if (deps.isDisposed()) return
        if (task.status === 'completed') {
          await completeVideoTask(apiKey, task.id, messageId, task.cost, task.currency)
          return
        }
        if (task.status === 'failed') {
          const error = videoTaskErrorMessage(task)
          deps.patchMessage(messageId, {
            status: 'failed',
            error,
            errorCode: classifyMediaStudioError(error),
            completedAt: now(),
          })
          deps.persist()
          return
        }
        const processingPatch: Partial<MediaStudioMessage> = { status: 'processing' }
        if (task.progress !== undefined) processingPatch.progress = task.progress
        // 处理中回显冻结金额，完成后替换为实际结算金额。
        if (task.cost !== undefined) processingPatch.cost = task.cost
        if (task.currency) processingPatch.currency = task.currency
        deps.patchMessage(messageId, processingPatch)
        await wait(pollDelayMs)
      }
      if (!deps.isDisposed()) {
        deps.patchMessage(messageId, {
          status: 'failed',
          error: 'video generation polling timed out',
          errorCode: 'timeout',
          completedAt: now(),
        })
        deps.persist()
      }
    } catch (error) {
      if (!deps.isDisposed()) {
        deps.patchMessage(messageId, {
          status: 'failed',
          error: error instanceof Error ? error.message : 'video generation failed',
          errorCode: classifyMediaStudioError(error),
          completedAt: now(),
        })
        deps.persist()
      }
    } finally {
      setPolling(taskId, false)
    }
  }

  // 刷新后恢复图片任务：重新轮询每个未完成任务，把结果合并回同一条助手消息。
  async function resumeImageTask(messageId: string, taskIds: string[], apiKey: string) {
    const message = messageById(messageId)
    if (!message) return
    const collected = [...(message.images ?? [])]
    const errors: string[] = []
    await Promise.all(taskIds.map(async (taskId) => {
      try {
        const task = await pollImageTask(apiKey, taskId)
        if (task.status === 'completed') {
          collected.push(...await deps.cacheImages(messageId, extractImages(task)))
        } else if (task.status === 'failed') {
          errors.push(imageTaskErrorMessage(task))
        }
      } catch (error) {
        errors.push(error instanceof Error ? error.message : 'image generation failed')
      }
    }))
    if (deps.isDisposed()) return
    const current = messageById(messageId)
    if (!current) return
    const images = uniqueImages(collected).slice(0, current.count || 9)
    const lastError = errors[errors.length - 1] ?? ''
    deps.patchMessage(messageId, {
      images,
      status: images.length > 0 ? 'completed' : 'failed',
      error: images.length > 0 ? '' : lastError,
      errorCode: images.length > 0 ? undefined : classifyMediaStudioError(lastError),
      completedAt: now(),
    })
    deps.persist()
  }

  // 会话建立后调用：只恢复当前分组里未完成的任务，避免拿错分组的会话密钥查询任务。
  function resumePendingJobs() {
    const apiKey = deps.getApiKey()
    const groupId = deps.getActiveGroupId()
    if (!apiKey || !groupId) return
    for (const message of deps.conversation.value.messages) {
      if (message.role !== 'assistant') continue
      if (message.status !== 'queued' && message.status !== 'processing') continue
      if (resumedMessageIds.has(message.id)) continue
      if (message.groupId && message.groupId !== groupId) continue
      const taskIds = taskIDsOf(message)
      if (taskIds.length === 0) {
        deps.patchMessage(message.id, {
          status: 'failed',
          error: 'task id is missing',
          errorCode: 'unknown',
          completedAt: now(),
        })
        continue
      }
      resumedMessageIds.add(message.id)
      if (message.mode === 'video') {
        void pollVideoTask(apiKey, taskIds[0], message.id)
      } else {
        void resumeImageTask(message.id, taskIds, apiKey)
      }
    }
    deps.persist()
  }

  const jobs = computed<MediaStudioJobView[]>(() => {
    const current = now()
    return deps.conversation.value.messages
      .filter((message): message is MediaStudioMessage & { status: MediaStudioJobView['status'] } => {
        if (message.role !== 'assistant' || !message.status) return false
        const hasTask = Boolean(message.taskIds?.length || message.taskId)
        switch (message.status) {
          case 'queued':
          case 'processing':
          case 'failed':
            return true
          case 'completed':
            return hasTask &&
              typeof message.completedAt === 'number' &&
              current - message.completedAt < RECENT_COMPLETED_WINDOW_MS
          default:
            return false
        }
      })
      .slice(-20)
      .map(message => ({
        messageId: message.id,
        mode: message.mode,
        prompt: message.prompt,
        status: message.status,
        progress: message.progress,
        taskIds: taskIDsOf(message),
        cost: message.cost,
        currency: message.currency,
        error: message.error,
        errorCode: message.errorCode,
        createdAt: message.createdAt,
        completedAt: message.completedAt,
      }))
      .reverse()
  })

  return {
    pollingTaskIds,
    jobs,
    pollImageTask,
    completeVideoTask,
    pollVideoTask,
    resumePendingJobs,
  }
}

export type MediaStudioJobs = ReturnType<typeof useMediaStudioJobs>
