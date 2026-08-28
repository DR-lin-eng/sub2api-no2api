type Translate = (key: string) => string

const legacyTextKeys: Record<string, string> = {
  Default: 'legacy.defaultPool',
  'Basic pool': 'legacy.defaultPool',
  Thanks: 'legacy.defaultPrize',
  'Thanks for joining': 'legacy.defaultPrize',
  Balance: 'legacy.balance',
  'Balance reward': 'legacy.balance',
  Card: 'legacy.card',
  'Card code': 'legacy.card',
}

/** Localizes system-generated legacy values while preserving custom activity copy. */
export function activityText(value: string | null | undefined, t: Translate, namespace = 'activityCenter'): string {
  const text = value?.trim() || ''
  if (!text) return ''
  const key = legacyTextKeys[text]
  if (!key) return text
  const translated = t(`${namespace}.${key}`)
  return translated === `${namespace}.${key}` ? text : translated
}
