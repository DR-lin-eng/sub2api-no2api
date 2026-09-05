import DOMPurify from 'dompurify'

export function sanitizeActivityBannerHtml(html: string): string {
  return DOMPurify.sanitize(html, { USE_PROFILES: { html: true }, FORBID_TAGS: ['style', 'form', 'input', 'button', 'textarea', 'select'] })
}
