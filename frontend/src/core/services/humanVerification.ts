import type { PublicSettings } from '@/types'

export type ExternalHumanVerificationProvider = 'turnstile' | 'recaptcha' | 'cap' | 'tencent' | 'aliyun'
export type HumanVerificationProvider = 'none' | 'local' | ExternalHumanVerificationProvider
export type AliyunCaptchaRegion = 'cn' | 'sgp'
export type TencentCaptchaRegion = 'cn' | 'intl'

export interface HumanVerificationConfig {
  provider: HumanVerificationProvider
  externalProvider: ExternalHumanVerificationProvider
  external: boolean
  siteKey: string
  apiEndpoint: string
  tencentRegion: TencentCaptchaRegion
  aliyunSceneId: string
  aliyunPrefix: string
  aliyunRegion: AliyunCaptchaRegion
}

const emptyAliyunConfig = {
  aliyunSceneId: '',
  aliyunPrefix: '',
  aliyunRegion: 'cn' as AliyunCaptchaRegion
}

const emptyTencentConfig = {
  tencentRegion: 'cn' as TencentCaptchaRegion
}

export function resolveHumanVerification(settings: PublicSettings): HumanVerificationConfig {
  if (settings.turnstile_enabled) {
    return {
      provider: 'turnstile',
      externalProvider: 'turnstile',
      external: true,
      siteKey: settings.turnstile_site_key || '',
      apiEndpoint: '',
      ...emptyTencentConfig,
      ...emptyAliyunConfig
    }
  }
  if (settings.recaptcha_enabled) {
    return {
      provider: 'recaptcha',
      externalProvider: 'recaptcha',
      external: true,
      siteKey: settings.recaptcha_site_key || '',
      apiEndpoint: '',
      ...emptyTencentConfig,
      ...emptyAliyunConfig
    }
  }
  if (settings.cap_enabled) {
    return {
      provider: 'cap',
      externalProvider: 'cap',
      external: true,
      siteKey: '',
      apiEndpoint: settings.cap_api_endpoint || '',
      ...emptyTencentConfig,
      ...emptyAliyunConfig
    }
  }
  if (settings.tencent_captcha_enabled) {
    return {
      provider: 'tencent',
      externalProvider: 'tencent',
      external: true,
      siteKey: settings.tencent_captcha_app_id || '',
      apiEndpoint: '',
      tencentRegion: normalizeTencentCaptchaRegion(settings.tencent_captcha_region),
      ...emptyAliyunConfig
    }
  }
  if (settings.aliyun_captcha_enabled) {
    return {
      provider: 'aliyun',
      externalProvider: 'aliyun',
      external: true,
      siteKey: '',
      apiEndpoint: '',
      ...emptyTencentConfig,
      aliyunSceneId: settings.aliyun_captcha_scene_id || '',
      aliyunPrefix: settings.aliyun_captcha_prefix || '',
      aliyunRegion: settings.aliyun_captcha_region === 'sgp' ? 'sgp' : 'cn'
    }
  }
  if (settings.local_captcha_enabled) {
    return {
      provider: 'local',
      externalProvider: 'turnstile',
      external: false,
      siteKey: '',
      apiEndpoint: '',
      ...emptyTencentConfig,
      ...emptyAliyunConfig
    }
  }
  return {
    provider: 'none',
    externalProvider: 'turnstile',
    external: false,
    siteKey: '',
    apiEndpoint: '',
    ...emptyTencentConfig,
    ...emptyAliyunConfig
  }
}

export interface TencentCaptchaProof {
  ticket: string
  randstr: string
}

export interface TencentCaptchaResult {
  ret: number
  ticket?: string | null
  randstr?: string | null
  errorCode?: number
  errorMessage?: string
}

export interface TencentCaptchaInstance {
  show(): void
  destroy(): void
}

type TencentCaptchaCallback = (result: TencentCaptchaResult) => void

export type TencentCaptchaConstructor = {
  new (
    appId: string,
    callback: TencentCaptchaCallback,
    options?: Record<string, unknown>
  ): TencentCaptchaInstance
  new (
    element: HTMLElement,
    appId: string,
    callback: TencentCaptchaCallback,
    options?: Record<string, unknown>
  ): TencentCaptchaInstance
}

declare global {
  interface Window {
    TencentCaptcha?: TencentCaptchaConstructor
    TCaptchaGlobal?: boolean
  }
}

const tencentCaptchaScriptSrc: Record<TencentCaptchaRegion, string> = {
  cn: 'https://turing.captcha.qcloud.com/TJCaptcha.js',
  intl: 'https://ca.turing.captcha.qcloud.com/TJNCaptcha-global.js'
}
let tencentCaptchaLoadPromise: Promise<TencentCaptchaConstructor> | null = null
let tencentCaptchaLoadedRegion: TencentCaptchaRegion | null = null

export function normalizeTencentCaptchaRegion(value?: string | null): TencentCaptchaRegion {
  return value?.trim().toLowerCase() === 'intl' ? 'intl' : 'cn'
}

function existingTencentCaptchaRegion(): TencentCaptchaRegion {
  return window.TCaptchaGlobal === true ? 'intl' : 'cn'
}

