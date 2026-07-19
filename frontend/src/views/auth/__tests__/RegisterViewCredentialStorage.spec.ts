import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import RegisterView from '@/views/auth/RegisterView.vue'
import {
  clearPendingRegistrationCredentials,
  getPendingRegistrationCredentials
} from '@/utils/pendingRegistrationCredentials'

const {
  pushMock,
  getPublicSettingsMock,
  prefetchCredentialKeyMock,
  clearCredentialKeyPrefetchMock,
  registerMock,
  showErrorMock,
} = vi.hoisted(() => ({
  pushMock: vi.fn(),
  getPublicSettingsMock: vi.fn(),
  prefetchCredentialKeyMock: vi.fn(),
  clearCredentialKeyPrefetchMock: vi.fn(),
  registerMock: vi.fn(),
  showErrorMock: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushMock }),
  useRoute: () => ({ query: {} }),
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: { t: (key: string) => key },
  }),
  useI18n: () => ({
    t: (key: string) => key,
    locale: { value: 'en' },
  }),
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({ register: registerMock }),
  useAppStore: () => ({
    showError: showErrorMock,
    showSuccess: vi.fn(),
    showWarning: vi.fn(),
  }),
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getPublicSettings: (...args: any[]) => getPublicSettingsMock(...args),
    prefetchCredentialKey: (...args: any[]) => prefetchCredentialKeyMock(...args),
    clearCredentialKeyPrefetch: (...args: any[]) => clearCredentialKeyPrefetchMock(...args),
  }
})

describe('RegisterView credential storage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    sessionStorage.clear()
    localStorage.clear()
    clearPendingRegistrationCredentials()
    pushMock.mockResolvedValue(undefined)
    prefetchCredentialKeyMock.mockResolvedValue(undefined)
    getPublicSettingsMock.mockResolvedValue({
      registration_enabled: true,
      email_verify_enabled: true,
      promo_code_enabled: false,
      invitation_code_enabled: false,
      turnstile_enabled: false,
      local_captcha_enabled: false,
      linuxdo_oauth_enabled: false,
      wechat_oauth_enabled: false,
      oidc_oauth_enabled: false,
      github_oauth_enabled: false,
      google_oauth_enabled: false,
      registration_email_suffix_whitelist: [],
      login_agreement_enabled: false,
      login_agreement_documents: [],
    })
  })

  it('stores only non-secret metadata before navigating to email verification', async () => {
    const wrapper = mount(RegisterView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          RouterLink: true,
          TurnstileWidget: true,
          LocalCaptchaWidget: true,
          LoginAgreementPrompt: true,
          transition: false,
        },
      },
    })

    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(pushMock).toHaveBeenCalledWith('/email-verify')
    expect(registerMock).not.toHaveBeenCalled()
    expect(sessionStorage.getItem('register_data')).toBe(JSON.stringify({
      email: 'user@example.com',
      captcha_token: '',
    }))
    expect(sessionStorage.getItem('register_data')).not.toContain('secret-123')
    expect(sessionStorage.getItem('register_data')).not.toContain('credential_envelope')
    expect(getPendingRegistrationCredentials()).toEqual({
      email: 'user@example.com',
      password: 'secret-123',
    })
  })
})
