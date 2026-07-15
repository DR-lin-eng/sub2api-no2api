import { apiClient } from '../client'

export type ClusterInstanceStatus = 'online' | 'stale' | 'stopped'
export type ClusterTaskStatus = 'running' | 'succeeded' | 'failed' | 'lost'

export interface ClusterDeploymentStatus {
  mode: 'standalone' | 'multi_instance'
  node_name: string
  runner_id: string
  worker_mode: 'auto' | 'true' | 'false'
  worker_enabled: boolean
  frontend_enabled: boolean
  heartbeat_interval_seconds: number
  stale_after_seconds: number
  task_lease_seconds: number
}

export interface ClusterSummary {
  online_nodes: number
  stale_nodes: number
  stopped_nodes: number
  worker_nodes: number
  active_tasks: number
  unhealthy_nodes: number
}

export interface ClusterInstance {
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
  observed_at: string
}

export async function getClusterStatus(): Promise<ClusterStatusResponse> {
  const { data } = await apiClient.get<ClusterStatusResponse>('/admin/cluster/status')
  return data
}

export const clusterAPI = { getStatus: getClusterStatus }

export default clusterAPI

