import { beforeEach, describe, expect, it, vi } from 'vitest'

const { deleteRequest, get, post, put } = vi.hoisted(() => ({
  deleteRequest: vi.fn(),
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
}))

vi.mock('@/core/networks/client', () => ({
  apiClient: {
    delete: deleteRequest,
    get,
    post,
    put,
  },
}))

import groupsAPI, {
  batchSetGroupRateMultipliers as facadeBatchSetGroupRateMultipliers,
  list as facadeList,
} from '@/features/admin-groups/data/datasources/adminGroupsDatasource'
import {
  batchSetGroupRateMultipliers,
  batchSetGroupRPMOverrides,
  clearGroupRPMOverrides,
  createCompositeRoute,
  updateCompositeRoute,
} from '@/features/admin-groups/data/datasources/adminGroupActions'
import {
  getGroupRPMOverrides,
  list,
  listCompositeRoutes,
  previewCompositeRoute,
} from '@/features/admin-groups/data/datasources/adminGroupQueries'

describe('admin group protocol owners', () => {
  beforeEach(() => {
    deleteRequest.mockReset()
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  it('keeps list filters, pagination, and cancellation on the query owner', async () => {
    const response = { items: [], total: 0, page: 2, page_size: 25, pages: 0 }
    const controller = new AbortController()
    get.mockResolvedValueOnce({ data: response })

    await expect(
      list(
        2,
        25,
        {
          platform: 'openai',
          status: 'inactive',
          is_exclusive: false,
          search: 'image',
          sort_by: 'name',
          sort_order: 'asc',
        },
        { signal: controller.signal },
      ),
    ).resolves.toEqual(response)

    expect(get).toHaveBeenCalledWith('/admin/groups', {
      params: {
        page: 2,
        page_size: 25,
        platform: 'openai',
        status: 'inactive',
        is_exclusive: false,
        search: 'image',
        sort_by: 'name',
        sort_order: 'asc',
      },
      signal: controller.signal,
    })
  })

  it('keeps composite route reads, writes, and preview paths distinct', async () => {
    const route = {
      id: 11,
      group_id: 7,
      public_model: 'gpt-image-*',
      match_type: 'prefix' as const,
      target_platform: 'openai' as const,
      upstream_model: '',
      endpoint: 'images' as const,
      priority: 10,
      enabled: true,
      notes: '',
    }
    const input = {
      public_model: route.public_model,
      match_type: route.match_type,
      target_platform: route.target_platform,
      upstream_model: route.upstream_model,
      endpoint: route.endpoint,
      priority: route.priority,
      enabled: route.enabled,
      notes: route.notes,
    }
    const decision = {
      matched: true,
      source: 'route',
      group_id: 7,
      public_model: 'gpt-image-1',
      target_platform: 'openai',
      upstream_model: 'gpt-image-1',
      endpoint: 'images',
      route,
    }
    get.mockResolvedValueOnce({ data: [route] })
    post
      .mockResolvedValueOnce({ data: route })
      .mockResolvedValueOnce({ data: decision })
    put.mockResolvedValueOnce({ data: route })

    await expect(listCompositeRoutes(7)).resolves.toEqual([route])
    await expect(createCompositeRoute(7, input)).resolves.toEqual(route)
    await expect(updateCompositeRoute(7, 11, input)).resolves.toEqual(route)
    await expect(
      previewCompositeRoute(7, { model: 'gpt-image-1', endpoint: 'images' }),
    ).resolves.toEqual(decision)

    expect(get).toHaveBeenCalledWith('/admin/groups/7/composite-routes')
    expect(post).toHaveBeenNthCalledWith(1, '/admin/groups/7/composite-routes', input)
    expect(put).toHaveBeenCalledWith('/admin/groups/7/composite-routes/11', input)
    expect(post).toHaveBeenNthCalledWith(
      2,
      '/admin/groups/7/composite-routes/preview',
      { model: 'gpt-image-1', endpoint: 'images' },
    )
  })

  it('keeps rate and RPM payloads isolated while deriving RPM rows from the shared response', async () => {
    get.mockResolvedValueOnce({
      data: [
        {
          user_id: 1,
          user_name: 'Rate only',
          user_email: 'rate@example.com',
          user_notes: '',
          user_status: 'active',
          rate_multiplier: 0.8,
          rpm_override: null,
        },
        {
          user_id: 2,
          user_name: 'RPM user',
          user_email: 'rpm@example.com',
          user_notes: 'priority',
          user_status: 'active',
          rate_multiplier: null,
          rpm_override: 12,
        },
      ],
    })
    put.mockResolvedValue({ data: { message: 'ok' } })
    deleteRequest.mockResolvedValue({ data: { message: 'ok' } })

    await expect(getGroupRPMOverrides(9)).resolves.toEqual([
      {
        user_id: 2,
        user_name: 'RPM user',
        user_email: 'rpm@example.com',
        user_notes: 'priority',
        user_status: 'active',
        rpm_override: 12,
      },
    ])
    await batchSetGroupRateMultipliers(9, [{ user_id: 1, rate_multiplier: 0.75 }])
    await batchSetGroupRPMOverrides(9, [{ user_id: 2, rpm_override: 20 }])
    await clearGroupRPMOverrides(9)

    expect(get).toHaveBeenCalledWith('/admin/groups/9/rate-multipliers')
    expect(put).toHaveBeenNthCalledWith(1, '/admin/groups/9/rate-multipliers', {
      entries: [{ user_id: 1, rate_multiplier: 0.75 }],
    })
    expect(put).toHaveBeenNthCalledWith(2, '/admin/groups/9/rpm-overrides', {
      entries: [{ user_id: 2, rpm_override: 20 }],
    })
    expect(deleteRequest).toHaveBeenCalledWith('/admin/groups/9/rpm-overrides')
  })

  it('keeps the compatibility facade wired to the exact owner functions', () => {
    expect(facadeList).toBe(list)
    expect(facadeBatchSetGroupRateMultipliers).toBe(batchSetGroupRateMultipliers)
    expect(groupsAPI.list).toBe(list)
    expect(groupsAPI.batchSetGroupRateMultipliers).toBe(batchSetGroupRateMultipliers)
    expect(groupsAPI.createCompositeRoute).toBe(createCompositeRoute)
    expect(groupsAPI.previewCompositeRoute).toBe(previewCompositeRoute)
  })
})
