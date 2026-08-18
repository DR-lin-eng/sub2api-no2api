import { describe, expect, it } from 'vitest'
import enLocale from '@/core/i18n/locales/en/admin/settings'
import zhLocale from '@/core/i18n/locales/zh/admin/settings'
import {
  clientIPRangesSourceLabel,
  clientIPResolutionModeLabel,
} from '@/features/admin-settings/presentation/clientIpResolutionLocale'

function translator(messages: typeof enLocale, unknownLabel: string) {
  return (key: string): string => {
    if (key === 'common.unknown') return unknownLabel
    const path = key.replace(/^admin\./, '').split('.')
    let value: unknown = messages
    for (const part of path) {
      value = typeof value === 'object' && value !== null
        ? (value as Record<string, unknown>)[part]
        : undefined
    }
    return typeof value === 'string' ? value : key
  }
}

describe.each([
  { name: 'English', t: translator(enLocale, 'Unknown'), expected: ['Strict trusted proxies', 'Refreshed from official API', 'Unknown'] },
  { name: 'Chinese', t: translator(zhLocale as typeof enLocale, '未知'), expected: ['严格可信代理', '官方接口已刷新', '未知'] },
])('client IP locale mapping - $name', ({ t, expected }) => {
  it('maps known backend values and localizes future values', () => {
    expect(clientIPResolutionModeLabel(t, 'trusted_proxy')).toBe(expected[0])
    expect(clientIPRangesSourceLabel(t, 'refreshed')).toBe(expected[1])
    expect(clientIPResolutionModeLabel(t, 'future_mode')).toBe(expected[2])
    expect(clientIPRangesSourceLabel(t, 'future_source')).toBe(expected[2])
  })
})
