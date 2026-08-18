import type { PaginatedResponse } from '@/types'

export type OpsSeverity = string
export type OpsPhase = string

export interface OpsErrorLog {
  id: number
  created_at: string

  phase: OpsPhase
  type: string
  error_owner: 'client' | 'provider' | 'platform' | string
  error_source: 'client_request' | 'upstream_http' | 'gateway' | string

  severity: OpsSeverity
  status_code: number
  platform: string
  model: string

  resolved: boolean
  resolved_at?: string | null
  resolved_by_user_id?: number | null

  client_request_id: string
  request_id: string
  message: string

  user_id?: number | null
  user_email: string
  api_key_id?: number | null
  api_key_name?: string
  api_key_deleted?: boolean
  account_id?: number | null
  account_name: string
  group_id?: number | null
  group_name: string

  client_ip?: string | null
  request_path?: string
  stream?: boolean

  inbound_endpoint?: string
  upstream_endpoint?: string
  requested_model?: string
  upstream_model?: string
  request_type?: number | null
  user_agent?: string
}

export interface OpsErrorDetail extends OpsErrorLog {
  error_body: string

  upstream_status_code?: number | null
  upstream_error_message?: string
  upstream_error_detail?: string
  upstream_errors?: string

  auth_latency_ms?: number | null
  routing_latency_ms?: number | null
  upstream_latency_ms?: number | null
  response_latency_ms?: number | null
  time_to_first_token_ms?: number | null

  is_business_limited: boolean
  api_key_prefix?: string | null
}

export type OpsErrorLogsResponse = PaginatedResponse<OpsErrorLog>
export type OpsErrorDetailsResponse = PaginatedResponse<OpsErrorDetail>

export type OpsErrorListView = 'errors' | 'excluded' | 'all'

export interface OpsErrorListQueryParams {
  page?: number
  page_size?: number
  time_range?: string
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  account_id?: number | null
  user_id?: number
  api_key_id?: number
  model?: string

  phase?: string
  category?: string
  error_owner?: string
  error_source?: string
  resolved?: string
  view?: OpsErrorListView

  q?: string
  status_codes?: string
  status_codes_other?: string

  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export interface OpsErrorCorrelationOptions {
  include_detail?: boolean
}
