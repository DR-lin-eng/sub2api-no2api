import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const redeemTypeKeys = {
  balance: 'admin.redeem.types.balance',
  concurrency: 'admin.redeem.types.concurrency',
  subscription: 'admin.redeem.types.subscription',
  invitation: 'admin.redeem.types.invitation',
} as const

const redeemStatusKeys = {
  unused: 'admin.redeem.status.unused',
  used: 'admin.redeem.status.used',
  expired: 'admin.redeem.status.expired',
  disabled: 'admin.redeem.status.disabled',
} as const

export function redeemTypeLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, redeemTypeKeys, value)
}

export function redeemStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, redeemStatusKeys, value)
}
