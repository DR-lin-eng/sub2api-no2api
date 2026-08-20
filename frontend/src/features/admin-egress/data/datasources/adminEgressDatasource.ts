import { apiClient } from '@/core/networks/client'
import type { PaginatedResponse } from '@/types'

export type EgressMode = 'inherit' | 'direct' | 'external_proxy' | 'ipv6_pool'
export type IPv6EgressPoolStatus = 'active' | 'disabled'

export interface IPv6EgressRuntime {
  enabled: boolean
  ready: boolean
  supported: boolean
  platform: string
  freebind: boolean
  secret_configured: boolean
  fail_closed: boolean
  reconcile_interval_seconds: number
  probe_configured: boolean
  control_enabled: boolean
  detected_prefix?: string
}

export interface IPv6EgressDetectedNetwork {
  address: string
  prefix: string
  interface: string
}

export interface HETunnelConfig {
  enabled: boolean
  server_ipv4: string
  local_ipv4?: string
  client_ipv6: string
  server_ipv6: string
  pool_cidr: string
  mtu: number
  route_metric: number
  probe_ipv6?: string
  probe_timeout_seconds: number
  allow_private_ipv4: boolean
  update_enabled: boolean
  tunnel_id?: string
  username?: string
  update_key_configured: boolean
}

export interface SaveHETunnelConfigInput extends Omit<HETunnelConfig, 'update_key_configured'> {
  update_key?: string
  clear_update_key?: boolean
}

export interface HETunnelAgentStatus {
  online: boolean
  state: 'unavailable' | 'offline' | 'idle' | 'pending' | 'applying' | 'succeeded' | 'failed'
  action?: 'apply' | 'check' | 'remove'
  message?: string
  request_id?: string
  updated_at?: string
}

export interface HETunnelControlSnapshot {
  available: boolean
  config: HETunnelConfig
  agent: HETunnelAgentStatus
}

export interface IPv6EgressPool {
  id: number
  name: string
  cidr: string
  node_id?: string | null
  status: IPv6EgressPoolStatus
  is_default: boolean
  allocation_version: number
  allocated_count: number
  capacity: string
  route_healthy?: boolean | null
  last_probe_at?: string | null
  probe_error?: string
  created_at: string
  updated_at: string
}

export interface IPv6EgressBinding {
  id: number
  account_id: number
  account_name?: string
  pool_id: number
  pool_name?: string
  pool_status?: IPv6EgressPoolStatus
  source_ipv6: string
  status: 'active' | 'disabled'
  version: number
  rotated_at?: string | null
  created_at: string
  updated_at: string
}

export interface CreateIPv6EgressPoolInput {
  name: string
  cidr: string
  node_id?: string | null
  is_default: boolean
}

export interface UpdateIPv6EgressPoolInput {
  name?: string
  node_id?: string | null
  status?: IPv6EgressPoolStatus
  is_default?: boolean
}

export interface IPv6EgressProbeResult {
  source_ipv6: string
  observed_ip: string
  latency_ms: number
  probe_target: string
}

export interface IPv6EgressAutoConfigureResult {
  enabled: boolean
  created: boolean
  detected: IPv6EgressDetectedNetwork
  pool: IPv6EgressPool
  probe: IPv6EgressProbeResult
}

export async function getRuntime(): Promise<IPv6EgressRuntime> {
  const { data } = await apiClient.get<IPv6EgressRuntime>('/admin/egress/runtime')
  return data
}

export async function updateRuntime(enabled: boolean): Promise<IPv6EgressRuntime> {
  const { data } = await apiClient.put<IPv6EgressRuntime>('/admin/egress/runtime', { enabled })
  return data
}

export async function autoConfigure(): Promise<IPv6EgressAutoConfigureResult> {
  const { data } = await apiClient.post<IPv6EgressAutoConfigureResult>('/admin/egress/auto-configure')
  return data
}

export async function listPools(): Promise<IPv6EgressPool[]> {
  const { data } = await apiClient.get<IPv6EgressPool[]>('/admin/egress/ipv6-pools')
  return data
}

export async function createPool(input: CreateIPv6EgressPoolInput): Promise<IPv6EgressPool> {
  const { data } = await apiClient.post<IPv6EgressPool>('/admin/egress/ipv6-pools', input)
  return data
}

export async function updatePool(id: number, input: UpdateIPv6EgressPoolInput): Promise<IPv6EgressPool> {
  const { data } = await apiClient.put<IPv6EgressPool>(`/admin/egress/ipv6-pools/${id}`, input)
  return data
}

export async function deletePool(id: number): Promise<void> {
  await apiClient.delete(`/admin/egress/ipv6-pools/${id}`)
}

export async function listBindings(
  page = 1,
  pageSize = 50,
  search = '',
  signal?: AbortSignal,
): Promise<PaginatedResponse<IPv6EgressBinding>> {
  const { data } = await apiClient.get<PaginatedResponse<IPv6EgressBinding>>('/admin/egress/bindings', {
    params: { page, page_size: pageSize, search },
    signal,
  })
  return data
}

export async function setAccountRoute(
  accountId: number,
  mode: EgressMode,
  poolId?: number,
): Promise<{ mode: EgressMode; binding?: IPv6EgressBinding | null }> {
  const { data } = await apiClient.put<{ mode: EgressMode; binding?: IPv6EgressBinding | null }>(
    `/admin/egress/accounts/${accountId}`,
    { mode, pool_id: poolId || null },
  )
  return data
}

export async function rotateBinding(accountId: number): Promise<IPv6EgressBinding> {
  const { data } = await apiClient.post<IPv6EgressBinding>(`/admin/egress/accounts/${accountId}/rotate`)
  return data
}

export async function probeAccount(accountId: number): Promise<IPv6EgressProbeResult> {
  const { data } = await apiClient.post<IPv6EgressProbeResult>(`/admin/egress/accounts/${accountId}/probe`)
  return data
}

export async function reconcileDefault(limit = 1000): Promise<{ allocated: number; limit: number }> {
  const { data } = await apiClient.post<{ allocated: number; limit: number }>(
    '/admin/egress/reconcile-default',
    undefined,
    { params: { limit } },
  )
  return data
}

export async function getHETunnel(): Promise<HETunnelControlSnapshot> {
  const { data } = await apiClient.get<HETunnelControlSnapshot>('/admin/egress/he-tunnel')
  return data
}

export async function saveHETunnel(input: SaveHETunnelConfigInput): Promise<HETunnelControlSnapshot> {
  const { data } = await apiClient.put<HETunnelControlSnapshot>('/admin/egress/he-tunnel', input)
  return data
}

export async function runHETunnelAction(action: 'apply' | 'check' | 'remove'): Promise<HETunnelControlSnapshot> {
  const { data } = await apiClient.post<HETunnelControlSnapshot>(`/admin/egress/he-tunnel/${action}`)
  return data
}

export const egressAPI = {
  getRuntime,
  updateRuntime,
  autoConfigure,
  listPools,
  createPool,
  updatePool,
  deletePool,
  listBindings,
  setAccountRoute,
  rotateBinding,
  probeAccount,
  reconcileDefault,
  getHETunnel,
  saveHETunnel,
  runHETunnelAction,
}

export default egressAPI
