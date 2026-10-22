import type { OpenAIEndpointCapability } from '@/types'

export interface ModelMapping {
  from: string
  to: string
}

export function buildAPIKeySearchExtra(
  base: Record<string, unknown> | undefined,
  platform: string,
  category: string,
  searchMode: string,
  anthropicPassthrough: boolean,
  anthropicAuthScheme: string,
): Record<string, unknown> | undefined {
  if (category !== 'apikey' || (platform !== 'anthropic' && platform !== 'openai')) return base
  const extra = { ...(base || {}) }
  if (platform === 'anthropic') {
    if (anthropicPassthrough) extra.anthropic_passthrough = true
    else delete extra.anthropic_passthrough
    if (anthropicAuthScheme === 'authorization_bearer') extra.anthropic_apikey_auth_scheme = anthropicAuthScheme
    else delete extra.anthropic_apikey_auth_scheme
  }
  if (searchMode === 'default') delete extra.web_search_emulation
  else extra.web_search_emulation = searchMode
  return Object.keys(extra).length > 0 ? extra : undefined
}

export function isOpenAIPersonalAccessTokenCredentials(credentials?: Record<string, unknown>): boolean {
  const authMode = String(credentials?.auth_mode ?? credentials?.openai_auth_mode ?? '')
    .trim()
    .toLowerCase()
  return authMode === 'personalaccesstoken' || authMode === 'personal_access_token'
}

export function readCodexWebSearchEnabled(credentials?: Record<string, unknown>): boolean {
  const raw = credentials?.openai_capabilities
  if (Array.isArray(raw)) return raw.includes('alpha_search')
  if (raw !== null && typeof raw === 'object') {
    return (raw as Record<string, unknown>).alpha_search === true
  }
  return true
}

export function normalizeOpenAIEndpointCapabilities(
  values: OpenAIEndpointCapability[],
): OpenAIEndpointCapability[] {
  const allowed: OpenAIEndpointCapability[] = ['chat_completions', 'embeddings', 'decisions']
  const selected = allowed.filter((value) => values.includes(value))
  return selected.length > 0 ? selected : allowed
}

export function readOpenAIEndpointCapabilities(
  credentials?: Record<string, unknown>,
): OpenAIEndpointCapability[] {
  const raw = credentials?.openai_capabilities
  if (Array.isArray(raw)) {
    return normalizeOpenAIEndpointCapabilities(
      raw.filter((value): value is OpenAIEndpointCapability =>
        value === 'chat_completions' || value === 'embeddings' || value === 'decisions',
      ),
    )
  }
  if (raw !== null && typeof raw === 'object') {
    const capabilityMap = raw as Record<string, unknown>
    return normalizeOpenAIEndpointCapabilities(
      ['chat_completions', 'embeddings', 'decisions'].filter(
        (value): value is OpenAIEndpointCapability => capabilityMap[value] === true,
      ),
    )
  }
  return ['chat_completions', 'embeddings', 'decisions']
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

export type TempUnschedScope = '' | 'account' | 'model'

export function normalizeTempUnschedScope(value: unknown): TempUnschedScope {
  const scope = typeof value === 'string' ? value.trim().toLowerCase() : ''
  return scope === 'account' || scope === 'model' ? scope : ''
}

export interface TempUnschedRuleForm {
  scope?: TempUnschedScope
  error_code: number | null
  keywords: string
  duration_minutes: number | null
  description: string
}

export interface TempUnschedRulePayload {
  scope?: 'account' | 'model'
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
        scope: '',
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

    const scope = normalizeTempUnschedScope(rule.scope)
    result.push({
      ...(scope ? { scope } : {}),
      error_code: Math.trunc(errorCode),
      keywords,
      duration_minutes: Math.trunc(duration),
      description: rule.description.trim(),
    })
  }
  return result
}

// Preserve legacy defaults while hydrating optional scope from stored credentials.
export function parseTempUnschedRules(value: unknown): TempUnschedRuleForm[] {
  if (!Array.isArray(value)) return []
  const positiveInteger = (input: unknown) => {
    const number = Number(input)
    return Number.isFinite(number) && number > 0 ? Math.trunc(number) : null
  }
  return value.map((raw) => {
    const entry = raw !== null && typeof raw === 'object' ? raw as Record<string, unknown> : {}
    return {
      scope: normalizeTempUnschedScope(entry.scope),
      error_code: positiveInteger(entry.error_code),
      keywords: formatTempUnschedKeywords(entry.keywords),
      duration_minutes: positiveInteger(entry.duration_minutes),
      description: typeof entry.description === 'string' ? entry.description : '',
    }
  })
}

function formatTempUnschedKeywords(value: unknown) {
  if (Array.isArray(value)) {
    return value
      .filter((item): item is string => typeof item === 'string')
      .map((item) => item.trim())
      .filter((item) => item.length > 0)
      .join(', ')
  }
  if (typeof value === 'string') {
    return value
  }
  return ''
}
