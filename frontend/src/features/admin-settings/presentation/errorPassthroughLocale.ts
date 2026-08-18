import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const matchModeKeys = {
  any: 'admin.errorPassthrough.matchMode.any',
  all: 'admin.errorPassthrough.matchMode.all',
} as const

export function errorPassthroughMatchModeLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, matchModeKeys, value)
}
