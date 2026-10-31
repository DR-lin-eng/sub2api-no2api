import { computed, getCurrentInstance, onBeforeUnmount, reactive, ref } from 'vue'
import {
  clearMediaStudioGallery,
  deleteMediaStudioGalleryItems,
  getMediaStudioGalleryMedia,
  listMediaStudioGalleryItems,
  mediaStudioGalleryMediaKey,
  putMediaStudioGalleryItem,
  putMediaStudioGalleryMedia,
  readMediaStudioGalleryArchiveState,
  writeMediaStudioGalleryArchiveState,
  type MediaStudioGalleryItem,
} from '@/features/media-studio/data/datasources/mediaStudioGalleryStore'
import {
  loadMediaStudioImage,
  loadMediaStudioVideo,
  openMediaStudioDatabase,
  normalizeMediaStudioOwnerID,
} from '@/features/media-studio/data/datasources/mediaStudioImageStore'
import {
  persistedImageSource,
  safeRemoteImageURL,
  type MediaStudioMessage,
} from '@/features/media-studio/presentation/composables/mediaStudioConversation'

export interface MediaStudioGalleryOptions {
  ownerID?: number
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
  const ownerID = normalizeMediaStudioOwnerID(options.ownerID)
  const excludedIDs = new Set<string>()
  let clearedBefore = 0
  let mutationEpoch = 0
  let sourceEpoch = 0
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

  async function resolveSource(item: MediaStudioGalleryItem, epoch = sourceEpoch) {
    if (sources[item.id]) return
    if (item.remoteURL) {
      sources[item.id] = item.remoteURL
      return
    }
    if (!item.mediaKey) return

    const media = await getMediaStudioGalleryMedia(item.mediaKey, ownerID)
    if (disposed || !media || epoch !== sourceEpoch) return
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
    // Metadata is cheap; media is loaded only for the visible gallery page.
    const visible = new Set(nextItems.map(item => item.id))
    for (const id of Object.keys(sources)) if (!visible.has(id)) forgetSource({ id } as MediaStudioGalleryItem)
  }

