import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import EmailVerifyView from '@/features/auth/presentation/pages/EmailVerifyPage.vue'
import {
  clearPendingRegistrationCredentials,
  setPendingRegistrationCredentials
} from '@/core/utils/pendingRegistrationCredentials'

const {
  pushMock,
  showSuccessMock,
  showErrorMock,
  registerMock,
  setTokenMock,
  setPendingAuthSessionMock,
  clearPendingAuthSessionMock,
  getPublicSettingsMock,
  sendVerifyCodeMock,
  sendPendingOAuthVerifyCodeMock,
  persistOAuthTokenContextMock,
  createCredentialEnvelopeMock,
  clearCredentialKeyPrefetchMock,
  prefetchCredentialKeyMock,
  apiClientPostMock,
  authStoreState,
} = vi.hoisted(() => ({
  pushMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn(),
  registerMock: vi.fn(),
  setTokenMock: vi.fn(),
  setPendingAuthSessionMock: vi.fn(),
  clearPendingAuthSessionMock: vi.fn(),
  getPublicSettingsMock: vi.fn(),
  sendVerifyCodeMock: vi.fn(),
  sendPendingOAuthVerifyCodeMock: vi.fn(),
  persistOAuthTokenContextMock: vi.fn(),
  createCredentialEnvelopeMock: vi.fn(),
  clearCredentialKeyPrefetchMock: vi.fn(),
  prefetchCredentialKeyMock: vi.fn(),
  apiClientPostMock: vi.fn(),
  authStoreState: {
    pendingAuthSession: null as null | {
      token: string
      token_field: 'pending_auth_token' | 'pending_oauth_token'
      provider: string
      redirect?: string
      adoption_required?: boolean
      suggested_display_name?: string
      suggested_avatar_url?: string
    }
  },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: pushMock,
  }),
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      t: (key: string) => key,
    },
  }),
  useI18n: () => ({
    t: (key: string, params?: Record<string, string | number>) => {
      if (key === 'auth.accountCreatedSuccess') {
        return `Account created for ${params?.siteName ?? 'Sub2API'}`
      }
      return key
    },
    locale: { value: 'en' },
  }),
}))

vi.mock('@/features/auth', () => ({
  useAuthStore: () => ({
    pendingAuthSession: authStoreState.pendingAuthSession,
    register: (...args: any[]) => registerMock(...args),
    setToken: (...args: any[]) => setTokenMock(...args),
    setPendingAuthSession: (...args: any[]) => setPendingAuthSessionMock(...args),
    clearPendingAuthSession: (...args: any[]) => clearPendingAuthSessionMock(...args),
  }),
}))

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({
    showSuccess: (...args: any[]) => showSuccessMock(...args),
    showError: (...args: any[]) => showErrorMock(...args),
  }),
}))

vi.mock('@/features/auth/data/datasources/authQueries', async () => {
  const actual = await vi.importActual<typeof import('@/features/auth/data/datasources/authQueries')>('@/features/auth/data/datasources/authQueries')
  return {
    ...actual,
    getPublicSettings: (...args: any[]) => getPublicSettingsMock(...args),
  }
})

vi.mock('@/features/auth/data/datasources/authVerificationActions', async () => {
  const actual = await vi.importActual<typeof import('@/features/auth/data/datasources/authVerificationActions')>('@/features/auth/data/datasources/authVerificationActions')
  return {
    ...actual,
    sendVerifyCode: (...args: any[]) => sendVerifyCodeMock(...args),
  }
})

vi.mock('@/features/auth/data/datasources/authOAuthActions', async () => {
  const actual = await vi.importActual<typeof import('@/features/auth/data/datasources/authOAuthActions')>('@/features/auth/data/datasources/authOAuthActions')
  return {
    ...actual,
    sendPendingOAuthVerifyCode: (...args: any[]) => sendPendingOAuthVerifyCodeMock(...args),
    persistOAuthTokenContext: (...args: any[]) => persistOAuthTokenContextMock(...args),
  }
})

vi.mock('@/features/auth/data/datasources/authSessionActions', async () => {
  const actual = await vi.importActual<typeof import('@/features/auth/data/datasources/authSessionActions')>('@/features/auth/data/datasources/authSessionActions')
  return {
    ...actual,
    createCredentialEnvelope: (...args: any[]) => createCredentialEnvelopeMock(...args),
    clearCredentialKeyPrefetch: (...args: any[]) => clearCredentialKeyPrefetchMock(...args),
    prefetchCredentialKey: (...args: any[]) => prefetchCredentialKeyMock(...args),
  }
})

vi.mock('@/core/networks/client', () => ({
  apiClient: {
    post: (...args: any[]) => apiClientPostMock(...args),
  },
}))

