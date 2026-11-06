/**
 * Account authorization actions owned by the admin-accounts feature.
 *
 * The generic exports keep the legacy accountsAPI facade compatible while the
 * named helpers give presentation code a protocol-specific owner.
 */

import { apiClient } from '@/core/networks/client'

export type AccountOAuthMethod = 'oauth' | 'setup-token'

export interface AccountAuthUrlResponse {
  auth_url: string
  session_id: string
}

export interface AccountOAuthExchangeRequest {
  session_id: string
  code: string
  state?: string
  proxy_id?: number
}

export interface AccountOAuthTokenInfo {
  org_uuid?: string
  account_uuid?: string
  email_address?: string
  [key: string]: unknown
}

export interface OpenAIAuthUrlRequest {
  proxy_id?: number
  redirect_uri?: string
	flow?: 'codex' | 'siwc'
}

export interface OpenAIExchangeCodeRequest {
  session_id: string
  code: string
  state: string
  proxy_id?: number
  client_id?: string
}

export interface OpenAITokenInfo {
  access_token?: string
  refresh_token?: string
  client_id?: string
  id_token?: string
  token_type?: string
  expires_in?: number
  expires_at?: number
  scope?: string
  auth_mode?: string
  device_id?: string
  email?: string
  name?: string
  plan_type?: string
  subscription_expires_at?: string
  privacy_mode?: string
  chatgpt_account_id?: string
  chatgpt_user_id?: string
  organization_id?: string
  [key: string]: unknown
}

const accountOAuthEndpoints: Record<AccountOAuthMethod, {
  authUrl: string
  exchangeCode: string
  cookieAuth: string
}> = {
  oauth: {
    authUrl: '/admin/accounts/generate-auth-url',
    exchangeCode: '/admin/accounts/exchange-code',
    cookieAuth: '/admin/accounts/cookie-auth'
  },
  'setup-token': {
    authUrl: '/admin/accounts/generate-setup-token-url',
    exchangeCode: '/admin/accounts/exchange-setup-token-code',
    cookieAuth: '/admin/accounts/setup-token-cookie-auth'
  }
}

function proxyPayload(proxyId?: number | null): { proxy_id?: number } {
  return proxyId ? { proxy_id: proxyId } : {}
}

/** Compatibility-shaped request used by the legacy accountsAPI facade. */
export async function generateAuthUrl(
  endpoint: string,
  config: { proxy_id?: number } = {}
): Promise<AccountAuthUrlResponse> {
  const { data } = await apiClient.post<AccountAuthUrlResponse>(endpoint, config)
  return data
}

/** Compatibility-shaped request used by the legacy accountsAPI facade. */
export async function exchangeCode(
  endpoint: string,
  exchangeData: AccountOAuthExchangeRequest
): Promise<AccountOAuthTokenInfo> {
  const { data } = await apiClient.post<AccountOAuthTokenInfo>(endpoint, exchangeData)
  return data
}

export async function generateAccountAuthUrl(
  method: AccountOAuthMethod,
  proxyId?: number | null
): Promise<AccountAuthUrlResponse> {
  return generateAuthUrl(accountOAuthEndpoints[method].authUrl, proxyPayload(proxyId))
}

export async function exchangeAccountAuthCode(
  method: AccountOAuthMethod,
  payload: Omit<AccountOAuthExchangeRequest, 'state'>
): Promise<AccountOAuthTokenInfo> {
  return exchangeCode(accountOAuthEndpoints[method].exchangeCode, payload)
}

export async function authenticateAccountWithCookie(
  method: AccountOAuthMethod,
  sessionKey: string,
  proxyId?: number | null
): Promise<AccountOAuthTokenInfo> {
  return exchangeCode(accountOAuthEndpoints[method].cookieAuth, {
    session_id: '',
    code: sessionKey,
    ...proxyPayload(proxyId)
  })
}

export async function generateOpenAIAuthUrl(
  payload: OpenAIAuthUrlRequest
): Promise<AccountAuthUrlResponse> {
  return generateAuthUrl('/admin/openai/generate-auth-url', payload)
}

export async function exchangeOpenAICode(
  payload: OpenAIExchangeCodeRequest
): Promise<OpenAITokenInfo> {
  const { data } = await apiClient.post<OpenAITokenInfo>('/admin/openai/exchange-code', payload)
  return data
}

export async function refreshOpenAIToken(
  refreshToken: string,
  proxyId?: number | null,
  endpoint: string = '/admin/openai/refresh-token',
  clientId?: string
): Promise<OpenAITokenInfo> {
  const payload: { refresh_token: string; proxy_id?: number; client_id?: string } = {
    refresh_token: refreshToken
  }
  if (proxyId) payload.proxy_id = proxyId
  if (clientId) payload.client_id = clientId

  const { data } = await apiClient.post<OpenAITokenInfo>(endpoint, payload)
  return data
}

export default {
  generateAuthUrl,
  exchangeCode,
  generateAccountAuthUrl,
  exchangeAccountAuthCode,
  authenticateAccountWithCookie,
  generateOpenAIAuthUrl,
  exchangeOpenAICode,
  refreshOpenAIToken
}
