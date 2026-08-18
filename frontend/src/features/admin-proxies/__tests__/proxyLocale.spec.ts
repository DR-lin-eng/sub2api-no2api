import { describe, expect, it } from 'vitest'
import { proxyStatusLabel } from '@/features/admin-proxies/presentation/proxyLocale'

describe.each([
  { name: 'English', messages: { 'common.active': 'Active', 'common.inactive': 'Inactive', 'admin.proxies.expired': 'Expired', 'common.unknown': 'Unknown' } },
  { name: 'Chinese', messages: { 'common.active': '启用', 'common.inactive': '禁用', 'admin.proxies.expired': '已过期', 'common.unknown': '未知' } },
])('proxy locale mapping - $name', ({ messages }) => {
  const t = (key: string) => messages[key as keyof typeof messages] ?? key

  it('maps every status and localizes future values', () => {
    expect(proxyStatusLabel(t, 'active')).toBe(messages['common.active'])
    expect(proxyStatusLabel(t, 'inactive')).toBe(messages['common.inactive'])
    expect(proxyStatusLabel(t, 'expired')).toBe(messages['admin.proxies.expired'])
    expect(proxyStatusLabel(t, 'future_status')).toBe(messages['common.unknown'])
  })
})
