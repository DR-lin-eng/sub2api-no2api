import DOMPurify from 'dompurify'
import { getDomain } from 'tldts'
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
 * Trust sibling hosts only within one registrable site. Hostname suffixes
 * alone can match unrelated tenants under public suffixes such as co.uk or
 * github.io, so include private suffix rules in the domain check.
 */
function isSiblingHostOf(targetHostname: string, parentHostname: string): boolean {
  const parentLabels = parentHostname.split('.')
  const targetLabels = targetHostname.split('.')
  if (parentLabels.length < 3 || targetLabels.length !== parentLabels.length) return false
  if (parentLabels[0] === targetLabels[0]) return false
  const siteDomain = getDomain(parentHostname, { allowPrivateDomains: true })
  if (!siteDomain || siteDomain !== getDomain(targetHostname, { allowPrivateDomains: true })) {
    return false
  }
  return parentLabels.slice(1).join('.') === targetLabels.slice(1).join('.')
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
    const isSubdomainOfSite =
      targetHostname !== parentHostname && targetHostname.endsWith(`.${parentHostname}`)
    const isSiblingHost = isSiblingHostOf(targetHostname, parentHostname)

    return isSubdomainOfSite || isSiblingHost ? TRUSTED_HOME_CONTENT_IFRAME_SANDBOX : strictSandbox
  } catch {
    return strictSandbox
  }
}
