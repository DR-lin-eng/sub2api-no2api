import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const roleKeys = {
  admin: 'admin.users.roles.admin',
  user: 'admin.users.roles.user',
} as const

export function adminUserRoleLabel(t: LocaleTranslate, value: unknown, customLabels: Record<string, string> = {}): string {
  if (typeof value === 'string' && customLabels[value]) return customLabels[value]
  return enumLocaleLabel(t, roleKeys, value)
}
