/**
 * Web Storage can be unavailable even when the global exists (for example in
 * a sandboxed iframe, a blocked third-party context, or a strict privacy mode).
 * Keep those failures out of the application bootstrap and use a short-lived
 * in-memory fallback for non-critical preferences.
 */

type StorageArea = 'local' | 'session'
const blockedAreas = new Set<StorageArea>()

function createMemoryStorage(): Storage {
  const values = new Map<string, string>()

  return {
    get length() {
      return values.size
    },
    clear() {
      values.clear()
    },
    getItem(key: string) {
      return values.has(key) ? values.get(key)! : null
    },
    key(index: number) {
      return Array.from(values.keys())[index] ?? null
    },
    removeItem(key: string) {
      values.delete(key)
    },
    setItem(key: string, value: string) {
      values.set(key, String(value))
    },
  }
}

export function createSafeStorage(area: StorageArea, getStorage?: () => Storage | null): Storage {
  const fallback = createMemoryStorage()
  const getNativeStorage = (): Storage | null => {
    try {
      if (getStorage) return getStorage()
      if (blockedAreas.has(area)) return null
      if (typeof window === 'undefined') return null
      return area === 'local' ? window.localStorage : window.sessionStorage
    } catch {
      return null
    }
  }

  return {
    get length() {
      const native = getNativeStorage()
      if (native) {
        try {
          return native.length
        } catch {
          // Fall through to the in-memory copy.
        }
      }
      return fallback.length
    },
    clear() {
      fallback.clear()
      try {
        getNativeStorage()?.clear()
      } catch {
        // Storage is optional; keep the in-memory fallback available.
      }
    },
    getItem(key: string) {
      try {
        const native = getNativeStorage()
        if (native) return native.getItem(key)
      } catch {
        // Fall through to the in-memory copy.
      }
      return fallback.getItem(key)
    },
    key(index: number) {
      try {
        const native = getNativeStorage()
        if (native) return native.key(index)
      } catch {
        // Fall through to the in-memory copy.
      }
      return fallback.key(index)
    },
    removeItem(key: string) {
      fallback.removeItem(key)
      try {
        getNativeStorage()?.removeItem(key)
      } catch {
        // Storage is optional; keep the application running.
      }
    },
    setItem(key: string, value: string) {
      fallback.setItem(key, value)
      try {
        getNativeStorage()?.setItem(key, value)
      } catch {
        // Storage is optional; the value remains available in memory.
      }
    },
  }
}

export const safeLocalStorage = createSafeStorage('local')
export const safeSessionStorage = createSafeStorage('session')

/**
 * Make legacy direct `window.localStorage`/`sessionStorage` calls safe after
 * bootstrap. Browsers normally expose configurable Window accessors; when an
 * opaque document denies storage, replace only that denied accessor with the
 * corresponding in-memory adapter. The operation is best-effort because some
 * browsers expose the accessor as non-configurable.
 */
export function installSafeStorageGuards(): void {
  if (typeof window === 'undefined') return

  for (const area of ['local', 'session'] as const) {
    try {
      const native = area === 'local' ? window.localStorage : window.sessionStorage
      native.getItem('__sub2api_storage_probe__')
    } catch {
      blockedAreas.add(area)
      try {
        Object.defineProperty(window, `${area}Storage`, {
          configurable: true,
          value: area === 'local' ? safeLocalStorage : safeSessionStorage,
        })
      } catch {
        // Critical call sites use the adapters directly; continue if the
        // browser does not permit redefining the Window property.
      }
    }
  }
}

// Run once during module evaluation so legacy modules that read storage while
// they are being imported are protected before the Vue bootstrap starts.
installSafeStorageGuards()
