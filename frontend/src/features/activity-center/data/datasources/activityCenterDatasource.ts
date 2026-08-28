import { apiClient } from '@/core/networks/client'
import type { UserActivityCampaign } from '@/types'

export async function list(): Promise<UserActivityCampaign[]> {
  const { data } = await apiClient.get<UserActivityCampaign[]>('/activity-center/campaigns')
  return data
}

export async function getById(id: number): Promise<UserActivityCampaign> {
  const { data } = await apiClient.get<UserActivityCampaign>(`/activity-center/campaigns/${id}`)
  return data
}

const activityCenterAPI = {
  list,
  getById
}

export default activityCenterAPI
