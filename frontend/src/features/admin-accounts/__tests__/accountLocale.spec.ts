import { describe, expect, it } from 'vitest'
import enLocale from '@/core/i18n/locales/en/admin/accounts'
import zhLocale from '@/core/i18n/locales/zh/admin/accounts'
import {
  accountStatusLabel,
  ollamaCloudUsageErrorLabel,
  ollamaCloudUsageStatusLabel,
} from '@/features/admin-accounts/presentation/accountLocale'

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
  { name: 'English', t: translator(enLocale, 'Unknown'), expected: ['Active', 'Inactive', 'Error', 'Unknown'] },
  { name: 'Chinese', t: translator(zhLocale as typeof enLocale, '未知'), expected: ['正常', '停用', '错误', '未知'] },
])('account locale mapping - $name', ({ t, expected }) => {
  it('maps known statuses and localizes future values', () => {
    expect(accountStatusLabel(t, 'active')).toBe(expected[0])
    expect(accountStatusLabel(t, 'inactive')).toBe(expected[1])
    expect(accountStatusLabel(t, 'error')).toBe(expected[2])
    expect(accountStatusLabel(t, 'future_status')).toBe(expected[3])
    expect(accountStatusLabel(t, null)).toBe(expected[3])
  })

  it('does not treat future Ollama Cloud states as successful or expose error codes', () => {
    expect(ollamaCloudUsageStatusLabel(t, 'ok')).toBe(t('admin.accounts.ollamaCloud.ok'))
    expect(ollamaCloudUsageStatusLabel(t, 'future_status')).toBe(t('common.unknown'))
    expect(ollamaCloudUsageErrorLabel(t, 'response_too_large')).toBe(t('admin.accounts.ollamaCloud.errors.response_too_large'))
    expect(ollamaCloudUsageErrorLabel(t, 'future_error')).toBe(t('admin.accounts.ollamaCloud.refreshFailed'))
  })
})
