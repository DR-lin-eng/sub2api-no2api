import type { AccountPlatform } from '@/types'

export const CN_ACCOUNT_PLATFORMS = ['kimi', 'zhipu', 'deepseek', 'minimax'] as const
export type CNAccountPlatform = (typeof CN_ACCOUNT_PLATFORMS)[number]
export type CNAccountMode = 'payg' | 'coding'
export type OpenCodeAccountMode = 'zen' | 'go'
export type CNAPIProtocol = 'chat_completions' | 'adaptive' | 'anthropic' | 'responses'
export type CNNativeAPIProtocol = Exclude<CNAPIProtocol, 'adaptive'>
export type CNAdaptiveBaseURLs = Record<CNNativeAPIProtocol, string>

export const CN_ACCOUNT_PLATFORM_LABELS: Record<CNAccountPlatform, string> = {
  kimi: 'Kimi',
  zhipu: 'Zhipu GLM',
  deepseek: 'DeepSeek',
  minimax: 'MiniMax',
}

export function isCNAccountPlatform(platform: string): platform is CNAccountPlatform {
  return (CN_ACCOUNT_PLATFORMS as readonly string[]).includes(platform)
}

export function cnSupportsNativeResponses(platform: string): boolean {
  return platform === 'kimi' || platform === 'deepseek' || platform === 'minimax' || platform === 'opencode_go'
}

export function defaultCNBaseURL(
  platform: string,
  mode: CNAccountMode | OpenCodeAccountMode,
  protocol: CNAPIProtocol = 'chat_completions',
): string {
  if (protocol === 'anthropic') {
    switch (platform) {
      case 'kimi': return mode === 'coding' ? 'https://api.kimi.com/coding' : 'https://api.moonshot.cn/anthropic'
      case 'zhipu': return 'https://open.bigmodel.cn/api/anthropic'
      case 'deepseek': return 'https://api.deepseek.com/anthropic'
      case 'minimax': return 'https://api.minimaxi.com/anthropic'
      case 'opencode_go': return mode === 'zen' ? 'https://opencode.ai/zen' : 'https://opencode.ai/zen/go'
      default: return ''
    }
  }

  switch (platform) {
    case 'kimi': return mode === 'coding' ? 'https://api.kimi.com/coding/v1' : 'https://api.moonshot.cn/v1'
    case 'zhipu': return mode === 'coding'
      ? 'https://open.bigmodel.cn/api/coding/paas/v4'
      : 'https://open.bigmodel.cn/api/paas/v4'
    case 'deepseek': return 'https://api.deepseek.com'
    case 'minimax': return 'https://api.minimaxi.com/v1'
    case 'opencode_go': return mode === 'zen' ? 'https://opencode.ai/zen/v1' : 'https://opencode.ai/zen/go/v1'
    default: return ''
  }
}

export function defaultCNAdaptiveBaseURLs(
  platform: CNAccountPlatform | 'opencode_go',
  mode: CNAccountMode | OpenCodeAccountMode,
): CNAdaptiveBaseURLs {
  return {
    chat_completions: defaultCNBaseURL(platform, mode, 'chat_completions'),
    anthropic: defaultCNBaseURL(platform, mode, 'anthropic'),
    responses: cnSupportsNativeResponses(platform) ? defaultCNBaseURL(platform, mode, 'responses') : '',
  }
}

export function updateCNAdaptiveDefaults(
  current: CNAdaptiveBaseURLs,
  platform: CNAccountPlatform | 'opencode_go',
  previousMode: CNAccountMode | OpenCodeAccountMode,
  nextMode: CNAccountMode | OpenCodeAccountMode,
): CNAdaptiveBaseURLs {
  const previousDefaults = defaultCNAdaptiveBaseURLs(platform, previousMode)
  const nextDefaults = defaultCNAdaptiveBaseURLs(platform, nextMode)
  return Object.fromEntries(
    (['chat_completions', 'anthropic', 'responses'] as CNNativeAPIProtocol[]).map((protocol) => {
      const value = current[protocol].trim()
      return [protocol, !value || value === previousDefaults[protocol] ? nextDefaults[protocol] : value]
    }),
  ) as CNAdaptiveBaseURLs
}

export interface OpenCodeProtocolRule {
  pattern: string
  protocol: CNNativeAPIProtocol
}

