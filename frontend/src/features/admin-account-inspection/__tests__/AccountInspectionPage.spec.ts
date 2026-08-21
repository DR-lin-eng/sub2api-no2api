import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getOverview, runInspection, updateSettings, showSuccess } = vi.hoisted(() => ({
  getOverview: vi.fn(),
  runInspection: vi.fn(),
  updateSettings: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('../data/datasources/accountInspectionDatasource', () => ({
  getOverview,
  runInspection,
  updateSettings,
}))

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({ showSuccess }),
}))

vi.mock('@/core/utils/apiError', () => ({
  extractApiErrorMessage: (_error: unknown, fallback: string) => fallback,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import AccountInspectionPage from '../presentation/pages/AccountInspectionPage.vue'

const overviewWithHealthyResult = {
  settings: {
    enabled: true,
    interval_minutes: 60,
    auto_disable: false,
    lookback_minutes: 60,
    min_requests: 1,
    ttft_threshold_ms: 30_000,
    success_rate_threshold: 0.6,
    oauth_quota_check_enabled: true,
    api_key_quota_check_enabled: true,
    api_key_min_cache_hit_rate: 0,
    api_key_max_rate_multiplier: 0,
    api_key_min_remaining_quota: 0,
  },
  run: {
    status: 'succeeded',
    summary: {
      inspected: 1,
      healthy: 1,
      flagged: 0,
      disabled: 0,
      already_disabled: 0,
      oauth_accounts: 1,
      api_key_accounts: 0,
    },
  },
  results: {
    // Deliberately omit `reasons`, matching the legacy backend payload.
    items: [{
      account_id: 1,
      name: 'healthy-account',
      platform: 'openai',
      type: 'oauth',
      status: 'healthy',
      schedulable: true,
      action: 'none',
      total_requests: 2,
      successful_requests: 2,
      observed_at: '2026-08-21T00:00:00Z',
    }],
    total: 1,
    page: 1,
    page_size: 50,
    pages: 1,
  },
}

function mountPage(errors: unknown[]) {
  return mount(AccountInspectionPage, {
    global: {
      config: { errorHandler: (error: unknown) => errors.push(error) },
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: { template: '<span />' },
        Toggle: { template: '<button type="button" />' },
        Pagination: { template: '<div />' },
      },
    },
  })
}

describe('AccountInspectionPage', () => {
  beforeEach(() => {
    getOverview.mockReset()
    runInspection.mockReset()
    updateSettings.mockReset()
    showSuccess.mockReset()
    getOverview.mockResolvedValue(overviewWithHealthyResult)
  })

  it('keeps rendering when a healthy result omits reasons', async () => {
    const errors: unknown[] = []
    const wrapper = mountPage(errors)
    await flushPromises()

    expect(getOverview).toHaveBeenCalledTimes(1)
    expect(errors).toEqual([])
    expect(wrapper.text()).toContain('healthy-account')
    expect(wrapper.text()).toContain('admin.accountInspection.results.healthy')
  })
})
