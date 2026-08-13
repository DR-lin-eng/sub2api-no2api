import DOMPurify from 'dompurify'

const CHAT_ALLOWED_TAGS = [
  'a',
  'b',
  'br',
  'code',
  'div',
  'em',
  'i',
  'img',
  'li',
  'ol',
  'p',
  'pre',
  'span',
  'strong',
  'u',
  'ul',
]

const CHAT_ALLOWED_ATTR = ['alt', 'href', 'src', 'title']

// Keep links useful for support replies while rejecting executable, data, and
// protocol-relative URLs. DOMPurify still applies its normal URI handling.
const CHAT_ALLOWED_URI = /^(?![^\s]*\\)(?:(?:https?|mailto|tel|blob):|(?:\/(?!\/)|\.\.?\/|[#?]|[^:/?#\s]+(?:[/?#]|$)))/i

export function sanitizeChatHtml(content: string): string {
  return DOMPurify.sanitize(content, {
    ALLOWED_TAGS: CHAT_ALLOWED_TAGS,
    ALLOWED_ATTR: CHAT_ALLOWED_ATTR,
    ALLOW_DATA_ATTR: false,
    FORBID_ATTR: ['class', 'style', 'id', 'name'],
    ALLOWED_URI_REGEXP: CHAT_ALLOWED_URI,
  })
}
