export interface ModelMapping {
  from: string
  to: string
}

export type CodexFingerprintMode = 'off' | 'device' | 'session' | 'full'

export function normalizeCodexFingerprintMode(value: unknown): CodexFingerprintMode {
  if (typeof value !== 'string') return 'off'

  switch (value.trim().toLowerCase()) {
    case 'device':
      return 'device'
    case 'session':
      return 'session'
    case 'full':
      return 'full'
    default:
      return 'off'
  }
}

export function getCodexFingerprintModeOptions(
  translate: (key: string) => string,
): Array<{ value: CodexFingerprintMode; label: string }> {
  return [
    { value: 'off', label: translate('admin.accounts.openai.codexFingerprintModeOff') },
    { value: 'device', label: translate('admin.accounts.openai.codexFingerprintModeDevice') },
    { value: 'session', label: translate('admin.accounts.openai.codexFingerprintModeSession') },
    { value: 'full', label: translate('admin.accounts.openai.codexFingerprintModeFull') },
  ]
}

export interface TempUnschedRuleForm {
  error_code: number | null
  keywords: string
  duration_minutes: number | null
  description: string
}

export interface TempUnschedRulePayload {
  error_code: number
  keywords: string[]
  duration_minutes: number
  description: string
}

export const DEFAULT_POOL_MODE_RETRY_COUNT = 3
export const MAX_POOL_MODE_RETRY_COUNT = 10
export const DEFAULT_POOL_MODE_RETRY_STATUS_CODES = [401, 403, 429] as const

export function parsePoolModeRetryStatusCodes(input: string): number[] {
  if (!input || !input.trim()) return []

  const seen = new Set<number>()
  const result: number[] = []
  for (const token of input.split(/[,\s]+/)) {
    const value = Number(token.trim())
    if (!Number.isInteger(value) || value < 100 || value > 599 || seen.has(value)) {
      continue
    }
    seen.add(value)
    result.push(value)
  }
  return result.sort((left, right) => left - right)
}

export function formatPoolModeRetryStatusCodes(value: unknown): string {
  if (!Array.isArray(value)) return ''
  return parsePoolModeRetryStatusCodes(value.map((item) => String(item)).join(','))
    .join(', ')
}

export function normalizePoolModeRetryCount(value: number): number {
  const normalized = Number(value)
  if (!Number.isFinite(normalized)) return DEFAULT_POOL_MODE_RETRY_COUNT
  return Math.min(MAX_POOL_MODE_RETRY_COUNT, Math.max(0, Math.trunc(normalized)))
}

export function addEmptyModelMapping(mappings: ModelMapping[]): void {
  mappings.push({ from: '', to: '' })
}

export function removeModelMapping<T>(items: T[], index: number): void {
  items.splice(index, 1)
}

export function addPresetModelMapping(
  mappings: ModelMapping[],
  from: string,
  to: string,
): boolean {
  if (mappings.some((mapping) => mapping.from === from)) {
    return false
  }
  mappings.push({ from, to })
  return true
}

export function createTempUnschedRule(
  preset?: TempUnschedRuleForm,
): TempUnschedRuleForm {
  return preset
    ? { ...preset }
    : {
        error_code: null,
        keywords: '',
        duration_minutes: 30,
        description: '',
      }
}

export function moveTempUnschedRule(
  rules: TempUnschedRuleForm[],
  index: number,
  direction: number,
): void {
  const target = index + direction
  if (target < 0 || target >= rules.length) return
  ;[rules[index], rules[target]] = [rules[target], rules[index]]
}

export function splitTempUnschedKeywords(value: string): string[] {
  return value
    .split(/[,;]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

export function buildTempUnschedRules(
  rules: TempUnschedRuleForm[],
): TempUnschedRulePayload[] {
  const result: TempUnschedRulePayload[] = []
  for (const rule of rules) {
    const errorCode = Number(rule.error_code)
    const duration = Number(rule.duration_minutes)
    const keywords = splitTempUnschedKeywords(rule.keywords)
    if (!Number.isFinite(errorCode) || errorCode < 100 || errorCode > 599) continue
    if (!Number.isFinite(duration) || duration <= 0 || keywords.length === 0) continue

    result.push({
      error_code: Math.trunc(errorCode),
      keywords,
      duration_minutes: Math.trunc(duration),
      description: rule.description.trim(),
    })
  }
  return result
}
