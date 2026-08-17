import { apiClient } from '@/core/networks/client'

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
  cloudflare?: CloudflareIngressHealth
}

export interface CloudflareIngressHealth {
  enabled: boolean
  mode?: CloudflareIngressMode
  running: boolean
  queue_depth: number
  queue_capacity: number
  active_rules: number
  enqueued: number
  applied: number
  released: number
  failures: number
  dropped: number
  last_error?: string
  last_success_at?: string
  waf?: CloudflareWAFHealth
}

export type CloudflareIngressMode = 'zone_access_rules' | 'waf_custom_rules'

export interface CloudflareWAFHealth {
  hostname: string
  hostnames?: string[]
  hostname_stats?: CloudflareWAFHostnameHealth[]
  rule_count: number
  synced_entries: number
  overflow_entries: number
  hostname_requests_24h: number
  blocked_requests_24h: number
  last_synced_at?: string
  analytics_updated_at?: string
  analytics_window_start?: string
  analytics_error?: string
}

export interface CloudflareWAFHostnameHealth {
  hostname: string
  requests_24h: number
  blocked_requests_24h: number
}

export interface CloudflareIngressSettings {
  enabled: boolean
  mode: CloudflareIngressMode
  zone_id: string
  api_token_configured: boolean
  waf_hostname: string
  waf_hostnames?: string[]
  waf_rule_ids: string[]
  waf_sync_interval_seconds: number
  analytics_interval_seconds: number
  request_timeout_seconds: number
  queue_capacity: number
  max_active_rules: number
  reconcile_interval_seconds: number
}

export interface UpdateCloudflareIngressSettings {
  enabled: boolean
  mode: CloudflareIngressMode
  zone_id: string
  api_token: string
  waf_hostname: string
  waf_hostnames: string[]
  waf_rule_ids: string[]
  waf_sync_interval_seconds: number
  analytics_interval_seconds: number
  request_timeout_seconds: number
  queue_capacity: number
  max_active_rules: number
  reconcile_interval_seconds: number
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

export async function getCloudflareIngressSettings(): Promise<CloudflareIngressSettings> {
  const { data } = await apiClient.get<CloudflareIngressSettings>('/admin/ops/ingress-rejections/cloudflare')
  return data
}

export async function updateCloudflareIngressSettings(
  payload: UpdateCloudflareIngressSettings,
): Promise<CloudflareIngressSettings> {
  const { data } = await apiClient.put<CloudflareIngressSettings>('/admin/ops/ingress-rejections/cloudflare', payload)
  return data
}

export const ingressRiskAPI = {
  listIngressRejections,
  getIngressCollectorHealth,
  getAuthCacheHealth,
  getCloudflareIngressSettings,
  updateCloudflareIngressSettings,
}

export default ingressRiskAPI
