import type { PublicSettings } from '@/types'

export type ExternalHumanVerificationProvider = 'turnstile' | 'recaptcha' | 'cap'
export type HumanVerificationProvider = 'none' | 'local' | ExternalHumanVerificationProvider

export interface HumanVerificationConfig {
  provider: HumanVerificationProvider
  externalProvider: ExternalHumanVerificationProvider
  external: boolean
  siteKey: string
  apiEndpoint: string
}

export function resolveHumanVerification(settings: PublicSettings): HumanVerificationConfig {
  if (settings.turnstile_enabled) {
    return {
      provider: 'turnstile',
      externalProvider: 'turnstile',
      external: true,
      siteKey: settings.turnstile_site_key || '',
      apiEndpoint: ''
    }
  }
  if (settings.recaptcha_enabled) {
    return {
      provider: 'recaptcha',
      externalProvider: 'recaptcha',
      external: true,
      siteKey: settings.recaptcha_site_key || '',
      apiEndpoint: ''
    }
  }
  if (settings.cap_enabled) {
    return {
      provider: 'cap',
      externalProvider: 'cap',
      external: true,
      siteKey: '',
      apiEndpoint: settings.cap_api_endpoint || ''
    }
  }
  if (settings.local_captcha_enabled) {
    return { provider: 'local', externalProvider: 'turnstile', external: false, siteKey: '', apiEndpoint: '' }
  }
  return { provider: 'none', externalProvider: 'turnstile', external: false, siteKey: '', apiEndpoint: '' }
}
