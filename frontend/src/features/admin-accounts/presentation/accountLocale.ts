import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const accountStatusKeys = {
  active: 'admin.accounts.status.active',
  inactive: 'admin.accounts.status.inactive',
  error: 'admin.accounts.status.error',
} as const

const ollamaCloudUsageStatusKeys = {
  ok: 'admin.accounts.ollamaCloud.ok',
  unauthorized: 'admin.accounts.ollamaCloud.unauthorized',
  failed: 'admin.accounts.ollamaCloud.failed',
} as const

const ollamaCloudUsageErrorKeys = {
  request_failed: 'admin.accounts.ollamaCloud.errors.request_failed',
  empty_response: 'admin.accounts.ollamaCloud.errors.empty_response',
  response_host_mismatch: 'admin.accounts.ollamaCloud.errors.response_host_mismatch',
  redirect_blocked: 'admin.accounts.ollamaCloud.errors.redirect_blocked',
  unauthorized: 'admin.accounts.ollamaCloud.errors.unauthorized',
  http_error: 'admin.accounts.ollamaCloud.errors.http_error',
  response_read_failed: 'admin.accounts.ollamaCloud.errors.response_read_failed',
  response_too_large: 'admin.accounts.ollamaCloud.errors.response_too_large',
  invalid_html: 'admin.accounts.ollamaCloud.errors.invalid_html',
  OLLAMA_CLOUD_USAGE_REFRESH_RATE_LIMITED: 'admin.accounts.ollamaCloud.refreshFailed',
} as const

export function accountStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, accountStatusKeys, value)
}

export function ollamaCloudUsageStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, ollamaCloudUsageStatusKeys, value)
}

export function ollamaCloudUsageErrorLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, ollamaCloudUsageErrorKeys, value, 'admin.accounts.ollamaCloud.refreshFailed')
}
