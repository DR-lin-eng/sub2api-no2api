import { apiClient } from '@/core/networks/client'
import type {
  OpsDashboardOverview,
  OpsDashboardQueryParams,
  OpsDashboardSnapshotParams,
  OpsDashboardSnapshotV2Response,
  OpsErrorDistributionResponse,
  OpsErrorTrendResponse,
  OpsLatencyHistogramResponse,
  OpsNetworkBandwidthQueryParams,
  OpsNetworkBandwidthTrendResponse,
  OpsRequestOptions,
  OpsSwitchTrendParams,
  OpsThroughputTrendResponse
} from '@/features/admin-ops/data/dtos/opsDashboardDtos'

export async function getDashboardOverview(
  params: OpsDashboardQueryParams,
  options: OpsRequestOptions = {}
): Promise<OpsDashboardOverview> {
  const { data } = await apiClient.get<OpsDashboardOverview>('/admin/ops/dashboard/overview', {
    params,
    signal: options.signal
  })
  return data
}

export async function getDashboardSnapshotV2(
  params: OpsDashboardSnapshotParams,
  options: OpsRequestOptions = {}
): Promise<OpsDashboardSnapshotV2Response> {
  const { data } = await apiClient.get<OpsDashboardSnapshotV2Response>('/admin/ops/dashboard/snapshot-v2', {
    params,
    signal: options.signal
  })
  return data
}

export async function getThroughputTrend(
  params: OpsDashboardQueryParams,
  options: OpsRequestOptions = {}
): Promise<OpsThroughputTrendResponse> {
  const { data } = await apiClient.get<OpsThroughputTrendResponse>('/admin/ops/dashboard/throughput-trend', {
    params,
    signal: options.signal
  })
  return data
}

export async function getNetworkBandwidthTrend(
  params: OpsNetworkBandwidthQueryParams,
  options: OpsRequestOptions = {}
): Promise<OpsNetworkBandwidthTrendResponse> {
  const { data } = await apiClient.get<OpsNetworkBandwidthTrendResponse>('/admin/ops/dashboard/network-bandwidth-trend', {
    params,
    signal: options.signal
  })
  return data
}

export async function getSwitchTrend(
  params: OpsSwitchTrendParams,
  options: OpsRequestOptions = {}
): Promise<OpsThroughputTrendResponse> {
  const { data } = await apiClient.get<OpsThroughputTrendResponse>('/admin/ops/dashboard/switch-trend', {
    params,
    signal: options.signal
  })
  return data
}

export async function getLatencyHistogram(
  params: OpsDashboardQueryParams,
  options: OpsRequestOptions = {}
): Promise<OpsLatencyHistogramResponse> {
  const { data } = await apiClient.get<OpsLatencyHistogramResponse>('/admin/ops/dashboard/latency-histogram', {
    params,
    signal: options.signal
  })
  return data
}

export async function getErrorTrend(
  params: OpsDashboardQueryParams,
  options: OpsRequestOptions = {}
): Promise<OpsErrorTrendResponse> {
  const { data } = await apiClient.get<OpsErrorTrendResponse>('/admin/ops/dashboard/error-trend', {
    params,
    signal: options.signal
  })
  return data
}

export async function getErrorDistribution(
  params: OpsDashboardQueryParams,
  options: OpsRequestOptions = {}
): Promise<OpsErrorDistributionResponse> {
  const { data } = await apiClient.get<OpsErrorDistributionResponse>('/admin/ops/dashboard/error-distribution', {
    params,
    signal: options.signal
  })
  return data
}