export function loadTencentCaptcha(
  region: TencentCaptchaRegion = 'cn'
): Promise<TencentCaptchaConstructor> {
  const globalRegion = window.TencentCaptcha ? existingTencentCaptchaRegion() : null
  if (
    window.TencentCaptcha &&
    (tencentCaptchaLoadedRegion === region || globalRegion === region)
  ) {
    return Promise.resolve(window.TencentCaptcha)
  }
  if (tencentCaptchaLoadPromise && tencentCaptchaLoadedRegion === region) {
    return tencentCaptchaLoadPromise
  }

  tencentCaptchaLoadedRegion = region
  tencentCaptchaLoadPromise = new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = tencentCaptchaScriptSrc[region]
    script.async = true
    const fail = (message: string): void => {
      script.remove()
      tencentCaptchaLoadPromise = null
      tencentCaptchaLoadedRegion = null
      reject(new Error(message))
    }
    script.onload = () => {
      if (window.TencentCaptcha && existingTencentCaptchaRegion() === region) {
        resolve(window.TencentCaptcha)
        return
      }
      fail('Tencent Captcha SDK is unavailable')
    }
    script.onerror = () => fail('Failed to load Tencent Captcha SDK')
    document.head.appendChild(script)
  })

  return tencentCaptchaLoadPromise
}

export function resetTencentCaptchaLoaderForTest(): void {
  tencentCaptchaLoadPromise = null
  tencentCaptchaLoadedRegion = null
}

export interface AliyunCaptchaVerifyResult {
  captchaResult: boolean
  bizResult?: boolean
}

export interface AliyunCaptchaInstance {
  readonly config?: unknown
}

export interface AliyunCaptchaInitOptions {
  SceneId: string
  prefix: string
  mode: 'popup' | 'embed'
  element: string
  button: string
  captchaVerifyCallback: (captchaVerifyParam: string) => AliyunCaptchaVerifyResult | Promise<AliyunCaptchaVerifyResult>
  onBizResultCallback: (bizResult: boolean) => void
  getInstance: (instance: AliyunCaptchaInstance) => void
  slideStyle?: { width: number; height: number }
  language?: string
}

export type AliyunCaptchaInitializer = (options: AliyunCaptchaInitOptions) => void | Promise<void>

declare global {
  interface Window {
    initAliyunCaptcha?: AliyunCaptchaInitializer
    AliyunCaptchaConfig?: { region: AliyunCaptchaRegion; prefix: string }
  }
}

const aliyunCaptchaScriptSrc = 'https://o.alicdn.com/captcha-frontend/aliyunCaptcha/AliyunCaptcha.js'
const aliyunCaptchaLoadTimeoutMs = 15_000
let aliyunCaptchaLoadPromise: Promise<AliyunCaptchaInitializer> | null = null
let aliyunCaptchaConfigKey = ''

export function loadAliyunCaptcha(
  prefix: string,
  region: AliyunCaptchaRegion
): Promise<AliyunCaptchaInitializer> {
  const normalizedPrefix = prefix.trim()
  if (!normalizedPrefix) return Promise.reject(new Error('Aliyun Captcha prefix is required'))

  const normalizedRegion: AliyunCaptchaRegion = region === 'sgp' ? 'sgp' : 'cn'
  const configKey = `${normalizedRegion}:${normalizedPrefix}`
  if (aliyunCaptchaConfigKey && aliyunCaptchaConfigKey !== configKey) {
    return Promise.reject(new Error('Aliyun Captcha is already loaded with different settings'))
  }
  aliyunCaptchaConfigKey = configKey
  window.AliyunCaptchaConfig = { region: normalizedRegion, prefix: normalizedPrefix }

  if (window.initAliyunCaptcha) return Promise.resolve(window.initAliyunCaptcha)
  if (aliyunCaptchaLoadPromise) return aliyunCaptchaLoadPromise

  aliyunCaptchaLoadPromise = new Promise((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(
      `script[src="${aliyunCaptchaScriptSrc}"]`
    )
    const script = existing || document.createElement('script')
    let timeoutId: ReturnType<typeof setTimeout> | null = null

    const cleanup = (): void => {
      script.removeEventListener('load', handleLoad)
      script.removeEventListener('error', handleError)
      if (timeoutId) clearTimeout(timeoutId)
    }
    const fail = (message: string): void => {
      cleanup()
      if (!existing) script.remove()
      aliyunCaptchaLoadPromise = null
      aliyunCaptchaConfigKey = ''
      reject(new Error(message))
    }
    const handleLoad = (): void => {
      if (!window.initAliyunCaptcha) {
        fail('Aliyun Captcha SDK is unavailable')
        return
      }
      cleanup()
      resolve(window.initAliyunCaptcha)
    }
    const handleError = (): void => fail('Failed to load Aliyun Captcha SDK')

    script.addEventListener('load', handleLoad, { once: true })
    script.addEventListener('error', handleError, { once: true })
    timeoutId = setTimeout(
      () => fail('Timed out loading Aliyun Captcha SDK'),
      aliyunCaptchaLoadTimeoutMs
    )

    if (!existing) {
      script.src = aliyunCaptchaScriptSrc
      script.async = true
      document.head.appendChild(script)
    }
  })

  return aliyunCaptchaLoadPromise
}

export function resetAliyunCaptchaLoaderForTest(): void {
  aliyunCaptchaLoadPromise = null
  aliyunCaptchaConfigKey = ''
}
