import { apiClient } from '@/core/networks/client'
import type {
  ActionCaptchaRequestProof,
  SendVerifyCodeRequest,
} from '@/types'
import type {
  OAuthAdoptionDecision,
  OAuthCompletionKind,
  OAuthLoginStart,
  OAuthLoginStartResponse,
  OAuthTokenResponse,
  PendingOAuthBindLoginResponse,
  PendingOAuthCreateAccountResponse,
  PendingOAuthExchangeResponse,
  PendingOAuthSendVerifyCodeResponse,
  ResolvedWeChatOAuthStart,
  WeChatOAuthPublicSettings,
} from '../dtos/authDtos'
import {
  getAuthToken,
  setRefreshToken,
  setTokenExpiresAt,
} from './authSessionActions'

export function buildOAuthLoginStartURL(request: OAuthLoginStart): string {
  const apiBase = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
  const normalized = apiBase.replace(/\/$/, '')
  const query = new URLSearchParams(request.params).toString()
  const path = `${normalized}/auth/oauth/${request.provider}/start`
  return query ? `${path}?${query}` : path
}

export async function startOAuthLogin(
  request: OAuthLoginStart,
  proof: ActionCaptchaRequestProof,
): Promise<OAuthLoginStartResponse> {
  const { data } = await apiClient.post<OAuthLoginStartResponse>(
    `/auth/oauth/${request.provider}/start`,
    proof,
    { params: request.params },
  )
  return data
}

function serializeOAuthAdoptionDecision(
  decision?: OAuthAdoptionDecision,
): Record<string, boolean> {
  const payload: Record<string, boolean> = {}
  if (typeof decision?.adoptDisplayName === 'boolean') {
    payload.adopt_display_name = decision.adoptDisplayName
  }
  if (typeof decision?.adoptAvatar === 'boolean') {
    payload.adopt_avatar = decision.adoptAvatar
  }
  return payload
}

export function isOAuthLoginCompletion(
  completion: Partial<OAuthTokenResponse>,
): completion is OAuthTokenResponse {
  return typeof completion.access_token === 'string' && completion.access_token.trim().length > 0
}

export function getOAuthCompletionKind(
  completion: Partial<OAuthTokenResponse>,
): OAuthCompletionKind {
  return isOAuthLoginCompletion(completion) ? 'login' : 'bind'
}

export function getPendingOAuthBindLoginKind(
  completion: PendingOAuthBindLoginResponse,
): OAuthCompletionKind {
  return getOAuthCompletionKind(completion)
}

export function isPendingOAuthCreateAccountRequired(
  completion: Pick<PendingOAuthBindLoginResponse, 'error'>,
): boolean {
  return completion.error === 'invitation_required'
}

export function hasPendingOAuthSuggestedProfile(
  completion: Pick<
    PendingOAuthBindLoginResponse,
    'suggested_display_name' | 'suggested_avatar_url'
  >,
): boolean {
  return Boolean(completion.suggested_display_name || completion.suggested_avatar_url)
}

export function persistOAuthTokenContext(tokens: Partial<OAuthTokenResponse>): void {
  if (tokens.refresh_token) setRefreshToken(tokens.refresh_token)
  if (tokens.expires_in) setTokenExpiresAt(tokens.expires_in)
}

export async function prepareOAuthBindAccessTokenCookie(): Promise<void> {
  if (!getAuthToken()) return
  await apiClient.post('/auth/oauth/bind-token')
}

export function isWeChatWebOAuthEnabled(
  settings: WeChatOAuthPublicSettings | null | undefined,
): boolean {
  const legacyEnabled = settings?.wechat_oauth_enabled ?? false
  const hasExplicitCapabilities =
    typeof settings?.wechat_oauth_open_enabled === 'boolean'
    || typeof settings?.wechat_oauth_mp_enabled === 'boolean'

  if (!hasExplicitCapabilities) return legacyEnabled
  return settings?.wechat_oauth_open_enabled === true || settings?.wechat_oauth_mp_enabled === true
}

export function hasExplicitWeChatOAuthCapabilities(
  settings: WeChatOAuthPublicSettings | null | undefined,
): settings is WeChatOAuthPublicSettings & {
  wechat_oauth_open_enabled: boolean
  wechat_oauth_mp_enabled: boolean
} {
  return typeof settings?.wechat_oauth_open_enabled === 'boolean'
    && typeof settings?.wechat_oauth_mp_enabled === 'boolean'
}

