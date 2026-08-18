import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const orderStatusKeys = {
  pending: 'payment.status.pending',
  paid: 'payment.status.paid',
  recharging: 'payment.status.recharging',
  completed: 'payment.status.completed',
  expired: 'payment.status.expired',
  cancelled: 'payment.status.cancelled',
  failed: 'payment.status.failed',
  refund_requested: 'payment.status.refund_requested',
  refunding: 'payment.status.refunding',
  refund_pending: 'payment.status.refund_pending',
  refunded: 'payment.status.refunded',
  partially_refunded: 'payment.status.partially_refunded',
  refund_failed: 'payment.status.refund_failed',
} as const

const orderTypeKeys = {
  balance: 'payment.admin.balanceOrder',
  subscription: 'payment.admin.subscriptionOrder',
} as const

const validityUnitKeys = {
  day: 'payment.admin.days',
  days: 'payment.admin.days',
  week: 'payment.admin.weeks',
  weeks: 'payment.admin.weeks',
  month: 'payment.admin.months',
  months: 'payment.admin.months',
} as const

function normalizedLower(value: unknown): string {
  return typeof value === 'string' ? value.trim().toLowerCase() : ''
}

export function paymentOrderStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, orderStatusKeys, normalizedLower(value))
}

export function paymentOrderTypeLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, orderTypeKeys, normalizedLower(value))
}

export function paymentPlanValidityUnitLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, validityUnitKeys, normalizedLower(value))
}
