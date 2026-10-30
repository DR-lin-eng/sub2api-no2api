const DATABASE_NAME = 'sub2api-media-studio'
// 版本 3 新增作品库元数据与素材 store；升级时只补建缺失的 store，旧缓存不受影响。
const DATABASE_VERSION = 3
const STORE_NAME = 'generated-images'
const VIDEO_STORE_NAME = 'generated-videos'
// 作品库与会话缓存分开存放：清空会话只清 generated-*，作品库独立保留。
export const GALLERY_ITEM_STORE_NAME = 'gallery-items'
export const GALLERY_MEDIA_STORE_NAME = 'gallery-media'

export function openMediaStudioDatabase(): Promise<IDBDatabase | null> {
  if (typeof indexedDB === 'undefined') return Promise.resolve(null)

  return new Promise((resolve) => {
    const request = indexedDB.open(DATABASE_NAME, DATABASE_VERSION)
    request.onupgradeneeded = () => {
      const database = request.result
      if (!database.objectStoreNames.contains(STORE_NAME)) {
        database.createObjectStore(STORE_NAME)
      }
      if (!database.objectStoreNames.contains(VIDEO_STORE_NAME)) {
        database.createObjectStore(VIDEO_STORE_NAME)
      }
      if (!database.objectStoreNames.contains(GALLERY_ITEM_STORE_NAME)) {
        const store = database.createObjectStore(GALLERY_ITEM_STORE_NAME, { keyPath: 'id' })
        store.createIndex('createdAt', 'createdAt', { unique: false })
      }
      if (!database.objectStoreNames.contains(GALLERY_MEDIA_STORE_NAME)) {
        database.createObjectStore(GALLERY_MEDIA_STORE_NAME)
      }
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => resolve(null)
    request.onblocked = () => resolve(null)
  })
}

export function mediaStudioImageStorageKey(messageID: string, imageID: string): string {
  return `${messageID}:${imageID}`
}

export async function storeMediaStudioImage(key: string, source: string): Promise<boolean> {
  const database = await openMediaStudioDatabase()
  if (!database || !key || !source) return false

  return new Promise((resolve) => {
    const transaction = database.transaction(STORE_NAME, 'readwrite')
    transaction.objectStore(STORE_NAME).put(source, key)
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

export async function loadMediaStudioImage(key: string): Promise<string> {
  const database = await openMediaStudioDatabase()
  if (!database || !key) return ''

  return new Promise((resolve) => {
    const transaction = database.transaction(STORE_NAME, 'readonly')
    const request = transaction.objectStore(STORE_NAME).get(key)
    request.onsuccess = () => resolve(typeof request.result === 'string' ? request.result : '')
    request.onerror = () => resolve('')
    transaction.oncomplete = () => database.close()
    transaction.onerror = () => database.close()
    transaction.onabort = () => database.close()
  })
}

export async function clearMediaStudioImages(): Promise<void> {
  const database = await openMediaStudioDatabase()
  if (!database) return

  await new Promise<void>((resolve) => {
    const transaction = database.transaction(STORE_NAME, 'readwrite')
    transaction.objectStore(STORE_NAME).clear()
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

export async function deleteMediaStudioImages(keys: string[]): Promise<void> {
  const normalizedKeys = [...new Set(keys.filter(Boolean))]
  if (normalizedKeys.length === 0) return

  const database = await openMediaStudioDatabase()
  if (!database) return

  await new Promise<void>((resolve) => {
    const transaction = database.transaction(STORE_NAME, 'readwrite')
    const store = transaction.objectStore(STORE_NAME)
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

export function mediaStudioVideoStorageKey(messageID: string): string {
  return `${messageID}:video`
}

export async function storeMediaStudioVideo(key: string, blob: Blob): Promise<boolean> {
  const database = await openMediaStudioDatabase()
  if (!database || !key || !blob) return false

  return new Promise((resolve) => {
    const transaction = database.transaction(VIDEO_STORE_NAME, 'readwrite')
    transaction.objectStore(VIDEO_STORE_NAME).put(blob, key)
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

export async function loadMediaStudioVideo(key: string): Promise<Blob | null> {
  const database = await openMediaStudioDatabase()
  if (!database || !key) return null

  return new Promise((resolve) => {
    const transaction = database.transaction(VIDEO_STORE_NAME, 'readonly')
    const request = transaction.objectStore(VIDEO_STORE_NAME).get(key)
    request.onsuccess = () => resolve(request.result instanceof Blob ? request.result : null)
    request.onerror = () => resolve(null)
    transaction.oncomplete = () => database.close()
    transaction.onerror = () => database.close()
    transaction.onabort = () => database.close()
  })
}

export async function clearMediaStudioVideos(): Promise<void> {
  const database = await openMediaStudioDatabase()
  if (!database) return

  await new Promise<void>((resolve) => {
    const transaction = database.transaction(VIDEO_STORE_NAME, 'readwrite')
    transaction.objectStore(VIDEO_STORE_NAME).clear()
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

export async function deleteMediaStudioVideos(keys: string[]): Promise<void> {
  const normalizedKeys = [...new Set(keys.filter(Boolean))]
  if (normalizedKeys.length === 0) return

  const database = await openMediaStudioDatabase()
  if (!database) return

  await new Promise<void>((resolve) => {
    const transaction = database.transaction(VIDEO_STORE_NAME, 'readwrite')
    const store = transaction.objectStore(VIDEO_STORE_NAME)
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
