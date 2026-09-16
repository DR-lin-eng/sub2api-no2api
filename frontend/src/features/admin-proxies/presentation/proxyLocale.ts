import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const proxyStatusKeys = {
  active: 'common.active',
  inactive: 'common.inactive',
  expired: 'admin.proxies.expired',
} as const

export function proxyStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, proxyStatusKeys, value)
}

const proxyHealthStatusKeys = {
  unknown: 'admin.proxies.autoAssignment.healthUnknown',
  healthy: 'admin.proxies.autoAssignment.healthHealthy',
  degraded: 'admin.proxies.autoAssignment.healthDegraded',
  unhealthy: 'admin.proxies.autoAssignment.healthUnhealthy'
} as const

export function proxyHealthStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, proxyHealthStatusKeys, value)
}
