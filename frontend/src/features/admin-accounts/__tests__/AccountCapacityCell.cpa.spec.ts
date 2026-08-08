import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountCapacityCell from '@/features/admin-accounts/presentation/widgets/AccountCapacityCell.vue'
import CapacityBadge from '@/features/admin-accounts/presentation/widgets/CapacityBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params?.count === undefined ? key : `${key}:${params.count}`
    })
  }
})

function accountWithCapacity(
  state: 'fresh' | 'stale' | 'unavailable',
  available: number,
  capacity: number,
  effective: number,
  excludeAbnormal = false,
) {
  return {
    id: 1,
    type: 'apikey',
    concurrency: 99,
    current_concurrency: 0,
    credentials: { cpa_mode: true },
    cpa_capacity: {
      total_credentials: 4,
      enabled_credentials: 3,
      abnormal_credentials: 3 - available,
      available_credentials: available,
      capacity_credentials: capacity,
      effective_concurrency: effective,
      concurrency_per_credential: 10,
      exclude_abnormal_credentials: excludeAbnormal,
      state
    }
  } as any
}

describe('AccountCapacityCell CPA capacity', () => {
  it('shows fresh zero capacity when abnormal credential exclusion is enabled', () => {
    const wrapper = mount(AccountCapacityCell, { props: { account: accountWithCapacity('fresh', 0, 0, 0, true) } })

    expect(wrapper.getComponent(CapacityBadge).props('max')).toBe(0)
    expect(wrapper.get('[data-testid="cpa-capacity-credentials"]').text()).toContain('admin.accounts.capacity.cpaCapacityCredentials:0')
    expect(wrapper.get('[data-testid="cpa-capacity-credentials"]').attributes('title')).toBe('admin.accounts.capacity.cpaFresh')
  })

  it('shows enabled credentials as capacity when abnormal exclusion is off', () => {
    const wrapper = mount(AccountCapacityCell, { props: { account: accountWithCapacity('fresh', 0, 3, 30) } })

    expect(wrapper.getComponent(CapacityBadge).props('max')).toBe(30)
    expect(wrapper.get('[data-testid="cpa-capacity-credentials"]').text()).toContain('admin.accounts.capacity.cpaCapacityCredentials:3')
  })

  it('keeps stale capacity visible and marks unavailable capacity as unknown', () => {
    const stale = mount(AccountCapacityCell, { props: { account: accountWithCapacity('stale', 3, 3, 30) } })
    expect(stale.getComponent(CapacityBadge).props('max')).toBe(30)
    expect(stale.get('[data-testid="cpa-capacity-credentials"]').attributes('title')).toBe('admin.accounts.capacity.cpaStale')

    const unavailable = mount(AccountCapacityCell, { props: { account: accountWithCapacity('unavailable', 0, 0, 0) } })
    expect(unavailable.getComponent(CapacityBadge).props('max')).toBe(0)
    expect(unavailable.get('[data-testid="cpa-capacity-credentials"]').text()).toContain('admin.accounts.capacity.cpaCapacityCredentialsUnknown')
  })

  it('uses configured concurrency and hides the credential row outside CPA mode', () => {
    const account = accountWithCapacity('fresh', 2, 2, 20)
    delete account.cpa_capacity
    account.credentials = {}
    const wrapper = mount(AccountCapacityCell, { props: { account } })

    expect(wrapper.getComponent(CapacityBadge).props('max')).toBe(99)
    expect(wrapper.find('[data-testid="cpa-capacity-credentials"]').exists()).toBe(false)
  })
})
