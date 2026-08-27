import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, deleteRequest } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  deleteRequest: vi.fn()
}))

vi.mock('@/core/networks/client', () => ({
  apiClient: { get, post, put, delete: deleteRequest }
}))

import accountsAPI from '@/features/admin-accounts/data/datasources/adminAccountsDatasource'
import {
  batchRefresh,
  bulkUpdate,
  checkMixedChannelRisk,
  applyOAuthCredentials,
  clearAccountError,
  createAccount,
  createOpenAICodexPAT,
  deleteAccount,
  exportData,
  importCodexSession,
  importData,
  refreshOpenAIQuotaBatch,
  syncFromCrs,
  syncUpstreamModels,
  syncUpstreamModelsPreview,
  testCPAConnection,
  updateAccount
} from '@/features/admin-accounts/data/datasources/adminAccountActions'

describe('admin account actions', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
    deleteRequest.mockReset()
  })

  it('keeps bulk-update and delete payloads compatible', async () => {
    const result = { success: 2, failed: 0, success_ids: [7, 11], results: [] }
    post.mockResolvedValueOnce({ data: result })
    deleteRequest.mockResolvedValueOnce({ data: { message: 'deleted' } })

    await expect(bulkUpdate([7, 11], { schedulable: false })).resolves.toEqual(result)
    await expect(deleteAccount(7)).resolves.toEqual({ message: 'deleted' })

    expect(post).toHaveBeenCalledWith('/admin/accounts/bulk-update', {
      account_ids: [7, 11],
      schedulable: false
    })
    expect(deleteRequest).toHaveBeenCalledWith('/admin/accounts/7')
  })

  it('preserves batch-refresh timeout and export filter encoding', async () => {
    const batchResult = { total: 2, success: 2, failed: 0 }
    const exportResult = { accounts: [] }
    post.mockResolvedValueOnce({ data: batchResult })
    get.mockResolvedValueOnce({ data: exportResult })

    await expect(batchRefresh([7, 11])).resolves.toEqual(batchResult)
    await expect(exportData({
      filters: { platform: 'openai', status: 'active', sort_by: 'name', sort_order: 'asc' },
      includeProxies: false
    })).resolves.toEqual(exportResult)

    expect(post).toHaveBeenCalledWith('/admin/accounts/batch-refresh', {
      account_ids: [7, 11]
    }, {
      timeout: 120000
    })
    expect(get).toHaveBeenCalledWith('/admin/accounts/data', {
      params: {
        platform: 'openai',
        status: 'active',
        sort_by: 'name',
        sort_order: 'asc',
        include_proxies: 'false'
      }
    })
  })

  it('keeps the OpenAI quota batch endpoint bounded by the long action timeout', async () => {
    const result = {
      results: { '7': { fetched_at: 123, cache_persisted: true } },
      errors: {},
      skipped_account_ids: [11]
    }
    post.mockResolvedValueOnce({ data: result })

    await expect(refreshOpenAIQuotaBatch([7, 11])).resolves.toEqual(result)
    expect(post).toHaveBeenCalledWith('/admin/openai/accounts/quota/refresh/batch', {
      account_ids: [7, 11]
    }, {
      timeout: 180000
    })
  })

  it('keeps mixed-channel risk payload compatible', async () => {
    const result = { has_risk: true, message: 'mixed channel risk' }
    post.mockResolvedValueOnce({ data: result })

    await expect(checkMixedChannelRisk({
      platform: 'antigravity',
      group_ids: [3, 5]
    })).resolves.toEqual(result)

    expect(post).toHaveBeenCalledWith('/admin/accounts/check-mixed-channel', {
      platform: 'antigravity',
      group_ids: [3, 5]
    })
  })

  it('keeps account update and reauthorization endpoints compatible', async () => {
    const updatedAccount = { id: 7, status: 'active' }
    put.mockResolvedValueOnce({ data: updatedAccount })
    post
      .mockResolvedValueOnce({ data: updatedAccount })
      .mockResolvedValueOnce({ data: updatedAccount })

    await expect(updateAccount(7, {
      type: 'oauth',
      credentials: { access_token: 'access-token' },
      extra: { email_address: 'user@example.com' }
    })).resolves.toEqual(updatedAccount)
    await expect(applyOAuthCredentials(7, {
      type: 'oauth',
      credentials: { access_token: 'new-access-token' },
      extra: { email_address: 'user@example.com' }
    })).resolves.toEqual(updatedAccount)
    await expect(clearAccountError(7)).resolves.toEqual(updatedAccount)

    expect(put).toHaveBeenCalledWith('/admin/accounts/7', {
      type: 'oauth',
      credentials: { access_token: 'access-token' },
      extra: { email_address: 'user@example.com' }
    })
    expect(post).toHaveBeenNthCalledWith(1, '/admin/accounts/7/apply-oauth-credentials', {
      type: 'oauth',
      credentials: { access_token: 'new-access-token' },
      extra: { email_address: 'user@example.com' }
    })
    expect(post).toHaveBeenNthCalledWith(2, '/admin/accounts/7/clear-error')
  })

  it('keeps create and upstream model-sync payloads compatible', async () => {
    const createdAccount = { id: 7, platform: 'openai', type: 'apikey' }
    const models = { models: ['gpt-5.6'] }
    const createPayload = {
      name: 'OpenAI key',
      platform: 'openai',
      type: 'apikey',
      credentials: { api_key: 'sk-test' },
      concurrency: 1,
      priority: 1,
      rate_multiplier: 1,
      group_ids: []
    } as any
    const previewPayload = {
      platform: 'openai',
      type: 'apikey',
      base_url: 'https://api.openai.com',
      api_key: 'sk-test'
    }
    post
      .mockResolvedValueOnce({ data: createdAccount })
      .mockResolvedValueOnce({ data: models })
      .mockResolvedValueOnce({ data: models })

    await expect(createAccount(createPayload)).resolves.toEqual(createdAccount)
    await expect(syncUpstreamModels(7)).resolves.toEqual(models)
    await expect(syncUpstreamModelsPreview(previewPayload)).resolves.toEqual(models)

    expect(post).toHaveBeenNthCalledWith(1, '/admin/accounts', createPayload)
    expect(post).toHaveBeenNthCalledWith(2, '/admin/accounts/7/models/sync-upstream')
    expect(post).toHaveBeenNthCalledWith(3, '/admin/accounts/models/sync-upstream-preview', previewPayload)
  })

  it('preserves CPA test and Codex import endpoint contracts', async () => {
    const cpaResult = {
      total_credentials: 4,
      enabled_credentials: 3,
      abnormal_credentials: 1,
      available_credentials: 2,
      capacity_credentials: 3,
      effective_concurrency: 30,
      concurrency_per_credential: 10,
      exclude_abnormal_credentials: false,
      state: 'fresh',
      latency_ms: 12
    }
    const importResult = { created: 1, updated: 0, skipped: 0, failed: 0 }
    const patAccount = { id: 8, platform: 'openai', type: 'oauth' }
    const cpaPayload = {
      use_account_base_url: false,
      base_url: 'http://cpa:8317/v1',
      management_url: 'http://cpa:8317',
      management_password: 'secret',
      concurrency_per_credential: 10,
      exclude_abnormal_credentials: false
    }
    const importPayload = {
      content: '{"auth_mode":"agent_identity"}',
      name: 'Codex import',
      group_ids: [3],
      concurrency: 1,
      priority: 1,
      rate_multiplier: 1
    } as any
    const patPayload = {
      personal_access_token: 'pat-token',
      name: 'Codex PAT',
      group_ids: [3],
      concurrency: 1,
      priority: 1,
      rate_multiplier: 1
    } as any
    post
      .mockResolvedValueOnce({ data: cpaResult })
      .mockResolvedValueOnce({ data: importResult })
      .mockResolvedValueOnce({ data: patAccount })

    await expect(testCPAConnection(7, cpaPayload)).resolves.toEqual(cpaResult)
    await expect(importCodexSession(importPayload)).resolves.toEqual(importResult)
    await expect(createOpenAICodexPAT(patPayload)).resolves.toEqual(patAccount)

    expect(post).toHaveBeenNthCalledWith(1, '/admin/accounts/7/cpa/test', cpaPayload)
    expect(post).toHaveBeenNthCalledWith(2, '/admin/accounts/import/codex-session', importPayload, {
      timeout: 120000
    })
    expect(post).toHaveBeenNthCalledWith(3, '/admin/openai/create-from-codex-pat', patPayload)
  })

  it('preserves CRS sync timeout and account data import payloads', async () => {
    const syncResult = {
      created: 1,
      updated: 2,
      skipped: 0,
      failed: 0,
      items: []
    }
    const importResult = {
      proxy_created: 1,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 2,
      account_failed: 0
    }
    const syncPayload = {
      base_url: 'https://crs.example.com',
      username: 'admin',
      password: 'secret',
      sync_proxies: true,
      selected_account_ids: ['crs-1', 'crs-2']
    }
    const importPayload = {
      data: { proxies: [], accounts: [{ name: 'OpenAI key' }] } as any,
      skip_default_group_bind: true
    }
    post
      .mockResolvedValueOnce({ data: syncResult })
      .mockResolvedValueOnce({ data: importResult })

    await expect(syncFromCrs(syncPayload)).resolves.toEqual(syncResult)
    await expect(importData(importPayload)).resolves.toEqual(importResult)

    expect(post).toHaveBeenNthCalledWith(1, '/admin/accounts/sync/crs', syncPayload, {
      timeout: 180000
    })
    expect(post).toHaveBeenNthCalledWith(2, '/admin/accounts/data', {
      data: importPayload.data,
      skip_default_group_bind: true
    })
  })

  it('keeps the compatibility datasource wired to the same owner functions', () => {
    expect(accountsAPI.bulkUpdate).toBe(bulkUpdate)
    expect(accountsAPI.checkMixedChannelRisk).toBe(checkMixedChannelRisk)
    expect(accountsAPI.batchRefresh).toBe(batchRefresh)
    expect(accountsAPI.exportData).toBe(exportData)
    expect(accountsAPI.delete).toBe(deleteAccount)
    expect(accountsAPI.update).toBe(updateAccount)
    expect(accountsAPI.applyOAuthCredentials).toBe(applyOAuthCredentials)
    expect(accountsAPI.clearError).toBe(clearAccountError)
    expect(accountsAPI.create).toBe(createAccount)
    expect(accountsAPI.createOpenAICodexPAT).toBe(createOpenAICodexPAT)
    expect(accountsAPI.importCodexSession).toBe(importCodexSession)
    expect(accountsAPI.importData).toBe(importData)
    expect(accountsAPI.syncFromCrs).toBe(syncFromCrs)
    expect(accountsAPI.syncUpstreamModels).toBe(syncUpstreamModels)
    expect(accountsAPI.syncUpstreamModelsPreview).toBe(syncUpstreamModelsPreview)
    expect(accountsAPI.testCPAConnection).toBe(testCPAConnection)
  })
})
