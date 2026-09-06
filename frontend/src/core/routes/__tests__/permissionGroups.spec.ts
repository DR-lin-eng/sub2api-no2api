import { describe, expect, it } from 'vitest'
import { adminLandingPath, canAccessAdminPage, type AdminAccess } from '../adminPermissions'

function staff(permissions: string[]): AdminAccess {
  return { isAdmin: false, canAccessAdmin: true, hasPermission: (p) => permissions.includes(p) }
}

describe('permission group navigation', () => {
  const support = staff(['support.read', 'support.write', 'users.read_basic'])
  it('lands support in the inbox without exposing dashboard or settings', () => {
    expect(adminLandingPath(support, true)).toBe('/admin/support')
    expect(canAccessAdminPage(support, '/admin/support')).toBe(true)
    for (const path of ['/admin/dashboard', '/admin/settings', '/admin/users', '/admin/accounts']) {
      expect(canAccessAdminPage(support, path)).toBe(false)
    }
  })
  it('avoids a disabled chat redirect loop and provides a reachable personal page', () => {
    expect(adminLandingPath(support, false)).toBe('/profile')
    expect(adminLandingPath(staff(['users.read_basic']), true)).toBe('/profile')
  })
  it('denies unassigned pages even with a similar broad grant', () => {
    expect(canAccessAdminPage(staff(['dashboard.read']), '/admin/ops')).toBe(false)
    expect(canAccessAdminPage(staff(['settings.manage']), '/admin/audit-logs')).toBe(false)
    expect(canAccessAdminPage(staff(['users.manage']), '/admin/subscriptions')).toBe(false)
    expect(canAccessAdminPage(staff(['accounts.manage']), '/admin/accounts/')).toBe(true)
  })
  it('retains full administrator and regular-user defaults', () => {
    const admin = { ...support, isAdmin: true }
    expect(canAccessAdminPage(admin, '/admin/ops')).toBe(true)
    expect(adminLandingPath(admin, false)).toBe('/admin/dashboard')
    expect(adminLandingPath({ ...support, canAccessAdmin: false }, true)).toBe('/dashboard')
  })
})