const OPENCODE_GO_PROTOCOL_RULES: OpenCodeProtocolRule[] = [
  { pattern: 'grok-*', protocol: 'responses' },
  { pattern: 'gpt-*', protocol: 'responses' },
  { pattern: 'muse-spark-*', protocol: 'responses' },
  { pattern: 'minimax-*', protocol: 'anthropic' },
  { pattern: 'qwen*', protocol: 'anthropic' },
]

const OPENCODE_ZEN_PROTOCOL_RULES: OpenCodeProtocolRule[] = [
  { pattern: 'grok-*', protocol: 'responses' },
  { pattern: 'gpt-*', protocol: 'responses' },
  { pattern: 'muse-spark-*', protocol: 'responses' },
  { pattern: 'claude-*', protocol: 'anthropic' },
  { pattern: 'qwen*', protocol: 'anthropic' },
]

export function cloneOpenCodeProtocolRules(rules: OpenCodeProtocolRule[]): OpenCodeProtocolRule[] {
  return rules.map((rule) => ({ ...rule }))
}

export function defaultOpenCodeProtocolRules(mode: OpenCodeAccountMode): OpenCodeProtocolRule[] {
  return cloneOpenCodeProtocolRules(mode === 'zen' ? OPENCODE_ZEN_PROTOCOL_RULES : OPENCODE_GO_PROTOCOL_RULES)
}

export function parseOpenCodeProtocolRules(raw: unknown, mode: OpenCodeAccountMode): OpenCodeProtocolRule[] {
  if (!Array.isArray(raw)) return defaultOpenCodeProtocolRules(mode)
  return raw.flatMap((item): OpenCodeProtocolRule[] => {
    if (!item || typeof item !== 'object') return []
    const pattern = typeof (item as Record<string, unknown>).pattern === 'string'
      ? ((item as Record<string, unknown>).pattern as string).trim()
      : ''
    const protocol = (item as Record<string, unknown>).protocol
    if (!pattern || !['chat_completions', 'anthropic', 'responses'].includes(String(protocol))) return []
    return [{ pattern, protocol: protocol as CNNativeAPIProtocol }]
  })
}

export function normalizeOpenCodeProtocolRules(rules: OpenCodeProtocolRule[]): OpenCodeProtocolRule[] {
  return rules.flatMap((rule): OpenCodeProtocolRule[] => {
    const pattern = rule.pattern.trim().toLowerCase()
    if (!pattern || !['chat_completions', 'anthropic', 'responses'].includes(rule.protocol)) return []
    return [{ pattern, protocol: rule.protocol }]
  })
}

export function updateOpenCodeProtocolRuleDefaults(
  current: OpenCodeProtocolRule[],
  previousMode: OpenCodeAccountMode,
  nextMode: OpenCodeAccountMode,
): OpenCodeProtocolRule[] {
  const normalized = normalizeOpenCodeProtocolRules(current)
  const previousDefaults = defaultOpenCodeProtocolRules(previousMode)
  return JSON.stringify(normalized) === JSON.stringify(previousDefaults)
    ? defaultOpenCodeProtocolRules(nextMode)
    : cloneOpenCodeProtocolRules(current)
}

export function defaultAPIKeyBaseURL(platform: AccountPlatform): string {
  switch (platform) {
    case 'openai': return 'https://api.openai.com'
    case 'gemini': return 'https://generativelanguage.googleapis.com'
    case 'antigravity': return 'https://cloudcode-pa.googleapis.com'
    case 'grok': return 'https://api.x.ai/v1'
    case 'kimi':
    case 'zhipu':
    case 'deepseek':
    case 'minimax': return defaultCNBaseURL(platform, 'payg')
    case 'opencode_go': return defaultCNBaseURL(platform, 'go')
    default: return 'https://api.anthropic.com'
  }
}

export function defaultAPIKeyPlaceholder(platform: AccountPlatform): string {
  switch (platform) {
    case 'anthropic': return 'sk-ant-...'
    case 'openai': return 'sk-proj-...'
    case 'gemini': return 'AIza...'
    case 'grok': return 'xai-...'
    case 'kimi':
    case 'deepseek':
    case 'antigravity': return 'sk-...'
    case 'opencode_go': return 'API Key'
    default: return 'API Key'
  }
}

