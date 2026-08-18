export type LocaleTranslate = (key: string) => string

export function enumLocaleLabel(
  t: LocaleTranslate,
  keys: Readonly<Record<string, string>>,
  value: unknown,
  fallbackKey = 'common.unknown',
): string {
  const normalized = typeof value === 'string' ? value.trim() : ''
  const key = Object.prototype.hasOwnProperty.call(keys, normalized)
    ? keys[normalized]
    : undefined
  return t(key || fallbackKey)
}
