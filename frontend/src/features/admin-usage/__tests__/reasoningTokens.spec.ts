import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { AdminUsageLog } from '@/types'
import DataTable from '@/common/widgets/data/DataTable.vue'
import UsageTable from '../presentation/widgets/UsageTable.vue'
import UsageDetailDialog from '../presentation/widgets/UsageDetailDialog.vue'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key === 'usage.reasoningTokens' ? 'Reasoning tokens' : key }),
}))

const usage = (reasoning_tokens?: number | null) => ({
  id: 1, user_id: 1, api_key_id: 2, request_id: 'reasoning-request', model: 'reasoning-model',
  input_tokens: 20, output_tokens: 2000, reasoning_tokens,
  cache_creation_tokens: 0, cache_read_tokens: 0, cache_creation_5m_tokens: 0, cache_creation_1h_tokens: 0,
  input_cost: 0, output_cost: 0, total_cost: 0, actual_cost: 0,
  cache_creation_cost: 0, cache_read_cost: 0, rate_multiplier: 1,
  image_count: 0, image_input_tokens: 0, image_output_tokens: 0, billing_mode: 'token',
  created_at: '2026-10-18T00:00:00Z',
} as AdminUsageLog)

describe('reasoning token visibility and unknown values', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it.each([
    [1234, '1,234'],
    [0, '0'],
    [null, '-'],
    [undefined, '-'],
  ])('renders administrator table count %s as %s', (count, expected) => {
    const wrapper = mount(UsageTable, {
      props: {
        data: [usage(count)], audience: 'admin',
        columns: [{ key: 'reasoning_tokens', label: 'Reasoning tokens' }],
      },
      global: { stubs: { Icon: true, Teleport: true } },
    })
    console.info('reasoning-table', JSON.stringify({ count: count ?? null, text: wrapper.text() }))
    expect(wrapper.text()).toContain(expected)
    if (count === 1234) expect(wrapper.text()).not.toContain('2000')
    wrapper.unmount()
  })

  it('removes the reasoning column for a user audience even if supplied by its caller', () => {
    const wrapper = mount(UsageTable, {
      props: {
        data: [usage(1234)], audience: 'user',
        columns: [{ key: 'reasoning_tokens', label: 'Reasoning tokens' }],
      },
      global: { stubs: { Icon: true, Teleport: true } },
    })
    expect(wrapper.getComponent(DataTable).props('columns')).toEqual([])
    expect(wrapper.text()).not.toContain('1234')
    wrapper.unmount()
  })

  it.each([
    ['admin', 1234, '1,234'],
    ['admin', 0, '0'],
    ['admin', null, '-'],
    ['admin', undefined, '-'],
    ['user', 1234, null],
  ] as const)('renders detail for %s with count %s', (audience, count, expected) => {
    const wrapper = mount(UsageDetailDialog, {
      props: { show: true, usage: usage(count), audience },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, Icon: true } },
    })
    const field = wrapper.findAll('dl > div').find(item => item.find('dt').text() === 'Reasoning tokens')
    console.info('reasoning-detail', JSON.stringify({ audience, count: count ?? null, value: field?.find('dd').text() ?? null }))
    if (expected == null) expect(field).toBeUndefined()
    else expect(field?.find('dd').text()).toBe(expected)
    wrapper.unmount()
  })
})
