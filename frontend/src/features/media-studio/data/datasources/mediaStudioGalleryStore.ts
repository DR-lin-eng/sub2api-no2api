import {
  GALLERY_ITEM_STORE_NAME,
  GALLERY_MEDIA_STORE_NAME,
  GALLERY_SETTINGS_STORE_NAME,
  openMediaStudioDatabase,
} from '@/features/media-studio/data/datasources/mediaStudioImageStore'

// 作品库条目：元数据可筛选、可排序，素材本体单独放 gallery-media，避免与会话缓存互相牵连。
export interface MediaStudioGalleryItem {
  id: string
  mode: 'image' | 'video'
  prompt: string
  model?: string
  // 参数快照：作品库卡片展示用，字段名与会话消息保持一致。
  size?: string
  quality?: string
  imageResolution?: string
  imageAspectRatio?: string
  count?: number
  resolution?: string
  aspectRatio?: string
  duration?: number
  // 远程结果直接引用上游地址；本地结果拷贝进 gallery-media，由 mediaKey 指向。
  remoteURL?: string
  mediaKey?: string
  mimeType?: string
  byteSize?: number
  favorite: boolean
  createdAt: number
  sourceMessageId?: string
  sourceImageId?: string
}

// 固定 key，保证同一张图重复归档时不会产生多份素材。
export function mediaStudioGalleryMediaKey(itemID: string): string {
  return `gallery:${itemID}`
}

export async function listMediaStudioGalleryItems(ownerID?: number): Promise<MediaStudioGalleryItem[]> {
  const database = await openMediaStudioDatabase(ownerID)
  if (!database) return []

  return new Promise((resolve) => {
    const transaction = database.transaction(GALLERY_ITEM_STORE_NAME, 'readonly')
    const request = transaction.objectStore(GALLERY_ITEM_STORE_NAME).getAll()
    request.onsuccess = () => {
      const records = Array.isArray(request.result) ? request.result : []
      const items = records.filter(
        (record): record is MediaStudioGalleryItem => (
          !!record && typeof record === 'object' && typeof (record as MediaStudioGalleryItem).id === 'string'
        ),
      )
      resolve(items.sort((a, b) => (b.createdAt || 0) - (a.createdAt || 0)))
    }
    request.onerror = () => resolve([])
    transaction.oncomplete = () => database.close()
    transaction.onerror = () => database.close()
    transaction.onabort = () => database.close()
  })
}

export async function putMediaStudioGalleryItem(item: MediaStudioGalleryItem, ownerID?: number): Promise<boolean> {
  const database = await openMediaStudioDatabase(ownerID)
  if (!database || !item.id) return false

  return new Promise((resolve) => {
    const transaction = database.transaction(GALLERY_ITEM_STORE_NAME, 'readwrite')
    transaction.objectStore(GALLERY_ITEM_STORE_NAME).put(item)
    transaction.oncomplete = () => {
      database.close()
      resolve(true)
    }
    transaction.onerror = () => {
      database.close()
      resolve(false)
    }
    transaction.onabort = () => {
      database.close()
      resolve(false)
    }
  })
}

export async function putMediaStudioGalleryMedia(
  key: string,
  value: string | Blob,
  ownerID?: number,
): Promise<boolean> {
  const database = await openMediaStudioDatabase(ownerID)
  if (!database || !key || !value) return false

  return new Promise((resolve) => {
    const transaction = database.transaction(GALLERY_MEDIA_STORE_NAME, 'readwrite')
    transaction.objectStore(GALLERY_MEDIA_STORE_NAME).put(value, key)
    transaction.oncomplete = () => {
      database.close()
      resolve(true)
    }
    transaction.onerror = () => {
      database.close()
      resolve(false)
    }
    transaction.onabort = () => {
      database.close()
      resolve(false)
    }
  })
}

export async function getMediaStudioGalleryMedia(key: string, ownerID?: number): Promise<string | Blob | null> {
  const database = await openMediaStudioDatabase(ownerID)
  if (!database || !key) return null

  return new Promise((resolve) => {
    const transaction = database.transaction(GALLERY_MEDIA_STORE_NAME, 'readonly')
    const request = transaction.objectStore(GALLERY_MEDIA_STORE_NAME).get(key)
    request.onsuccess = () => {
      const result = request.result
      resolve(typeof result === 'string' || result instanceof Blob ? result : null)
    }
    request.onerror = () => resolve(null)
    transaction.oncomplete = () => database.close()
    transaction.onerror = () => database.close()
    transaction.onabort = () => database.close()
  })
}

