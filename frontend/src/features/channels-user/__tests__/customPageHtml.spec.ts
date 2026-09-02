import { describe, expect, it } from 'vitest'
import {
  CUSTOM_PAGE_IFRAME_SANDBOX,
  sanitizeCustomPageHtml,
} from '@/features/channels-user/presentation/customPageHtml'

describe('custom page HTML security', () => {
  it('sandboxes valid iframe sources without same-origin access', () => {
    const result = sanitizeCustomPageHtml(
      '<iframe id="named-frame" name="named-frame" src="https://trusted.example.com/embed" allowfullscreen></iframe>',
      'Embedded content',
    )
    const container = document.createElement('div')
    container.innerHTML = result
    const frame = container.querySelector('iframe')

    expect(frame?.getAttribute('src')).toBe('https://trusted.example.com/embed')
    expect(frame?.getAttribute('sandbox')).toBe(CUSTOM_PAGE_IFRAME_SANDBOX)
    expect(frame?.getAttribute('sandbox')).not.toContain('allow-same-origin')
    expect(frame?.getAttribute('referrerpolicy')).toBe('no-referrer')
    expect(frame?.getAttribute('loading')).toBe('lazy')
    expect(frame?.getAttribute('title')).toBe('Embedded content')
    expect(frame?.hasAttribute('id')).toBe(false)
    expect(frame?.hasAttribute('name')).toBe(false)
  })

  it('removes unsafe iframe sources, srcdoc, handlers, and scripts', () => {
    const result = sanitizeCustomPageHtml(`
      <h1>Safe heading</h1>
      <iframe src="javascript:alert(1)" srcdoc="<script>alert(2)</script>" onload="alert(3)"></iframe>
      <script>alert(4)</script>
    `, 'Embedded content')
    const container = document.createElement('div')
    container.innerHTML = result

    expect(container.querySelector('h1')?.textContent).toBe('Safe heading')
    expect(container.querySelector('iframe, script')).toBeNull()
  })

  it('keeps relative iframe paths but rejects protocol-relative URL variants', () => {
    const relative = sanitizeCustomPageHtml('<iframe src="/docs/embed"></iframe>', 'Embedded content')
    const protocolRelative = sanitizeCustomPageHtml('<iframe src="//evil.example.com/embed"></iframe>', 'Embedded content')
    const backslashRelative = sanitizeCustomPageHtml('<iframe src="/\\evil.example.com/embed"></iframe>', 'Embedded content')

    const relativeContainer = document.createElement('div')
    relativeContainer.innerHTML = relative
    const protocolRelativeContainer = document.createElement('div')
    protocolRelativeContainer.innerHTML = protocolRelative
    const backslashRelativeContainer = document.createElement('div')
    backslashRelativeContainer.innerHTML = backslashRelative
    expect(relativeContainer.querySelector('iframe')?.getAttribute('src')).toBe('/docs/embed')
    expect(protocolRelativeContainer.querySelector('iframe')).toBeNull()
    expect(backslashRelativeContainer.querySelector('iframe')).toBeNull()
  })
})
