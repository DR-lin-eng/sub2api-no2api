import { describe, expect, it } from 'vitest'

import { getLocaleScopesForRoute } from '@/core/i18n'

describe('route locale scopes', () => {
  it.each(['/home', '/login', '/register', '/key-usage', '/setup'])(
    'keeps public route %s on the base messages',
    (path) => {
      expect(getLocaleScopesForRoute(path)).toEqual(['base'])
    },
  )

  it('loads user messages for authenticated and data-rich public pages', () => {
    expect(getLocaleScopesForRoute('/dashboard')).toEqual(['base', 'user'])
    expect(getLocaleScopesForRoute('/model-plaza')).toEqual(['base', 'user'])
    expect(getLocaleScopesForRoute('/monitor/public')).toEqual(['base', 'user'])
  })

  it('adds feature messages only for their routes', () => {
    expect(getLocaleScopesForRoute('/batch-image')).toEqual(['base', 'user', 'batchImage'])
    expect(getLocaleScopesForRoute('/support')).toEqual(['base', 'user', 'supportChat'])
  })

  it('loads admin messages only for admin routes', () => {
    expect(getLocaleScopesForRoute('/admin/dashboard')).toEqual(['base', 'user', 'admin'])
    expect(getLocaleScopesForRoute('/admin/support')).toEqual([
      'base',
      'user',
      'admin',
      'supportChat',
    ])
  })
})
