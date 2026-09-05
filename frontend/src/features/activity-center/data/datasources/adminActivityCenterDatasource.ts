import { apiClient } from '@/core/networks/client'
import type {
  ActivityCampaign,
  ActivityParticipationRecord,
  CreateActivityCampaignRequest,
  UpdateActivityCampaignRequest,
  PaginatedResponse,
} from '@/types'

export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    status?: string
    type?: string
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: {
    signal?: AbortSignal
  }
): Promise<PaginatedResponse<ActivityCampaign>> {
  const { data } = await apiClient.get<PaginatedResponse<ActivityCampaign>>('/admin/activity-center/campaigns', {
    params: { page, page_size: pageSize, ...filters },
    signal: options?.signal
  })
  return data
}

export async function getById(id: number): Promise<ActivityCampaign> {
  const { data } = await apiClient.get<ActivityCampaign>(`/admin/activity-center/campaigns/${id}`)
  return data
}

export async function create(request: CreateActivityCampaignRequest): Promise<ActivityCampaign> {
  const { data } = await apiClient.post<ActivityCampaign>('/admin/activity-center/campaigns', request)
  return data
}

export async function update(id: number, request: UpdateActivityCampaignRequest): Promise<ActivityCampaign> {
  const { data } = await apiClient.put<ActivityCampaign>(`/admin/activity-center/campaigns/${id}`, request)
  return data
}

export async function remove(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/activity-center/campaigns/${id}`)
  return data
}

export async function listRecords(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    campaign_id?: number
    user_id?: number
    type?: string
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  }
): Promise<PaginatedResponse<ActivityParticipationRecord>> {
  const { data } = await apiClient.get<PaginatedResponse<ActivityParticipationRecord>>('/admin/activity-center/records', {
    params: { page, page_size: pageSize, ...filters }
  })
  return data
}

const activityCenterAPI = {
  list,
  getById,
  create,
  update,
  delete: remove,
  listRecords
}

export default activityCenterAPI
