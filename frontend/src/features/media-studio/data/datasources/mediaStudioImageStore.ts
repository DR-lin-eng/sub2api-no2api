const DATABASE_NAME = 'sub2api-media-studio'
const DATABASE_VERSION = 1
const STORE_NAME = 'generated-images'

function openDatabase(): Promise<IDBDatabase | null> {
  if (typeof indexedDB === 'undefined') return Promise.resolve(null)

  return new Promise((resolve) => {
    const request = indexedDB.open(DATABASE_NAME, DATABASE_VERSION)
    request.onupgradeneeded = () => {
      const database = request.result
      if (!database.objectStoreNames.contains(STORE_NAME)) {
        database.createObjectStore(STORE_NAME)
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
  const database = await openDatabase()
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
  const database = await openDatabase()
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
  const database = await openDatabase()
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

  const database = await openDatabase()
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
