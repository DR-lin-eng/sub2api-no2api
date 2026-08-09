import { apiClient } from '@/core/networks/client'
import type { OpsRequestOptions } from '@/features/admin-ops/data/dtos/opsDashboardDtos'
import type {
  OpsAccountAvailabilityStatsResponse,
  OpsConcurrencySnapshotResponse,
  OpsConcurrencyStatsResponse,
  OpsImageGenerationStatsParams,
  OpsImageGenerationStatsResponse,
  OpsOpenAITokenStatsParams,
  OpsOpenAITokenStatsResponse,
  OpsRealtimeTrafficSummaryResponse,
  OpsUserConcurrencyStatsResponse,
  OpsUserUsageStatsParams,
  OpsUserUsageStatsResponse
} from '@/features/admin-ops/data/dtos/opsMetricsDtos'

function buildConcurrencyParams(platform?: string, groupId?: number | null): Record<string, string | number> {
  const params: Record<string, string | number> = {}
  if (platform) params.platform = platform
  if (typeof groupId === 'number' && groupId > 0) params.group_id = groupId
  return params
}

export async function getConcurrencyStats(
  platform?: string,
  groupId?: number | null
): Promise<OpsConcurrencyStatsResponse> {
  const { data } = await apiClient.get<OpsConcurrencyStatsResponse>('/admin/ops/concurrency', {
    params: buildConcurrencyParams(platform, groupId)
  })
  return data
}

export async function getUserConcurrencyStats(): Promise<OpsUserConcurrencyStatsResponse> {
  const { data } = await apiClient.get<OpsUserConcurrencyStatsResponse>('/admin/ops/user-concurrency')
  return data
}

export async function getConcurrencySnapshot(
  platform?: string,
  groupId?: number | null
): Promise<OpsConcurrencySnapshotResponse> {
  const { data } = await apiClient.get<OpsConcurrencySnapshotResponse>('/admin/ops/concurrency-snapshot', {
    params: buildConcurrencyParams(platform, groupId)
  })
  return data
}

export async function getAccountAvailabilityStats(
  platform?: string,
  groupId?: number | null
): Promise<OpsAccountAvailabilityStatsResponse> {
  const { data } = await apiClient.get<OpsAccountAvailabilityStatsResponse>('/admin/ops/account-availability', {
    params: buildConcurrencyParams(platform, groupId)
  })
  return data
}

export async function getRealtimeTrafficSummary(
  window: string,
  platform?: string,
  groupId?: number | null
): Promise<OpsRealtimeTrafficSummaryResponse> {
  const params = buildConcurrencyParams(platform, groupId)
  params.window = window
  const { data } = await apiClient.get<OpsRealtimeTrafficSummaryResponse>('/admin/ops/realtime-traffic', { params })
  return data
}

export async function getImageGenerationStats(
  params: OpsImageGenerationStatsParams,
  options: OpsRequestOptions = {}
): Promise<OpsImageGenerationStatsResponse> {
  const { data } = await apiClient.get<OpsImageGenerationStatsResponse>('/admin/ops/dashboard/image-generation-stats', {
    params,
    signal: options.signal
  })
  return data
}

export async function getOpenAITokenStats(
  params: OpsOpenAITokenStatsParams,
  options: OpsRequestOptions = {}
): Promise<OpsOpenAITokenStatsResponse> {
  const { data } = await apiClient.get<OpsOpenAITokenStatsResponse>('/admin/ops/dashboard/openai-token-stats', {
    params,
    signal: options.signal
  })
  return data
}

export async function getUserUsageStats(
  params: OpsUserUsageStatsParams,
  options: OpsRequestOptions = {}
): Promise<OpsUserUsageStatsResponse> {
  const { data } = await apiClient.get<OpsUserUsageStatsResponse>('/admin/ops/dashboard/user-usage-stats', {
    params,
    signal: options.signal
  })
  return data
}
