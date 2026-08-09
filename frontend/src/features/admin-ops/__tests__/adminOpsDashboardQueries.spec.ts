import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('@/core/networks/client', () => ({
  apiClient: { get }
}))

import opsAPI from '@/features/admin-ops/data/datasources/adminOpsDatasource'
import {
  getDashboardOverview,
  getDashboardSnapshotV2,
  getErrorDistribution,
  getErrorTrend,
  getLatencyHistogram,
  getSwitchTrend,
  getThroughputTrend
} from '@/features/admin-ops/data/datasources/opsDashboardQueries'
import {
  getAccountAvailabilityStats,
  getConcurrencySnapshot,
  getConcurrencyStats,
  getImageGenerationStats,
  getOpenAITokenStats,
  getRealtimeTrafficSummary,
  getUserConcurrencyStats,
  getUserUsageStats
} from '@/features/admin-ops/data/datasources/opsMetricsQueries'

describe('admin ops dashboard and metrics query owners', () => {
  beforeEach(() => {
    get.mockReset()
  })

  it('keeps the compatibility facade on the query owner function identities', () => {
    expect(opsAPI.getDashboardOverview).toBe(getDashboardOverview)
    expect(opsAPI.getDashboardSnapshotV2).toBe(getDashboardSnapshotV2)
    expect(opsAPI.getThroughputTrend).toBe(getThroughputTrend)
    expect(opsAPI.getSwitchTrend).toBe(getSwitchTrend)
    expect(opsAPI.getLatencyHistogram).toBe(getLatencyHistogram)
    expect(opsAPI.getErrorTrend).toBe(getErrorTrend)
    expect(opsAPI.getErrorDistribution).toBe(getErrorDistribution)
    expect(opsAPI.getConcurrencyStats).toBe(getConcurrencyStats)
    expect(opsAPI.getConcurrencySnapshot).toBe(getConcurrencySnapshot)
    expect(opsAPI.getUserConcurrencyStats).toBe(getUserConcurrencyStats)
    expect(opsAPI.getAccountAvailabilityStats).toBe(getAccountAvailabilityStats)
    expect(opsAPI.getRealtimeTrafficSummary).toBe(getRealtimeTrafficSummary)
    expect(opsAPI.getImageGenerationStats).toBe(getImageGenerationStats)
    expect(opsAPI.getOpenAITokenStats).toBe(getOpenAITokenStats)
    expect(opsAPI.getUserUsageStats).toBe(getUserUsageStats)
  })

  it('preserves dashboard paths, params, include flags, and abort signals', async () => {
    const response = { marker: 'dashboard' }
    const controller = new AbortController()
    const params = {
      time_range: '1h' as const,
      platform: 'openai',
      group_id: 7,
      mode: 'raw' as const
    }
    const snapshotParams = {
      ...params,
      include_throughput_trend: true,
      include_latency_histogram: false,
      include_error_trend: true,
      include_error_distribution: false,
      include_switch_count: true
    }
    const switchParams = { ...params, time_range: '5h' as const }
    get.mockResolvedValue({ data: response })

    await expect(getDashboardOverview(params, { signal: controller.signal })).resolves.toBe(response)
    await expect(getDashboardSnapshotV2(snapshotParams, { signal: controller.signal })).resolves.toBe(response)
    await expect(getThroughputTrend(params, { signal: controller.signal })).resolves.toBe(response)
    await expect(getSwitchTrend(switchParams, { signal: controller.signal })).resolves.toBe(response)
    await expect(getLatencyHistogram(params, { signal: controller.signal })).resolves.toBe(response)
    await expect(getErrorTrend(params, { signal: controller.signal })).resolves.toBe(response)
    await expect(getErrorDistribution(params, { signal: controller.signal })).resolves.toBe(response)

    const request = (path: string, requestParams: object) => [path, {
      params: requestParams,
      signal: controller.signal
    }]
    expect(get).toHaveBeenNthCalledWith(1, ...request('/admin/ops/dashboard/overview', params))
    expect(get).toHaveBeenNthCalledWith(2, ...request('/admin/ops/dashboard/snapshot-v2', snapshotParams))
    expect(get).toHaveBeenNthCalledWith(3, ...request('/admin/ops/dashboard/throughput-trend', params))
    expect(get).toHaveBeenNthCalledWith(4, ...request('/admin/ops/dashboard/switch-trend', switchParams))
    expect(get).toHaveBeenNthCalledWith(5, ...request('/admin/ops/dashboard/latency-histogram', params))
    expect(get).toHaveBeenNthCalledWith(6, ...request('/admin/ops/dashboard/error-trend', params))
    expect(get).toHaveBeenNthCalledWith(7, ...request('/admin/ops/dashboard/error-distribution', params))
  })

  it('preserves concurrency, realtime, and independent metrics query contracts', async () => {
    const response = { marker: 'metrics' }
    const controller = new AbortController()
    const imageParams = { time_range: '30m' as const, platform: 'openai', group_id: 7 }
    const tokenParams = { time_range: '30d' as const, page: 2, page_size: 20, top_n: 12 }
    const userParams = { time_range: '24h' as const, platform: 'anthropic', group_id: 8, page: 3 }
    get.mockResolvedValue({ data: response })

    await expect(getConcurrencyStats('openai', 7)).resolves.toBe(response)
    await expect(getUserConcurrencyStats()).resolves.toBe(response)
    await expect(getConcurrencySnapshot('openai', 7)).resolves.toBe(response)
    await expect(getAccountAvailabilityStats('', 0)).resolves.toBe(response)
    await expect(getRealtimeTrafficSummary('5min', 'anthropic', 8)).resolves.toBe(response)
    await expect(getImageGenerationStats(imageParams, { signal: controller.signal })).resolves.toBe(response)
    await expect(getOpenAITokenStats(tokenParams, { signal: controller.signal })).resolves.toBe(response)
    await expect(getUserUsageStats(userParams, { signal: controller.signal })).resolves.toBe(response)

    expect(get).toHaveBeenNthCalledWith(1, '/admin/ops/concurrency', {
      params: { platform: 'openai', group_id: 7 }
    })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/ops/user-concurrency')
    expect(get).toHaveBeenNthCalledWith(3, '/admin/ops/concurrency-snapshot', {
      params: { platform: 'openai', group_id: 7 }
    })
    expect(get).toHaveBeenNthCalledWith(4, '/admin/ops/account-availability', { params: {} })
    expect(get).toHaveBeenNthCalledWith(5, '/admin/ops/realtime-traffic', {
      params: { platform: 'anthropic', group_id: 8, window: '5min' }
    })
    expect(get).toHaveBeenNthCalledWith(6, '/admin/ops/dashboard/image-generation-stats', {
      params: imageParams,
      signal: controller.signal
    })
    expect(get).toHaveBeenNthCalledWith(7, '/admin/ops/dashboard/openai-token-stats', {
      params: tokenParams,
      signal: controller.signal
    })
    expect(get).toHaveBeenNthCalledWith(8, '/admin/ops/dashboard/user-usage-stats', {
      params: userParams,
      signal: controller.signal
    })
  })
})
