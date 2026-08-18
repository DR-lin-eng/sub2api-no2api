import { describe, expect, it } from 'vitest'
import {
  compositeRouteEndpointLabel,
  compositeRouteMatchLabel,
  compositeRouteSourceLabel,
  groupPlatformLabel,
  groupStatusLabel,
} from '@/features/admin-groups/groupsLocale'

describe.each([
  {
    name: 'English',
    messages: {
      'admin.groups.platforms.openai': 'OpenAI',
      'admin.groups.compositeRoutes.match.prefix': 'Prefix',
      'admin.groups.compositeRoutes.endpoints.chatCompletions': 'Chat Completions',
      'admin.groups.compositeRoutes.sources.detector': 'Detector',
      'common.unknown': 'Unknown',
      'common.active': 'Active',
    },
  },
  {
    name: 'Chinese',
    messages: {
      'admin.groups.platforms.openai': 'OpenAI',
      'admin.groups.compositeRoutes.match.prefix': '前缀',
      'admin.groups.compositeRoutes.endpoints.chatCompletions': 'Chat Completions',
      'admin.groups.compositeRoutes.sources.detector': '内置识别',
      'common.unknown': '未知',
      'common.active': '启用',
    },
  },
])('group locale mapping - $name', ({ messages }) => {
  const t = (key: string) => messages[key as keyof typeof messages] ?? key

  it('maps known values and localizes future values', () => {
    expect(groupPlatformLabel(t, 'openai')).toBe(messages['admin.groups.platforms.openai'])
    expect(compositeRouteMatchLabel(t, 'prefix')).toBe(messages['admin.groups.compositeRoutes.match.prefix'])
    expect(compositeRouteEndpointLabel(t, 'chat_completions')).toBe(messages['admin.groups.compositeRoutes.endpoints.chatCompletions'])
    expect(compositeRouteSourceLabel(t, 'detector')).toBe(messages['admin.groups.compositeRoutes.sources.detector'])
    expect(groupStatusLabel(t, 'active')).toBe(messages['common.active'])
    expect(groupPlatformLabel(t, 'future_platform')).toBe(messages['common.unknown'])
    expect(compositeRouteMatchLabel(t, 'future_match')).toBe(messages['common.unknown'])
    expect(compositeRouteEndpointLabel(t, 'future_endpoint')).toBe(messages['common.unknown'])
    expect(compositeRouteSourceLabel(t, 'future_source')).toBe(messages['common.unknown'])
    expect(groupStatusLabel(t, 'future_status')).toBe(messages['common.unknown'])
  })
})
