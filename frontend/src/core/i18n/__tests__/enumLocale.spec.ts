import { describe, expect, it, vi } from 'vitest'
import { enumLocaleLabel } from '@/core/i18n/enumLocale'

describe('enumLocaleLabel', () => {
  it('resolves only own explicitly mapped values', () => {
    const t = vi.fn((key: string) => `translated:${key}`)
    const keys = Object.create({ inherited: 'unsafe.inherited' }) as Record<string, string>
    keys.ready = 'status.ready'

    expect(enumLocaleLabel(t, keys, ' ready ')).toBe('translated:status.ready')
    expect(enumLocaleLabel(t, keys, 'inherited')).toBe('translated:common.unknown')
    expect(enumLocaleLabel(t, keys, '__proto__')).toBe('translated:common.unknown')
  })

  it('uses the caller-owned fallback for non-string and future values', () => {
    const t = (key: string) => key
    expect(enumLocaleLabel(t, {}, null, 'feature.unknown')).toBe('feature.unknown')
    expect(enumLocaleLabel(t, {}, 'future', 'feature.unknown')).toBe('feature.unknown')
  })
})
