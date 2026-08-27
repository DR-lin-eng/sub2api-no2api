import { apiClient } from '@/core/networks/client'
import type { ActivityCampaign } from '@/types'

export async function list(): Promise<ActivityCampaign[]> {
  const { data } = await apiClient.get<ActivityCampaign[]>('/activity-center/campaigns')
  return data
}

export async function getById(id: number): Promise<ActivityCampaign> {
  const { data } = await apiClient.get<ActivityCampaign>(`/activity-center/campaigns/${id}`)
  return data
}

const activityCenterAPI = {
  list,
  getById
}

export default activityCenterAPI
