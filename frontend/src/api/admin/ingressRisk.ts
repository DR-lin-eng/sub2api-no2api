import { apiClient } from '../client'

export type IngressRiskTimeRange = '5m' | '30m' | '1h' | '6h' | '24h' | '7d' | '30d'

export interface IngressRejection {
  id: number
  bucket_start: string
  reject_reason: string
  route_family: string
  protocol: string
  client_ip: string
  user_id?: number
  api_key_id?: number
  request_count: number
  first_seen: string
  last_seen: string
}

export interface IngressRejectionQuery {
  time_range?: IngressRiskTimeRange
  start_time?: string
  end_time?: string
  reason?: string
  route_family?: string
  protocol?: string
  client_ip?: string
  user_id?: number
  api_key_id?: number
  page?: number
  page_size?: number
}

export interface IngressRejectionList {
  items: IngressRejection[]
  total: number
  page: number
  page_size: number
}

export interface IngressCollectorHealth {
  cardinality: number
  capacity: number
  pending_batches: number
  pending_rows: number
  overflowed_count: number
  dropped_count: number
  flushed_request_count: number
  flush_failure_count: number
  accepting: boolean
  last_error?: string
}

export interface AuthInvalidationOutboxHealth {
  running: boolean
  processed: number
  failures: number
  pending: number
  oldest_lag: number
  last_error?: string
  stats_error?: string
  healthy_sla: number
  recovery_sla: number
  max_attempts: number
}

export interface AuthInvalidationSubscriberHealth {
  connected: boolean
  failures: number
}

export interface AuthLookupHealth {
  total: number
  rejected: number
  in_flight: number
  capacity: number
}

export interface InvalidAuthAbuseHealth {
  enabled: boolean
  tracked: number
  capacity: number
  recorded: number
  blocks: number
  rejected: number
  expired: number
  overflowed: number
  global_blocked: number
}

export interface AuthCacheHealth {
  outbox: AuthInvalidationOutboxHealth
  subscriber: AuthInvalidationSubscriberHealth
  lookup: AuthLookupHealth
  invalid_abuse: InvalidAuthAbuseHealth
}

export async function listIngressRejections(params: IngressRejectionQuery): Promise<IngressRejectionList> {
  const { data } = await apiClient.get<IngressRejectionList>('/admin/ops/ingress-rejections', { params })
  return data
}

export async function getIngressCollectorHealth(): Promise<IngressCollectorHealth> {
  const { data } = await apiClient.get<IngressCollectorHealth>('/admin/ops/ingress-rejections/health')
  return data
}

export async function getAuthCacheHealth(): Promise<AuthCacheHealth> {
  const { data } = await apiClient.get<AuthCacheHealth>('/admin/ops/auth-cache-invalidation/health')
  return data
}

export const ingressRiskAPI = {
  listIngressRejections,
  getIngressCollectorHealth,
  getAuthCacheHealth,
}

export default ingressRiskAPI
