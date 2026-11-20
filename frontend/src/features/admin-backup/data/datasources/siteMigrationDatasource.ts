import { apiClient } from '@/core/networks/client'

export type MigrationDirection = 'push' | 'pull'
export type MigrationScope = 'all' | 'login' | 'login-quota' | 'clone'
export type MigrationPhase = 'connecting' | 'preparing' | 'transferring' | 'applying' | 'reporting' | 'completed' | 'failed' | 'interrupted'
export interface MigrationJob {
  id: string
  direction: MigrationDirection
  profile: MigrationScope
  peer: string
  phase: MigrationPhase
  error_code?: string
  acknowledged: number
  total: number
  inserted: number
  issues: number
  report_bytes: number
  created_at: string
  expires_at: string
}
export interface MigrationForm {
  id: string
  direction: MigrationDirection
  peer: string
  profile: MigrationScope
  key: string
}
const base = '/admin/migration-console/jobs'
export function secureMigrationOrigin(): boolean {
  return window.location.protocol === 'https:' && new URL(apiClient.defaults.baseURL || '/', window.location.href).protocol === 'https:'
}
export async function listMigrationJobs(signal?: AbortSignal): Promise<MigrationJob[]> {
  return (await apiClient.get<MigrationJob[]>(base, { signal })).data
}
export async function startMigration(form: MigrationForm): Promise<MigrationJob> {
  return (await apiClient.post<MigrationJob>(base, form)).data
}
export async function resumeMigration(id: string, key: string): Promise<MigrationJob> {
  return (await apiClient.post<MigrationJob>(`${base}/${encodeURIComponent(id)}/resume`, { key })).data
}
export async function downloadMigrationReport(id: string): Promise<Blob> {
  return (await apiClient.get<Blob>(`${base}/${encodeURIComponent(id)}/report`, { responseType: 'blob', timeout: 300000 })).data
}
