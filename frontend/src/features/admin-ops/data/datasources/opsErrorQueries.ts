import { apiClient } from '@/core/networks/client'
import type {
  OpsErrorCorrelationOptions,
  OpsErrorDetail,
  OpsErrorDetailsResponse,
  OpsErrorListQueryParams,
  OpsErrorLogsResponse
} from '@/features/admin-ops/data/dtos/opsErrorDtos'

export async function listErrorLogs(params: OpsErrorListQueryParams): Promise<OpsErrorLogsResponse> {
  const { data } = await apiClient.get<OpsErrorLogsResponse>('/admin/ops/errors', { params })
  return data
}

export async function getErrorLogDetail(id: number): Promise<OpsErrorDetail> {
  const { data } = await apiClient.get<OpsErrorDetail>(`/admin/ops/errors/${id}`)
  return data
}

export async function listRequestErrors(params: OpsErrorListQueryParams): Promise<OpsErrorLogsResponse> {
  const { data } = await apiClient.get<OpsErrorLogsResponse>('/admin/ops/request-errors', { params })
  return data
}

export async function listUpstreamErrors(params: OpsErrorListQueryParams): Promise<OpsErrorLogsResponse> {
  const { data } = await apiClient.get<OpsErrorLogsResponse>('/admin/ops/upstream-errors', { params })
  return data
}

export async function getRequestErrorDetail(id: number): Promise<OpsErrorDetail> {
  const { data } = await apiClient.get<OpsErrorDetail>(`/admin/ops/request-errors/${id}`)
  return data
}

export async function getUpstreamErrorDetail(id: number): Promise<OpsErrorDetail> {
  const { data } = await apiClient.get<OpsErrorDetail>(`/admin/ops/upstream-errors/${id}`)
  return data
}

export async function listRequestErrorUpstreamErrors(
  id: number,
  params: OpsErrorListQueryParams = {},
  options: OpsErrorCorrelationOptions = {}
): Promise<OpsErrorDetailsResponse> {
  const query: OpsErrorListQueryParams & { include_detail?: '1' } = { ...params }
  if (options.include_detail) query.include_detail = '1'

  const { data } = await apiClient.get<OpsErrorDetailsResponse>(
    `/admin/ops/request-errors/${id}/upstream-errors`,
    { params: query }
  )
  return data
}