describe('EmailVerifyView', () => {
  function seedRegisterData(data: Record<string, unknown> & { email: string; password?: string }): void {
    const { password = 'secret-123', ...metadata } = data
    sessionStorage.setItem('register_data', JSON.stringify(metadata))
    setPendingRegistrationCredentials(data.email, password)
  }

  beforeEach(() => {
    pushMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    registerMock.mockReset()
    setTokenMock.mockReset()
    setPendingAuthSessionMock.mockReset()
    clearPendingAuthSessionMock.mockReset()
    getPublicSettingsMock.mockReset()
    sendVerifyCodeMock.mockReset()
    sendPendingOAuthVerifyCodeMock.mockReset()
    persistOAuthTokenContextMock.mockReset()
    createCredentialEnvelopeMock.mockReset()
    clearCredentialKeyPrefetchMock.mockReset()
    prefetchCredentialKeyMock.mockReset()
    apiClientPostMock.mockReset()
    authStoreState.pendingAuthSession = null
    sessionStorage.clear()
    clearPendingRegistrationCredentials()
    localStorage.clear()

    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
    })
    sendVerifyCodeMock.mockResolvedValue({ countdown: 60 })
    sendPendingOAuthVerifyCodeMock.mockResolvedValue({ countdown: 60 })
    setTokenMock.mockResolvedValue({})
    prefetchCredentialKeyMock.mockResolvedValue(undefined)
    createCredentialEnvelopeMock.mockResolvedValue({
      algorithm: 'RSA-OAEP-256+A256GCM',
      key_id: 'test-key',
      encrypted_key: 'encrypted-key',
      iv: 'random-iv',
      ciphertext: 'encrypted-credentials',
    })
  })

  it('uses the pending oauth verify-code endpoint when register data carries a pending auth session', async () => {
    authStoreState.pendingAuthSession = {
      token: 'pending-token-1',
      token_field: 'pending_auth_token',
      provider: 'wechat',
      redirect: '/profile',
    }
    seedRegisterData({ email: 'fresh@example.com' })

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenCalledWith({
      email: 'fresh@example.com',
      pending_auth_token: 'pending-token-1',
    })
    expect(sendVerifyCodeMock).not.toHaveBeenCalled()
  })

  it('does not replay an Aliyun proof after a failed verify-code request', async () => {
    const resetHumanVerification = vi.fn()
    getPublicSettingsMock.mockResolvedValue({
      aliyun_captcha_enabled: true,
      aliyun_captcha_scene_id: 'scene-1',
      aliyun_captcha_prefix: 'tenant-1',
      aliyun_captcha_region: 'cn',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
    })
    sendVerifyCodeMock
      .mockRejectedValueOnce(new Error('send failed'))
      .mockResolvedValueOnce({ countdown: 60 })
    seedRegisterData({
      email: 'fresh@example.com',
      captcha_token: 'initial-aliyun-proof',
    })

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          HumanVerificationWidget: {
            template: '<button data-testid="aliyun-verify" @click="$emit(\'verify\', \'fresh-aliyun-proof\')">verify</button>',
            methods: { reset: resetHumanVerification },
          },
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(sendVerifyCodeMock).toHaveBeenNthCalledWith(1, {
      email: 'fresh@example.com',
      captcha_token: 'initial-aliyun-proof',
    })
    expect(JSON.parse(sessionStorage.getItem('register_data') || '{}')).not.toHaveProperty(
      'captcha_token',
    )

    await wrapper.get('[data-testid="aliyun-verify"]').trigger('click')
    const resendButton = wrapper
      .findAll('button')
      .find(button => button.text().includes('auth.resendCode'))
    expect(resendButton).toBeDefined()
    await resendButton?.trigger('click')
    await flushPromises()

    expect(sendVerifyCodeMock).toHaveBeenNthCalledWith(2, {
      email: 'fresh@example.com',
      captcha_token: 'fresh-aliyun-proof',
    })
    expect(sendVerifyCodeMock).toHaveBeenCalledTimes(2)
    expect(resetHumanVerification).toHaveBeenCalledTimes(1)
  })

  it('skips the registration email suffix whitelist for pending oauth verification', async () => {
    authStoreState.pendingAuthSession = {
      token: 'pending-token-2',
      token_field: 'pending_auth_token',
      provider: 'oidc',
      redirect: '/profile',
    }
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    })
    seedRegisterData({ email: 'fresh@example.com' })

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenCalledWith({
      email: 'fresh@example.com',
      pending_auth_token: 'pending-token-2',
    })
    expect(showErrorMock).not.toHaveBeenCalled()
  })

  it('uses the pending oauth verify-code endpoint when auth store only carries the pending provider', async () => {
    authStoreState.pendingAuthSession = {
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile',
    }
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    })
    seedRegisterData({ email: 'fresh@example.com' })

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(sendPendingOAuthVerifyCodeMock).toHaveBeenCalledWith({
      email: 'fresh@example.com',
      pending_oauth_token: undefined,
    })
    expect(sendVerifyCodeMock).not.toHaveBeenCalled()
    expect(showErrorMock).not.toHaveBeenCalled()
  })

  it('returns to the oauth callback flow when pending send-code detects an existing account email', async () => {
    authStoreState.pendingAuthSession = {
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile/security',
    }
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    })
    sendPendingOAuthVerifyCodeMock.mockResolvedValue({
      auth_result: 'pending_session',
      provider: 'oidc',
      redirect: '/profile/security',
    })
    seedRegisterData({ email: 'fresh@example.com' })

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(setPendingAuthSessionMock).toHaveBeenCalledWith({
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile/security',
    })
    expect(pushMock).toHaveBeenCalledWith('/auth/oidc/callback')
    expect(showErrorMock).not.toHaveBeenCalled()
  })

  it('submits pending auth account creation when session storage has no pending metadata but auth store does', async () => {
    authStoreState.pendingAuthSession = {
      token: 'pending-token-1',
      token_field: 'pending_auth_token',
      provider: 'wechat',
      redirect: '/profile',
    }
    seedRegisterData({
      email: 'fresh@example.com',
      aff_code: 'AFF123',
    })
    apiClientPostMock.mockResolvedValue({
      data: {
        access_token: 'oauth-access-token',
        refresh_token: 'oauth-refresh-token',
        expires_in: 3600,
        token_type: 'Bearer',
      },
    })

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()
    await wrapper.get('#code').setValue('123456')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(apiClientPostMock).toHaveBeenCalledWith('/auth/oauth/pending/create-account', {
      credential_envelope: {
        algorithm: 'RSA-OAEP-256+A256GCM',
        key_id: 'test-key',
        encrypted_key: 'encrypted-key',
        iv: 'random-iv',
        ciphertext: 'encrypted-credentials',
      },
      verify_code: '123456',
      aff_code: 'AFF123',
    })
    expect(persistOAuthTokenContextMock).toHaveBeenCalledWith({
      access_token: 'oauth-access-token',
      refresh_token: 'oauth-refresh-token',
      expires_in: 3600,
      token_type: 'Bearer',
    })
    expect(setTokenMock).toHaveBeenCalledWith('oauth-access-token')
    expect(clearPendingAuthSessionMock).toHaveBeenCalled()
    expect(pushMock).toHaveBeenCalledWith('/profile')
    expect(registerMock).not.toHaveBeenCalled()
  })

  it('returns to the oauth callback flow when pending account creation becomes bind-login', async () => {
    authStoreState.pendingAuthSession = {
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile/security',
    }
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    })
    seedRegisterData({ email: 'fresh@example.com' })
    apiClientPostMock.mockResolvedValue({
      data: {
        auth_result: 'pending_session',
        provider: 'oidc',
        step: 'bind_login_required',
        redirect: '/profile/security',
        email: 'fresh@example.com',
      },
    })

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()
    await wrapper.get('#code').setValue('123456')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(apiClientPostMock).toHaveBeenCalledWith('/auth/oauth/pending/create-account', {
      credential_envelope: {
        algorithm: 'RSA-OAEP-256+A256GCM',
        key_id: 'test-key',
        encrypted_key: 'encrypted-key',
        iv: 'random-iv',
        ciphertext: 'encrypted-credentials',
      },
      verify_code: '123456',
    })
    expect(setPendingAuthSessionMock).toHaveBeenCalledWith({
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'oidc',
      redirect: '/profile/security',
    })
    expect(pushMock).toHaveBeenCalledWith('/auth/oidc/callback')
    expect(setTokenMock).not.toHaveBeenCalled()
    expect(persistOAuthTokenContextMock).not.toHaveBeenCalled()
    expect(clearPendingAuthSessionMock).not.toHaveBeenCalled()
    expect(showSuccessMock).not.toHaveBeenCalled()
  })

  it('encrypts normal registration credentials at verification submit time', async () => {
    seedRegisterData({
      email: 'normal@example.com',
      promo_code: 'PROMO',
      invitation_code: 'INVITE',
    })
    registerMock.mockResolvedValue({})

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()
    await wrapper.get('#code').setValue('654321')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith({
      credential_envelope: {
        algorithm: 'RSA-OAEP-256+A256GCM',
        key_id: 'test-key',
        encrypted_key: 'encrypted-key',
        iv: 'random-iv',
        ciphertext: 'encrypted-credentials',
      },
      verify_code: '654321',
      turnstile_token: undefined,
      promo_code: 'PROMO',
      invitation_code: 'INVITE',
    })
    expect(createCredentialEnvelopeMock).toHaveBeenCalledWith('normal@example.com', 'secret-123')
    expect(apiClientPostMock).not.toHaveBeenCalled()
    expect(pushMock).toHaveBeenCalledWith('/dashboard')
  })

  it('returns to registration after refresh loses volatile credentials', async () => {
    sessionStorage.setItem('register_data', JSON.stringify({ email: 'fresh@example.com' }))

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(pushMock).toHaveBeenCalledWith('/register')
    expect(sessionStorage.getItem('register_data')).toBeNull()
    expect(sendVerifyCodeMock).not.toHaveBeenCalled()
    expect(createCredentialEnvelopeMock).not.toHaveBeenCalled()
  })

})
