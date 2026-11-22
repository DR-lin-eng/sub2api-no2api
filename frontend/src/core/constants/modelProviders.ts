import type { AccountPlatform } from '@/types'
import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

export type ProviderCategory = 'vendors' | 'platforms'

export const providerCategories: Record<AccountPlatform, ProviderCategory> = {
  anthropic: 'vendors',
  openai: 'vendors',
  gemini: 'vendors',
  antigravity: 'platforms',
  grok: 'vendors',
  kimi: 'vendors',
  zhipu: 'vendors',
  deepseek: 'vendors',
  minimax: 'vendors',
  opencode_go: 'platforms',
  typesafe: 'vendors',
  multimodal: 'platforms',
}

export const modelProviderPlatforms = Object.keys(providerCategories) as AccountPlatform[]

const platformNameKeys = {
  anthropic: 'admin.groups.platforms.anthropic',
  openai: 'admin.groups.platforms.openai',
  gemini: 'admin.groups.platforms.gemini',
  antigravity: 'admin.groups.platforms.antigravity',
  grok: 'admin.groups.platforms.grok',
  kimi: 'admin.groups.platforms.kimi',
  zhipu: 'admin.groups.platforms.zhipu',
  deepseek: 'admin.groups.platforms.deepseek',
  minimax: 'admin.groups.platforms.minimax',
  opencode_go: 'admin.groups.platforms.opencode_go',
  typesafe: 'admin.groups.platforms.typesafe',
  multimodal: 'admin.groups.platforms.multimodal',
  composite: 'admin.groups.platforms.composite',
} as const

export function modelProviderLabel(t: LocaleTranslate, platform: unknown): string {
  return enumLocaleLabel(t, platformNameKeys, platform)
}
