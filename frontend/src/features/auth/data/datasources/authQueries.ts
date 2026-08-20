import { apiClient } from '@/core/networks/client'
import { buildApiUrl } from '@/core/networks/url'
import type { CurrentUserResponse, PublicSettings } from '@/types'
import type { LocalCaptchaChallenge } from '../dtos/authDtos'

type PublicSettingsError = Error & { status?: number }
const PUBLIC_SETTINGS_TIMEOUT_MS = 10_000

function createPublicSettingsError(message: string, status?: number): PublicSettingsError {
  const error = new Error(message) as PublicSettingsError
  if (status !== undefined) {
    error.status = status
  }
  return error
}

function isNetworkFailure(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false
  const candidate = error as { status?: unknown; code?: unknown }
  return candidate.status === 0 || candidate.code === 'ERR_NETWORK'
}

async function fetchPublicSettingsWithoutXHR(): Promise<PublicSettings> {
  if (typeof globalThis.fetch !== 'function') {
    throw createPublicSettingsError('Public settings fallback is unavailable', 0)
  }

  const controller = typeof AbortController !== 'undefined' ? new AbortController() : null
  const timeout = controller
    ? setTimeout(() => controller.abort(), PUBLIC_SETTINGS_TIMEOUT_MS)
    : null

  let response: Response
  try {
    response = await globalThis.fetch(buildApiUrl('/settings/public'), {
      method: 'GET',
      credentials: 'omit',
      cache: 'no-store',
      signal: controller?.signal,
      // Keep this a CORS-safelisted request. Opaque documents and browser
      // extensions often reject a credentialed/preflighted public GET.
      headers: { Accept: 'application/json' },
    })
  } finally {
    if (timeout) clearTimeout(timeout)
  }

  let payload: unknown = null
  try {
    payload = await response.json()
  } catch {
    throw createPublicSettingsError(
      `Public settings returned an invalid response (${response.status})`,
      response.status,
    )
  }

  if (
    !response.ok ||
    !payload ||
    typeof payload !== 'object' ||
    !('code' in payload) ||
    (payload as { code?: unknown }).code !== 0 ||
    !('data' in payload) ||
    !payload.data ||
    typeof payload.data !== 'object'
  ) {
    const message =
      payload && typeof payload === 'object' && 'message' in payload && typeof payload.message === 'string'
        ? payload.message
        : `Public settings request failed (${response.status})`
    throw createPublicSettingsError(message, response.status)
  }

  return payload.data as PublicSettings
}

export function getCurrentUser() {
  return apiClient.get<CurrentUserResponse>('/auth/me')
}

export async function getPublicSettings(): Promise<PublicSettings> {
  try {
    const { data } = await apiClient.get<PublicSettings>('/settings/public', {
      // This endpoint is public and does not issue or consume auth cookies.
      // Omitting credentials also makes wildcard CORS responses valid in a
      // sandboxed/opaque browser context.
      withCredentials: false,
      timeout: PUBLIC_SETTINGS_TIMEOUT_MS,
    })
    return data
  } catch (error) {
    if (!isNetworkFailure(error)) {
      throw error
    }

    // Axios/XHR can be intercepted by privacy, translation, or verification
    // extensions. Retry once with a simple fetch request that avoids a custom
    // content header and browser credentials; the login POST remains on the
    // normal encrypted Axios path.
    return fetchPublicSettingsWithoutXHR()
  }
}

export async function getLocalCaptcha(): Promise<LocalCaptchaChallenge> {
  const { data } = await apiClient.get<LocalCaptchaChallenge>('/auth/captcha')
  return data
}