type AccountHintTranslate = (key: string, params?: Record<string, string>) => string

export function accountAPIKeyFieldHint(
  t: AccountHintTranslate,
  platform: AccountPlatform,
  field: 'baseUrl' | 'apiKey',
): string {
  if (platform === 'openai') return t(`admin.accounts.openai.${field}Hint`)
  if (platform === 'gemini') return t(`admin.accounts.gemini.${field}Hint`)
  if (platform === 'grok') return ''
  if (isCNAccountPlatform(platform)) {
    return t(`admin.accounts.cnProvider.${field}Hint`, {
      provider: CN_ACCOUNT_PLATFORM_LABELS[platform],
    })
  }
  return t(`admin.accounts.${field}Hint`)
}

/** WebSearch emulation mode values (must match backend WebSearchMode* constants in account.go) */
export const WEB_SEARCH_MODE_DEFAULT = 'default' as const
export const WEB_SEARCH_MODE_ENABLED = 'enabled' as const
export const WEB_SEARCH_MODE_DISABLED = 'disabled' as const
export type WebSearchMode = typeof WEB_SEARCH_MODE_DEFAULT | typeof WEB_SEARCH_MODE_ENABLED | typeof WEB_SEARCH_MODE_DISABLED

/** Quota notification threshold type values (must match thresholdType* constants in balance_notify_service.go) */
export const QUOTA_THRESHOLD_TYPE_FIXED = 'fixed' as const
export const QUOTA_THRESHOLD_TYPE_PERCENTAGE = 'percentage' as const
export type QuotaThresholdType = typeof QUOTA_THRESHOLD_TYPE_FIXED | typeof QUOTA_THRESHOLD_TYPE_PERCENTAGE

/** Quota reset mode values */
export const QUOTA_RESET_MODE_ROLLING = 'rolling' as const
export const QUOTA_RESET_MODE_FIXED = 'fixed' as const
export type QuotaResetMode = typeof QUOTA_RESET_MODE_ROLLING | typeof QUOTA_RESET_MODE_FIXED

/** Vertex AI location options for Service Account accounts */
export const VERTEX_LOCATION_OPTIONS = [
  {
    label: 'Common',
    options: [
      { value: 'us-central1', label: 'us-central1 (Iowa)' },
      { value: 'global', label: 'global' },
      { value: 'us', label: 'us' },
      { value: 'eu', label: 'eu' }
    ]
  },
  {
    label: 'United States',
    options: [
      { value: 'us-east1', label: 'us-east1 (South Carolina)' },
      { value: 'us-east4', label: 'us-east4 (Northern Virginia)' },
      { value: 'us-east5', label: 'us-east5 (Columbus)' },
      { value: 'us-south1', label: 'us-south1 (Dallas)' },
      { value: 'us-west1', label: 'us-west1 (Oregon)' },
      { value: 'us-west4', label: 'us-west4 (Las Vegas)' }
    ]
  },
  {
    label: 'Europe',
    options: [
      { value: 'europe-west1', label: 'europe-west1 (Belgium)' },
      { value: 'europe-west2', label: 'europe-west2 (London)' },
      { value: 'europe-west3', label: 'europe-west3 (Frankfurt)' },
      { value: 'europe-west4', label: 'europe-west4 (Netherlands)' },
      { value: 'europe-west6', label: 'europe-west6 (Zurich)' },
      { value: 'europe-west8', label: 'europe-west8 (Milan)' },
      { value: 'europe-west9', label: 'europe-west9 (Paris)' }
    ]
  },
  {
    label: 'Asia Pacific',
    options: [
      { value: 'asia-east1', label: 'asia-east1 (Taiwan)' },
      { value: 'asia-east2', label: 'asia-east2 (Hong Kong)' },
      { value: 'asia-northeast1', label: 'asia-northeast1 (Tokyo)' },
      { value: 'asia-northeast3', label: 'asia-northeast3 (Seoul)' },
      { value: 'asia-south1', label: 'asia-south1 (Mumbai)' },
      { value: 'asia-southeast1', label: 'asia-southeast1 (Singapore)' },
      { value: 'australia-southeast1', label: 'australia-southeast1 (Sydney)' }
    ]
  }
] as const
