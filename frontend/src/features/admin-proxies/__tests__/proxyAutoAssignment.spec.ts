import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn()
}))

vi.mock('@/core/networks/client', () => ({ apiClient: { get, post, put } }))

import {
  getAutoAssignmentSettings,
  rebalanceAutoAssignments,
  updateAutoAssignmentSettings
} from '../data/datasources/adminProxiesDatasource'

const settings = {
  enabled: true,
  health_check_enabled: true,
  health_check_interval_minutes: 5,
  failure_threshold: 2
}

describe('proxy auto-assignment datasource', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  it('loads and updates the complete policy document', async () => {
    get.mockResolvedValue({ data: settings })
    await expect(getAutoAssignmentSettings()).resolves.toEqual(settings)
    expect(get).toHaveBeenCalledWith('/admin/proxies/auto-assignment')

    put.mockResolvedValue({ data: settings })
    await expect(updateAutoAssignmentSettings(settings)).resolves.toEqual(settings)
    expect(put).toHaveBeenCalledWith('/admin/proxies/auto-assignment', settings)
  })

  it('requests an immediate rebalance', async () => {
    post.mockResolvedValue({ data: { reassigned_accounts: 7 } })
    await expect(rebalanceAutoAssignments()).resolves.toEqual({ reassigned_accounts: 7 })
    expect(post).toHaveBeenCalledWith('/admin/proxies/auto-assignment/rebalance')
  })
})
