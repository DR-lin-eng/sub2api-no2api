/**
 * Shared URL builder for iframe-embedded pages.
 * Authentication is deliberately excluded from URLs. Opt-in token forwarding
 * uses postEmbeddedAuthContext after the iframe has loaded.
 */

const EMBEDDED_USER_ID_QUERY_KEY = 'user_id'
const EMBEDDED_THEME_QUERY_KEY = 'theme'
const EMBEDDED_LANG_QUERY_KEY = 'lang'
const EMBEDDED_UI_MODE_QUERY_KEY = 'ui_mode'
const EMBEDDED_UI_MODE_VALUE = 'embedded'
const EMBEDDED_SRC_HOST_QUERY_KEY = 'src_host'
const EMBEDDED_SRC_QUERY_KEY = 'src_url'
const SENSITIVE_EMBEDDED_QUERY_KEYS = ['token', 'access_token', 'auth_token'] as const

export const EMBEDDED_AUTH_MESSAGE_TYPE = 'sub2api:embedded-auth'

/** Sandboxed iframes expose an opaque `null` origin and cannot complete auth. */
export function isOpaqueDocument(): boolean {
  if (typeof window === 'undefined') return false
  try {
    return window.location.origin === 'null'
  } catch {
    return true
  }
}

export interface EmbeddedAuthContext {
  userId?: number
  authToken?: string | null
}

export interface EmbeddedUrlOptions {
  authToken?: string | null
  forwardAccessTokenInUrl?: boolean
}

export function buildEmbeddedUrl(
  baseUrl: string,
  userId?: number,
  theme: 'light' | 'dark' = 'light',
  lang?: string,
  options?: EmbeddedUrlOptions,
): string {
  if (!baseUrl) return baseUrl
  try {
    const url = new URL(baseUrl)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return ''
    SENSITIVE_EMBEDDED_QUERY_KEYS.forEach((key) => url.searchParams.delete(key))
    if (options?.forwardAccessTokenInUrl && options.authToken) {
      url.searchParams.set('token', options.authToken)
    }
    if (userId) {
      url.searchParams.set(EMBEDDED_USER_ID_QUERY_KEY, String(userId))
    }
    url.searchParams.set(EMBEDDED_THEME_QUERY_KEY, theme)
    if (lang) {
      url.searchParams.set(EMBEDDED_LANG_QUERY_KEY, lang)
    }
    url.searchParams.set(EMBEDDED_UI_MODE_QUERY_KEY, EMBEDDED_UI_MODE_VALUE)
    // Source tracking: let the embedded page know where it's being loaded from
    if (typeof window !== 'undefined') {
      url.searchParams.set(EMBEDDED_SRC_HOST_QUERY_KEY, window.location.origin)
      url.searchParams.set(
        EMBEDDED_SRC_QUERY_KEY,
        `${window.location.origin}${window.location.pathname}`,
      )
    }
    return url.toString()
  } catch {
    return ''
  }
}

export function postEmbeddedAuthContext(
  targetWindow: Pick<Window, 'postMessage'> | null,
  targetUrl: string,
  context: EmbeddedAuthContext,
): boolean {
  if (!targetWindow || !context.authToken) return false

  try {
    const url = new URL(targetUrl)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return false
    targetWindow.postMessage({
      type: EMBEDDED_AUTH_MESSAGE_TYPE,
      version: 1,
      token: context.authToken,
      ...(context.userId ? { user_id: context.userId } : {}),
    }, url.origin)
    return true
  } catch {
    return false
  }
}

export function detectTheme(): 'light' | 'dark' {
  if (typeof document === 'undefined') return 'light'
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light'
}
