import type { AccountPlatform } from '@/types'
export { modelProviderPlatforms as channelPlatformOrder } from '@/core/constants/modelProviders'

export type ChannelPlatform = AccountPlatform
type ProviderOption = 'web_search_emulation' | 'codex_image_generation_bridge' | 'bedrock_cc_compat'

interface ChannelProvider {
  options?: readonly ProviderOption[]
  catalogSync: boolean
}

export const channelProviders: Record<ChannelPlatform, ChannelProvider> = {
  anthropic: { catalogSync: true, options: ['web_search_emulation', 'bedrock_cc_compat'] },
  openai: { catalogSync: true, options: ['web_search_emulation', 'codex_image_generation_bridge'] },
  gemini: { catalogSync: true },
  antigravity: { catalogSync: true },
  grok: { catalogSync: true },
  kimi: { catalogSync: true },
  zhipu: { catalogSync: true },
  deepseek: { catalogSync: true },
  minimax: { catalogSync: true },
  opencode_go: { catalogSync: true },
  typesafe: { catalogSync: true },
  multimodal: { catalogSync: false },
}

export function supportsChannelOption(platform: ChannelPlatform, option: ProviderOption): boolean {
  return channelProviders[platform].options?.includes(option) === true
}
