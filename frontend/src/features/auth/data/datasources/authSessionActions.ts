import { apiClient } from '@/core/networks/client'
import { createCredentialEnvelope } from '@/core/networks/credentialEncryption'
import {
  clearTokenMemory,
  getAccessToken,
  getRefreshTokenMemory,
  getTokenExpiresAtMemory,
  setAccessToken,
  setRefreshTokenMemory,
  setTokenExpiresAtMemory,
} from '@/core/networks/tokenStore'
import { refreshBrowserSession } from '@/core/networks/sessionRefresh'
import { safeLocalStorage } from '@/core/utils/safeStorage'
import type {
  AuthResponse,
  EncryptedRegisterRequest,
  LoginRequest,
  RegisterRequest,
  TotpLoginResponse,
  TotpLogin2FARequest,
} from '@/types'
import type { LoginResponse, RefreshTokenResponse } from '../dtos/authDtos'

export {
  clearCredentialKeyPrefetch,
  createCredentialEnvelope,
  prefetchCredentialKey,
} from '@/core/networks/credentialEncryption'

export function isTotp2FARequired(response: LoginResponse): response is TotpLoginResponse {
  return 'requires_2fa' in response && response.requires_2fa === true
}

export function setAuthToken(token: string): void {
  setAccessToken(token)
}

export function setRefreshToken(token: string): void {
  setRefreshTokenMemory(token)
}

export function setTokenExpiresAt(expiresIn: number): void {
  setTokenExpiresAtMemory(Date.now() + expiresIn * 1000)
}

export function getAuthToken(): string | null {
  return getAccessToken()
}

export function getRefreshToken(): string | null {
  return getRefreshTokenMemory()
}

export function getTokenExpiresAt(): number | null {
  return getTokenExpiresAtMemory()
}

export function clearAuthToken(): void {
  clearTokenMemory()
  safeLocalStorage.removeItem('auth_user')
}

export async function login(credentials: LoginRequest): Promise<LoginResponse> {
  const { email, password, ...requestData } = credentials
  const credentialEnvelope = await createCredentialEnvelope(email, password)
  const { data } = await apiClient.post<LoginResponse>('/auth/login', {
    ...requestData,
    credential_envelope: credentialEnvelope,
  })

  if (!isTotp2FARequired(data)) {
    setAuthToken(data.access_token)
    if (data.refresh_token) setRefreshToken(data.refresh_token)
    if (data.expires_in) setTokenExpiresAt(data.expires_in)
    safeLocalStorage.setItem('auth_user', JSON.stringify(data.user))
  }
  return data
}

export async function login2FA(request: TotpLogin2FARequest): Promise<AuthResponse> {
  const { data } = await apiClient.post<AuthResponse>('/auth/login/2fa', request)
  setAuthToken(data.access_token)
  if (data.refresh_token) setRefreshToken(data.refresh_token)
  if (data.expires_in) setTokenExpiresAt(data.expires_in)
  safeLocalStorage.setItem('auth_user', JSON.stringify(data.user))
  return data
}

export async function register(
  userData: RegisterRequest | EncryptedRegisterRequest,
): Promise<AuthResponse> {
  let requestData: EncryptedRegisterRequest
  if ('credential_envelope' in userData) {
    requestData = userData
  } else {
    const { email, password, ...registrationData } = userData
    requestData = {
      ...registrationData,
      credential_envelope: await createCredentialEnvelope(email, password),
    }
  }
  const { data } = await apiClient.post<AuthResponse>('/auth/register', requestData)
  setAuthToken(data.access_token)
  if (data.refresh_token) setRefreshToken(data.refresh_token)
  if (data.expires_in) setTokenExpiresAt(data.expires_in)
  safeLocalStorage.setItem('auth_user', JSON.stringify(data.user))
  return data
}

export async function logout(): Promise<void> {
  try {
    await apiClient.post('/auth/logout')
  } catch {
    // Local logout must still complete when server revocation fails.
  }
  clearAuthToken()
}

export function refreshToken(): Promise<RefreshTokenResponse> {
  return refreshBrowserSession()
}

export async function revokeAllSessions(): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>('/auth/revoke-all-sessions')
  return data
}

export function isAuthenticated(): boolean {
  return getAuthToken() !== null
}
