import { describe, expect, it } from 'vitest'
import {
  paymentOrderStatusLabel,
  paymentOrderTypeLabel,
  paymentPlanValidityUnitLabel,
} from '@/features/billing/paymentLocale'

describe.each([
  { name: 'English', messages: { 'payment.status.refund_pending': 'Refund Pending', 'payment.admin.subscriptionOrder': 'Subscription', 'payment.admin.months': 'months', 'common.unknown': 'Unknown' } },
  { name: 'Chinese', messages: { 'payment.status.refund_pending': '退款处理中', 'payment.admin.subscriptionOrder': '订阅', 'payment.admin.months': '月', 'common.unknown': '未知' } },
])('payment locale mapping - $name', ({ messages }) => {
  const t = (key: string) => messages[key as keyof typeof messages] ?? key

  it('normalizes known values and localizes future values', () => {
    expect(paymentOrderStatusLabel(t, 'REFUND_PENDING')).toBe(messages['payment.status.refund_pending'])
    expect(paymentOrderTypeLabel(t, 'subscription')).toBe(messages['payment.admin.subscriptionOrder'])
    expect(paymentPlanValidityUnitLabel(t, 'month')).toBe(messages['payment.admin.months'])
    expect(paymentOrderStatusLabel(t, 'future_status')).toBe(messages['common.unknown'])
    expect(paymentOrderTypeLabel(t, 'future_type')).toBe(messages['common.unknown'])
    expect(paymentPlanValidityUnitLabel(t, 'future_unit')).toBe(messages['common.unknown'])
  })
})
