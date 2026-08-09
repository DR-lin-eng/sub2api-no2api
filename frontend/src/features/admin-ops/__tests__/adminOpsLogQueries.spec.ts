import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn()
}))

vi.mock('@/core/networks/client', () => ({
  apiClient: { get, post, put }
}))

import opsAPI from '@/features/admin-ops/data/datasources/adminOpsDatasource'
import {
  cleanupSystemLogs,
  resetRuntimeLogConfig,
  updateRuntimeLogConfig
} from '@/features/admin-ops/data/datasources/opsLogActions'
import {
  getRuntimeLogConfig,
  getSystemLogSinkHealth,
  listRequestDetails,
  listSystemLogs
} from '@/features/admin-ops/data/datasources/opsLogQueries'

describe('admin ops log query and action owners', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  it('keeps the compatibility facade on the log owner function identities', () => {
    expect(opsAPI.listRequestDetails).toBe(listRequestDetails)
    expect(opsAPI.getRuntimeLogConfig).toBe(getRuntimeLogConfig)
    expect(opsAPI.updateRuntimeLogConfig).toBe(updateRuntimeLogConfig)
    expect(opsAPI.resetRuntimeLogConfig).toBe(resetRuntimeLogConfig)
    expect(opsAPI.listSystemLogs).toBe(listSystemLogs)
    expect(opsAPI.cleanupSystemLogs).toBe(cleanupSystemLogs)
    expect(opsAPI.getSystemLogSinkHealth).toBe(getSystemLogSinkHealth)
  })

  it('preserves request detail, system log, runtime config, and sink health queries', async () => {
    const response = { marker: 'logs' }
    const requestParams = {
      start_time: '2026-08-09T09:00:00Z',
      end_time: '2026-08-09T10:00:00Z',
      kind: 'success' as const,
      sort: 'ttft_desc' as const,
      ttft_only: true,
      platform: 'openai',
      group_id: 7,
      page: 2,
      page_size: 10
    }
    const logParams = {
      page: 3,
      page_size: 20,
      time_range: '1h' as const,
      host: 'api-node-1',
      request_id: 'req-1',
      platform: 'openai',
      q: 'timeout'
    }
    get.mockResolvedValue({ data: response })

    await expect(listRequestDetails(requestParams)).resolves.toBe(response)
    await expect(getRuntimeLogConfig()).resolves.toBe(response)
    await expect(listSystemLogs(logParams)).resolves.toBe(response)
    await expect(getSystemLogSinkHealth()).resolves.toBe(response)

    expect(get).toHaveBeenNthCalledWith(1, '/admin/ops/requests', { params: requestParams })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/ops/runtime/logging')
    expect(get).toHaveBeenNthCalledWith(3, '/admin/ops/system-logs', { params: logParams })
    expect(get).toHaveBeenNthCalledWith(4, '/admin/ops/system-logs/health')
  })

  it('preserves runtime config writes, reset, and explicit cleanup payloads', async () => {
    const response = { marker: 'actions' }
    const config = {
      level: 'info' as const,
      enable_sampling: false,
      sampling_initial: 100,
      sampling_thereafter: 100,
      caller: true,
      stacktrace_level: 'error' as const,
      retention_days: 30,
      redis_only: true
    }
    const filteredCleanup = {
      clear_all: false,
      start_time: '2026-08-09T09:00:00Z',
      host: 'api-node-1',
      q: 'timeout'
    }
    put.mockResolvedValue({ data: response })
    post.mockResolvedValue({ data: response })

    await expect(updateRuntimeLogConfig(config)).resolves.toBe(response)
    await expect(resetRuntimeLogConfig()).resolves.toBe(response)
    await expect(cleanupSystemLogs({ clear_all: true })).resolves.toBe(response)
    await expect(cleanupSystemLogs(filteredCleanup)).resolves.toBe(response)

    expect(put).toHaveBeenCalledWith('/admin/ops/runtime/logging', config)
    expect(post).toHaveBeenNthCalledWith(1, '/admin/ops/runtime/logging/reset')
    expect(post).toHaveBeenNthCalledWith(2, '/admin/ops/system-logs/cleanup', { clear_all: true })
    expect(post).toHaveBeenNthCalledWith(3, '/admin/ops/system-logs/cleanup', filteredCleanup)
  })
})
