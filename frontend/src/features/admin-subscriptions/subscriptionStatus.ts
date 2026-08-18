import {
  normalizeSubscriptionStatus,
  subscriptionStatusValues,
  type SubscriptionStatus,
  type SubscriptionStatusTranslate,
} from '@/features/subscriptions/subscriptionStatus'

const adminStatusKeys = {
  active: 'admin.subscriptions.status.active',
  suspended: 'admin.subscriptions.status.suspended',
  expired: 'admin.subscriptions.status.expired',
  revoked: 'admin.subscriptions.status.revoked',
} as const satisfies Record<SubscriptionStatus, string>

export function adminSubscriptionStatusLabel(
  t: SubscriptionStatusTranslate,
  status: unknown,
): string {
  const normalized = normalizeSubscriptionStatus(status)
  return normalized ? t(adminStatusKeys[normalized]) : t('common.unknown')
}

export function adminSubscriptionStatusOptions(t: SubscriptionStatusTranslate) {
  return subscriptionStatusValues.map((value) => ({
    value,
    label: adminSubscriptionStatusLabel(t, value),
  }))
}
