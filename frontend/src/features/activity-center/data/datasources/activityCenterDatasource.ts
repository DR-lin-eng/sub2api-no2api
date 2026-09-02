import { apiClient } from '@/core/networks/client'
import type { ActivityCheckinLeaderboardEntry, ActivityCheckinStatus, ActivityParticipationRecord, PaginatedResponse, UserActivityCampaign } from '@/types'

export async function list(): Promise<UserActivityCampaign[]> {
  const { data } = await apiClient.get<UserActivityCampaign[]>('/activity-center/campaigns')
  return data
}

export async function getById(id: number): Promise<UserActivityCampaign> {
  const { data } = await apiClient.get<UserActivityCampaign>(`/activity-center/campaigns/${id}`)
  return data
}

export async function participate(id: number, poolId?: string): Promise<ActivityParticipationRecord> {
  const { data } = await apiClient.post<ActivityParticipationRecord>(`/activity-center/campaigns/${id}/participate`, {
    pool_id: poolId || ''
  })
  return data
}

export async function getCheckinStatus(id: number): Promise<ActivityCheckinStatus> {
  const { data } = await apiClient.get<ActivityCheckinStatus>(`/activity-center/campaigns/${id}/checkin/status`)
  return data
}

export async function checkin(id: number): Promise<{ record: ActivityParticipationRecord; status: ActivityCheckinStatus }> {
  const { data } = await apiClient.post<{ record: ActivityParticipationRecord; status: ActivityCheckinStatus }>(`/activity-center/campaigns/${id}/checkin`)
  return data
}

export async function getCheckinLeaderboard(id: number): Promise<ActivityCheckinLeaderboardEntry[]> {
  const { data } = await apiClient.get<ActivityCheckinLeaderboardEntry[]>(`/activity-center/campaigns/${id}/checkin/leaderboard`)
  return data
}

export async function redeemCode(code: string) {
  const { data } = await apiClient.post('/redeem', { code })
  return data
}

export async function listMyRecords(
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<ActivityParticipationRecord>> {
  const { data } = await apiClient.get<PaginatedResponse<ActivityParticipationRecord>>('/activity-center/records', {
    params: { page, page_size: pageSize }
  })
  return data
}

const activityCenterAPI = {
  list,
  getById,
  participate,
  getCheckinStatus,
  checkin,
  getCheckinLeaderboard,
  redeemCode,
  listMyRecords
}

export default activityCenterAPI
