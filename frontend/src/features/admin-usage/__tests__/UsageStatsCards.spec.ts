import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageStatsCards from '@/features/admin-usage/presentation/widgets/UsageStatsCards.vue'

const messages: Record<string, string> = {
  'usage.totalRequests': 'Total Requests',
  'usage.inSelectedRange': 'in selected range',
  'usage.totalTokens': 'Total Tokens',
  'usage.in': 'In',
  'usage.out': 'Out',
  'usage.cacheTotal': 'Cache',
  'usage.cacheBreakdown': 'Cache Token Breakdown',
  'usage.cacheCreationTokensLabel': 'Cache Creation',
  'usage.cacheReadTokensLabel': 'Cache Read',
  'usage.totalCost': 'Total Cost',
  'usage.accountCost': 'Cost',
  'usage.standardCost': 'Standard',
  'usage.avgDuration': 'Avg Duration',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const stats = {
  total_requests: 1,
  total_input_tokens: 100,
  total_output_tokens: 50,
  total_cache_tokens: 34,
  total_cache_creation_tokens: 12,
  total_cache_read_tokens: 22,
  total_tokens: 184,
  total_cost: 0.001,
  total_actual_cost: 0.001,
  total_account_cost: 0.001,
  average_duration_ms: 250,
}

describe('UsageStatsCards', () => {
  it('shows cache token breakdown values', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats,
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    const text = wrapper.text()
    expect(text).toContain('Cache: 34')
    expect(text).toContain('Cache Token Breakdown')
    expect(text).toContain('Cache Creation')
    expect(text).toContain('12')
    expect(text).toContain('Cache Read')
    expect(text).toContain('22')
  })

  it('keeps the cache breakdown tooltip inside narrow viewports', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats,
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    const tooltip = wrapper.get('[data-testid="cache-breakdown-tooltip"]')
    expect(tooltip.classes()).toContain('right-0')
    expect(tooltip.classes()).toContain('sm:left-1/2')
    expect(tooltip.classes()).toContain('sm:-translate-x-1/2')
  })

  it('ignores injected account cost for user audiences', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        audience: 'user',
        stats,
        showAccountCost: true,
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    expect(wrapper.find('.text-orange-500').exists()).toBe(false)
    expect(wrapper.text()).toContain('Standard $0.0010')
  })

  it('keeps account cost visible for admin audiences', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        audience: 'admin',
        stats,
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    expect(wrapper.get('.text-orange-500').text()).toBe('Cost $0.0010')
  })
})
