import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, put, post } = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/core/networks/client', () => ({
  apiClient: { get, put, post },
}))

import { getOverview, runInspection, updateSettings } from '../data/datasources/accountInspectionDatasource'

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
