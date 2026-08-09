import { apiClient } from '@/core/networks/client'
import type {
  OpsRequestDetailsParams,
  OpsRequestDetailsResponse,
  OpsRuntimeLogConfig,
  OpsSystemLogListResponse,
  OpsSystemLogQuery,
  OpsSystemLogSinkHealth
} from '@/features/admin-ops/data/dtos/opsLogDtos'

export async function listRequestDetails(params: OpsRequestDetailsParams): Promise<OpsRequestDetailsResponse> {
  const { data } = await apiClient.get<OpsRequestDetailsResponse>('/admin/ops/requests', { params })
  return data
}

export async function getRuntimeLogConfig(): Promise<OpsRuntimeLogConfig> {
  const { data } = await apiClient.get<OpsRuntimeLogConfig>('/admin/ops/runtime/logging')
  return data
}

export async function listSystemLogs(params: OpsSystemLogQuery): Promise<OpsSystemLogListResponse> {
  const { data } = await apiClient.get<OpsSystemLogListResponse>('/admin/ops/system-logs', { params })
  return data
}

export async function getSystemLogSinkHealth(): Promise<OpsSystemLogSinkHealth> {
  const { data } = await apiClient.get<OpsSystemLogSinkHealth>('/admin/ops/system-logs/health')
  return data
}
