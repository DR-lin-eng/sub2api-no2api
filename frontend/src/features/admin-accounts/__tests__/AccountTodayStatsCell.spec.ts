import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountTodayStatsCell from '@/features/admin-accounts/presentation/widgets/AccountTodayStatsCell.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

describe('AccountTodayStatsCell', () => {
  it('shows upstream_errors only when greater than zero', () => {
    const withErrors = mount(AccountTodayStatsCell, {
      props: {
        stats: {
          requests: 12,
          tokens: 1500,
          cost: 0.25,
          user_cost: 0.3,
          upstream_errors: 4,
        },
      },
    })
    expect(withErrors.text()).toContain('admin.accounts.stats.upstreamErrors:')
    expect(withErrors.text()).toContain('4')

    const zeroErrors = mount(AccountTodayStatsCell, {
      props: {
        stats: {
          requests: 12,
          tokens: 1500,
          cost: 0.25,
          upstream_errors: 0,
        },
      },
    })
    expect(zeroErrors.text()).not.toContain('admin.accounts.stats.upstreamErrors')
  })
})
