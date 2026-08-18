import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/core/networks/client', () => ({
  apiClient: { get, post },
}))

import adminUsageAPI, {
  cancelCleanupTask,
  createCleanupTask,
  getStats,
  list,
  listCleanupTasks,
  searchApiKeys,
  searchUsers,
} from '@/features/admin-usage/data/datasources/adminUsageDatasource'

describe('admin usage protocol owner', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('keeps list filters, exact totals, sorting, and cancellation on the usage owner', async () => {
    const response = { items: [], total: 0, page: 2, page_size: 50, pages: 0 }
    const controller = new AbortController()
    const params = {
      page: 2,
      page_size: 50,
      exact_total: false,
      user_id: 7,
      request_type: 'stream' as const,
      upstream_model_mismatch: false,
      sort_by: 'created_at',
      sort_order: 'desc' as const,
    }
    get.mockResolvedValueOnce({ data: response })

    await expect(list(params, { signal: controller.signal })).resolves.toEqual(response)

    expect(get).toHaveBeenCalledWith('/admin/usage', {
      params,
      signal: controller.signal,
    })
  })

  it('keeps stats and search query mappings unchanged', async () => {
    const stats = { total_requests: 3, total_tokens: 12 }
    const users = [{ id: 7, email: 'user@example.com', deleted: false }]
    const apiKeys = [{ id: 9, name: 'primary', user_id: 7 }]
    get
      .mockResolvedValueOnce({ data: stats })
      .mockResolvedValueOnce({ data: users })
      .mockResolvedValueOnce({ data: apiKeys })

    await expect(
      getStats({ user_id: 7, upstream_model_mismatch: true, nocache: 1 }),
    ).resolves.toEqual(stats)
    await expect(searchUsers('user@example.com')).resolves.toEqual(users)
    await expect(searchApiKeys(7, 'primary')).resolves.toEqual(apiKeys)

    expect(get).toHaveBeenNthCalledWith(1, '/admin/usage/stats', {
      params: { user_id: 7, upstream_model_mismatch: true, nocache: 1 },
    })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/usage/search-users', {
      params: { q: 'user@example.com' },
    })
    expect(get).toHaveBeenNthCalledWith(3, '/admin/usage/search-api-keys', {
      params: { user_id: 7, q: 'primary' },
    })
  })

  it('keeps cleanup list, create, and cancel paths distinct', async () => {
    const controller = new AbortController()
    const task = {
      id: 5,
      status: 'pending',
      filters: { start_time: '2026-08-16T00:00:00Z', end_time: '2026-08-17T00:00:00Z' },
      created_by: 1,
      deleted_rows: 0,
      created_at: '2026-08-17T00:00:00Z',
      updated_at: '2026-08-17T00:00:00Z',
    }
    const payload = {
      start_date: '2026-08-16',
      end_date: '2026-08-17',
      user_id: 7,
      timezone: 'Asia/Shanghai',
    }
    get.mockResolvedValueOnce({ data: { items: [task], total: 1, page: 1, page_size: 5, pages: 1 } })
    post
      .mockResolvedValueOnce({ data: task })
      .mockResolvedValueOnce({ data: { id: 5, status: 'canceled' } })

    await listCleanupTasks({ page: 1, page_size: 5 }, { signal: controller.signal })
    await expect(createCleanupTask(payload)).resolves.toEqual(task)
    await expect(cancelCleanupTask(5)).resolves.toEqual({ id: 5, status: 'canceled' })

    expect(get).toHaveBeenCalledWith('/admin/usage/cleanup-tasks', {
      params: { page: 1, page_size: 5 },
      signal: controller.signal,
    })
    expect(post).toHaveBeenNthCalledWith(1, '/admin/usage/cleanup-tasks', payload)
    expect(post).toHaveBeenNthCalledWith(2, '/admin/usage/cleanup-tasks/5/cancel')
  })

  it('keeps the compatibility facade wired to the exact owner functions', () => {
    expect(adminUsageAPI.list).toBe(list)
    expect(adminUsageAPI.getStats).toBe(getStats)
    expect(adminUsageAPI.searchUsers).toBe(searchUsers)
    expect(adminUsageAPI.searchApiKeys).toBe(searchApiKeys)
    expect(adminUsageAPI.listCleanupTasks).toBe(listCleanupTasks)
    expect(adminUsageAPI.createCleanupTask).toBe(createCleanupTask)
    expect(adminUsageAPI.cancelCleanupTask).toBe(cancelCleanupTask)
  })
})
