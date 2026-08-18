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
