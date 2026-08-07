import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import {
  loadAliyunCaptcha,
  loadTencentCaptcha,
  resetAliyunCaptchaLoaderForTest,
  resetTencentCaptchaLoaderForTest,
  resolveHumanVerification,
  type AliyunCaptchaInitializer,
  type TencentCaptchaConstructor
} from '@/core/services/humanVerification'
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
    [settings({ tencent_captcha_enabled: true, tencent_captcha_app_id: '123456789' }), 'tencent', '123456789', ''],
    [settings({
      aliyun_captcha_enabled: true,
      aliyun_captcha_scene_id: 'scene-id',
      aliyun_captcha_prefix: 'prefix-id',
      aliyun_captcha_region: 'sgp'
    }), 'aliyun', '', ''],
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

  it('normalizes Aliyun settings for the active region', () => {
    expect(resolveHumanVerification(settings({
      aliyun_captcha_enabled: true,
      aliyun_captcha_scene_id: 'scene-id',
      aliyun_captcha_prefix: 'prefix-id',
      aliyun_captcha_region: 'sgp'
    }))).toMatchObject({
      provider: 'aliyun',
      aliyunSceneId: 'scene-id',
      aliyunPrefix: 'prefix-id',
      aliyunRegion: 'sgp'
    })
  })

  it('normalizes the Tencent service site and defaults legacy settings to China', () => {
    expect(resolveHumanVerification(settings({
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: '123456789',
      tencent_captcha_region: 'INTL'
    })).tencentRegion).toBe('intl')
    expect(resolveHumanVerification(settings({
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: '123456789'
    })).tencentRegion).toBe('cn')
  })
})

describe('loadTencentCaptcha', () => {
  const scriptSelector = 'script[src="https://turing.captcha.qcloud.com/TJCaptcha.js"]'
  const internationalScriptSelector =
    'script[src="https://ca.turing.captcha.qcloud.com/TJNCaptcha-global.js"]'

  beforeEach(() => {
    resetTencentCaptchaLoaderForTest()
    delete window.TencentCaptcha
    delete window.TCaptchaGlobal
    document.querySelectorAll(`${scriptSelector}, ${internationalScriptSelector}`).forEach(element => element.remove())
  })

  afterEach(() => {
    resetTencentCaptchaLoaderForTest()
    delete window.TencentCaptcha
    delete window.TCaptchaGlobal
    document.querySelectorAll(`${scriptSelector}, ${internationalScriptSelector}`).forEach(element => element.remove())
  })

  it('singleflights concurrent SDK loads', async () => {
    const first = loadTencentCaptcha()
    const second = loadTencentCaptcha()

    expect(first).toBe(second)
    expect(document.querySelectorAll(scriptSelector)).toHaveLength(1)

    class TencentCaptchaMock {
      show() {}
      destroy() {}
    }
    window.TencentCaptcha = TencentCaptchaMock as unknown as TencentCaptchaConstructor
    document.querySelector<HTMLScriptElement>(scriptSelector)?.dispatchEvent(new Event('load'))

    await expect(first).resolves.toBe(window.TencentCaptcha)
    await expect(second).resolves.toBe(window.TencentCaptcha)
  })

  it('allows retry after a script load failure', async () => {
    const first = loadTencentCaptcha()
    document.querySelector<HTMLScriptElement>(scriptSelector)?.dispatchEvent(new Event('error'))
    await expect(first).rejects.toThrow('Failed to load Tencent Captcha SDK')

    const second = loadTencentCaptcha()
    expect(second).not.toBe(first)
    expect(document.querySelectorAll(scriptSelector)).toHaveLength(1)
  })

  it('loads the international SDK only for an international app', async () => {
    const pending = loadTencentCaptcha('intl')
    expect(document.querySelectorAll(internationalScriptSelector)).toHaveLength(1)
    expect(document.querySelectorAll(scriptSelector)).toHaveLength(0)

    class TencentCaptchaMock {
      show() {}
      destroy() {}
    }
    window.TCaptchaGlobal = true
    window.TencentCaptcha = TencentCaptchaMock as unknown as TencentCaptchaConstructor
    document.querySelector<HTMLScriptElement>(internationalScriptSelector)?.dispatchEvent(new Event('load'))

    await expect(pending).resolves.toBe(window.TencentCaptcha)
  })
})

describe('loadAliyunCaptcha', () => {
  const scriptSelector =
    'script[src="https://o.alicdn.com/captcha-frontend/aliyunCaptcha/AliyunCaptcha.js"]'

  beforeEach(() => {
    resetAliyunCaptchaLoaderForTest()
    delete window.initAliyunCaptcha
    delete window.AliyunCaptchaConfig
    document.querySelectorAll(scriptSelector).forEach(element => element.remove())
  })

  afterEach(() => {
    resetAliyunCaptchaLoaderForTest()
    delete window.initAliyunCaptcha
    delete window.AliyunCaptchaConfig
    document.querySelectorAll(scriptSelector).forEach(element => element.remove())
  })

  it('singleflights concurrent SDK loads with one global configuration', async () => {
    const first = loadAliyunCaptcha(' prefix-id ', 'sgp')
    const second = loadAliyunCaptcha('prefix-id', 'sgp')

    expect(first).toBe(second)
    expect(document.querySelectorAll(scriptSelector)).toHaveLength(1)
    expect(window.AliyunCaptchaConfig).toEqual({ region: 'sgp', prefix: 'prefix-id' })

    const initializer = (() => {}) as AliyunCaptchaInitializer
    window.initAliyunCaptcha = initializer
    document.querySelector<HTMLScriptElement>(scriptSelector)?.dispatchEvent(new Event('load'))

    await expect(first).resolves.toBe(initializer)
    await expect(second).resolves.toBe(initializer)
  })

  it('rejects conflicting settings after the shared SDK configuration is frozen', async () => {
    const first = loadAliyunCaptcha('prefix-id', 'cn')
    await expect(loadAliyunCaptcha('other-prefix', 'cn')).rejects.toThrow(
      'already loaded with different settings'
    )

    window.initAliyunCaptcha = (() => {}) as AliyunCaptchaInitializer
    document.querySelector<HTMLScriptElement>(scriptSelector)?.dispatchEvent(new Event('load'))
    await expect(first).resolves.toBe(window.initAliyunCaptcha)
  })

  it('allows retry after a script load failure', async () => {
    const first = loadAliyunCaptcha('prefix-id', 'cn')
    document.querySelector<HTMLScriptElement>(scriptSelector)?.dispatchEvent(new Event('error'))
    await expect(first).rejects.toThrow('Failed to load Aliyun Captcha SDK')

    const second = loadAliyunCaptcha('prefix-id', 'cn')
    expect(second).not.toBe(first)
    expect(document.querySelectorAll(scriptSelector)).toHaveLength(1)

    window.initAliyunCaptcha = (() => {}) as AliyunCaptchaInitializer
    document.querySelector<HTMLScriptElement>(scriptSelector)?.dispatchEvent(new Event('load'))
    await expect(second).resolves.toBe(window.initAliyunCaptcha)
  })
})
