import { computed, getCurrentInstance, onBeforeUnmount, reactive, ref } from 'vue'
import {
  clearMediaStudioGallery,
  deleteMediaStudioGalleryItems,
  getMediaStudioGalleryMedia,
  listMediaStudioGalleryItems,
  mediaStudioGalleryMediaKey,
  putMediaStudioGalleryItem,
  putMediaStudioGalleryMedia,
  type MediaStudioGalleryItem,
} from '@/features/media-studio/data/datasources/mediaStudioGalleryStore'
import {
  loadMediaStudioImage,
  loadMediaStudioVideo,
  openMediaStudioDatabase,
} from '@/features/media-studio/data/datasources/mediaStudioImageStore'
import {
  persistedImageSource,
  safeRemoteImageURL,
  type MediaStudioMessage,
} from '@/features/media-studio/presentation/composables/mediaStudioConversation'

export interface MediaStudioGalleryOptions {
  createObjectURL?: (blob: Blob) => string
  revokeObjectURL?: (url: string) => void
}

function galleryItemID(messageID: string, suffix: string): string {
  return `gallery_${messageID}_${suffix}`
}

function mediaStudioGalleryMimeType(item: MediaStudioMessage): string {
  if (item.mode === 'video') return item.video?.mimeType || 'video/mp4'
  return 'image/png'
}

