import { apiClient } from '@/core/networks/client'
import type {
  Account,
  AccountUsageInfo,
  AccountUsageStatsResponse,
  OllamaCloudUsageSettings,
  OllamaCloudUsageState,
  PaginatedResponse,
  UpstreamBillingProbeSettings,
  UpstreamBillingRatesResponse,
  WindowStats
} from '@/types'
import type { ClaudeModel, TempUnschedulableStatus } from '../dtos/adminAccountDtos'
import { loadRuntimeModelCapabilities } from '@/features/custom-model-config/modelCapabilityCache'

export interface AccountListFilters {
  platform?: string
  type?: string
  status?: string
  oauth_quota?: string
  group?: string
  search?: string
  privacy_mode?: string
  lite?: string
  include_scheduler_score?: string
  include_hourly_usage?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export interface AccountListOptions {
  signal?: AbortSignal
}

export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: AccountListFilters,
  options?: AccountListOptions
): Promise<PaginatedResponse<Account>> {
  const { data } = await apiClient.get<PaginatedResponse<Account>>('/admin/accounts', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    },
    signal: options?.signal
  })
  return data
}

export interface AccountListWithEtagResult {
  notModified: boolean
  etag: string | null
  data: PaginatedResponse<Account> | null
}

export interface AccountListWithEtagOptions extends AccountListOptions {
  etag?: string | null
}

export async function listWithEtag(
  page: number = 1,
  pageSize: number = 20,
  filters?: AccountListFilters,
  options?: AccountListWithEtagOptions
): Promise<AccountListWithEtagResult> {
  const headers: Record<string, string> = {}
  if (options?.etag) headers['If-None-Match'] = options.etag

  const response = await apiClient.get<PaginatedResponse<Account>>('/admin/accounts', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    },
    headers,
    signal: options?.signal,
    validateStatus: (status) => (status >= 200 && status < 300) || status === 304
  })

  const etag = typeof response.headers?.etag === 'string' ? response.headers.etag : null
  return response.status === 304
    ? { notModified: true, etag, data: null }
    : { notModified: false, etag, data: response.data }
}

export type AccountUpstreamBillingRateFilters = Pick<
  AccountListFilters,
  'platform' | 'type' | 'status' | 'oauth_quota' | 'group' | 'search' | 'privacy_mode' | 'sort_by' | 'sort_order'
>

export interface AccountUpstreamBillingRatesWithEtagResult {
  notModified: boolean
  etag: string | null
  data: UpstreamBillingRatesResponse | null
}

export async function getUpstreamBillingRatesWithEtag(
  page: number = 1,
  pageSize: number = 20,
  filters?: AccountUpstreamBillingRateFilters,
  options?: AccountListWithEtagOptions
): Promise<AccountUpstreamBillingRatesWithEtagResult> {
  const headers: Record<string, string> = {}
  if (options?.etag) headers['If-None-Match'] = options.etag

  const response = await apiClient.get<UpstreamBillingRatesResponse>('/admin/accounts/upstream-billing-rates', {
    params: { page, page_size: pageSize, ...filters },
    headers,
    signal: options?.signal,
    validateStatus: (status) => (status >= 200 && status < 300) || status === 304
  })
  const etag = typeof response.headers?.etag === 'string' ? response.headers.etag : null
  return response.status === 304
    ? { notModified: true, etag, data: null }
    : { notModified: false, etag, data: response.data }
}

export async function getById(id: number): Promise<Account> {
  const { data } = await apiClient.get<Account>(`/admin/accounts/${id}`)
  return data
}

export async function getStats(id: number, days: number = 30): Promise<AccountUsageStatsResponse> {
  const { data } = await apiClient.get<AccountUsageStatsResponse>(`/admin/accounts/${id}/stats`, {
    params: { days }
  })
  return data
}

export async function getUsage(
  id: number,
  source?: 'passive' | 'active',
  force?: boolean
): Promise<AccountUsageInfo> {
  const params: Record<string, string> = {}
  if (source) params.source = source
  if (force) params.force = 'true'
  const { data } = await apiClient.get<AccountUsageInfo>(`/admin/accounts/${id}/usage`, {
    params: Object.keys(params).length > 0 ? params : undefined
  })
  return data
}

export async function getTempUnschedulableStatus(id: number): Promise<TempUnschedulableStatus> {
  const { data } = await apiClient.get<TempUnschedulableStatus>(
    `/admin/accounts/${id}/temp-unschedulable`
  )
  return data
}

export async function getOllamaCloudUsageSettings(): Promise<OllamaCloudUsageSettings> {
  const { data } = await apiClient.get<OllamaCloudUsageSettings>('/admin/accounts/ollama-cloud-usage/settings')
  return data
}

export async function getOllamaCloudUsage(id: number): Promise<OllamaCloudUsageState> {
  const { data } = await apiClient.get<OllamaCloudUsageState>(`/admin/accounts/${id}/ollama-cloud-usage`)
  return data
}

export interface AccountSummary {
  id: number
  name: string
}

export async function getBatchSummaries(accountIds: number[]): Promise<AccountSummary[]> {
  const { data } = await apiClient.post<{ items: AccountSummary[] }>(
    '/admin/accounts/summaries/batch',
    { account_ids: accountIds }
  )
  return data.items
}

export async function getTodayStats(id: number): Promise<WindowStats> {
  const { data } = await apiClient.get<WindowStats>(`/admin/accounts/${id}/today-stats`)
  return data
}

export interface BatchTodayStatsResponse {
  stats: Record<string, WindowStats>
}

export async function getBatchTodayStats(accountIds: number[]): Promise<BatchTodayStatsResponse> {
  const { data } = await apiClient.post<BatchTodayStatsResponse>('/admin/accounts/today-stats/batch', {
    account_ids: accountIds
  })
  return data
}

export async function getAvailableModels(id: number): Promise<ClaudeModel[]> {
	const [response] = await Promise.all([
		apiClient.get<ClaudeModel[]>(`/admin/accounts/${id}/models`),
		loadRuntimeModelCapabilities().catch(() => undefined),
	])
	const { data } = response
	return data
}

export async function getUpstreamBillingProbeSettings(): Promise<UpstreamBillingProbeSettings> {
  const { data } = await apiClient.get<UpstreamBillingProbeSettings>('/admin/accounts/upstream-billing-probe/settings')
  return data
}

export async function getAntigravityDefaultModelMapping(): Promise<Record<string, string>> {
  const { data } = await apiClient.get<Record<string, string>>(
    '/admin/accounts/antigravity/default-model-mapping'
  )
  return data
}

export interface CRSConnectionParams {
  base_url: string
  username: string
  password: string
}

export interface CRSPreviewAccount {
  crs_account_id: string
  kind: string
  name: string
  platform: string
  type: string
}

export interface PreviewFromCRSResult {
  new_accounts: CRSPreviewAccount[]
  existing_accounts: CRSPreviewAccount[]
}

export async function previewFromCrs(
  params: CRSConnectionParams
): Promise<PreviewFromCRSResult> {
  const { data } = await apiClient.post<PreviewFromCRSResult>(
    '/admin/accounts/sync/crs/preview',
    params
  )
  return data
}