export function resolveWeChatOAuthStart(
  settings: WeChatOAuthPublicSettings | null | undefined,
  userAgent?: string,
): ResolvedWeChatOAuthStart {
  const normalizedUserAgent = (
    userAgent ?? (typeof navigator !== 'undefined' ? navigator.userAgent : '') ?? ''
  ).trim()
  const isWeChatBrowser = /MicroMessenger/i.test(normalizedUserAgent)
  const legacyEnabled = settings?.wechat_oauth_enabled ?? false
  const openEnabled = typeof settings?.wechat_oauth_open_enabled === 'boolean'
    ? settings.wechat_oauth_open_enabled
    : legacyEnabled
  const mpEnabled = typeof settings?.wechat_oauth_mp_enabled === 'boolean'
    ? settings.wechat_oauth_mp_enabled
    : legacyEnabled
  const mobileEnabled = typeof settings?.wechat_oauth_mobile_enabled === 'boolean'
    ? settings.wechat_oauth_mobile_enabled
    : false

  if (isWeChatBrowser) {
    if (mpEnabled) {
      return {
        mode: 'mp',
        openEnabled,
        mpEnabled,
        mobileEnabled,
        isWeChatBrowser,
        unavailableReason: null,
      }
    }
    if (openEnabled) {
      return {
        mode: null,
        openEnabled,
        mpEnabled,
        mobileEnabled,
        isWeChatBrowser,
        unavailableReason: 'external_browser_required',
      }
    }
    return {
      mode: null,
      openEnabled,
      mpEnabled,
      mobileEnabled,
      isWeChatBrowser,
      unavailableReason: 'not_configured',
    }
  }

  if (openEnabled) {
    return {
      mode: 'open',
      openEnabled,
      mpEnabled,
      mobileEnabled,
      isWeChatBrowser,
      unavailableReason: null,
    }
  }
  if (mpEnabled) {
    return {
      mode: null,
      openEnabled,
      mpEnabled,
      mobileEnabled,
      isWeChatBrowser,
      unavailableReason: 'wechat_browser_required',
    }
  }
  return {
    mode: null,
    openEnabled,
    mpEnabled,
    mobileEnabled,
    isWeChatBrowser,
    unavailableReason: 'not_configured',
  }
}

export function resolveWeChatOAuthStartStrict(
  settings: WeChatOAuthPublicSettings | null | undefined,
  userAgent?: string,
): ResolvedWeChatOAuthStart {
  const normalizedUserAgent = (
    userAgent ?? (typeof navigator !== 'undefined' ? navigator.userAgent : '') ?? ''
  ).trim()
  const isWeChatBrowser = /MicroMessenger/i.test(normalizedUserAgent)

  if (!hasExplicitWeChatOAuthCapabilities(settings)) {
    return {
      mode: null,
      openEnabled: false,
      mpEnabled: false,
      mobileEnabled: false,
      isWeChatBrowser,
      unavailableReason: 'capability_unknown',
    }
  }
  return resolveWeChatOAuthStart(settings, normalizedUserAgent)
}

export async function sendPendingOAuthVerifyCode(
  request: SendVerifyCodeRequest,
): Promise<PendingOAuthSendVerifyCodeResponse> {
  const { data } = await apiClient.post<PendingOAuthSendVerifyCodeResponse>(
    '/auth/oauth/pending/send-verify-code',
    request,
  )
  return data
}

async function createPendingOAuthAccount(
  provider: 'linuxdo' | 'oidc' | 'wechat' | 'dingtalk',
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string,
): Promise<PendingOAuthCreateAccountResponse> {
  const normalizedAffiliateCode = affiliateCode?.trim()
  const { data } = await apiClient.post<PendingOAuthCreateAccountResponse>(
    `/auth/oauth/${provider}/complete-registration`,
    {
      invitation_code: invitationCode,
      ...(normalizedAffiliateCode ? { aff_code: normalizedAffiliateCode } : {}),
      ...serializeOAuthAdoptionDecision(decision),
    },
  )
  return data
}

export function createPendingLinuxDoOAuthAccount(
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string,
): Promise<PendingOAuthCreateAccountResponse> {
  return createPendingOAuthAccount('linuxdo', invitationCode, decision, affiliateCode)
}

export function createPendingOIDCOAuthAccount(
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string,
): Promise<PendingOAuthCreateAccountResponse> {
  return createPendingOAuthAccount('oidc', invitationCode, decision, affiliateCode)
}

export function createPendingWeChatOAuthAccount(
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string,
): Promise<PendingOAuthCreateAccountResponse> {
  return createPendingOAuthAccount('wechat', invitationCode, decision, affiliateCode)
}

export function createPendingDingTalkOAuthAccount(
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string,
): Promise<PendingOAuthCreateAccountResponse> {
  return createPendingOAuthAccount('dingtalk', invitationCode, decision, affiliateCode)
}

export function completeLinuxDoOAuthRegistration(
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string,
): Promise<OAuthTokenResponse> {
  return createPendingLinuxDoOAuthAccount(invitationCode, decision, affiliateCode)
}

export function completeOIDCOAuthRegistration(
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string,
): Promise<OAuthTokenResponse> {
  return createPendingOIDCOAuthAccount(invitationCode, decision, affiliateCode)
}

export function completeWeChatOAuthRegistration(
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string,
): Promise<OAuthTokenResponse> {
  return createPendingWeChatOAuthAccount(invitationCode, decision, affiliateCode)
}

export async function completePendingOAuthBindLogin(
  decision?: OAuthAdoptionDecision,
): Promise<PendingOAuthBindLoginResponse> {
  const { data } = await apiClient.post<PendingOAuthBindLoginResponse>(
    '/auth/oauth/pending/exchange',
    serializeOAuthAdoptionDecision(decision),
  )
  return data
}

export function exchangePendingOAuthCompletion(
  decision?: OAuthAdoptionDecision,
): Promise<PendingOAuthExchangeResponse> {
  return completePendingOAuthBindLogin(decision)
}
