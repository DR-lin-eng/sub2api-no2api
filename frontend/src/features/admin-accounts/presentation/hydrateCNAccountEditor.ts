import type { AccountPlatform } from '@/types'
import {
  defaultCNAdaptiveBaseURLs,
  defaultCNBaseURL,
  isCNAccountPlatform,
  parseOpenCodeProtocolRules,
  type CNAPIProtocol,
  type CNAdaptiveBaseURLs,
  type OpenCodeAccountMode,
  type OpenCodeProtocolRule,
} from '@/core/constants/account'

interface HydratedCNAccountEditor {
  accountMode: 'payg' | 'coding'
  adaptiveBaseURLs: CNAdaptiveBaseURLs
  baseURL: string
  openCodeAccountMode: OpenCodeAccountMode
  openCodeProtocolRules: OpenCodeProtocolRule[]
  protocol: CNAPIProtocol
  zhipuOrganization: string
  zhipuProject: string
}

export function hydrateCNAccountEditor(
  platform: AccountPlatform,
  credentials: Record<string, unknown>,
): HydratedCNAccountEditor | null {
  if (!isCNAccountPlatform(platform) && platform !== 'opencode_go') return null

  const accountMode = credentials.account_mode === 'coding' && platform !== 'deepseek' ? 'coding' : 'payg'
  const openCodeAccountMode: OpenCodeAccountMode = credentials.account_mode === 'zen' ? 'zen' : 'go'
  const storedProtocol = String(credentials.api_protocol || '')
  const candidate = ['adaptive', 'chat_completions', 'anthropic', 'responses'].includes(storedProtocol)
    ? storedProtocol as CNAPIProtocol
    : platform === 'opencode_go' ? 'adaptive' : 'chat_completions'
  const protocol = platform === 'zhipu' && candidate === 'responses' ? 'chat_completions' : candidate
  const mode = platform === 'opencode_go' ? openCodeAccountMode : accountMode
  const defaults = defaultCNAdaptiveBaseURLs(platform, mode)
  const rawBaseURLs = credentials.api_base_urls && typeof credentials.api_base_urls === 'object'
    ? credentials.api_base_urls as Record<string, unknown>
    : {}
  const legacyBaseURL = typeof credentials.base_url === 'string' ? credentials.base_url.trim() : ''
  const stored = (key: keyof CNAdaptiveBaseURLs) =>
    typeof rawBaseURLs[key] === 'string' ? (rawBaseURLs[key] as string).trim() : ''
  const adaptiveBaseURLs: CNAdaptiveBaseURLs = {
    chat_completions: stored('chat_completions') || (protocol === 'adaptive' && legacyBaseURL) || defaults.chat_completions,
    anthropic: stored('anthropic') || (protocol === 'anthropic' && legacyBaseURL) || defaults.anthropic,
    responses: stored('responses') || (protocol === 'responses' && legacyBaseURL) || defaults.responses,
  }

  return {
    accountMode,
    adaptiveBaseURLs,
    baseURL: protocol === 'adaptive'
      ? adaptiveBaseURLs.chat_completions
      : legacyBaseURL || defaultCNBaseURL(platform, mode, protocol),
    openCodeAccountMode,
    openCodeProtocolRules: parseOpenCodeProtocolRules(credentials.protocol_rules, openCodeAccountMode),
    protocol,
    zhipuOrganization: typeof credentials.zhipu_organization === 'string' ? credentials.zhipu_organization : '',
    zhipuProject: typeof credentials.zhipu_project === 'string' ? credentials.zhipu_project : '',
  }
}
