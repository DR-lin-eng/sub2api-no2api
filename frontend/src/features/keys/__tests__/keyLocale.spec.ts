import { describe, expect, it } from 'vitest'
import { apiKeyStatusLabel } from '@/features/keys/presentation/keyLocale'

describe.each([
  { name: 'English', messages: { 'keys.status.active': 'Active', 'keys.status.quota_exhausted': 'Quota Exhausted', 'common.unknown': 'Unknown' } },
  { name: 'Chinese', messages: { 'keys.status.active': '活跃', 'keys.status.quota_exhausted': '额度耗尽', 'common.unknown': '未知' } },
])('API key locale mapping - $name', ({ messages }) => {
  const t = (key: string) => messages[key as keyof typeof messages] ?? key

  it('maps known statuses and localizes future values', () => {
    expect(apiKeyStatusLabel(t, 'active')).toBe(messages['keys.status.active'])
    expect(apiKeyStatusLabel(t, 'quota_exhausted')).toBe(messages['keys.status.quota_exhausted'])
    expect(apiKeyStatusLabel(t, 'future_status')).toBe(messages['common.unknown'])
    expect(apiKeyStatusLabel(t, null)).toBe(messages['common.unknown'])
  })
})
