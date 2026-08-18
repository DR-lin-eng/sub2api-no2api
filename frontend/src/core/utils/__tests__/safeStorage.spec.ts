import { describe, expect, it } from 'vitest'
import { createSafeStorage } from '@/core/utils/safeStorage'

describe('safe storage', () => {
  it('keeps the application usable when the browser denies storage access', () => {
    const storage = createSafeStorage('local', () => {
      throw new DOMException('Storage access is denied', 'SecurityError')
    })

    expect(() => storage.setItem('theme', 'dark')).not.toThrow()
    expect(storage.getItem('theme')).toBe('dark')
    expect(() => storage.removeItem('theme')).not.toThrow()
    expect(storage.getItem('theme')).toBeNull()
  })

  it('uses the native storage when it is available', () => {
    const native = new Map<string, string>()
    const storage = createSafeStorage('local', () => ({
      get length() {
        return native.size
      },
      clear: () => native.clear(),
      getItem: (key: string) => native.get(key) ?? null,
      key: (index: number) => Array.from(native.keys())[index] ?? null,
      removeItem: (key: string) => native.delete(key),
      setItem: (key: string, value: string) => native.set(key, value),
    }))

    storage.setItem('locale', 'zh')
    expect(native.get('locale')).toBe('zh')
    expect(storage.getItem('locale')).toBe('zh')
  })
})
