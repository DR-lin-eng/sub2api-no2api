import { apiClient } from '@/core/networks/client'
import type { ApiKey, PaginatedResponse } from '@/types'
import type { GroupPlatform } from '@/types/group'
import type {
  AdminGroup,
  CompositeModelRoute,
  CompositeRouteDecision,
  CompositeRoutePreviewRequest,
  GroupCapacitySummary,
  GroupListFilters,
  GroupListOptions,
  GroupRateMultiplierEntry,
  GroupRPMOverrideEntry,
  GroupStats,
  GroupUsageSummary,
  LiveCapability,
  ModelDefaultPricing,
} from '../dtos/adminGroupDtos'

export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: GroupListFilters,
  options?: GroupListOptions,
): Promise<PaginatedResponse<AdminGroup>> {
  const { data } = await apiClient.get<PaginatedResponse<AdminGroup>>('/admin/groups', {
    params: {
      page,
      page_size: pageSize,
      ...filters,
    },
    signal: options?.signal,
  })
  return data
}

export async function getAll(platform?: GroupPlatform): Promise<AdminGroup[]> {
  const { data } = await apiClient.get<AdminGroup[]>('/admin/groups/all', {
    params: platform ? { platform } : undefined,
  })
  return data
}

export async function getAllIncludingInactive(): Promise<AdminGroup[]> {
  const { data } = await apiClient.get<AdminGroup[]>('/admin/groups/all', {
    params: { include_inactive: true },
  })
  return data
}

export async function getByPlatform(platform: GroupPlatform): Promise<AdminGroup[]> {
  return getAll(platform)
}

export async function getLiveCapability(): Promise<LiveCapability> {
  const { data } = await apiClient.get<LiveCapability>('/admin/groups/live-capability')
  return data
}

export async function getModelDefaultPricing(model: string): Promise<ModelDefaultPricing> {
  const { data } = await apiClient.get<ModelDefaultPricing>('/admin/channels/model-pricing', {
    params: { model },
  })
  return data
}

export async function getById(id: number): Promise<AdminGroup> {
  const { data } = await apiClient.get<AdminGroup>(`/admin/groups/${id}`)
  return data
}

export async function getModelsListCandidates(
  id: number,
  platform?: GroupPlatform,
): Promise<string[]> {
  const { data } = await apiClient.get<{ models: string[] }>(
    `/admin/groups/${id}/models-list-candidates`,
    { params: platform ? { platform } : undefined },
  )
  return data.models || []
}

export async function getStats(id: number): Promise<GroupStats> {
  const { data } = await apiClient.get<GroupStats>(`/admin/groups/${id}/stats`)
  return data
}

export async function getGroupApiKeys(
  id: number,
  page: number = 1,
  pageSize: number = 20,
): Promise<PaginatedResponse<ApiKey>> {
  const { data } = await apiClient.get<PaginatedResponse<ApiKey>>(`/admin/groups/${id}/api-keys`, {
    params: { page, page_size: pageSize },
  })
  return data
}

export async function listCompositeRoutes(id: number): Promise<CompositeModelRoute[]> {
  const { data } = await apiClient.get<CompositeModelRoute[]>(`/admin/groups/${id}/composite-routes`)
  return data
}

export async function previewCompositeRoute(
  id: number,
  request: CompositeRoutePreviewRequest,
): Promise<CompositeRouteDecision> {
  const { data } = await apiClient.post<CompositeRouteDecision>(
    `/admin/groups/${id}/composite-routes/preview`,
    request,
  )
  return data
}

export async function getGroupRateMultipliers(id: number): Promise<GroupRateMultiplierEntry[]> {
  const { data } = await apiClient.get<GroupRateMultiplierEntry[]>(
    `/admin/groups/${id}/rate-multipliers`,
  )
  return data
}

export async function getGroupRPMOverrides(id: number): Promise<GroupRPMOverrideEntry[]> {
  const entries = await getGroupRateMultipliers(id)
  return entries
    .filter((entry) => entry.rpm_override != null)
    .map((entry) => ({
      user_id: entry.user_id,
      user_name: entry.user_name,
      user_email: entry.user_email,
      user_notes: entry.user_notes,
      user_status: entry.user_status,
      rpm_override: entry.rpm_override as number,
    }))
}

export async function getUsageSummary(): Promise<GroupUsageSummary[]> {
  const { data } = await apiClient.get<GroupUsageSummary[]>('/admin/groups/usage-summary')
  return data
}

export async function getCapacitySummary(): Promise<GroupCapacitySummary[]> {
  const { data } = await apiClient.get<GroupCapacitySummary[]>('/admin/groups/capacity-summary')
  return data
}
