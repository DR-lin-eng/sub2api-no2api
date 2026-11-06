import { apiClient } from '@/core/networks/client'

export interface BlacklistedEmailUser { id: number; email: string; username: string }
export async function findBlacklistedEmailUsers(emails: string[]): Promise<{ items: BlacklistedEmailUser[]; total: number }> {
  const { data } = await apiClient.post<{ items: BlacklistedEmailUser[]; total: number }>('/admin/users/blacklist-matches', { emails })
  return data
}
export async function batchDisable(userIds: number[]): Promise<{ affected: number }> {
  const { data } = await apiClient.post<{ affected: number }>('/admin/users/batch-disable', { user_ids: userIds })
  return data
}
