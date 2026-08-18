import type {
  AuthResponse,
  SendVerifyCodeResponse,
  TotpLoginResponse,
} from '@/types'
import type { SessionRefreshResult } from '@/core/networks/sessionRefresh'

export type LoginResponse = AuthResponse | TotpLoginResponse

export type RefreshTokenResponse = SessionRefreshResult

export type OAuthLoginProvider =
  | 'github'
  | 'google'
  | 'linuxdo'
  | 'dingtalk'
  | 'wechat'
  | 'oidc'

export interface OAuthLoginStart {
  provider: OAuthLoginProvider
  params: Record<string, string>
}

export interface OAuthLoginStartResponse {
  authorize_url: string
}

export interface LocalCaptchaChallenge {
  captcha_id: string
  image_data: string
  expires_in: number
}

export interface OAuthTokenResponse {
  access_token: string
  refresh_token?: string
  expires_in?: number
  token_type?: string
}

export interface PendingOAuthBindLoginResponse extends Partial<OAuthTokenResponse> {
  auth_result?: string
  redirect?: string
  error?: string
  requires_2fa?: boolean
  temp_token?: string
  user_email_masked?: string
  adoption_required?: boolean
  suggested_display_name?: string
  suggested_avatar_url?: string
}

export type PendingOAuthExchangeResponse = PendingOAuthBindLoginResponse

export interface PendingOAuthCreateAccountResponse extends OAuthTokenResponse {
  auth_result?: string
}

export interface PendingOAuthSendVerifyCodeResponse extends SendVerifyCodeResponse {
  auth_result?: string
  provider?: string
  redirect?: string
}

export type OAuthCompletionKind = 'login' | 'bind'

export interface OAuthAdoptionDecision {
  adoptDisplayName?: boolean
  adoptAvatar?: boolean
}

export type WeChatOAuthMode = 'open' | 'mp'

export type WeChatOAuthUnavailableReason =
  | 'not_configured'
  | 'capability_unknown'
  | 'external_browser_required'
  | 'wechat_browser_required'
  | 'native_app_required'

export interface ResolvedWeChatOAuthStart {
  mode: WeChatOAuthMode | null
  openEnabled: boolean
  mpEnabled: boolean
  mobileEnabled: boolean
  isWeChatBrowser: boolean
  unavailableReason: WeChatOAuthUnavailableReason | null
}

export type WeChatOAuthPublicSettings = {
  wechat_oauth_enabled?: boolean
  wechat_oauth_open_enabled?: boolean
  wechat_oauth_mp_enabled?: boolean
  wechat_oauth_mobile_enabled?: boolean
}

export interface ValidatePromoCodeResponse {
  valid: boolean
  bonus_amount?: number
  error_code?: string
  message?: string
}

export interface ValidateInvitationCodeResponse {
  valid: boolean
  error_code?: string
}

export interface ForgotPasswordRequest {
  email: string
  turnstile_token?: string
  captcha_token?: string
  captcha_id?: string
  captcha_code?: string
  tencent_captcha_ticket?: string
  tencent_captcha_randstr?: string
}

export interface ForgotPasswordResponse {
  message: string
}

export interface ResetPasswordRequest {
  email: string
  token: string
  new_password: string
}

export interface ResetPasswordResponse {
  message: string
}
