import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, put } = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn()
}))

vi.mock('@/core/networks/client', () => ({
  apiClient: { get, put }
}))

import opsAPI from '@/features/admin-ops/data/datasources/adminOpsDatasource'
import {
  updateErrorResolved,
  updateRequestErrorResolved,
  updateUpstreamErrorResolved
} from '@/features/admin-ops/data/datasources/opsErrorActions'
import {
  getErrorLogDetail,
  getRequestErrorDetail,
  getUpstreamErrorDetail,
  listErrorLogs,
  listRequestErrors,
  listRequestErrorUpstreamErrors,
  listUpstreamErrors
} from '@/features/admin-ops/data/datasources/opsErrorQueries'
import type { OpsErrorListQueryParams } from '@/features/admin-ops/data/dtos/opsErrorDtos'

describe('admin ops error query and action owners', () => {
  beforeEach(() => {
    get.mockReset()
    put.mockReset()
  })

  it('keeps the compatibility facade on the error owner function identities', () => {
    expect(opsAPI.listErrorLogs).toBe(listErrorLogs)
    expect(opsAPI.getErrorLogDetail).toBe(getErrorLogDetail)
    expect(opsAPI.updateErrorResolved).toBe(updateErrorResolved)
    expect(opsAPI.listRequestErrors).toBe(listRequestErrors)
    expect(opsAPI.listUpstreamErrors).toBe(listUpstreamErrors)
    expect(opsAPI.getRequestErrorDetail).toBe(getRequestErrorDetail)
    expect(opsAPI.getUpstreamErrorDetail).toBe(getUpstreamErrorDetail)
    expect(opsAPI.updateRequestErrorResolved).toBe(updateRequestErrorResolved)
    expect(opsAPI.updateUpstreamErrorResolved).toBe(updateUpstreamErrorResolved)
    expect(opsAPI.listRequestErrorUpstreamErrors).toBe(listRequestErrorUpstreamErrors)
  })

  it('preserves unified and split list query parameters', async () => {
    const response = { items: [], total: 0, pages: 0 }
    const params: OpsErrorListQueryParams = {
      page: 2,
      page_size: 25,
      start_time: '2026-08-09T09:00:00Z',
      end_time: '2026-08-09T10:00:00Z',
      platform: 'openai',
      group_id: 7,
      account_id: 11,
      user_id: 13,
      api_key_id: 17,
      model: 'gpt-5',
      phase: 'upstream',
      category: 'rate_limit',
      error_owner: 'provider',
      error_source: 'upstream_http',
      resolved: 'false',
      view: 'excluded',
      q: 'timeout',
      status_codes: '429',
      status_codes_other: '1',
      sort_by: 'status_code',
      sort_order: 'asc'
    }
    get.mockResolvedValue({ data: response })

    await expect(listErrorLogs(params)).resolves.toBe(response)
    await expect(listRequestErrors(params)).resolves.toBe(response)
    await expect(listUpstreamErrors(params)).resolves.toBe(response)

    expect(get).toHaveBeenNthCalledWith(1, '/admin/ops/errors', { params })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/ops/request-errors', { params })
    expect(get).toHaveBeenNthCalledWith(3, '/admin/ops/upstream-errors', { params })
  })

  it('preserves unified and split detail endpoints', async () => {
    const response = { id: 23 }
    get.mockResolvedValue({ data: response })

    await expect(getErrorLogDetail(23)).resolves.toBe(response)
    await expect(getRequestErrorDetail(29)).resolves.toBe(response)
    await expect(getUpstreamErrorDetail(31)).resolves.toBe(response)

    expect(get).toHaveBeenNthCalledWith(1, '/admin/ops/errors/23')
    expect(get).toHaveBeenNthCalledWith(2, '/admin/ops/request-errors/29')
    expect(get).toHaveBeenNthCalledWith(3, '/admin/ops/upstream-errors/31')
  })

  it('adds include_detail only for correlated upstream detail requests', async () => {
    const response = { items: [], total: 0, pages: 0 }
    const params: OpsErrorListQueryParams = { page: 1, page_size: 100, view: 'all' }
    get.mockResolvedValue({ data: response })

    await expect(
      listRequestErrorUpstreamErrors(37, params, { include_detail: true })
    ).resolves.toBe(response)
    await expect(listRequestErrorUpstreamErrors(41, params)).resolves.toBe(response)

    expect(params).toEqual({ page: 1, page_size: 100, view: 'all' })
    expect(get).toHaveBeenNthCalledWith(
      1,
      '/admin/ops/request-errors/37/upstream-errors',
      { params: { ...params, include_detail: '1' } }
    )
    expect(get).toHaveBeenNthCalledWith(
      2,
      '/admin/ops/request-errors/41/upstream-errors',
      { params }
    )
  })

  it('preserves resolved action paths and payloads', async () => {
    put.mockResolvedValue({ data: undefined })

    await expect(updateErrorResolved(43, true)).resolves.toBeUndefined()
    await expect(updateRequestErrorResolved(47, false)).resolves.toBeUndefined()
    await expect(updateUpstreamErrorResolved(53, true)).resolves.toBeUndefined()

    expect(put).toHaveBeenNthCalledWith(1, '/admin/ops/errors/43/resolve', { resolved: true })
    expect(put).toHaveBeenNthCalledWith(2, '/admin/ops/request-errors/47/resolve', { resolved: false })
    expect(put).toHaveBeenNthCalledWith(3, '/admin/ops/upstream-errors/53/resolve', { resolved: true })
  })
})
