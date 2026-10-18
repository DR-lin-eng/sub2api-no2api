import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpsOAuthPlanExpiryDistributionCard from '../presentation/widgets/OpsOAuthPlanExpiryDistributionCard.vue'

const mockGetOAuthPlanExpiryDistribution = vi.fn()

vi.mock('@/features/admin-ops/data/datasources/opsMetricsQueries', () => ({
  getOAuthPlanExpiryDistribution: (...args: any[]) => mockGetOAuthPlanExpiryDistribution(...args)
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      if (key.endsWith('.updatedAt')) return `updated ${String(params?.value ?? '')}`
      return key
    }
  })
}))

vi.mock('@/core/utils/format', () => ({
  formatDateTime: (value: string) => value
}))

const EmptyStateStub = {
  props: ['title', 'description'],
  template: '<div class="empty-state">{{ title }}|{{ description }}</div>'
}

const response = {
  generated_at: '2026-10-18T00:00:00Z',
  platform: 'openai',
  group_id: null,
  total_oauth_accounts: 3,
  accounts_with_expiry: 2,
  accounts_without_expiry: 1,
  buckets: [
    { key: 'expired', count: 1 },
    { key: 'up_to_1', count: 1 },
    { key: 'up_to_2', count: 0 }
  ],
  plans: [{ plan_type: 'plus', count: 3 }]
}

describe('OpsOAuthPlanExpiryDistributionCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetOAuthPlanExpiryDistribution.mockResolvedValue(response)
  })

  it('loads the OAuth distribution with the dashboard scope and renders buckets', async () => {
    const wrapper = mount(OpsOAuthPlanExpiryDistributionCard, {
      props: { platformFilter: 'openai', groupIdFilter: 7, refreshToken: 1 },
      global: { stubs: { EmptyState: EmptyStateStub } }
    })
    await flushPromises()

    expect(mockGetOAuthPlanExpiryDistribution).toHaveBeenCalledWith('openai', 7)
    expect(wrapper.text()).toContain('expired')
    expect(wrapper.text()).toContain('up_to_1')
    expect(wrapper.text()).toContain('admin.ops.oauthPlanExpiry.planBreakdown')
  })

  it('shows an empty state when no OAuth accounts match', async () => {
    mockGetOAuthPlanExpiryDistribution.mockResolvedValueOnce({ ...response, total_oauth_accounts: 0, buckets: [], plans: [] })
    const wrapper = mount(OpsOAuthPlanExpiryDistributionCard, {
      props: { refreshToken: 0 },
      global: { stubs: { EmptyState: EmptyStateStub } }
    })
    await flushPromises()

    expect(wrapper.find('.empty-state').exists()).toBe(true)
  })
})
