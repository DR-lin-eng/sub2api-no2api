import type { AccountPlatform } from '@/types'
import {
  cnSupportsNativeResponses,
  defaultCNAdaptiveBaseURLs,
  isCNAccountPlatform,
  normalizeOpenCodeProtocolRules,
  type CNAPIProtocol,
  type CNAdaptiveBaseURLs,
  type OpenCodeProtocolRule,
} from '@/core/constants/account'

interface CreateCNProviderCredentialInput {
  platform: AccountPlatform
  cnAccountMode: 'payg' | 'coding'
  cnAPIProtocol: CNAPIProtocol
  adaptiveBaseURLs: CNAdaptiveBaseURLs
  openCodeAccountMode: 'zen' | 'go'
  openCodeProtocolRules: OpenCodeProtocolRule[]
  zhipuOrganization: string
  zhipuProject: string
}

function adaptiveBaseURLs(input: CreateCNProviderCredentialInput): Record<string, string> {
  const mode = input.platform === 'opencode_go' ? input.openCodeAccountMode : input.cnAccountMode
  const defaults = defaultCNAdaptiveBaseURLs(
    input.platform as 'kimi' | 'zhipu' | 'deepseek' | 'minimax' | 'opencode_go',
    mode,
  )
  const urls: Record<string, string> = {
    chat_completions: input.adaptiveBaseURLs.chat_completions.trim() || defaults.chat_completions,
    anthropic: input.adaptiveBaseURLs.anthropic.trim() || defaults.anthropic,
  }
  if (cnSupportsNativeResponses(input.platform)) {
    urls.responses = input.adaptiveBaseURLs.responses.trim() || defaults.responses
  }
  return urls
}

export function applyCreateCNProviderCredentials(
  credentials: Record<string, unknown>,
  input: CreateCNProviderCredentialInput,
): void {
  if (isCNAccountPlatform(input.platform)) {
    credentials.account_mode = input.cnAccountMode
    credentials.api_protocol = input.cnAPIProtocol
    if (input.cnAPIProtocol === 'adaptive') {
      const urls = adaptiveBaseURLs(input)
      credentials.api_base_urls = urls
      credentials.base_url = urls.chat_completions
    }
    if (input.platform === 'zhipu' && input.cnAccountMode === 'coding') {
      if (input.zhipuOrganization.trim()) credentials.zhipu_organization = input.zhipuOrganization.trim()
      if (input.zhipuProject.trim()) credentials.zhipu_project = input.zhipuProject.trim()
    }
    return
  }

  if (input.platform === 'opencode_go') {
    credentials.account_mode = input.openCodeAccountMode
    credentials.api_protocol = 'adaptive'
    const urls = adaptiveBaseURLs(input)
    credentials.api_base_urls = urls
    credentials.base_url = urls.chat_completions
    credentials.protocol_rules = normalizeOpenCodeProtocolRules(input.openCodeProtocolRules)
  }
}
