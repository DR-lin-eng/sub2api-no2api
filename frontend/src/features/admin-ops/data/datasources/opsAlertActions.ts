import { apiClient } from '@/core/networks/client'
import type {
  AlertEventStatus,
  AlertRule,
  AlertSilenceRequest
} from '@/features/admin-ops/data/dtos/opsAlertDtos'

export async function createAlertRule(rule: AlertRule): Promise<AlertRule> {
  const { data } = await apiClient.post<AlertRule>('/admin/ops/alert-rules', rule)
  return data
}

export async function updateAlertRule(id: number, rule: Partial<AlertRule>): Promise<AlertRule> {
  const { data } = await apiClient.put<AlertRule>(`/admin/ops/alert-rules/${id}`, rule)
  return data
}

export async function deleteAlertRule(id: number): Promise<void> {
  await apiClient.delete(`/admin/ops/alert-rules/${id}`)
}

export async function updateAlertEventStatus(id: number, status: AlertEventStatus): Promise<void> {
  await apiClient.put(`/admin/ops/alert-events/${id}/status`, { status })
}

export async function createAlertSilence(payload: AlertSilenceRequest): Promise<void> {
  await apiClient.post('/admin/ops/alert-silences', payload)
}
