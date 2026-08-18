import { describe, expect, it } from 'vitest'
import { adminUserRoleLabel } from '@/features/admin-users/presentation/adminUserLocale'

describe.each([
  { name: 'English', messages: { 'admin.users.roles.admin': 'Admin', 'admin.users.roles.user': 'User', 'common.unknown': 'Unknown' } },
  { name: 'Chinese', messages: { 'admin.users.roles.admin': '管理员', 'admin.users.roles.user': '用户', 'common.unknown': '未知' } },
])('admin user locale mapping - $name', ({ messages }) => {
  const t = (key: string) => messages[key as keyof typeof messages] ?? key

  it('maps roles and localizes future values', () => {
    expect(adminUserRoleLabel(t, 'admin')).toBe(messages['admin.users.roles.admin'])
    expect(adminUserRoleLabel(t, 'user')).toBe(messages['admin.users.roles.user'])
    expect(adminUserRoleLabel(t, 'future_role')).toBe(messages['common.unknown'])
  })
})
