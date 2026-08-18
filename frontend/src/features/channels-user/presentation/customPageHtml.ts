import DOMPurify from 'dompurify'
import { sanitizeUrl } from '@/core/utils/url'

export const CUSTOM_PAGE_IFRAME_SANDBOX = 'allow-forms allow-scripts allow-popups'

export function sanitizeCustomPageHtml(value: string, iframeTitle: string): string {
  const sanitized = DOMPurify.sanitize(value, {
    ADD_TAGS: ['iframe'],
    ADD_ATTR: ['allowfullscreen', 'frameborder', 'src'],
    FORBID_ATTR: ['srcdoc'],
  })

  if (typeof DOMParser === 'undefined') {
    return DOMPurify.sanitize(sanitized, { FORBID_TAGS: ['iframe'] })
  }

  const parsed = new DOMParser().parseFromString(sanitized, 'text/html')
  parsed.querySelectorAll('iframe').forEach((frame) => {
    const safeSrc = sanitizeUrl(frame.getAttribute('src') || '', { allowRelative: true })
    if (!safeSrc) {
      frame.remove()
      return
    }

    frame.setAttribute('src', safeSrc)
    frame.setAttribute('sandbox', CUSTOM_PAGE_IFRAME_SANDBOX)
    frame.setAttribute('referrerpolicy', 'no-referrer')
    frame.setAttribute('loading', 'lazy')
    frame.setAttribute('title', frame.getAttribute('title') || iframeTitle)
    frame.removeAttribute('id')
    frame.removeAttribute('name')
    frame.removeAttribute('srcdoc')
  })

  return parsed.body.innerHTML
}
