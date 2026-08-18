import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const roleKeys = {
  admin: 'admin.users.roles.admin',
  user: 'admin.users.roles.user',
} as const

export function adminUserRoleLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, roleKeys, value)
}
