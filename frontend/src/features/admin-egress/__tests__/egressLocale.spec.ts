import { describe, expect, it } from 'vitest'
import {
  egressHEActionLabel,
  egressHEErrorMessage,
  egressHEStateLabel,
  egressHESuccessMessage,
  egressModeLabel,
  egressPoolStatusLabel,
} from '@/features/admin-egress/presentation/egressLocale'

describe.each([
  {
    name: 'English',
    messages: {
      'admin.egress.modes.ipv6_pool': 'IPv6 pool',
      'admin.egress.status.active': 'Active',
      'admin.egress.he.states.succeeded': 'Last action succeeded',
      'admin.egress.he.actions.apply': 'Save and apply',
      'admin.egress.he.success.apply': 'Apply queued',
      'admin.egress.he.errors.apply': 'Apply failed',
      'admin.egress.errors.load': 'Load failed',
      'common.unknown': 'Unknown',
    },
  },
  {
    name: 'Chinese',
    messages: {
      'admin.egress.modes.ipv6_pool': 'IPv6 地址池',
      'admin.egress.status.active': '启用',
      'admin.egress.he.states.succeeded': '上次操作成功',
      'admin.egress.he.actions.apply': '保存并应用',
      'admin.egress.he.success.apply': 'HE 隧道应用请求已排队',
      'admin.egress.he.errors.apply': 'HE 隧道应用请求排队失败',
      'admin.egress.errors.load': '加载 IPv6 出口数据失败',
      'common.unknown': '未知',
    },
  },
])('egress locale mapping - $name', ({ messages }) => {
  const t = (key: string) => messages[key as keyof typeof messages] ?? key

  it('maps known values and localizes future values', () => {
    expect(egressModeLabel(t, 'ipv6_pool')).toBe(messages['admin.egress.modes.ipv6_pool'])
    expect(egressPoolStatusLabel(t, 'active')).toBe(messages['admin.egress.status.active'])
    expect(egressHEStateLabel(t, 'succeeded')).toBe(messages['admin.egress.he.states.succeeded'])
    expect(egressHEActionLabel(t, 'apply')).toBe(messages['admin.egress.he.actions.apply'])
    expect(egressHESuccessMessage(t, 'apply')).toBe(messages['admin.egress.he.success.apply'])
    expect(egressHEErrorMessage(t, 'apply')).toBe(messages['admin.egress.he.errors.apply'])
    expect(egressModeLabel(t, 'future_mode')).toBe(messages['common.unknown'])
    expect(egressHEStateLabel(t, 'future_state')).toBe(messages['common.unknown'])
    expect(egressHEActionLabel(t, 'future_action')).toBe(messages['common.unknown'])
    expect(egressHESuccessMessage(t, 'future_action')).toBe(messages['admin.egress.errors.load'])
  })
})
