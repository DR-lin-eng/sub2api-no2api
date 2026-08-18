import { apiClient } from '@/core/networks/client'
import type {
  AlertEvent,
  AlertEventsQuery,
  AlertRule
} from '@/features/admin-ops/data/dtos/opsAlertDtos'

export async function listAlertRules(): Promise<AlertRule[]> {
  const { data } = await apiClient.get<AlertRule[]>('/admin/ops/alert-rules')
  return data
}

export async function listAlertEvents(params: AlertEventsQuery = {}): Promise<AlertEvent[]> {
  const { data } = await apiClient.get<AlertEvent[]>('/admin/ops/alert-events', { params })
  return data
}

export async function getAlertEvent(id: number): Promise<AlertEvent> {
  const { data } = await apiClient.get<AlertEvent>(`/admin/ops/alert-events/${id}`)
  return data
}
