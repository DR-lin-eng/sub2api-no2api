import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  EMBEDDED_AUTH_MESSAGE_TYPE,
  buildEmbeddedUrl,
  detectTheme,
  isOpaqueDocument,
  postEmbeddedAuthContext,
} from '../embedded-url'

describe('embedded-url', () => {
  const originalLocation = window.location

  beforeEach(() => {
    Object.defineProperty(window, 'location', {
      value: {
        origin: 'https://app.example.com',
        href: 'https://app.example.com/custom/help?invite=secret#callback',
        pathname: '/custom/help',
      },
      writable: true,
      configurable: true,
    })
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    })
    document.documentElement.classList.remove('dark')
    vi.restoreAllMocks()
  })

  it('adds non-sensitive embedded context and removes legacy token parameters', () => {
    const result = buildEmbeddedUrl(
      'https://pay.example.com/checkout?plan=pro&token=legacy&access_token=legacy-2&auth_token=legacy-3',
      42,
      'dark',
      'zh-CN',
    )

    const url = new URL(result)
    expect(url.searchParams.get('plan')).toBe('pro')
    expect(url.searchParams.get('user_id')).toBe('42')
    expect(url.searchParams.has('token')).toBe(false)
    expect(url.searchParams.has('access_token')).toBe(false)
    expect(url.searchParams.has('auth_token')).toBe(false)
    expect(url.searchParams.get('theme')).toBe('dark')
    expect(url.searchParams.get('lang')).toBe('zh-CN')
    expect(url.searchParams.get('ui_mode')).toBe('embedded')
    expect(url.searchParams.get('src_host')).toBe('https://app.example.com')
    expect(url.searchParams.get('src_url')).toBe('https://app.example.com/custom/help')
  })

  it('omits optional params when they are empty', () => {
    const result = buildEmbeddedUrl('https://pay.example.com/checkout', undefined, 'light')

    const url = new URL(result)
    expect(url.searchParams.get('theme')).toBe('light')
    expect(url.searchParams.get('ui_mode')).toBe('embedded')
    expect(url.searchParams.has('user_id')).toBe(false)
    expect(url.searchParams.has('token')).toBe(false)
    expect(url.searchParams.has('lang')).toBe(false)
  })

  it('fails closed for invalid or active-scheme URLs', () => {
    expect(buildEmbeddedUrl('not a url', 1)).toBe('')
    expect(buildEmbeddedUrl('javascript:alert(1)', 1)).toBe('')
    expect(buildEmbeddedUrl('data:text/html,hello', 1)).toBe('')
  })

  it('forwards an explicitly delegated token only to the exact iframe origin', () => {
    const postMessage = vi.fn()

    expect(postEmbeddedAuthContext(
      { postMessage } as unknown as Pick<Window, 'postMessage'>,
      'https://trusted.example.com/embed?theme=dark',
      { userId: 42, authToken: 'access-token' },
    )).toBe(true)
    expect(postMessage).toHaveBeenCalledWith({
      type: EMBEDDED_AUTH_MESSAGE_TYPE,
      version: 1,
      token: 'access-token',
      user_id: 42,
    }, 'https://trusted.example.com')
  })

  it('does not post authentication context without a target, token, or safe URL', () => {
    const postMessage = vi.fn()
    const target = { postMessage } as unknown as Pick<Window, 'postMessage'>

    expect(postEmbeddedAuthContext(null, 'https://trusted.example.com', { authToken: 'token' })).toBe(false)
    expect(postEmbeddedAuthContext(target, 'https://trusted.example.com', { authToken: '' })).toBe(false)
    expect(postEmbeddedAuthContext(target, 'javascript:alert(1)', { authToken: 'token' })).toBe(false)
    expect(postMessage).not.toHaveBeenCalled()
  })

  it('detects dark mode from document root class', () => {
    document.documentElement.classList.add('dark')
    expect(detectTheme()).toBe('dark')
  })

  it('detects sandboxed documents with an opaque origin', () => {
    expect(isOpaqueDocument()).toBe(false)
    Object.defineProperty(window, 'location', {
      value: { origin: 'null' },
      writable: true,
      configurable: true,
    })
    expect(isOpaqueDocument()).toBe(true)
  })
})
