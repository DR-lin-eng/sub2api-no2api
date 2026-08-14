import { apiClient } from '@/core/networks/client'

export type ClusterInstanceStatus = 'online' | 'stale' | 'stopped'
export type ClusterTaskStatus = 'running' | 'succeeded' | 'failed' | 'lost'
export type ClusterRolloutStatus = 'running' | 'paused' | 'completed' | 'cancelled'
export type ClusterRolloutTargetStatus =
  | 'pending'
  | 'draining'
  | 'installing'
  | 'restarting'
  | 'verifying'
  | 'succeeded'
  | 'failed'
  | 'cancelled'

export interface ClusterDeploymentStatus {
  mode: 'standalone' | 'multi_instance'
  node_id: string
  node_name: string
  runner_id: string
  worker_mode: 'auto' | 'true' | 'false'
  worker_enabled: boolean
  frontend_enabled: boolean
  heartbeat_interval_seconds: number
  stale_after_seconds: number
  task_lease_seconds: number
  update_driver: 'external' | 'binary'
  rollout_poll_seconds: number
  rollout_drain_grace_seconds: number
  rollout_drain_timeout_seconds: number
  rollout_verify_heartbeats: number
}

export interface ClusterSummary {
  online_nodes: number
  stale_nodes: number
  stopped_nodes: number
  worker_nodes: number
  active_tasks: number
  unhealthy_nodes: number
}

export interface ClusterInstanceLoad {
  cpu_usage_percent?: number
  memory_used_bytes?: number
  memory_limit_bytes?: number
  memory_usage_percent?: number
  in_flight_requests: number
  active_tasks: number
  goroutine_count: number
  db_connections_active: number
  db_connections_idle: number
  db_connections_max: number
  redis_connections_active: number
  redis_connections_idle: number
  redis_connections_max: number
  collected_at: string
}

export interface ClusterInstance {
  node_id: string
  runner_id: string
  node_name: string
  deployment_mode: string
  worker_mode: string
  worker_enabled: boolean
  version: string
  hostname: string
  process_id: number
  database_ok: boolean
  redis_ok: boolean
  started_at: string
  last_seen_at: string
  stopped_at?: string
  status: ClusterInstanceStatus
  current: boolean
  load?: ClusterInstanceLoad
}

export interface ClusterReleaseState {
  desired_version: string
  active_rollout_id?: string
  generation: number
  updated_at: string
}

export interface ClusterRolloutTarget {
  rollout_id: string
  node_id: string
  node_name: string
  ordinal: number
  source_version: string
  target_version: string
  status: ClusterRolloutTargetStatus
  attempt: number
  lease_owner?: string
  lease_until?: string
  source_runner_id?: string
  observed_runner_id?: string
  verification_count: number
  last_verified_heartbeat?: string
  error_message?: string
  started_at?: string
  completed_at?: string
  created_at: string
  updated_at: string
}

export interface ClusterRollout {
  id: string
  source_version?: string
  target_version: string
  status: ClusterRolloutStatus
  strategy: 'rolling'
  max_unavailable: number
  created_by: number
  error_message?: string
  started_at: string
  completed_at?: string
  created_at: string
  updated_at: string
  targets: ClusterRolloutTarget[]
}

export interface ClusterReleaseOverview {
  state: ClusterReleaseState
  active_rollout?: ClusterRollout
  recent_rollouts: ClusterRollout[]
  version_counts: Array<{ version: string; nodes: number }>
  consistent: boolean
}

export interface ClusterTaskRun {
  id: number
  run_id: string
  task_key: string
  status: ClusterTaskStatus
  node_name: string
  runner_id: string
  metadata: Record<string, unknown>
  result: Record<string, unknown>
  error_message: string
  started_at: string
  heartbeat_at: string
  lease_until: string
  finished_at?: string
}

export interface ClusterStatusResponse {
  deployment: ClusterDeploymentStatus
  summary: ClusterSummary
  instances: ClusterInstance[]
  tasks: ClusterTaskRun[]
  release?: ClusterReleaseOverview
  observed_at: string
}

export async function getClusterStatus(): Promise<ClusterStatusResponse> {
  const { data } = await apiClient.get<ClusterStatusResponse>('/admin/cluster/status')
  return data
}

export async function renameNode(nodeId: string, name: string): Promise<void> {
  await apiClient.put(`/admin/cluster/nodes/${encodeURIComponent(nodeId)}`, { name })
}

export async function createRollout(targetVersion = ''): Promise<ClusterRollout> {
  const { data } = await apiClient.post<ClusterRollout>('/admin/cluster/rollouts', {
    target_version: targetVersion,
    confirm: true,
  })
  return data
}

export async function getRollout(id: string): Promise<ClusterRollout> {
  const { data } = await apiClient.get<ClusterRollout>(
    `/admin/cluster/rollouts/${encodeURIComponent(id)}`
  )
  return data
}

async function mutateRollout(id: string, action: 'pause' | 'resume'): Promise<ClusterRollout> {
  const { data } = await apiClient.post<ClusterRollout>(
    `/admin/cluster/rollouts/${encodeURIComponent(id)}/${action}`
  )
  return data
}

export function pauseRollout(id: string): Promise<ClusterRollout> {
  return mutateRollout(id, 'pause')
}

export function resumeRollout(id: string): Promise<ClusterRollout> {
  return mutateRollout(id, 'resume')
}

export async function cancelRollout(id: string): Promise<ClusterRollout> {
  const { data } = await apiClient.post<ClusterRollout>(
    `/admin/cluster/rollouts/${encodeURIComponent(id)}/cancel`,
    { confirm: true }
  )
  return data
}

export async function retryRolloutTarget(id: string, nodeId: string): Promise<ClusterRollout> {
  const { data } = await apiClient.post<ClusterRollout>(
    `/admin/cluster/rollouts/${encodeURIComponent(id)}/targets/${encodeURIComponent(nodeId)}/retry`
  )
  return data
}

export const clusterAPI = {
  getStatus: getClusterStatus,
  renameNode,
  createRollout,
  getRollout,
  pauseRollout,
  resumeRollout,
  cancelRollout,
  retryRolloutTarget,
}

export default clusterAPI
