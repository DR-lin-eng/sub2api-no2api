import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, put, post } = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/core/networks/client', () => ({
  apiClient: { get, put, post },
}))

import { getOverview, normalizeAccountInspectionSettings, runInspection, updateSettings } from '../data/datasources/accountInspectionDatasource'

describe('account inspection datasource', () => {
  beforeEach(() => {
    get.mockReset()
    put.mockReset()
    post.mockReset()
  })

  it('loads filtered overview pages', async () => {
    get.mockResolvedValueOnce({ data: { results: { items: [] } } })
    const query = { page: 2, page_size: 25, status: 'flagged', type: 'apikey', search: 'key' }
    await getOverview(query)
    expect(get).toHaveBeenCalledWith('/admin/account-inspection', { params: query })
  })

  it('normalizes omitted or null reasons for healthy results', async () => {
    get.mockResolvedValueOnce({
      data: {
        results: {
          items: [
            { account_id: 1, reasons: undefined },
            { account_id: 2, reasons: null },
            { account_id: 3, reasons: ['success_rate_below_threshold', 42] },
          ],
        },
      },
    })

    const overview = await getOverview()

    expect(overview.results.items.map((item) => item.reasons)).toEqual([
      [],
      [],
      ['success_rate_below_threshold'],
    ])
  })

  it('maps legacy shared settings into independent type policies', () => {
    const settings = normalizeAccountInspectionSettings({
      auto_disable: false,
      min_requests: 4,
      ttft_threshold_ms: 1200,
      success_rate_threshold: 0.8,
      protected_account_ids: [9, 9, -1, 3.5],
    })

    expect(settings.oauth_auto_disable).toBe(false)
    expect(settings.api_key_auto_disable).toBe(false)
    expect(settings.oauth_min_requests).toBe(4)
    expect(settings.api_key_min_requests).toBe(4)
    expect(settings.oauth_ttft_threshold_ms).toBe(1200)
    expect(settings.api_key_ttft_threshold_ms).toBe(1200)
    expect(settings.oauth_success_rate_threshold).toBe(0.8)
    expect(settings.api_key_success_rate_threshold).toBe(0.8)
    expect(settings.protected_account_ids).toEqual([9])
  })

  it('persists settings and triggers a run', async () => {
    const settings = { enabled: true, interval_minutes: 60 } as any
    put.mockResolvedValueOnce({ data: settings })
    post.mockResolvedValueOnce({ data: { run: { status: 'succeeded' } } })
    await expect(updateSettings(settings)).resolves.toEqual(settings)
    await expect(runInspection()).resolves.toEqual({ run: { status: 'succeeded' } })
    expect(put).toHaveBeenCalledWith('/admin/account-inspection/settings', settings)
    expect(post).toHaveBeenCalledWith('/admin/account-inspection/run')
  })
})
