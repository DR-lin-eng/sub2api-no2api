import type { PaginatedResponse } from '@/types'

export type OpsRequestKind = 'success' | 'error'
export type OpsRequestDetailsKind = OpsRequestKind | 'all'
export type OpsRequestDetailsSort = 'created_at_desc' | 'duration_desc' | 'ttft_desc'

export interface OpsRequestDetail {
  kind: OpsRequestKind
  created_at: string
  request_id: string

  platform?: string
  model?: string
  duration_ms?: number | null
  first_token_ms?: number | null
  status_code?: number | null

  error_id?: number | null
  phase?: string
  severity?: string
  message?: string

  user_id?: number | null
  api_key_id?: number | null
  account_id?: number | null
  group_id?: number | null

  stream?: boolean
}

export interface OpsRequestDetailsParams {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string

  kind?: OpsRequestDetailsKind

  platform?: string
  group_id?: number | null

  user_id?: number
  api_key_id?: number
  account_id?: number

  model?: string
  request_id?: string
  q?: string

  min_duration_ms?: number
  max_duration_ms?: number
  ttft_only?: boolean

  sort?: OpsRequestDetailsSort

  page?: number
  page_size?: number
}

export type OpsRequestDetailsResponse = PaginatedResponse<OpsRequestDetail>

export interface OpsRuntimeLogConfig {
  level: 'debug' | 'info' | 'warn' | 'error'
  enable_sampling: boolean
  sampling_initial: number
  sampling_thereafter: number
  caller: boolean
  stacktrace_level: 'none' | 'error' | 'fatal'
  retention_days: number
  redis_only: boolean
  source?: string
  updated_at?: string
  updated_by_user_id?: number
}

export interface OpsSystemLog {
  id: number
  created_at: string
  host: string
  level: string
  component: string
  message: string
  request_id?: string
  client_request_id?: string
  user_id?: number | null
  api_key_id?: number | null
  account_id?: number | null
  platform?: string
  model?: string
  extra?: Record<string, any>
}

export type OpsSystemLogListResponse = PaginatedResponse<OpsSystemLog>

export interface OpsSystemLogQuery {
  page?: number
  page_size?: number
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h' | '7d' | '30d'
  start_time?: string
  end_time?: string
  host?: string
  level?: string
  component?: string
  request_id?: string
  client_request_id?: string
  user_id?: number | null
  api_key_id?: number | null
  account_id?: number | null
  platform?: string
  model?: string
  q?: string
}

export interface OpsSystemLogCleanupRequest {
  clear_all?: boolean
  start_time?: string
  end_time?: string
  host?: string
  level?: string
  component?: string
  request_id?: string
  client_request_id?: string
  user_id?: number | null
  api_key_id?: number | null
  account_id?: number | null
  platform?: string
  model?: string
  q?: string
}

export interface OpsSystemLogCleanupResponse {
  deleted: number
}

export interface OpsSystemLogSinkHealth {
  queue_depth: number
  queue_capacity: number
  dropped_count: number
  write_failed_count: number
  written_count: number
  avg_write_delay_ms: number
  last_error?: string
}
