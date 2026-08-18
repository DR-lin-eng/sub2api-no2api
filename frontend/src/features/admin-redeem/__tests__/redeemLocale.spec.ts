import { describe, expect, it } from 'vitest'
import { redeemStatusLabel, redeemTypeLabel } from '@/features/admin-redeem/presentation/redeemLocale'

describe.each([
  { name: 'English', messages: { 'admin.redeem.types.subscription': 'Subscription', 'admin.redeem.status.expired': 'Expired', 'common.unknown': 'Unknown' } },
  { name: 'Chinese', messages: { 'admin.redeem.types.subscription': '订阅', 'admin.redeem.status.expired': '已过期', 'common.unknown': '未知' } },
])('redeem locale mapping - $name', ({ messages }) => {
  const t = (key: string) => messages[key as keyof typeof messages] ?? key

  it('maps known values and localizes future values', () => {
    expect(redeemTypeLabel(t, 'subscription')).toBe(messages['admin.redeem.types.subscription'])
    expect(redeemStatusLabel(t, 'expired')).toBe(messages['admin.redeem.status.expired'])
    expect(redeemTypeLabel(t, 'future_type')).toBe(messages['common.unknown'])
    expect(redeemStatusLabel(t, 'future_status')).toBe(messages['common.unknown'])
  })
})
