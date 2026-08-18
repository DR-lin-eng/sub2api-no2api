import { describe, expect, it } from 'vitest'
import { userAttributeTypeLabel } from '@/features/admin-users/presentation/userAttributeLocale'

describe.each([
  { name: 'English', messages: { 'admin.users.attributes.types.textarea': 'Textarea', 'admin.users.attributes.types.multi_select': 'Multi-Select', 'common.unknown': 'Unknown' } },
  { name: 'Chinese', messages: { 'admin.users.attributes.types.textarea': '多行文本', 'admin.users.attributes.types.multi_select': '多选', 'common.unknown': '未知' } },
])('user attribute locale mapping - $name', ({ messages }) => {
  const t = (key: string) => messages[key as keyof typeof messages] ?? key

  it('maps known types and localizes future values', () => {
    expect(userAttributeTypeLabel(t, 'textarea')).toBe(messages['admin.users.attributes.types.textarea'])
    expect(userAttributeTypeLabel(t, 'multi_select')).toBe(messages['admin.users.attributes.types.multi_select'])
    expect(userAttributeTypeLabel(t, 'future_type')).toBe(messages['common.unknown'])
    expect(userAttributeTypeLabel(t, null)).toBe(messages['common.unknown'])
  })
})
