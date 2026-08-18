import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, deleteRequest } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  deleteRequest: vi.fn(),
}))

vi.mock('@/core/networks/client', () => ({
  apiClient: {
    get,
    post,
    put,
    delete: deleteRequest,
  },
}))

import usersAPI, {
  batchUpdateLimits,
  bindUserAuthIdentity,
  deleteUser,
  getBatchPlatformQuotas,
  getUserBalanceHistory,
  list,
  resetPlatformQuotaWindow,
  updatePlatformQuotas,
} from '@/features/admin-users/data/datasources/adminUsersDatasource'
import type {
  AdminBindAuthIdentityRequest,
  AdminBoundAuthIdentity,
  BatchUpdateUserLimitsRequest,
  BatchUpdateUserLimitsResponse,
  PlatformQuotaUpdateItem,
} from '@/features/admin-users/data/dtos/adminUserDtos'

type Assert<T extends true> = T
type IsExact<T, U> = (
  (<G>() => G extends T ? 1 : 2) extends (<G>() => G extends U ? 1 : 2)
    ? ((<G>() => G extends U ? 1 : 2) extends (<G>() => G extends T ? 1 : 2) ? true : false)
    : false
)

type ExpectedAdminBindAuthIdentityRequest = {
  provider_type: string
  provider_key: string
  provider_subject: string
  issuer?: string
  metadata?: Record<string, unknown>
  channel?: {
    channel: string
    channel_app_id: string
    channel_subject: string
    metadata?: Record<string, unknown>
  }
}

type ExpectedAdminBoundAuthIdentity = {
  user_id: number
  provider_type: string
  provider_key: string
  provider_subject: string
  verified_at?: string | null
  issuer?: string | null
  metadata: Record<string, unknown> | null
  created_at: string
  updated_at: string
  channel?: {
    channel: string
    channel_app_id: string
    channel_subject: string
    metadata: Record<string, unknown> | null
    created_at: string
    updated_at: string
  } | null
}

const requestContractExact: Assert<
  IsExact<AdminBindAuthIdentityRequest, ExpectedAdminBindAuthIdentityRequest>
> = true
const responseContractExact: Assert<
  IsExact<AdminBoundAuthIdentity, ExpectedAdminBoundAuthIdentity>
> = true
const batchRequestContractExact: Assert<
  IsExact<
    BatchUpdateUserLimitsRequest,
    {
      user_ids: number[]
      all?: boolean
      concurrency?: number
      rpm_limit?: number
      scheduling_tier?: 0 | 1 | 2
    }
  >
> = true
const batchResponseContractExact: Assert<
  IsExact<BatchUpdateUserLimitsResponse, { affected: number }>
> = true

