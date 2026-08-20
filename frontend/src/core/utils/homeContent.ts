import DOMPurify from 'dompurify'
import { sanitizeUrl } from './url'

const FORBIDDEN_HOME_CONTENT_TAGS = [
  'base',
  'button',
  'embed',
  'form',
  'iframe',
  'input',
  'link',
  'meta',
  'object',
  'option',
  'script',
  'select',
  'style',
  'template',
  'textarea',
]

export const HOME_CONTENT_IFRAME_SANDBOX = 'allow-forms allow-scripts allow-popups'
export const TRUSTED_HOME_CONTENT_IFRAME_SANDBOX =
  `${HOME_CONTENT_IFRAME_SANDBOX} allow-same-origin allow-popups-to-escape-sandbox`

export function sanitizeHomeContentHtml(value: string): string {
  if (!value) return ''
  return DOMPurify.sanitize(value, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: FORBIDDEN_HOME_CONTENT_TAGS,
    FORBID_ATTR: ['formaction', 'srcdoc'],
  })
}

export function resolveHomeContentUrl(value: string): string {
  return sanitizeUrl(value)
}

/**
 * Keep arbitrary home URLs sandboxed, but let an explicitly configured HTTPS
 * subdomain of this site keep its own origin and popup behavior. This is
 * needed by trusted sibling apps that use localStorage or open their login
 * page in a new tab; the exact parent origin is never granted same-origin
 * access.
 */
export function resolveHomeContentIframeSandbox(
  value: string,
  parentOrigin?: string,
): string {
  const strictSandbox = HOME_CONTENT_IFRAME_SANDBOX
  const parent = parentOrigin || (typeof window !== 'undefined' ? window.location.origin : '')

  try {
    const targetURL = new URL(value)
    const parentURL = new URL(parent)
    if (targetURL.protocol !== 'https:' || parentURL.protocol !== 'https:') {
      return strictSandbox
    }
    if (targetURL.username || targetURL.password) {
      return strictSandbox
    }
    if (targetURL.port !== parentURL.port) {
      return strictSandbox
    }

    const parentHostname = parentURL.hostname.toLowerCase()
    const targetHostname = targetURL.hostname.toLowerCase()
    const isSiblingSubdomain =
      targetHostname !== parentHostname && targetHostname.endsWith(`.${parentHostname}`)

    return isSiblingSubdomain ? TRUSTED_HOME_CONTENT_IFRAME_SANDBOX : strictSandbox
  } catch {
    return strictSandbox
  }
}