  async function refresh() {
    loading.value = true
    try {
      const database = await openMediaStudioDatabase(ownerID)
      storageAvailable.value = database !== null
      database?.close()
      const state = await readMediaStudioGalleryArchiveState(ownerID)
      if (disposed) return
      state.excludedIDs.forEach(id => excludedIDs.add(id))
      clearedBefore = state.clearedBefore
      await hydrate(await listMediaStudioGalleryItems(ownerID))
      await enforceRetention()
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
    if (message.size) item.size = message.size
    if (message.quality) item.quality = message.quality
    if (message.imageResolution) item.imageResolution = message.imageResolution
    if (message.imageAspectRatio) item.imageAspectRatio = message.imageAspectRatio
    if (message.count) item.count = message.count

    if (remoteURL) {
      item.remoteURL = remoteURL
    } else {
      const cached = image.cacheKey ? await loadMediaStudioImage(image.cacheKey, ownerID) : ''
      const inline = cached || persistedImageSource(image.src)
      if (!inline.startsWith('data:image/')) return null
      const mediaKey = mediaStudioGalleryMediaKey(itemID)
      if (!await putMediaStudioGalleryMedia(mediaKey, inline, ownerID)) return null
      item.mediaKey = mediaKey
      item.byteSize = Math.ceil(inline.length * 3 / 4)
    }

    return item
  }

  async function archiveVideo(message: MediaStudioMessage): Promise<MediaStudioGalleryItem | null> {
    const cacheKey = message.video?.cacheKey
    if (!cacheKey) return null

    const blob = await loadMediaStudioVideo(cacheKey, ownerID)
    if (!blob) return null

    const itemID = galleryItemID(message.id, 'video')
    const mediaKey = mediaStudioGalleryMediaKey(itemID)
    if (!await putMediaStudioGalleryMedia(mediaKey, blob, ownerID)) return null

    const item: MediaStudioGalleryItem = {
      id: itemID,
      mode: 'video',
      prompt: message.prompt,
      favorite: false,
      createdAt: message.completedAt || message.createdAt,
      mediaKey,
      byteSize: blob.size,
      mimeType: mediaStudioGalleryMimeType(message),
      sourceMessageId: message.id,
    }
    if (message.model) item.model = message.model
    if (message.resolution) item.resolution = message.resolution
    if (message.aspectRatio) item.aspectRatio = message.aspectRatio
    if (message.duration) item.duration = message.duration
    return item
  }

  // 生成完成后自动归档：同一消息同一张图只入库一次，重复调用是幂等的。
  async function archiveMessages(messages: MediaStudioMessage[]) {
    if (!ownerID || disposed) return
    const epoch = mutationEpoch
    const created: MediaStudioGalleryItem[] = []
    for (const message of messages) {
      if (message.role !== 'assistant' || message.status !== 'completed' || (message.completedAt || message.createdAt) <= clearedBefore) continue
      if (message.mode === 'image') {
        for (const image of message.images ?? []) {
          if (knownIDs.has(galleryItemID(message.id, image.id)) || excludedIDs.has(galleryItemID(message.id, image.id))) continue
          const item = await archiveImage(message, image.id)
          if (item) created.push(item)
        }
      } else if (message.mode === 'video') {
        if (knownIDs.has(galleryItemID(message.id, 'video')) || excludedIDs.has(galleryItemID(message.id, 'video'))) continue
        const item = await archiveVideo(message)
        if (item) created.push(item)
      }
    }
    if (created.length === 0) return
    if (disposed || epoch !== mutationEpoch) { await deleteMediaStudioGalleryItems(created, ownerID); return }

    const persisted: MediaStudioGalleryItem[] = []
    for (const item of created) {
      if (disposed || epoch !== mutationEpoch || excludedIDs.has(item.id)) {
        await deleteMediaStudioGalleryItems([item], ownerID)
        continue
      }
      if (await putMediaStudioGalleryItem(item, ownerID)) {
        if (disposed || epoch !== mutationEpoch) await deleteMediaStudioGalleryItems([item], ownerID)
        else persisted.push(item)
      } else await deleteMediaStudioGalleryItems([item], ownerID)
    }
    if (disposed || persisted.length === 0) return
    await hydrate([...persisted, ...items.value].sort((a, b) => b.createdAt - a.createdAt))
    await enforceRetention()
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
    await putMediaStudioGalleryItem(next, ownerID)
  }

  async function saveExclusions() {
    if (excludedIDs.size > 2_000) {
      clearedBefore = Date.now()
      excludedIDs.clear()
    }
    await writeMediaStudioGalleryArchiveState({ excludedIDs: [...excludedIDs], clearedBefore }, ownerID)
  }

  // Bound disk and preview work as this is a browser cache, not an unlimited archive.
  async function enforceRetention() {
    const MAX_ITEMS = 100
    const MAX_BYTES = 512 * 1024 * 1024
    let bytes = items.value.reduce((total, item) => total + (item.byteSize || 0), 0)
    const candidates = [...items.value].sort((a, b) => Number(a.favorite) - Number(b.favorite) || a.createdAt - b.createdAt)
    const removed: MediaStudioGalleryItem[] = []
    while (items.value.length - removed.length > MAX_ITEMS || bytes > MAX_BYTES) {
      const item = candidates.shift()
      if (!item) break
      removed.push(item)
      bytes -= item.byteSize || 0
      excludedIDs.add(item.id)
      forgetSource(item)
      knownIDs.delete(item.id)
    }
    if (removed.length) {
      const ids = new Set(removed.map(item => item.id))
      items.value = items.value.filter(item => !ids.has(item.id))
      await saveExclusions()
      await deleteMediaStudioGalleryItems(removed, ownerID)
    }
  }

  async function loadSources(ids: string[]) {
    const epoch = ++sourceEpoch
    const visible = new Set(ids.slice(0, 12))
    for (const id of Object.keys(sources)) if (!visible.has(id)) forgetSource({ id } as MediaStudioGalleryItem)
    for (const item of items.value.filter(item => visible.has(item.id))) {
      if (disposed || epoch !== sourceEpoch) return
      await resolveSource(item, epoch)
    }
  }

  async function remove(item: MediaStudioGalleryItem) {
    mutationEpoch += 1
    excludedIDs.add(item.id)
    await saveExclusions()
    forgetSource(item)
    knownIDs.delete(item.id)
    items.value = items.value.filter(current => current.id !== item.id)
    await deleteMediaStudioGalleryItems([item], ownerID)
  }

  async function clear() {
    mutationEpoch += 1
    clearedBefore = Date.now()
    excludedIDs.clear()
    await saveExclusions()
    items.value.forEach(item => forgetSource(item))
    items.value = []
    knownIDs.clear()
    await clearMediaStudioGallery(ownerID)
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
    loadSources,
    saveFromMessages,
    toggleFavorite,
    remove,
    clear,
  }
}

export type MediaStudioGallery = ReturnType<typeof useMediaStudioGallery>
