import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { post } }))

import { getDashboardApiKeysPendingUsage, getDashboardApiKeysUsage } from '../usage'

describe('getDashboardApiKeysUsage', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({ data: { stats: {} } })
  })

  it('sends the selected inclusive date range with owned key IDs', async () => {
    await getDashboardApiKeysUsage([4, 9], {
      startDate: '2026-07-01',
      endDate: '2026-07-07'
    })

    expect(post).toHaveBeenCalledWith(
      '/usage/dashboard/api-keys-usage',
      expect.objectContaining({
        api_key_ids: [4, 9],
        start_date: '2026-07-01',
        end_date: '2026-07-07'
      }),
      { signal: undefined }
    )
  })

  it('uses the lightweight pending usage endpoint without a historical range', async () => {
    await getDashboardApiKeysPendingUsage([4, 9])

    expect(post).toHaveBeenCalledWith(
      '/usage/dashboard/api-keys-pending-usage',
      { api_key_ids: [4, 9] },
      { signal: undefined }
    )
  })
})
