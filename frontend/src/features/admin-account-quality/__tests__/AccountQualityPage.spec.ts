import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getOverview, runQualityMonitoring, updateSettings, getAllGroups, showSuccess } = vi.hoisted(() => ({
  getOverview: vi.fn(),
  runQualityMonitoring: vi.fn(),
  updateSettings: vi.fn(),
  getAllGroups: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('../data/datasources/accountQualityDatasource', () => ({ getOverview, runQualityMonitoring, updateSettings }))
vi.mock('@/features/admin-groups/data/datasources/adminGroupQueries', () => ({ getAll: getAllGroups }))
vi.mock('@/core/stores/appStore', () => ({ useAppStore: () => ({ showSuccess }) }))
vi.mock('@/core/utils/apiError', () => ({ extractApiErrorMessage: (_error: unknown, fallback: string) => fallback }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import AccountQualityPage from '../presentation/pages/AccountQualityPage.vue'

describe('AccountQualityPage', () => {
  beforeEach(() => {
    getOverview.mockReset().mockResolvedValue({
      settings: { enabled: true, interval_minutes: 10, model: '', effort: 'medium', prompt: '', failure_threshold: 2, recovery_threshold: 2, degraded_group_id: null, source_group_id: null, max_concurrent: 4, min_confidence: 0.85, public_enabled: false },
      run: { status: 'succeeded', summary: { inspected: 1, passed: 1, degraded: 0, uncertain: 0, errors: 0, switched: 0 } },
      results: { items: [{ account_id: 1, name: 'quality-account', platform: 'openai', type: 'oauth', quality_status: 'healthy', observed_at: '2026-09-12T00:00:00Z' }], total: 1, page: 1, page_size: 50, pages: 1 },
    })
    getAllGroups.mockResolvedValue([])
    runQualityMonitoring.mockReset()
    updateSettings.mockReset()
    showSuccess.mockReset()
  })

  it('renders an independent quality overview', async () => {
    const wrapper = mount(AccountQualityPage, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: { template: '<span />' }, Toggle: { template: '<button />' } } } })
    await flushPromises()
    expect(getOverview).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('quality-account')
    expect(wrapper.text()).toContain('admin.accountQuality.title')
  })
})

