// Browser authentication material is intentionally kept in memory. The
// backend owns the long-lived refresh token in an HttpOnly cookie, so injected
// page scripts cannot read or persist it through Web Storage.
let accessToken: string | null = null
let refreshToken: string | null = null
let tokenExpiresAt: number | null = null

export function setAccessToken(value: string): void {
  accessToken = value
}

export function getAccessToken(): string | null {
  return accessToken
}

export function setRefreshTokenMemory(value: string | null): void {
  refreshToken = value
}

export function getRefreshTokenMemory(): string | null {
  return refreshToken
}

export function setTokenExpiresAtMemory(value: number | null): void {
  tokenExpiresAt = value
}

export function getTokenExpiresAtMemory(): number | null {
  return tokenExpiresAt
}

export function clearTokenMemory(): void {
  accessToken = null
  refreshToken = null
  tokenExpiresAt = null
}
