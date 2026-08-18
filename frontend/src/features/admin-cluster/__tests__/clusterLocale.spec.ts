import { describe, expect, it } from 'vitest'
import enLocale from '@/core/i18n/locales/en/admin/cluster'
import zhLocale from '@/core/i18n/locales/zh/admin/cluster'
import {
  clusterInstanceStatusLabel,
  clusterRolloutStatusLabel,
  clusterRolloutTargetStatusLabel,
  clusterTaskStatusLabel,
} from '@/features/admin-cluster/presentation/clusterLocale'

function translator(messages: typeof enLocale, unknownLabel: string) {
  return (key: string): string => {
    if (key === 'common.unknown') return unknownLabel
    const path = key.replace(/^admin\./, '').split('.')
    let value: unknown = messages
    for (const part of path) {
      value = typeof value === 'object' && value !== null
        ? (value as Record<string, unknown>)[part]
        : undefined
    }
    return typeof value === 'string' ? value : key
  }
}

describe.each([
  { name: 'English', t: translator(enLocale, 'Unknown'), expected: ['Online', 'Succeeded', 'Paused', 'Verifying', 'Unknown'] },
  { name: 'Chinese', t: translator(zhLocale as typeof enLocale, '未知'), expected: ['在线', '成功', '已暂停', '正在验收', '未知'] },
])('cluster locale mapping - $name', ({ t, expected }) => {
  it('maps known statuses and localizes future values', () => {
    expect(clusterInstanceStatusLabel(t, 'online')).toBe(expected[0])
    expect(clusterTaskStatusLabel(t, 'succeeded')).toBe(expected[1])
    expect(clusterRolloutStatusLabel(t, 'paused')).toBe(expected[2])
    expect(clusterRolloutTargetStatusLabel(t, 'verifying')).toBe(expected[3])
    expect(clusterInstanceStatusLabel(t, 'future_instance')).toBe(expected[4])
    expect(clusterTaskStatusLabel(t, 'future_task')).toBe(expected[4])
    expect(clusterRolloutStatusLabel(t, 'future_rollout')).toBe(expected[4])
    expect(clusterRolloutTargetStatusLabel(t, 'future_target')).toBe(expected[4])
  })
})
