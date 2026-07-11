import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

import type { DashboardStats } from '@/types'
import DashboardView from '../DashboardView.vue'

const { getSnapshotV2 } = vi.hoisted(() => ({
  getSnapshotV2: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    dashboard: {
      getSnapshotV2
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  })
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const RFC3339_RE = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[+-]\d{2}:\d{2}$/

const createDashboardStats = (): DashboardStats => ({
  total_users: 0,
  today_new_users: 0,
  active_users: 0,
  hourly_active_users: 0,
  stats_updated_at: '',
  stats_stale: false,
  total_api_keys: 0,
  active_api_keys: 0,
  total_accounts: 0,
  normal_accounts: 0,
  error_accounts: 0,
  ratelimit_accounts: 0,
  overload_accounts: 0,
  total_requests: 0,
  total_input_tokens: 0,
  total_output_tokens: 0,
  total_cache_creation_tokens: 0,
  total_cache_read_tokens: 0,
  total_tokens: 0,
  total_cost: 0,
  total_actual_cost: 0,
  today_requests: 0,
  today_input_tokens: 0,
  today_output_tokens: 0,
  today_cache_creation_tokens: 0,
  today_cache_read_tokens: 0,
  today_tokens: 0,
  today_cost: 0,
  today_actual_cost: 0,
  average_duration_ms: 0,
  uptime: 0,
  rpm: 0,
  tpm: 0
})

describe('admin DashboardView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())

    getSnapshotV2.mockReset()

    getSnapshotV2.mockResolvedValue({
      stats: createDashboardStats(),
      trend: [],
      models: [],
      users_trend: [],
      ranking: [],
      ranking_total_actual_cost: 0,
      ranking_total_requests: 0,
      ranking_total_tokens: 0
    })
  })

  it('uses last 24 hours as default dashboard range', async () => {
    mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          LoadingSpinner: true,
          Icon: true,
          DateRangePicker: true,
          Select: true,
          ModelDistributionChart: true,
          TokenUsageTrend: true,
          Line: true
        }
      }
    })

    await flushPromises()

    expect(getSnapshotV2).toHaveBeenCalledTimes(3)
    const snapshotParams = getSnapshotV2.mock.calls.map((call) => call[0])
    for (const params of snapshotParams) {
      expect(params.start_date).toMatch(RFC3339_RE)
      expect(params.end_date).toMatch(RFC3339_RE)
      expect(new Date(params.end_date).getTime() - new Date(params.start_date).getTime()).toBe(
        24 * 60 * 60 * 1000
      )
    }
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      granularity: 'hour',
      include_stats: true,
      include_trend: false,
      include_model_stats: false,
      include_users_trend: false,
      include_user_ranking: false
    }))
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      include_stats: false,
      include_trend: true,
      include_model_stats: true,
      include_users_trend: false,
      include_user_ranking: false
    }))
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      include_stats: false,
      include_trend: false,
      include_model_stats: false,
      include_users_trend: true,
      include_user_ranking: true
    }))
  })

  it('renders fast dashboard data without waiting for the Top 12 query', async () => {
    let resolveInsights: ((value: Record<string, unknown>) => void) | undefined
    const slowInsights = new Promise<Record<string, unknown>>((resolve) => {
      resolveInsights = resolve
    })
    getSnapshotV2.mockImplementation((params: { include_stats?: boolean; include_users_trend?: boolean }) => {
      if (params.include_stats) {
        return Promise.resolve({ stats: createDashboardStats() })
      }
      if (params.include_users_trend) {
        return slowInsights
      }
      return Promise.resolve({ trend: [], models: [] })
    })

    const wrapper = mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          LoadingSpinner: true,
          Icon: true,
          DateRangePicker: true,
          Select: true,
          ModelDistributionChart: true,
          TokenUsageTrend: true,
          Line: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('admin.dashboard.apiKeys')
    expect(getSnapshotV2).toHaveBeenCalledTimes(3)

    resolveInsights?.({
      users_trend: [],
      ranking: [],
      ranking_total_actual_cost: 0,
      ranking_total_requests: 0,
      ranking_total_tokens: 0
    })
    await flushPromises()
  })
})
