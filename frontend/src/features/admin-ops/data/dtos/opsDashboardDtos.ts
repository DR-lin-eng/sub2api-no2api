export type OpsQueryMode = 'auto' | 'raw' | 'preagg'

export interface OpsRequestOptions {
  signal?: AbortSignal
}

export type OpsDashboardTimeRange = '5m' | '30m' | '1h' | '6h' | '24h'
export type OpsSwitchTrendTimeRange = OpsDashboardTimeRange | '5h'

export interface OpsDashboardQueryParams {
  time_range?: OpsDashboardTimeRange
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  mode?: OpsQueryMode
}

export interface OpsDashboardSnapshotParams extends OpsDashboardQueryParams {
  include_throughput_trend?: boolean
  include_network_bandwidth?: boolean
  include_latency_histogram?: boolean
  include_error_trend?: boolean
  include_error_distribution?: boolean
  include_switch_count?: boolean
}

export interface OpsSwitchTrendParams {
  time_range?: OpsSwitchTrendTimeRange
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  mode?: OpsQueryMode
}

export interface OpsNetworkBandwidthQueryParams {
  time_range?: OpsDashboardTimeRange
  start_time?: string
  end_time?: string
}

export interface OpsSystemMetricsSnapshot {
  id: number
  created_at: string
  window_minutes: number

  cpu_usage_percent?: number | null
  memory_used_mb?: number | null
  memory_total_mb?: number | null
  memory_usage_percent?: number | null

  network_receive_bytes_per_second?: number | null
  network_transmit_bytes_per_second?: number | null
  network_interfaces?: string[] | null

  db_ok?: boolean | null
  redis_ok?: boolean | null

  db_max_open_conns?: number | null
  redis_pool_size?: number | null

  redis_conn_total?: number | null
  redis_conn_idle?: number | null

  db_conn_active?: number | null
  db_conn_idle?: number | null
  db_conn_waiting?: number | null

  goroutine_count?: number | null
  concurrency_queue_depth?: number | null
  account_switch_count?: number | null
}

export interface OpsJobHeartbeat {
  job_name: string
  last_run_at?: string | null
  last_success_at?: string | null
  last_error_at?: string | null
  last_error?: string | null
  last_duration_ms?: number | null
  last_result?: string | null
  updated_at: string
}

export interface OpsPercentiles {
  p50_ms?: number | null
  p90_ms?: number | null
  p95_ms?: number | null
  p99_ms?: number | null
  avg_ms?: number | null
  max_ms?: number | null
}

export interface OpsDashboardOverview {
  start_time: string
  end_time: string
  platform: string
  group_id?: number | null

  health_score?: number

  system_metrics?: OpsSystemMetricsSnapshot | null
  job_heartbeats?: OpsJobHeartbeat[] | null

  success_count: number
  error_count_total: number
  business_limited_count: number
  error_count_sla: number
  request_count_total: number
  request_count_sla: number

  token_consumed: number

  sla: number
  error_rate: number
  upstream_error_rate: number
  upstream_error_count_excl_429_529: number
  upstream_429_count: number
  upstream_529_count: number

  qps: {
    current: number
    peak: number
    avg: number
  }
  tps: {
    current: number
    peak: number
    avg: number
  }

  duration: OpsPercentiles
  ttft: OpsPercentiles
}

export interface OpsThroughputTrendPoint {
  bucket_start: string
  request_count: number
  token_consumed: number
  switch_count?: number
  qps: number
  tps: number
}

export interface OpsThroughputPlatformBreakdownItem {
  platform: string
  request_count: number
  token_consumed: number
}

export interface OpsThroughputGroupBreakdownItem {
  group_id: number
  group_name: string
  request_count: number
  token_consumed: number
}

export interface OpsThroughputTrendResponse {
  bucket: string
  points: OpsThroughputTrendPoint[]
  by_platform?: OpsThroughputPlatformBreakdownItem[]
  top_groups?: OpsThroughputGroupBreakdownItem[]
}

export interface OpsNetworkBandwidthTrendPoint {
  bucket_start: string
  receive_bytes_per_second?: number | null
  transmit_bytes_per_second?: number | null
}

export interface OpsNetworkBandwidthTrendResponse {
  start_time: string
  end_time: string
  bucket: string
  points: OpsNetworkBandwidthTrendPoint[]
}

export interface OpsLatencyHistogramBucket {
  range: string
  count: number
}

export interface OpsLatencyHistogramResponse {
  start_time: string
  end_time: string
  platform: string
  group_id?: number | null

  total_requests: number
  buckets: OpsLatencyHistogramBucket[]
}

export interface OpsErrorTrendPoint {
  bucket_start: string
  error_count_total: number
  business_limited_count: number
  error_count_sla: number
  upstream_error_count_excl_429_529: number
  upstream_429_count: number
  upstream_529_count: number
}

export interface OpsErrorTrendResponse {
  bucket: string
  points: OpsErrorTrendPoint[]
}

export interface OpsErrorDistributionItem {
  status_code: number
  total: number
  sla: number
  business_limited: number
}

export interface OpsErrorDistributionResponse {
  total: number
  items: OpsErrorDistributionItem[]
}

export interface OpsDashboardSnapshotV2Response {
  generated_at: string
  overview: OpsDashboardOverview
  throughput_trend?: OpsThroughputTrendResponse | null
  network_bandwidth_trend?: OpsNetworkBandwidthTrendResponse | null
  latency_histogram?: OpsLatencyHistogramResponse | null
  error_trend?: OpsErrorTrendResponse | null
  error_distribution?: OpsErrorDistributionResponse | null
}