// 作品库只归档已完成结果：失败与进行中的消息仍由会话与任务条负责。
export function useMediaStudioGallery(options: MediaStudioGalleryOptions = {}) {
  const createObjectURL = options.createObjectURL ?? ((blob: Blob) => (
    typeof URL.createObjectURL === 'function' ? URL.createObjectURL(blob) : ''
  ))
  const revokeObjectURL = options.revokeObjectURL ?? ((url: string) => {
    if (url && typeof URL.revokeObjectURL === 'function') URL.revokeObjectURL(url)
  })

  const items = ref<MediaStudioGalleryItem[]>([])
  // 已解析的展示地址：远程结果直接用上游地址，本地素材按需读出后缓存。
  const sources = reactive<Record<string, string>>({})
  const loading = ref(false)
  // 本地作品库依赖 IndexedDB；不可用时视图需要明确告知用户，而不是一直显示空列表。
  const storageAvailable = ref(true)
  const totalCount = computed(() => items.value.length)
  const knownIDs = new Set<string>()
  const objectURLs = new Set<string>()
  let saving = false
  let pendingMessages: MediaStudioMessage[] | null = null
  let disposed = false

  async function resolveSource(item: MediaStudioGalleryItem) {
    if (sources[item.id]) return
    if (item.remoteURL) {
      sources[item.id] = item.remoteURL
      return
    }
    if (!item.mediaKey) return

    const media = await getMediaStudioGalleryMedia(item.mediaKey)
    if (disposed || !media) return
    if (typeof media === 'string') {
      sources[item.id] = media
      return
    }
    const url = createObjectURL(media)
    if (!url) return
    objectURLs.add(url)
    sources[item.id] = url
  }

  function forgetSource(item: MediaStudioGalleryItem) {
    const source = sources[item.id]
    if (source && objectURLs.has(source)) {
      revokeObjectURL(source)
      objectURLs.delete(source)
    }
    delete sources[item.id]
  }

  async function hydrate(nextItems: MediaStudioGalleryItem[]) {
    items.value = nextItems
    knownIDs.clear()
    nextItems.forEach(item => knownIDs.add(item.id))
    await Promise.all(nextItems.map(item => resolveSource(item)))
  }

  async function refresh() {
    loading.value = true
    try {
      const database = await openMediaStudioDatabase()
      storageAvailable.value = database !== null
      database?.close()
      await hydrate(await listMediaStudioGalleryItems())
    } finally {
      if (!disposed) loading.value = false
    }
  }

  async function archiveImage(message: MediaStudioMessage, imageID: string): Promise<MediaStudioGalleryItem | null> {
    const images = message.images ?? []
    const image = images.find(entry => entry.id === imageID)
    if (!image) return null

    const itemID = galleryItemID(message.id, imageID)
    const remoteURL = safeRemoteImageURL(image.url) || safeRemoteImageURL(image.src)
    const item: MediaStudioGalleryItem = {
      id: itemID,
      mode: 'image',
      prompt: message.prompt,
      favorite: false,
      createdAt: message.completedAt || message.createdAt,
      mimeType: 'image/png',
      sourceMessageId: message.id,
      sourceImageId: imageID,
    }
    if (message.model) item.model = message.model
    if (message.groupId) item.groupId = message.groupId
    if (message.size) item.size = message.size
    if (message.quality) item.quality = message.quality
    if (message.imageResolution) item.imageResolution = message.imageResolution
    if (message.imageAspectRatio) item.imageAspectRatio = message.imageAspectRatio
    if (message.count) item.count = message.count
    if (message.attributes) item.attributes = { ...message.attributes }

    if (remoteURL) {
      item.remoteURL = remoteURL
    } else {
      const cached = image.cacheKey ? await loadMediaStudioImage(image.cacheKey) : ''
      const inline = cached || persistedImageSource(image.src)
      if (!inline.startsWith('data:image/')) return null
      const mediaKey = mediaStudioGalleryMediaKey(itemID)
      if (!await putMediaStudioGalleryMedia(mediaKey, inline)) return null
      item.mediaKey = mediaKey
    }

    return item
  }

  async function archiveVideo(message: MediaStudioMessage): Promise<MediaStudioGalleryItem | null> {
    const cacheKey = message.video?.cacheKey
    if (!cacheKey) return null

    const blob = await loadMediaStudioVideo(cacheKey)
    if (!blob) return null

    const itemID = galleryItemID(message.id, 'video')
    const mediaKey = mediaStudioGalleryMediaKey(itemID)
    if (!await putMediaStudioGalleryMedia(mediaKey, blob)) return null

    const item: MediaStudioGalleryItem = {
      id: itemID,
      mode: 'video',
      prompt: message.prompt,
      favorite: false,
      createdAt: message.completedAt || message.createdAt,
      mediaKey,
      mimeType: mediaStudioGalleryMimeType(message),
      sourceMessageId: message.id,
    }
    if (message.model) item.model = message.model
    if (message.groupId) item.groupId = message.groupId
    if (message.resolution) item.resolution = message.resolution
    if (message.aspectRatio) item.aspectRatio = message.aspectRatio
    if (message.duration) item.duration = message.duration
    if (message.attributes) item.attributes = { ...message.attributes }
    return item
  }

  // 生成完成后自动归档：同一消息同一张图只入库一次，重复调用是幂等的。
  async function archiveMessages(messages: MediaStudioMessage[]) {
    const created: MediaStudioGalleryItem[] = []
    for (const message of messages) {
      if (message.role !== 'assistant' || message.status !== 'completed') continue
      if (message.mode === 'image') {
        for (const image of message.images ?? []) {
          if (knownIDs.has(galleryItemID(message.id, image.id))) continue
          const item = await archiveImage(message, image.id)
          if (item) created.push(item)
        }
      } else if (message.mode === 'video') {
        if (knownIDs.has(galleryItemID(message.id, 'video'))) continue
        const item = await archiveVideo(message)
        if (item) created.push(item)
      }
    }
    if (created.length === 0 || disposed) return

    const persisted: MediaStudioGalleryItem[] = []
    for (const item of created) {
      if (await putMediaStudioGalleryItem(item)) persisted.push(item)
    }
    if (disposed || persisted.length === 0) return
    await hydrate([...persisted, ...items.value].sort((a, b) => b.createdAt - a.createdAt))
  }

  // 归档请求可能在上一轮读写期间到达；排队合并，避免完成事件被重入保护吞掉。
  async function saveFromMessages(messages: MediaStudioMessage[]) {
    pendingMessages = messages
    if (saving || disposed) return
    saving = true
    try {
      while (pendingMessages && !disposed) {
        const batch = pendingMessages
        pendingMessages = null
        await archiveMessages(batch)
      }
    } finally {
      saving = false
      pendingMessages = null
    }
  }

  async function toggleFavorite(item: MediaStudioGalleryItem) {
    const next: MediaStudioGalleryItem = { ...item, favorite: !item.favorite }
    items.value = items.value.map(current => (current.id === item.id ? next : current))
    await putMediaStudioGalleryItem(next)
  }

  async function remove(item: MediaStudioGalleryItem) {
    forgetSource(item)
    knownIDs.delete(item.id)
    items.value = items.value.filter(current => current.id !== item.id)
    await deleteMediaStudioGalleryItems([item])
  }

  async function clear() {
    items.value.forEach(item => forgetSource(item))
    items.value = []
    knownIDs.clear()
    await clearMediaStudioGallery()
  }

  if (getCurrentInstance()) {
    onBeforeUnmount(() => {
      disposed = true
      objectURLs.forEach(url => revokeObjectURL(url))
      objectURLs.clear()
    })
  }

  return {
    items,
    sources,
    loading,
    storageAvailable,
    totalCount,
    refresh,
    saveFromMessages,
    toggleFavorite,
    remove,
    clear,
  }
}

export type MediaStudioGallery = ReturnType<typeof useMediaStudioGallery>
