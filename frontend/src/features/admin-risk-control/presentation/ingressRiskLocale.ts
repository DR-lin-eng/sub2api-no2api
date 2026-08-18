import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const reasonKeys = {
  query_api_key_deprecated: 'admin.ingressRisk.reasons.query_api_key_deprecated',
  api_key_required: 'admin.ingressRisk.reasons.api_key_required',
  invalid_api_key: 'admin.ingressRisk.reasons.invalid_api_key',
  invalid_auth_rate_limited: 'admin.ingressRisk.reasons.invalid_auth_rate_limited',
  api_key_auth_overloaded: 'admin.ingressRisk.reasons.api_key_auth_overloaded',
  api_key_disabled: 'admin.ingressRisk.reasons.api_key_disabled',
  ip_restricted: 'admin.ingressRisk.reasons.ip_restricted',
  user_inactive: 'admin.ingressRisk.reasons.user_inactive',
  group_deleted: 'admin.ingressRisk.reasons.group_deleted',
  group_disabled: 'admin.ingressRisk.reasons.group_disabled',
  group_not_allowed: 'admin.ingressRisk.reasons.group_not_allowed',
  group_unassigned: 'admin.ingressRisk.reasons.group_unassigned',
  other: 'admin.ingressRisk.reasons.other',
} as const

const routeKeys = {
  antigravity: 'admin.ingressRisk.routes.antigravity',
  gemini: 'admin.ingressRisk.routes.gemini',
  codex: 'admin.ingressRisk.routes.codex',
  messages: 'admin.ingressRisk.routes.messages',
  responses: 'admin.ingressRisk.routes.responses',
  chat_completions: 'admin.ingressRisk.routes.chat_completions',
  images: 'admin.ingressRisk.routes.images',
  videos: 'admin.ingressRisk.routes.videos',
  embeddings: 'admin.ingressRisk.routes.embeddings',
  models: 'admin.ingressRisk.routes.models',
  other: 'admin.ingressRisk.routes.other',
} as const

const protocolKeys = {
  google: 'admin.ingressRisk.protocols.google',
  anthropic: 'admin.ingressRisk.protocols.anthropic',
  openai: 'admin.ingressRisk.protocols.openai',
  gateway: 'admin.ingressRisk.protocols.gateway',
  other: 'admin.ingressRisk.protocols.other',
} as const

const cloudflareStatusKeys = {
  disabled: 'admin.ingressRisk.cloudflare.status.disabled',
  cleanup: 'admin.ingressRisk.cloudflare.status.cleanup',
  healthy: 'admin.ingressRisk.cloudflare.status.healthy',
  warning: 'admin.ingressRisk.cloudflare.status.warning',
  stopped: 'admin.ingressRisk.cloudflare.status.stopped',
  unknown: 'admin.ingressRisk.cloudflare.status.unknown',
} as const

const cloudflareDescriptionKeys = {
  disabled: 'admin.ingressRisk.cloudflare.description.disabled',
  cleanup: 'admin.ingressRisk.cloudflare.description.cleanup',
  healthy: 'admin.ingressRisk.cloudflare.description.healthy',
  warning: 'admin.ingressRisk.cloudflare.description.warning',
  stopped: 'admin.ingressRisk.cloudflare.description.stopped',
  unknown: 'admin.ingressRisk.cloudflare.description.unknown',
} as const

const cloudflareTokenKeys = {
  tokenConfigured: 'admin.ingressRisk.cloudflare.settings.tokenConfigured',
  tokenMissing: 'admin.ingressRisk.cloudflare.settings.tokenMissing',
} as const

export function ingressRiskReasonLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, reasonKeys, value)
}

export function ingressRiskRouteLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, routeKeys, value)
}

export function ingressRiskProtocolLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, protocolKeys, value)
}

export function cloudflareStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, cloudflareStatusKeys, value)
}

export function cloudflareStatusDescription(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, cloudflareDescriptionKeys, value)
}

export function cloudflareTokenStatusLabel(t: LocaleTranslate, configured: boolean): string {
  return enumLocaleLabel(t, cloudflareTokenKeys, configured ? 'tokenConfigured' : 'tokenMissing')
}
