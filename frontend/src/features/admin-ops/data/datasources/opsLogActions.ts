import { apiClient } from '@/core/networks/client'
import type {
  OpsRuntimeLogConfig,
  OpsSystemLogCleanupRequest,
  OpsSystemLogCleanupResponse
} from '@/features/admin-ops/data/dtos/opsLogDtos'

export async function updateRuntimeLogConfig(config: OpsRuntimeLogConfig): Promise<OpsRuntimeLogConfig> {
  const { data } = await apiClient.put<OpsRuntimeLogConfig>('/admin/ops/runtime/logging', config)
  return data
}

export async function resetRuntimeLogConfig(): Promise<OpsRuntimeLogConfig> {
  const { data } = await apiClient.post<OpsRuntimeLogConfig>('/admin/ops/runtime/logging/reset')
  return data
}

export async function cleanupSystemLogs(
  payload: OpsSystemLogCleanupRequest
): Promise<OpsSystemLogCleanupResponse> {
  const { data } = await apiClient.post<OpsSystemLogCleanupResponse>('/admin/ops/system-logs/cleanup', payload)
  return data
}
