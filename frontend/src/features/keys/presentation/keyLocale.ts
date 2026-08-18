import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const keyStatusKeys = {
  active: 'keys.status.active',
  inactive: 'keys.status.inactive',
  quota_exhausted: 'keys.status.quota_exhausted',
  expired: 'keys.status.expired',
} as const

export function apiKeyStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, keyStatusKeys, value)
}
