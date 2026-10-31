// Each authenticated user owns a separate browser database. Never adopt unscoped legacy media.
const DATABASE_VERSION = 1
const STORE_NAME = 'generated-images'
const VIDEO_STORE_NAME = 'generated-videos'
const REFERENCE_STORE_NAME = 'reference-media'
export const GALLERY_ITEM_STORE_NAME = 'gallery-items'
export const GALLERY_MEDIA_STORE_NAME = 'gallery-media'
export const GALLERY_SETTINGS_STORE_NAME = 'gallery-settings'

export function normalizeMediaStudioOwnerID(value: unknown): number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value > 0 ? value : 0
}

export function openMediaStudioDatabase(ownerID?: number): Promise<IDBDatabase | null> {
  const owner = normalizeMediaStudioOwnerID(ownerID)
  if (!owner || typeof indexedDB === 'undefined') return Promise.resolve(null)
  // Capture the owner before asynchronous opening; an account switch cannot retarget this operation.
  const databaseName = `sub2api-media-studio-user-${owner}`
  return new Promise((resolve) => {
    const request = indexedDB.open(databaseName, DATABASE_VERSION)
    let settled = false
    const finish = (database: IDBDatabase | null) => {
      if (settled) { database?.close(); return }
      settled = true
      resolve(database)
    }
    request.onupgradeneeded = () => {
      const database = request.result
      for (const name of [STORE_NAME, VIDEO_STORE_NAME, REFERENCE_STORE_NAME, GALLERY_MEDIA_STORE_NAME, GALLERY_SETTINGS_STORE_NAME]) {
        if (!database.objectStoreNames.contains(name)) database.createObjectStore(name)
      }
      if (!database.objectStoreNames.contains(GALLERY_ITEM_STORE_NAME)) {
        const store = database.createObjectStore(GALLERY_ITEM_STORE_NAME, { keyPath: 'id' })
        store.createIndex('createdAt', 'createdAt', { unique: false })
      }
    }
    request.onsuccess = () => finish(request.result)
    request.onerror = () => finish(null)
    request.onblocked = () => finish(null)
  })
}

async function writeMedia(store: string, key: string, value: string | Blob, ownerID?: number): Promise<boolean> {
  if (!key || !value) return false
  const database = await openMediaStudioDatabase(ownerID)
  if (!database) return false
  return new Promise((resolve) => {
    const transaction = database.transaction(store, 'readwrite')
    transaction.objectStore(store).put(value, key)
    transaction.oncomplete = () => { database.close(); resolve(true) }
    transaction.onerror = transaction.onabort = () => { database.close(); resolve(false) }
  })
}

async function readMedia(store: string, key: string, ownerID?: number): Promise<string | Blob | null> {
  if (!key) return null
  const database = await openMediaStudioDatabase(ownerID)
  if (!database) return null
  return new Promise((resolve) => {
    const transaction = database.transaction(store, 'readonly')
    const request = transaction.objectStore(store).get(key)
    request.onsuccess = () => resolve(typeof request.result === 'string' || request.result instanceof Blob ? request.result : null)
    request.onerror = () => resolve(null)
    transaction.oncomplete = () => database.close()
    transaction.onerror = transaction.onabort = () => { database.close(); resolve(null) }
  })
}

async function deleteMedia(store: string, keys: string[] | null, ownerID?: number): Promise<void> {
  if (keys && keys.length === 0) return
  const database = await openMediaStudioDatabase(ownerID)
  if (!database) return
  return new Promise((resolve) => {
    const transaction = database.transaction(store, 'readwrite')
    if (keys) {
      for (const key of new Set(keys.filter(Boolean))) transaction.objectStore(store).delete(key)
    } else transaction.objectStore(store).clear()
    const finish = () => { database.close(); resolve() }
    transaction.oncomplete = transaction.onerror = transaction.onabort = finish
  })
}

export function mediaStudioImageStorageKey(messageID: string, imageID: string): string {
  return `${messageID}:${imageID}`
}
export const storeMediaStudioImage = (key: string, source: string, ownerID?: number) => writeMedia(STORE_NAME, key, source, ownerID)
export async function loadMediaStudioImage(key: string, ownerID?: number): Promise<string> {
  const source = await readMedia(STORE_NAME, key, ownerID)
  return typeof source === 'string' ? source : ''
}
export const clearMediaStudioImages = (ownerID?: number) => deleteMedia(STORE_NAME, null, ownerID)
export const deleteMediaStudioImages = (keys: string[], ownerID?: number) => deleteMedia(STORE_NAME, keys, ownerID)
export const mediaStudioVideoStorageKey = (messageID: string) => `${messageID}:video`
export const storeMediaStudioVideo = (key: string, blob: Blob, ownerID?: number) => writeMedia(VIDEO_STORE_NAME, key, blob, ownerID)
export async function loadMediaStudioVideo(key: string, ownerID?: number): Promise<Blob | null> {
  const source = await readMedia(VIDEO_STORE_NAME, key, ownerID)
  return source instanceof Blob ? source : null
}
export const clearMediaStudioVideos = (ownerID?: number) => deleteMedia(VIDEO_STORE_NAME, null, ownerID)
export const deleteMediaStudioVideos = (keys: string[], ownerID?: number) => deleteMedia(VIDEO_STORE_NAME, keys, ownerID)
export const storeMediaStudioReference = (key: string, blob: Blob, ownerID?: number) => writeMedia(REFERENCE_STORE_NAME, key, blob, ownerID)
export async function loadMediaStudioReference(key: string, ownerID?: number): Promise<Blob | null> {
  const source = await readMedia(REFERENCE_STORE_NAME, key, ownerID)
  return source instanceof Blob ? source : null
}
export const deleteMediaStudioReferences = (keys: string[], ownerID?: number) => deleteMedia(REFERENCE_STORE_NAME, keys, ownerID)
