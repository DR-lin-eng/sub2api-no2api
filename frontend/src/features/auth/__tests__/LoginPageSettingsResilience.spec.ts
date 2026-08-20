import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import LoginPage from '@/features/auth/presentation/pages/LoginPage.vue'

const {
  fetchPublicSettingsMock,
  showErrorMock,
  showSuccessMock,
  showWarningMock,
  prefetchCredentialKeyMock,
  authStore,
  appStore,
} = vi.hoisted(() => {
  const fetchPublicSettingsMock = vi.fn()
  const showErrorMock = vi.fn()
  const showSuccessMock = vi.fn()
  const showWarningMock = vi.fn()
  const prefetchCredentialKeyMock = vi.fn().mockResolvedValue(undefined)
  const authStore = {
    login: vi.fn(),
    login2FA: vi.fn(),
    loginWithPasskey: vi.fn(),
  }
  const appStore = {
    cachedPublicSettings: null as Record<string, unknown> | null,
    fetchPublicSettings: (...args: unknown[]) => fetchPublicSettingsMock(...args),
    showError: (...args: unknown[]) => showErrorMock(...args),
    showSuccess: (...args: unknown[]) => showSuccessMock(...args),
    showWarning: (...args: unknown[]) => showWarningMock(...args),
  }
  return {
    fetchPublicSettingsMock,
    showErrorMock,
    showSuccessMock,
    showWarningMock,
    prefetchCredentialKeyMock,
    authStore,
    appStore,
  }
})

vi.mock('vue-router', () => ({
  useRouter: () => ({
    currentRoute: { value: { query: {} } },
    push: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('@/features/auth/presentation/stores/authStore', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => appStore,
}))

vi.mock('@/features/auth/data/datasources/authOAuthActions', () => ({
  buildOAuthLoginStartURL: vi.fn(() => '/oauth/start'),
  isWeChatWebOAuthEnabled: vi.fn(() => false),
  startOAuthLogin: vi.fn(),
}))

vi.mock('@/features/auth/data/datasources/authSessionActions', () => ({
  clearCredentialKeyPrefetch: vi.fn(),
  isTotp2FARequired: vi.fn(() => false),
  prefetchCredentialKey: (...args: unknown[]) => prefetchCredentialKeyMock(...args),
}))

vi.mock('@/core/utils/apiError', () => ({
  extractI18nErrorMessage: vi.fn((_error, _t, _scope, fallback) => fallback),
}))

vi.mock('@/core/utils/oauthAffiliate', () => ({
  clearAllAffiliateReferralCodes: vi.fn(),
}))

vi.mock('@/core/services/humanVerification', () => ({
  resolveHumanVerification: vi.fn(() => ({
    provider: 'none',
    externalProvider: 'turnstile',
    external: false,
    siteKey: '',
    apiEndpoint: '',
    tencentRegion: 'cn',
    aliyunSceneId: '',
    aliyunPrefix: '',
    aliyunRegion: 'cn',
  })),
}))

const publicSettings = {
  registration_enabled: true,
  email_verify_enabled: false,
  force_email_on_third_party_signup: false,
  registration_email_suffix_whitelist: [],
  promo_code_enabled: false,
  password_reset_enabled: true,
  invitation_code_enabled: false,
  login_agreement_enabled: false,
  login_agreement_documents: [],
  turnstile_enabled: false,
  turnstile_site_key: '',
  recaptcha_enabled: false,
  recaptcha_site_key: '',
  cap_enabled: false,
  cap_api_endpoint: '',
  local_captcha_enabled: false,
  site_name: 'Sub2API',
  site_logo: '',
  site_subtitle: '',
  api_base_url: '',
  contact_info: '',
  doc_url: '',
  linuxdo_oauth_enabled: false,
  dingtalk_oauth_enabled: false,
  wechat_oauth_enabled: false,
  oidc_oauth_enabled: false,
  oidc_oauth_provider_name: 'OIDC',
  github_oauth_enabled: false,
  google_oauth_enabled: false,
  backend_mode_enabled: false,
  passkey_enabled: false,
  version: '0.1.186',
}

function mountLogin() {
  return mount(LoginPage, {
    global: {
      stubs: {
        AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: { template: '<span />' },
        LinuxDoOAuthSection: true,
        DingTalkOAuthSection: true,
        OidcOAuthSection: true,
        WechatOAuthSection: true,
        EmailOAuthButtons: true,
        LoginAgreementPrompt: true,
        TotpLoginModal: true,
        LocalCaptchaWidget: true,
        HumanVerificationWidget: true,
        RouterLink: { template: '<a><slot /></a>' },
      },
    },
  })
}

describe('LoginPage public-settings resilience', () => {
  const originalLocation = window.location

  beforeEach(() => {
    fetchPublicSettingsMock.mockReset()
    showErrorMock.mockReset()
    showSuccessMock.mockReset()
    showWarningMock.mockReset()
    prefetchCredentialKeyMock.mockReset()
    prefetchCredentialKeyMock.mockResolvedValue(undefined)
    authStore.login.mockReset()
    authStore.login2FA.mockReset()
    authStore.loginWithPasskey.mockReset()
    appStore.cachedPublicSettings = null
    localStorage.clear()
    sessionStorage.clear()
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    })
    vi.restoreAllMocks()
  })

  it('uses the injected/cache snapshot and enables the form without a second direct query', async () => {
    appStore.cachedPublicSettings = publicSettings
    fetchPublicSettingsMock.mockResolvedValue(publicSettings)

    const wrapper = mountLogin()
    await flushPromises()

    expect(fetchPublicSettingsMock).toHaveBeenCalledWith(false)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.get('#email').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('#password').attributes('disabled')).toBeUndefined()
  })

  it('keeps credentials disabled and exposes a retry when no settings snapshot is available', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    fetchPublicSettingsMock
      .mockResolvedValueOnce(null)
      .mockImplementationOnce(async () => {
        appStore.cachedPublicSettings = publicSettings
        return publicSettings
      })

    const wrapper = mountLogin()
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toContain('auth.settingsLoadFailed')
    expect(wrapper.get('#email').attributes('disabled')).toBeDefined()

    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()

    expect(fetchPublicSettingsMock).toHaveBeenNthCalledWith(2, true)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.get('#email').attributes('disabled')).toBeUndefined()
  })

  it('does not start auth requests inside an opaque sandbox and offers a top-level URL', async () => {
    Object.defineProperty(window, 'location', {
      value: {
        origin: 'null',
        href: 'https://gptcodex.top/login?ui_mode=embedded&src_host=https%3A%2F%2Fgptcodex.top',
      },
      writable: true,
      configurable: true,
    })

    const wrapper = mountLogin()
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toContain('auth.embeddedLoginTitle')
    expect(wrapper.get('a').attributes('href')).toBe('https://gptcodex.top/login')
    expect(fetchPublicSettingsMock).not.toHaveBeenCalled()
    expect(prefetchCredentialKeyMock).not.toHaveBeenCalled()
  })
})