describe('admin users api auth identity binding', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
    deleteRequest.mockReset()
  })

  it('posts the backend-compatible auth identity bind payload and returns the backend response shape', async () => {
    const payload: AdminBindAuthIdentityRequest = {
      provider_type: 'wechat',
      provider_key: 'wechat-main',
      provider_subject: 'union-123',
      metadata: { source: 'admin-repair' },
      channel: {
        channel: 'open',
        channel_app_id: 'wx-open',
        channel_subject: 'openid-123',
        metadata: { scene: 'migration' },
      },
    }

    const response: AdminBoundAuthIdentity = {
      user_id: 9,
      provider_type: 'wechat',
      provider_key: 'wechat-main',
      provider_subject: 'union-123',
      verified_at: '2026-04-22T00:00:00Z',
      issuer: null,
      metadata: { source: 'admin-repair' },
      created_at: '2026-04-22T00:00:00Z',
      updated_at: '2026-04-22T00:00:00Z',
      channel: {
        channel: 'open',
        channel_app_id: 'wx-open',
        channel_subject: 'openid-123',
        metadata: { scene: 'migration' },
        created_at: '2026-04-22T00:00:00Z',
        updated_at: '2026-04-22T00:00:00Z',
      },
    }
    post.mockResolvedValue({ data: response })

    const result = await bindUserAuthIdentity(9, payload)

    expect(post).toHaveBeenCalledWith('/admin/users/9/auth-identities', payload)
    expect(result).toEqual(response)
  })

  it('keeps bind auth identity request and response types aligned with the backend contract', () => {
    expect(requestContractExact).toBe(true)
    expect(responseContractExact).toBe(true)
  })

  it('posts batch limit updates once with only the supplied limit fields', async () => {
    const request: BatchUpdateUserLimitsRequest = {
      user_ids: [4, 7],
      all: false,
      rpm_limit: 0,
    }
    post.mockResolvedValue({ data: { affected: 2 } satisfies BatchUpdateUserLimitsResponse })

    const result = await batchUpdateLimits(request)

    expect(post).toHaveBeenCalledWith('/admin/users/batch-limits', request)
    expect(result).toEqual({ affected: 2 })
    expect(batchRequestContractExact).toBe(true)
    expect(batchResponseContractExact).toBe(true)
  })

  it('preserves priority tier zero in list filters', async () => {
    get.mockResolvedValue({ data: { items: [], total: 0, page: 1, page_size: 20 } })

    await list(1, 20, { scheduling_tier: 0 })

    expect(get).toHaveBeenCalledWith('/admin/users', expect.objectContaining({
      params: expect.objectContaining({ scheduling_tier: 0 })
    }))
  })

  it('preserves priority tier zero in batch updates', async () => {
    const request: BatchUpdateUserLimitsRequest = {
      user_ids: [4, 7],
      scheduling_tier: 0,
    }
    post.mockResolvedValue({ data: { affected: 2 } satisfies BatchUpdateUserLimitsResponse })

    await batchUpdateLimits(request)

    expect(post).toHaveBeenCalledWith('/admin/users/batch-limits', request)
  })

  it('keeps list filters, attribute query keys, and cancellation signal intact', async () => {
    const controller = new AbortController()
    get.mockResolvedValue({ data: { items: [], total: 0, page: 2, page_size: 50 } })

    await list(
      2,
      50,
      {
        status: 'active',
        role: 'user',
        search: 'member',
        attributes: { 3: 'gold', 4: '' },
        include_subscriptions: true,
      },
      { signal: controller.signal },
    )

    expect(get).toHaveBeenCalledWith('/admin/users', {
      params: expect.objectContaining({
        page: 2,
        page_size: 50,
        status: 'active',
        role: 'user',
        search: 'member',
        include_subscriptions: true,
        'attr[3]': 'gold',
      }),
      signal: controller.signal,
    })
    expect(get.mock.calls[0][1].params).not.toHaveProperty('attr[4]')
  })

  it('preserves balance history pagination and type filtering', async () => {
    const response = {
      items: [],
      total: 0,
      page: 3,
      page_size: 15,
      total_recharged: 12.5,
    }
    get.mockResolvedValue({ data: response })

    await expect(getUserBalanceHistory(9, 3, 15, 'admin_balance')).resolves.toEqual(response)
    expect(get).toHaveBeenCalledWith('/admin/users/9/balance-history', {
      params: { page: 3, page_size: 15, type: 'admin_balance' },
    })
  })

  it('preserves platform quota batch, update, and reset payloads', async () => {
    const batchResponse = { platform_quotas: { 9: [] } }
    const quotaResponse = { platform_quotas: [] }
    const quotas: PlatformQuotaUpdateItem[] = [{
      platform: 'openai',
      daily_limit_usd: 5,
      weekly_limit_usd: null,
      monthly_limit_usd: 50,
    }]
    post.mockResolvedValueOnce({ data: batchResponse }).mockResolvedValueOnce({ data: quotaResponse })
    put.mockResolvedValue({ data: quotaResponse })

    await expect(getBatchPlatformQuotas([9, 11])).resolves.toEqual(batchResponse)
    expect(post).toHaveBeenNthCalledWith(1, '/admin/users/platform-quotas/batch', {
      user_ids: [9, 11],
    })

    await expect(updatePlatformQuotas(9, quotas)).resolves.toEqual(quotaResponse)
    expect(put).toHaveBeenCalledWith('/admin/users/9/platform-quotas', { quotas })

    await expect(resetPlatformQuotaWindow(9, 'openai', 'weekly')).resolves.toEqual(quotaResponse)
    expect(post).toHaveBeenNthCalledWith(2, '/admin/users/9/platform-quotas/reset', {
      platform: 'openai',
      window: 'weekly',
    })
  })

  it('keeps the compatibility facade wired to the named owner functions', async () => {
    deleteRequest.mockResolvedValue({ data: { message: 'deleted' } })

    expect(usersAPI.list).toBe(list)
    expect(usersAPI.batchUpdateLimits).toBe(batchUpdateLimits)
    expect(usersAPI.delete).toBe(deleteUser)
    await expect(usersAPI.delete(9)).resolves.toEqual({ message: 'deleted' })
    expect(deleteRequest).toHaveBeenCalledWith('/admin/users/9')
  })
})
