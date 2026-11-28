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
      props: { stats },
      global: { stubs: { Icon: true, Teleport: true } }
    })

    try {
      expect(wrapper.text()).toContain('Cache: 34')
      const breakdown = wrapper.get('[data-testid="cache-breakdown-tooltip"]').text()
      expect(breakdown).toContain('Cache Token Breakdown')
      expect(breakdown).toContain('Cache Creation')
      expect(breakdown).toContain('12')
      expect(breakdown).toContain('Cache Read')
      expect(breakdown).toContain('22')
    } finally {
      wrapper.unmount()
    }
  })

  it('opens the cache breakdown for keyboard users', async () => {
    const wrapper = mount(UsageStatsCards, {
      props: { stats },
      attachTo: document.body,
      global: { stubs: { Teleport: true } }
    })

    try {
      const tooltip = wrapper.get('[role="tooltip"]').element as HTMLElement
      expect(tooltip.style.display).toBe('none')
      await wrapper.get('.group').trigger('focusin')
      expect((wrapper.get('[role="tooltip"]').element as HTMLElement).style.display).not.toBe('none')
      expect(tooltip.textContent).toContain('Cache Read')
    } finally {
      wrapper.unmount()
    }
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
