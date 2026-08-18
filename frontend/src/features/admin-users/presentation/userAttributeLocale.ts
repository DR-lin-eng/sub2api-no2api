import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const attributeTypeKeys = {
  text: 'admin.users.attributes.types.text',
  textarea: 'admin.users.attributes.types.textarea',
  number: 'admin.users.attributes.types.number',
  email: 'admin.users.attributes.types.email',
  url: 'admin.users.attributes.types.url',
  date: 'admin.users.attributes.types.date',
  select: 'admin.users.attributes.types.select',
  multi_select: 'admin.users.attributes.types.multi_select',
} as const

export function userAttributeTypeLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, attributeTypeKeys, value)
}
