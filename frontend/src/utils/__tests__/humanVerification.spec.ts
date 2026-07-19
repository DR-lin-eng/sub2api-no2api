import { describe, expect, it } from 'vitest'
import { resolveHumanVerification } from '@/utils/humanVerification'
import type { PublicSettings } from '@/types'

function settings(overrides: Partial<PublicSettings>): PublicSettings {
  return {
    turnstile_enabled: false,
    turnstile_site_key: '',
    recaptcha_enabled: false,
    recaptcha_site_key: '',
    cap_enabled: false,
    cap_api_endpoint: '',
    local_captcha_enabled: false,
    ...overrides
  } as PublicSettings
}

describe('resolveHumanVerification', () => {
  it.each([
    [settings({ turnstile_enabled: true, turnstile_site_key: 'cf-site' }), 'turnstile', 'cf-site', ''],
    [settings({ recaptcha_enabled: true, recaptcha_site_key: 'google-site' }), 'recaptcha', 'google-site', ''],
    [settings({ cap_enabled: true, cap_api_endpoint: 'https://cap.example/site' }), 'cap', '', 'https://cap.example/site'],
    [settings({ local_captcha_enabled: true }), 'local', '', '']
  ])('selects the configured provider', (publicSettings, provider, siteKey, apiEndpoint) => {
    expect(resolveHumanVerification(publicSettings)).toMatchObject({
      provider,
      siteKey,
      apiEndpoint
    })
  })

  it('keeps legacy Turnstile priority over the old local fallback combination', () => {
    expect(resolveHumanVerification(settings({
      turnstile_enabled: true,
      turnstile_site_key: 'cf-site',
      local_captcha_enabled: true
    })).provider).toBe('turnstile')
  })
})
