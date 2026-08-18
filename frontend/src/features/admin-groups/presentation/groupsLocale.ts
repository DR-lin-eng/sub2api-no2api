import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const platformKeys = {
  anthropic: 'admin.groups.platforms.anthropic',
  openai: 'admin.groups.platforms.openai',
  gemini: 'admin.groups.platforms.gemini',
  antigravity: 'admin.groups.platforms.antigravity',
  grok: 'admin.groups.platforms.grok',
  composite: 'admin.groups.platforms.composite',
} as const

const compositeMatchKeys = {
  exact: 'admin.groups.compositeRoutes.match.exact',
  prefix: 'admin.groups.compositeRoutes.match.prefix',
} as const

const compositeEndpointKeys = {
  any: 'admin.groups.compositeRoutes.endpoints.any',
  messages: 'admin.groups.compositeRoutes.endpoints.messages',
  count_tokens: 'admin.groups.compositeRoutes.endpoints.countTokens',
  responses: 'admin.groups.compositeRoutes.endpoints.responses',
  chat_completions: 'admin.groups.compositeRoutes.endpoints.chatCompletions',
  embeddings: 'admin.groups.compositeRoutes.endpoints.embeddings',
  images: 'admin.groups.compositeRoutes.endpoints.images',
  gemini: 'admin.groups.compositeRoutes.endpoints.gemini',
} as const

const compositeSourceKeys = {
  route: 'admin.groups.compositeRoutes.sources.route',
  detector: 'admin.groups.compositeRoutes.sources.detector',
} as const

const groupStatusKeys = {
  active: 'common.active',
  inactive: 'common.inactive',
} as const

export function groupPlatformLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, platformKeys, value)
}

export function compositeRouteMatchLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, compositeMatchKeys, value)
}

export function compositeRouteEndpointLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, compositeEndpointKeys, value)
}

export function compositeRouteSourceLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, compositeSourceKeys, value)
}

export function groupStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, groupStatusKeys, value)
}
