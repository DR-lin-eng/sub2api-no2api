import type { UserSubscription } from '@/types'

export type SubscriptionStatus = UserSubscription['status']
export type SubscriptionStatusTranslate = (key: string) => string

const userStatusMetadata = {
  active: { key: 'userSubscriptions.status.active', order: 0 },
  suspended: { key: 'userSubscriptions.status.suspended', order: 1 },
  expired: { key: 'userSubscriptions.status.expired', order: 2 },
  revoked: { key: 'userSubscriptions.status.revoked', order: 3 },
} as const satisfies Record<SubscriptionStatus, { key: string; order: number }>

export const subscriptionStatusValues = (
  Object.keys(userStatusMetadata) as SubscriptionStatus[]
).sort((left, right) => (
  userStatusMetadata[left].order - userStatusMetadata[right].order
))

export function normalizeSubscriptionStatus(status: unknown): SubscriptionStatus | null {
  const normalized = String(status ?? '').trim().toLowerCase()
  return subscriptionStatusValues.includes(normalized as SubscriptionStatus)
    ? normalized as SubscriptionStatus
    : null
}

export function userSubscriptionStatusLabel(
  t: SubscriptionStatusTranslate,
  status: unknown,
): string {
  const normalized = normalizeSubscriptionStatus(status)
  return normalized ? t(userStatusMetadata[normalized].key) : t('common.unknown')
}
