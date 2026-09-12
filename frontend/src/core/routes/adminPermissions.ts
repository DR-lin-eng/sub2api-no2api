/** UI navigation only. The backend independently checks every request. */
export interface AdminAccess {
  isAdmin: boolean
  canAccessAdmin: boolean
  hasPermission: (permission: string) => boolean
  isSimpleMode?: boolean
}

const pagePermissions: Record<string, string> = {
  '/admin/dashboard': 'dashboard.read',
  '/admin/support': 'support.read',
  '/admin/users': 'users.manage',
  '/admin/groups': 'groups.manage',
  '/admin/accounts': 'accounts.manage',
  '/admin/account-inspection': 'accounts.manage',
  '/admin/account-quality': 'accounts.manage',
  '/admin/proxies': 'accounts.manage',
  '/admin/egress': 'accounts.manage',
  '/admin/settings': 'settings.manage',
}

export function canAccessAdminPage(access: AdminAccess, path: string): boolean {
  if (access.isAdmin) return true
  if (!access.canAccessAdmin) return false
  const permission = pagePermissions[path.replace(/\/$/, '')]
  return !!permission && access.hasPermission(permission)
}

export function adminLandingPath(access: AdminAccess, supportEnabled: boolean): string {
  if (access.isAdmin) return '/admin/dashboard'
  for (const path of ['/admin/support', '/admin/dashboard', '/admin/users', '/admin/accounts', '/admin/groups', '/admin/settings']) {
    if (path === '/admin/support' && !supportEnabled) continue
    if (path === '/admin/groups' && access.isSimpleMode) continue
    if (canAccessAdminPage(access, path)) return path
  }
  // Personal profile is always reachable, including when chat is disabled.
  return access.canAccessAdmin ? '/profile' : '/dashboard'
}
