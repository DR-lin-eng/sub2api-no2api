import axios from 'axios'
import type { ApiResponse } from '@/types'
import { getAPIBaseURL } from './url'
import {
  setAccessToken,
  setTokenExpiresAtMemory,
} from './tokenStore'

export interface SessionRefreshResult {
  access_token: string
  refresh_token: string
  expires_in: number
  token_type: string
}

const AUTH_REFRESH_LOCK = 'sub2api-auth-refresh'
const AUTH_REFRESH_LEASE_KEY = 'sub2api_auth_refresh_lease'
const AUTH_REFRESH_LEASE_MS = 35000
let refreshInFlight: Promise<SessionRefreshResult> | null = null

async function requestSessionRefresh(): Promise<SessionRefreshResult> {
  const response = await axios.post<ApiResponse<SessionRefreshResult>>(
    `${getAPIBaseURL()}/auth/refresh`,
    undefined,
    {
      headers: { 'Content-Type': 'application/json' },
      timeout: 30000,
      withCredentials: true,
    },
  )
  const envelope = response.data
  if (!envelope || envelope.code !== 0 || !envelope.data) {
    throw new Error(envelope?.message || 'Token refresh failed')
  }

  const result = envelope.data
  setAccessToken(result.access_token)
  setTokenExpiresAtMemory(Date.now() + result.expires_in * 1000)
  return result
}

async function requestWithCrossTabLock(): Promise<SessionRefreshResult> {
  if (typeof navigator !== 'undefined' && navigator.locks?.request) {
    return navigator.locks.request(AUTH_REFRESH_LOCK, { mode: 'exclusive' }, requestSessionRefresh)
  }
  return requestWithStorageLease()
}

function waitForLease(delayMs: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, delayMs))
}

async function requestWithStorageLease(): Promise<SessionRefreshResult> {
  if (typeof localStorage === 'undefined') {
    return requestSessionRefresh()
  }

  const owner = typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random()}`
  const deadline = Date.now() + AUTH_REFRESH_LEASE_MS * 2

  try {
    localStorage.getItem(AUTH_REFRESH_LEASE_KEY)
  } catch {
    // Storage can be disabled by browser policy; fall back to the request.
    return requestSessionRefresh()
  }

  while (Date.now() < deadline) {
    const now = Date.now()
    let current: { owner?: string; expires_at?: number } | null = null
    try {
      current = JSON.parse(localStorage.getItem(AUTH_REFRESH_LEASE_KEY) || 'null')
    } catch {
      current = null
    }

    if (!current?.owner || !current.expires_at || current.expires_at <= now) {
      let acquired = false
      try {
        localStorage.setItem(AUTH_REFRESH_LEASE_KEY, JSON.stringify({
          owner,
          expires_at: now + AUTH_REFRESH_LEASE_MS,
        }))
        const confirmed = JSON.parse(localStorage.getItem(AUTH_REFRESH_LEASE_KEY) || 'null') as { owner?: string } | null
        acquired = confirmed?.owner === owner
      } catch {
        return requestSessionRefresh()
      }
      if (acquired) {
        try {
          return await requestSessionRefresh()
        } finally {
          try {
            const latest = JSON.parse(localStorage.getItem(AUTH_REFRESH_LEASE_KEY) || 'null') as { owner?: string } | null
            if (latest?.owner === owner) {
              localStorage.removeItem(AUTH_REFRESH_LEASE_KEY)
            }
          } catch {
            // The lease expires automatically if storage becomes unavailable.
          }
        }
      }
    }

    await waitForLease(75 + Math.floor(Math.random() * 75))
  }

  throw new Error('Timed out waiting for session refresh coordination')
}

// One request is shared inside a tab. Web Locks serialize rotation across
// same-origin tabs; older browsers use a short-lived non-secret storage lease.
// The RT itself remains HttpOnly and is never persisted by JavaScript.
export function refreshBrowserSession(): Promise<SessionRefreshResult> {
  if (refreshInFlight) {
    return refreshInFlight
  }
  refreshInFlight = requestWithCrossTabLock().finally(() => {
    refreshInFlight = null
  })
  return refreshInFlight
}
