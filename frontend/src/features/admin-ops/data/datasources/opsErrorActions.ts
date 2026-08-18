import { apiClient } from '@/core/networks/client'

export async function updateErrorResolved(errorId: number, resolved: boolean): Promise<void> {
  await apiClient.put(`/admin/ops/errors/${errorId}/resolve`, { resolved })
}

export async function updateRequestErrorResolved(errorId: number, resolved: boolean): Promise<void> {
  await apiClient.put(`/admin/ops/request-errors/${errorId}/resolve`, { resolved })
}

export async function updateUpstreamErrorResolved(errorId: number, resolved: boolean): Promise<void> {
  await apiClient.put(`/admin/ops/upstream-errors/${errorId}/resolve`, { resolved })
}
