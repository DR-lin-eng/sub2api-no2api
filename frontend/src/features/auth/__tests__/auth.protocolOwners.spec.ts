import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, refreshBrowserSession } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  refreshBrowserSession: vi.fn(),
}))

vi.mock('@/core/networks/client', () => ({
  apiClient: { get, post },
}))

vi.mock('@/core/networks/sessionRefresh', () => ({
  refreshBrowserSession,
}))

import authAPI from '@/features/auth/data/datasources/authDatasource'
import {
  completeLinuxDoOAuthRegistration,
  completeOIDCOAuthRegistration,
  completePendingOAuthBindLogin,
  completeWeChatOAuthRegistration,
  createPendingDingTalkOAuthAccount,
  createPendingLinuxDoOAuthAccount,
  createPendingOIDCOAuthAccount,
  createPendingWeChatOAuthAccount,
  exchangePendingOAuthCompletion,
  getPendingOAuthBindLoginKind,
  hasPendingOAuthSuggestedProfile,
  isPendingOAuthCreateAccountRequired,
  sendPendingOAuthVerifyCode,
} from '@/features/auth/data/datasources/authOAuthActions'
import {
  getCurrentUser,
  getLocalCaptcha,
  getPublicSettings,
} from '@/features/auth/data/datasources/authQueries'
import {
  clearAuthToken,
  getAuthToken,
  getRefreshToken,
  getTokenExpiresAt,
  isAuthenticated,
  isTotp2FARequired,
  login,
  login2FA,
  logout,
  refreshToken,
  register,
  revokeAllSessions,
  setAuthToken,
  setRefreshToken,
  setTokenExpiresAt,
} from '@/features/auth/data/datasources/authSessionActions'
import {
  forgotPassword,
  resetPassword,
  sendVerifyCode,
  validateInvitationCode,
  validatePromoCode,
} from '@/features/auth/data/datasources/authVerificationActions'
import { clearTokenMemory, getAccessToken } from '@/core/networks/tokenStore'

describe('auth protocol owners', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    refreshBrowserSession.mockReset()
    clearTokenMemory()
    localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps auth and public-settings query paths and response shapes unchanged', async () => {
    const currentUser = { id: 7, email: 'user@example.com' }
    const settings = { registration_enabled: true }
    const captcha = { captcha_id: 'captcha-1', image_data: 'image', expires_in: 60 }
    get
      .mockResolvedValueOnce({ data: currentUser })
      .mockResolvedValueOnce({ data: settings })
      .mockResolvedValueOnce({ data: captcha })

    await expect(getCurrentUser()).resolves.toEqual({ data: currentUser })
    await expect(getPublicSettings()).resolves.toEqual(settings)
    await expect(getLocalCaptcha()).resolves.toEqual(captcha)

    expect(get).toHaveBeenNthCalledWith(1, '/auth/me')
    expect(get).toHaveBeenNthCalledWith(2, '/settings/public', {
      withCredentials: false,
      timeout: 10_000,
    })
    expect(get).toHaveBeenNthCalledWith(3, '/auth/captcha')
  })

  it('falls back to a credential-free fetch when the XHR transport has no response', async () => {
    const settings = { registration_enabled: true, version: '0.1.186' }
    get.mockRejectedValueOnce({ status: 0, message: 'Network error' })
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ code: 0, data: settings }),
    } as Response)

    await expect(getPublicSettings()).resolves.toEqual(settings)
    expect(get).toHaveBeenCalledWith('/settings/public', {
      withCredentials: false,
      timeout: 10_000,
    })
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/settings/public',
      expect.objectContaining({
        credentials: 'omit',
        cache: 'no-store',
      }),
    )
  })

  it('keeps verification, invitation, and password recovery payloads unchanged', async () => {
    post.mockResolvedValue({ data: { message: 'ok', valid: true, countdown: 60 } })
    const proof = { email: 'user@example.com', turnstile_token: 'proof' }

    await sendVerifyCode(proof)
    await validatePromoCode('PROMO')
    await validateInvitationCode('INVITE')
    await forgotPassword(proof)
    await resetPassword({ email: 'user@example.com', token: 'reset', new_password: 'new-pass' })

    expect(post).toHaveBeenNthCalledWith(1, '/auth/send-verify-code', proof)
    expect(post).toHaveBeenNthCalledWith(2, '/auth/validate-promo-code', { code: 'PROMO' })
    expect(post).toHaveBeenNthCalledWith(3, '/auth/validate-invitation-code', { code: 'INVITE' })
    expect(post).toHaveBeenNthCalledWith(4, '/auth/forgot-password', proof)
    expect(post).toHaveBeenNthCalledWith(5, '/auth/reset-password', {
      email: 'user@example.com',
      token: 'reset',
      new_password: 'new-pass',
    })
  })

  it('keeps 2FA, refresh-cookie, revoke, and local logout semantics unchanged', async () => {
    const authResponse = {
      access_token: 'memory-access-token',
      refresh_token: 'legacy-memory-refresh',
      expires_in: 3600,
      token_type: 'Bearer',
      user: { id: 7, email: 'user@example.com' },
    }
    post.mockResolvedValueOnce({ data: authResponse })

    await login2FA({ temp_token: 'temporary', totp_code: '123456' })
    expect(post).toHaveBeenNthCalledWith(1, '/auth/login/2fa', {
      temp_token: 'temporary',
      totp_code: '123456',
    })
    expect(getAccessToken()).toBe('memory-access-token')
    expect(localStorage.getItem('access_token')).toBeNull()
    expect(localStorage.getItem('refresh_token')).toBeNull()

    const refreshed = {
      access_token: 'refreshed',
      refresh_token: 'rotated',
      expires_in: 3600,
      token_type: 'Bearer',
    }
    refreshBrowserSession.mockResolvedValueOnce(refreshed)
    await expect(refreshToken()).resolves.toEqual(refreshed)
    expect(refreshBrowserSession).toHaveBeenCalledTimes(1)

    post.mockResolvedValueOnce({ data: { message: 'revoked' } })
    await expect(revokeAllSessions()).resolves.toEqual({ message: 'revoked' })
    expect(post).toHaveBeenNthCalledWith(2, '/auth/revoke-all-sessions')

    post.mockResolvedValueOnce({ data: {} })
    await logout()
    expect(post).toHaveBeenNthCalledWith(3, '/auth/logout')
    expect(getAccessToken()).toBeNull()
  })

  it('keeps the compatibility facade wired to the exact owner functions', () => {
    expect(authAPI).toEqual({
      login,
      login2FA,
      isTotp2FARequired,
      register,
      getCurrentUser,
      logout,
      isAuthenticated,
      setAuthToken,
      setRefreshToken,
      setTokenExpiresAt,
      getAuthToken,
      getRefreshToken,
      getTokenExpiresAt,
      clearAuthToken,
      getPublicSettings,
      getLocalCaptcha,
      sendVerifyCode,
      sendPendingOAuthVerifyCode,
      validatePromoCode,
      validateInvitationCode,
      forgotPassword,
      resetPassword,
      refreshToken,
      revokeAllSessions,
      getPendingOAuthBindLoginKind,
      isPendingOAuthCreateAccountRequired,
      hasPendingOAuthSuggestedProfile,
      completePendingOAuthBindLogin,
      createPendingLinuxDoOAuthAccount,
      createPendingOIDCOAuthAccount,
      createPendingWeChatOAuthAccount,
      exchangePendingOAuthCompletion,
      completeLinuxDoOAuthRegistration,
      completeOIDCOAuthRegistration,
      completeWeChatOAuthRegistration,
      createPendingDingTalkOAuthAccount,
    })
  })
})
