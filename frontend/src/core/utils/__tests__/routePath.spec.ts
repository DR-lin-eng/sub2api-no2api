import { describe, expect, it } from 'vitest'

import { canonicalizeRoutePath } from '@/core/utils/routePath'

describe('canonicalizeRoutePath', () => {
  it('lowercases uppercase route segments', () => {
    expect(canonicalizeRoutePath('/ADMIN/dashboard')).toBe('/admin/dashboard')
  })

  it('leaves an already canonical path unchanged', () => {
    expect(canonicalizeRoutePath('/admin/dashboard')).toBe('/admin/dashboard')
  })

  it('preserves query and hash text when given a complete URL path', () => {
    expect(canonicalizeRoutePath('/ADMIN/dashboard?Tab=Overview#Top')).toBe(
      '/admin/dashboard?Tab=Overview#Top',
    )
  })
})
