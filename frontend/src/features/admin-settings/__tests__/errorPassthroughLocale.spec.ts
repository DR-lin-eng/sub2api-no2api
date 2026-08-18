import { describe, expect, it } from 'vitest'
import { errorPassthroughMatchModeLabel } from '@/features/admin-settings/presentation/errorPassthroughLocale'

describe.each([
  { name: 'English', messages: { 'admin.errorPassthrough.matchMode.any': 'Code OR Keyword', 'common.unknown': 'Unknown' } },
  { name: 'Chinese', messages: { 'admin.errorPassthrough.matchMode.any': '错误码 或 关键词', 'common.unknown': '未知' } },
])('error passthrough locale mapping - $name', ({ messages }) => {
  const t = (key: string) => messages[key as keyof typeof messages] ?? key

  it('maps known modes and localizes future values', () => {
    expect(errorPassthroughMatchModeLabel(t, 'any')).toBe(messages['admin.errorPassthrough.matchMode.any'])
    expect(errorPassthroughMatchModeLabel(t, 'future_mode')).toBe(messages['common.unknown'])
  })
})
