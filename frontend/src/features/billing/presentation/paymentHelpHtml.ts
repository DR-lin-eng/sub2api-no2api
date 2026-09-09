import { marked } from 'marked'
import DOMPurify from 'dompurify'

export function renderPaymentHelp(text: string): string {
  const html = marked.parse(text, { async: false, gfm: true, breaks: false })
  return DOMPurify.sanitize(html, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ['style', 'form', 'input', 'button', 'textarea', 'select'],
  })
}
