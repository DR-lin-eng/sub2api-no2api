import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const modeKeys = {
  auto_compat: 'admin.settings.apiKeyAcl.modes.auto_compat',
  trusted_proxy: 'admin.settings.apiKeyAcl.modes.trusted_proxy',
  direct: 'admin.settings.apiKeyAcl.modes.direct',
} as const

const rangesSourceKeys = {
  embedded: 'admin.settings.apiKeyAcl.sources.embedded',
  refreshed: 'admin.settings.apiKeyAcl.sources.refreshed',
} as const

export function clientIPResolutionModeLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, modeKeys, value)
}

export function clientIPRangesSourceLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, rangesSourceKeys, value)
}
