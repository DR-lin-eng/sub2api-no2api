import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const proxyStatusKeys = {
  active: 'common.active',
  inactive: 'common.inactive',
  expired: 'admin.proxies.expired',
} as const

export function proxyStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, proxyStatusKeys, value)
}