// 删除作品时连同素材一起清理；远程引用的作品没有本地素材，删除是幂等的。
export async function deleteMediaStudioGalleryItems(items: MediaStudioGalleryItem[], ownerID?: number): Promise<void> {
  const ids = [...new Set(items.map(item => item.id).filter(Boolean))]
  if (ids.length === 0) return

  const mediaKeys = items.flatMap(item => (item.mediaKey ? [item.mediaKey] : []))
  await Promise.all([deleteGalleryRecords(ids, ownerID), deleteGalleryMedia(mediaKeys, ownerID)])
}

export async function clearMediaStudioGallery(ownerID?: number): Promise<void> {
  const database = await openMediaStudioDatabase(ownerID)
  if (!database) return

  await new Promise<void>((resolve) => {
    const transaction = database.transaction(
      [GALLERY_ITEM_STORE_NAME, GALLERY_MEDIA_STORE_NAME],
      'readwrite',
    )
    transaction.objectStore(GALLERY_ITEM_STORE_NAME).clear()
    transaction.objectStore(GALLERY_MEDIA_STORE_NAME).clear()
    transaction.oncomplete = () => {
      database.close()
      resolve()
    }
    transaction.onerror = () => {
      database.close()
      resolve()
    }
    transaction.onabort = () => {
      database.close()
      resolve()
    }
  })
}

async function deleteGalleryRecords(ids: string[], ownerID?: number): Promise<void> {
  const database = await openMediaStudioDatabase(ownerID)
  if (!database) return

  await new Promise<void>((resolve) => {
    const transaction = database.transaction(GALLERY_ITEM_STORE_NAME, 'readwrite')
    const store = transaction.objectStore(GALLERY_ITEM_STORE_NAME)
    ids.forEach(id => store.delete(id))
    transaction.oncomplete = () => {
      database.close()
      resolve()
    }
    transaction.onerror = () => {
      database.close()
      resolve()
    }
    transaction.onabort = () => {
      database.close()
      resolve()
    }
  })
}

async function deleteGalleryMedia(keys: string[], ownerID?: number): Promise<void> {
  const normalizedKeys = [...new Set(keys.filter(Boolean))]
  if (normalizedKeys.length === 0) return

  const database = await openMediaStudioDatabase(ownerID)
  if (!database) return

  await new Promise<void>((resolve) => {
    const transaction = database.transaction(GALLERY_MEDIA_STORE_NAME, 'readwrite')
    const store = transaction.objectStore(GALLERY_MEDIA_STORE_NAME)
    normalizedKeys.forEach(key => store.delete(key))
    transaction.oncomplete = () => {
      database.close()
      resolve()
    }
    transaction.onerror = () => {
      database.close()
      resolve()
    }
    transaction.onabort = () => {
      database.close()
      resolve()
    }
  })
}

export interface MediaStudioGalleryArchiveState {
  excludedIDs: string[]
  clearedBefore: number
}
export async function readMediaStudioGalleryArchiveState(ownerID?: number): Promise<MediaStudioGalleryArchiveState> {
  const empty: MediaStudioGalleryArchiveState = { excludedIDs: [], clearedBefore: 0 }
  const database = await openMediaStudioDatabase(ownerID)
  if (!database) return empty
  return new Promise((resolve) => {
    const transaction = database.transaction(GALLERY_SETTINGS_STORE_NAME, 'readonly')
    const request = transaction.objectStore(GALLERY_SETTINGS_STORE_NAME).get('archive-state')
    request.onsuccess = () => {
      const state = request.result as Partial<MediaStudioGalleryArchiveState> | undefined
      resolve({
        excludedIDs: Array.isArray(state?.excludedIDs) ? state.excludedIDs.filter(id => typeof id === 'string').slice(-2_000) : [],
        clearedBefore: typeof state?.clearedBefore === 'number' && Number.isFinite(state.clearedBefore) ? state.clearedBefore : 0,
      })
    }
    request.onerror = () => resolve(empty)
    transaction.oncomplete = () => database.close()
    transaction.onerror = transaction.onabort = () => { database.close(); resolve(empty) }
  })
}
export async function writeMediaStudioGalleryArchiveState(state: MediaStudioGalleryArchiveState, ownerID?: number): Promise<void> {
  const database = await openMediaStudioDatabase(ownerID)
  if (!database) return
  return new Promise((resolve) => {
    const transaction = database.transaction(GALLERY_SETTINGS_STORE_NAME, 'readwrite')
    transaction.objectStore(GALLERY_SETTINGS_STORE_NAME).put(state, 'archive-state')
    const finish = () => { database.close(); resolve() }
    transaction.oncomplete = transaction.onerror = transaction.onabort = finish
  })
}
