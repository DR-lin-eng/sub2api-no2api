import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { Account } from '@/types'
import CNProviderUsageCell from '../presentation/widgets/CNProviderUsageCell.vue'

const { queryQuota, queryBalance } = vi.hoisted(() => ({
  queryQuota: vi.fn(),
  queryBalance: vi.fn(),
}))

vi.mock('../data/datasources/cnProviderDatasource', () => ({
  queryCNProviderQuota: queryQuota,
  queryCNProviderBalance: queryBalance,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const account = (overrides: Partial<Account>): Account => ({
  id: 1,
  name: 'provider',
  platform: 'kimi',
  type: 'apikey',
  credentials: { account_mode: 'coding' },
  extra: {},
  ...overrides,
} as Account)

const mountCell = (value: Account) => mount(CNProviderUsageCell, {
  props: { account: value },
  global: {
    stubs: {
      Icon: true,
      UsageProgressBar: {
        props: ['label', 'utilization', 'resetsAt'],
        template: '<div data-test="quota-tier">{{ label }}:{{ utilization }}:{{ resetsAt }}</div>',
      },
    },
  },
})

describe('CNProviderUsageCell', () => {
  beforeEach(() => {
    queryQuota.mockReset()
    queryBalance.mockReset()
  })

  it('renders the persisted coding-plan snapshot without probing on mount', () => {
    const wrapper = mountCell(account({
      extra: {
        kimi_5h_used_percent: 25,
        kimi_5h_reset_at: '2026-09-16T10:00:00Z',
        kimi_weekly_used_percent: 50,
      },
    }))

    expect(wrapper.findAll('[data-test="quota-tier"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('25')
    expect(wrapper.text()).toContain('50')
    expect(queryQuota).not.toHaveBeenCalled()
  })

  it('queries quota only after the administrator clicks refresh', async () => {
    queryQuota.mockResolvedValue({
      provider: 'opencode_go',
      success: true,
      credential_valid: true,
      fetched_at: 1,
      persisted: true,
      tiers: [{ window: 'monthly', used_percent: 22.2 }],
    })
    const wrapper = mountCell(account({
      platform: 'opencode_go',
      credentials: { account_mode: 'go' },
    }))

    expect(queryQuota).not.toHaveBeenCalled()
    await wrapper.get('button').trigger('click')
    await vi.waitFor(() => expect(queryQuota).toHaveBeenCalledWith(1))
    expect(wrapper.text()).toContain('22.2')
  })

  it('uses the balance endpoint for pay-as-you-go Kimi and preserves all currencies', async () => {
    queryBalance.mockResolvedValue({
      provider: 'kimi',
      success: true,
      balance: 3.5,
      currency: 'CNY',
      balances: [{ currency: 'CNY', balance: 3.5 }, { currency: 'USD', balance: 1.25 }],
      available: true,
      fetched_at: 1,
      persisted: true,
    })
    const wrapper = mountCell(account({ credentials: { account_mode: 'payg' } }))

    await wrapper.get('button').trigger('click')
    await vi.waitFor(() => expect(queryBalance).toHaveBeenCalledWith(1))
    expect(wrapper.text()).toContain('CNY 3.50')
    expect(wrapper.text()).toContain('USD 1.25')
  })

  it('does not offer a probe for combinations without a public endpoint', () => {
    const wrapper = mountCell(account({
      platform: 'zhipu',
      credentials: { account_mode: 'payg' },
    }))

    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.accounts.cnProvider.noPublicEndpoint')
  })

  it('does not expose the fixed Moonshot balance probe for a custom Kimi relay', () => {
    const wrapper = mountCell(account({
      credentials: {
        account_mode: 'payg',
        base_url: 'https://relay.example/v1',
      },
    }))

    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.accounts.cnProvider.noPublicEndpoint')
  })
})
