import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn()
}))

vi.mock('@/core/networks/client', () => ({
  apiClient: { get, post }
}))

import {
  getBatchTodayStats,
  getAvailableModels,
  getAntigravityDefaultModelMapping,
  getStats,
  getTempUnschedulableStatus,
  getUsage,
  list,
  listWithEtag,
  previewFromCrs
} from '@/features/admin-accounts/data/datasources/adminAccountQueries'
import {
  accountsAPI,
  getAvailableModels as getAvailableModelsFromFacade,
  getAntigravityDefaultModelMapping as getAntigravityDefaultModelMappingFromFacade,
  getStats as getStatsFromFacade,
  getTempUnschedulableStatus as getTempUnschedulableStatusFromFacade,
  getUsage as getUsageFromFacade,
  previewFromCrs as previewFromCrsFromFacade
} from '@/features/admin-accounts/data/datasources/adminAccountsDatasource'

describe('admin account queries', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('forwards list filters and AbortSignal without changing query parameters', async () => {
    const controller = new AbortController()
    const response = { items: [], total: 0, page: 2, page_size: 50 }
    get.mockResolvedValueOnce({ data: response })

    await expect(list(2, 50, {
      platform: 'openai',
      oauth_quota: 'exhausted',
      include_scheduler_score: 'true',
      sort_by: 'name',
      sort_order: 'asc'
    }, { signal: controller.signal })).resolves.toEqual(response)

    expect(get).toHaveBeenCalledWith('/admin/accounts', {
      params: {
        page: 2,
        page_size: 50,
        platform: 'openai',
        oauth_quota: 'exhausted',
        include_scheduler_score: 'true',
        sort_by: 'name',
        sort_order: 'asc'
      },
      signal: controller.signal
    })
  })

  it('preserves ETag revalidation and the 304 response contract', async () => {
    const controller = new AbortController()
    get.mockResolvedValueOnce({ status: 304, headers: { etag: '"accounts-v2"' }, data: '' })

    await expect(listWithEtag(3, 25, { status: 'active' }, {
      etag: '"accounts-v2"',
      signal: controller.signal
    })).resolves.toEqual({
      notModified: true,
      etag: '"accounts-v2"',
      data: null
    })

    expect(get).toHaveBeenCalledWith('/admin/accounts', expect.objectContaining({
      params: { page: 3, page_size: 25, status: 'active' },
      headers: { 'If-None-Match': '"accounts-v2"' },
      signal: controller.signal,
      validateStatus: expect.any(Function)
    }))
    const validateStatus = get.mock.calls[0][1].validateStatus
    expect(validateStatus(200)).toBe(true)
    expect(validateStatus(304)).toBe(true)
    expect(validateStatus(500)).toBe(false)
  })

  it('keeps the batch today-stat payload shape', async () => {
    const response = { stats: { '7': { requests: 2 } } }
    post.mockResolvedValueOnce({ data: response })

    await expect(getBatchTodayStats([7, 11])).resolves.toEqual(response)
    expect(post).toHaveBeenCalledWith('/admin/accounts/today-stats/batch', {
      account_ids: [7, 11]
    })
  })

  it('preserves account stats and usage query parameters', async () => {
    const stats = { history: [] }
    const passiveUsage = { source: 'passive' }
    const activeUsage = { source: 'active' }
    get
      .mockResolvedValueOnce({ data: stats })
      .mockResolvedValueOnce({ data: passiveUsage })
      .mockResolvedValueOnce({ data: activeUsage })

    await expect(getStats(7, 14)).resolves.toEqual(stats)
    await expect(getUsage(7)).resolves.toEqual(passiveUsage)
    await expect(getUsage(7, 'active', true)).resolves.toEqual(activeUsage)

    expect(get).toHaveBeenNthCalledWith(1, '/admin/accounts/7/stats', {
      params: { days: 14 }
    })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/accounts/7/usage', {
      params: undefined
    })
    expect(get).toHaveBeenNthCalledWith(3, '/admin/accounts/7/usage', {
      params: { source: 'active', force: 'true' }
    })
  })

  it('keeps the available-model query owned by the feature datasource', async () => {
    const models = [{ id: 'gpt-4.1', display_name: 'GPT-4.1' }]
    get.mockResolvedValueOnce({ data: models })

    await expect(getAvailableModels(42)).resolves.toEqual(models)
    expect(get).toHaveBeenCalledWith('/admin/accounts/42/models')
    expect(getAvailableModelsFromFacade).toBe(getAvailableModels)
    expect(accountsAPI.getAvailableModels).toBe(getAvailableModels)
  })

  it('preserves the temporary unschedulable status endpoint', async () => {
    const response = { active: false, state: null }
    get.mockResolvedValueOnce({ data: response })

    await expect(getTempUnschedulableStatus(19)).resolves.toEqual(response)
    expect(get).toHaveBeenCalledWith('/admin/accounts/19/temp-unschedulable')
  })

  it('keeps the Antigravity default mapping query on its read-only owner', async () => {
    const mapping = { 'gemini-3.1-pro': 'gemini-3.1-pro-preview' }
    get.mockResolvedValueOnce({ data: mapping })

    await expect(getAntigravityDefaultModelMapping()).resolves.toEqual(mapping)
    expect(get).toHaveBeenCalledWith('/admin/accounts/antigravity/default-model-mapping')
    expect(getAntigravityDefaultModelMappingFromFacade).toBe(getAntigravityDefaultModelMapping)
    expect(accountsAPI.getAntigravityDefaultModelMapping).toBe(getAntigravityDefaultModelMapping)
  })

  it('keeps CRS preview on the query owner without changing credentials', async () => {
    const params = {
      base_url: 'https://crs.example.com',
      username: 'admin',
      password: 'secret'
    }
    const preview = {
      new_accounts: [{
        crs_account_id: 'crs-1',
        kind: 'openai',
        name: 'OpenAI account',
        platform: 'openai',
        type: 'oauth'
      }],
      existing_accounts: []
    }
    post.mockResolvedValueOnce({ data: preview })

    await expect(previewFromCrs(params)).resolves.toEqual(preview)
    expect(post).toHaveBeenCalledWith('/admin/accounts/sync/crs/preview', params)
    expect(previewFromCrsFromFacade).toBe(previewFromCrs)
    expect(accountsAPI.previewFromCrs).toBe(previewFromCrs)
  })

  it('keeps compatibility facade query exports on the same function identities', () => {
    expect(getStatsFromFacade).toBe(getStats)
    expect(getUsageFromFacade).toBe(getUsage)
    expect(getTempUnschedulableStatusFromFacade).toBe(getTempUnschedulableStatus)
    expect(accountsAPI.getStats).toBe(getStats)
    expect(accountsAPI.getUsage).toBe(getUsage)
    expect(accountsAPI.getTempUnschedulableStatus).toBe(getTempUnschedulableStatus)
  })
})
