import { describe, expect, it } from 'vitest'
import {
  HOME_CONTENT_IFRAME_SANDBOX,
  TRUSTED_HOME_CONTENT_IFRAME_SANDBOX,
  resolveHomeContentIframeSandbox,
  resolveHomeContentUrl,
  sanitizeHomeContentHtml,
} from '../homeContent'

describe('home content security', () => {
  it('keeps display markup while removing executable and interactive HTML', () => {
    const result = sanitizeHomeContentHtml(`
      <section id="safe" class="hero" style="color: red">
        <h1>Safe title</h1>
        <img src="https://images.example.com/a.png" onerror="alert(1)">
        <a href="javascript:alert(2)" onclick="alert(3)">Unsafe link</a>
      </section>
      <script>window.pwned = true</script>
      <style>body { display: none }</style>
      <form action="/login"><input name="password"><button>Submit</button></form>
      <iframe srcdoc="<script>alert(4)</script>"></iframe>
      <object data="https://evil.example.com"></object>
    `)

    const container = document.createElement('div')
    container.innerHTML = result
    expect(container.querySelector('#safe')?.textContent).toContain('Safe title')
    expect(container.querySelector('img')?.getAttribute('onerror')).toBeNull()
    expect(container.querySelector('a')?.getAttribute('href')).toBeNull()
    expect(container.querySelector('a')?.getAttribute('onclick')).toBeNull()
    expect(container.querySelector('script, style, form, input, button, iframe, object')).toBeNull()
  })

  it('accepts only absolute HTTP(S) iframe URLs', () => {
    expect(resolveHomeContentUrl(' https://example.com/home?mode=1 '))
      .toBe('https://example.com/home?mode=1')
    expect(resolveHomeContentUrl('http://example.com/home')).toBe('http://example.com/home')
    expect(resolveHomeContentUrl('javascript:alert(1)')).toBe('')
    expect(resolveHomeContentUrl('data:text/html,hello')).toBe('')
    expect(resolveHomeContentUrl('//example.com/home')).toBe('')
  })

  it('keeps external URLs strict but permits trusted HTTPS sibling subdomains', () => {
    expect(resolveHomeContentIframeSandbox('https://example.com/home', 'https://gptcodex.top'))
      .toBe(HOME_CONTENT_IFRAME_SANDBOX)
    expect(resolveHomeContentIframeSandbox('https://gptcodex.top/home', 'https://gptcodex.top'))
      .toBe(HOME_CONTENT_IFRAME_SANDBOX)
    expect(resolveHomeContentIframeSandbox('https://gptcodex.top.evil.example/home', 'https://gptcodex.top'))
      .toBe(HOME_CONTENT_IFRAME_SANDBOX)
    expect(resolveHomeContentIframeSandbox('http://child.gptcodex.top/home', 'https://gptcodex.top'))
      .toBe(HOME_CONTENT_IFRAME_SANDBOX)
    expect(resolveHomeContentIframeSandbox('https://child.gptcodex.top/home', 'https://gptcodex.top'))
      .toBe(TRUSTED_HOME_CONTENT_IFRAME_SANDBOX)
  })
})
