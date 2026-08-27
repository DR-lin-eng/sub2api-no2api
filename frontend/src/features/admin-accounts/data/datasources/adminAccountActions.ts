import { apiClient } from '@/core/networks/client'
import type {
  Account,
  AdminDataImportResult,
  AdminDataPayload,
  OllamaCloudUsageSettings,
  OllamaCloudUsageState,
  UpstreamBillingProbeResult,
  UpstreamQuotaQueryResult
} from '@/types'
import type {
  CheckMixedChannelRequest,
  CheckMixedChannelResponse,
  CodexSessionImportRequest,
  CodexSessionImportResult,
  CreateAccountRequest,
  OpenAICodexPATCreateRequest,
  UpdateAccountRequest
} from '../dtos/adminAccountDtos'
import type { CRSConnectionParams } from './adminAccountQueries'
import type { OpenAIQuotaRefreshResult } from '../dtos/openAIQuotaDtos'

export interface CPACapacityStatus {
  total_credentials: number
  enabled_credentials: number
  abnormal_credentials: number
  available_credentials: number
  capacity_credentials?: number
  effective_concurrency: number
  concurrency_per_credential: number
  exclude_abnormal_credentials?: boolean
  fetched_at?: string
  state: 'fresh' | 'stale' | 'unavailable'
}

export interface BatchOperationResult {
  total: number
  success: number
  failed: number
  success_ids?: number[]
  failed_ids?: number[]
  errors?: Array<{ account_id: number; error: string }>
  warnings?: Array<{ account_id: number; warning: string }>
}

export interface BulkUpdateResult {
  success: number
  failed: number
  success_ids?: number[]
  failed_ids?: number[]
  results: Array<{ account_id: number; success: boolean; error?: string }>
}

export interface OpenAIQuotaRefreshBatchResult {
  results: Record<string, OpenAIQuotaRefreshResult>
  errors: Record<string, string>
  skipped_account_ids: number[]
}

export interface AccountExportOptions {
  ids?: number[]
  filters?: {
    platform?: string
    type?: string
    status?: string
    oauth_quota?: string
    group?: string
    privacy_mode?: string
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  }
  includeProxies?: boolean
}

export interface SparkShadowCreatePayload {
  name?: string
  priority?: number
  concurrency?: number
  group_ids?: number[]
}

export interface CPATestRequest {
  use_account_base_url?: boolean
  base_url?: string
  management_url?: string
  management_password?: string
  concurrency_per_credential?: number
  exclude_abnormal_credentials?: boolean
}

export interface CPATestResult extends CPACapacityStatus {
  latency_ms: number
}

export interface SyncUpstreamModelsResult {
  models: string[]
}

export interface SyncUpstreamPreviewParams {
  platform: string
  type: string
  base_url?: string
  api_key: string
}

export interface CRSSyncParams extends CRSConnectionParams {
  sync_proxies?: boolean
  selected_account_ids?: string[]
}

export interface CRSSyncItem {
  crs_account_id: string
  kind: string
  name: string
  action: string
  error?: string
}

export type CRSSyncResult = {
  created: number
  updated: number
  skipped: number
  failed: number
  items: CRSSyncItem[]
}

export interface AccountDataImportPayload {
  data: AdminDataPayload
  skip_default_group_bind?: boolean
}

export async function createAccount(accountData: CreateAccountRequest): Promise<Account> {
  const { data } = await apiClient.post<Account>('/admin/accounts', accountData)
  return data
}

/** Update account fields while keeping the request owned by this feature. */
export async function updateAccount(id: number, updates: UpdateAccountRequest): Promise<Account> {
  const { data } = await apiClient.put<Account>(`/admin/accounts/${id}`, updates)
  return data
}

export async function testCPAConnection(
  id: number,
  payload: CPATestRequest
): Promise<CPATestResult> {
  const { data } = await apiClient.post<CPATestResult>(`/admin/accounts/${id}/cpa/test`, payload)
  return data
}

export async function syncUpstreamModels(id: number): Promise<SyncUpstreamModelsResult> {
  const { data } = await apiClient.post<SyncUpstreamModelsResult>(
    `/admin/accounts/${id}/models/sync-upstream`
  )
  return data
}

export async function syncUpstreamModelsPreview(
  params: SyncUpstreamPreviewParams
): Promise<SyncUpstreamModelsResult> {
  const { data } = await apiClient.post<SyncUpstreamModelsResult>(
    '/admin/accounts/models/sync-upstream-preview',
    params
  )
  return data
}

export async function importCodexSession(
  payload: CodexSessionImportRequest
): Promise<CodexSessionImportResult> {
  const { data } = await apiClient.post<CodexSessionImportResult>(
    '/admin/accounts/import/codex-session',
    payload,
    { timeout: 120000 }
  )
  return data
}

export async function createOpenAICodexPAT(
  payload: OpenAICodexPATCreateRequest
): Promise<Account> {
  const { data } = await apiClient.post<Account>('/admin/openai/create-from-codex-pat', payload)
  return data
}

export async function syncFromCrs(params: CRSSyncParams): Promise<CRSSyncResult> {
  const { data } = await apiClient.post<CRSSyncResult>(
    '/admin/accounts/sync/crs',
    params,
    { timeout: 180000 }
  )
  return data
}

export async function importData(
  payload: AccountDataImportPayload
): Promise<AdminDataImportResult> {
  const { data } = await apiClient.post<AdminDataImportResult>('/admin/accounts/data', {
    data: payload.data,
    skip_default_group_bind: payload.skip_default_group_bind
  })
  return data
}

/** Apply refreshed OAuth credentials without replacing persistent account settings. */
export async function applyOAuthCredentials(
  id: number,
  payload: {
    type: 'oauth' | 'setup-token'
    credentials: Record<string, unknown>
    extra?: Record<string, unknown>
  }
): Promise<Account> {
  const { data } = await apiClient.post<Account>(
    `/admin/accounts/${id}/apply-oauth-credentials`,
    payload
  )
  return data
}

export async function clearAccountError(id: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${id}/clear-error`)
  return data
}

const duplicateOperationKeys = new Map<number, string>()

function duplicateOperationStorageKey(id: number): string {
  return `sub2api:admin:account-duplicate:${id}`
}

function getStoredDuplicateOperationKey(id: number): string | null {
  try {
    return globalThis.sessionStorage?.getItem(duplicateOperationStorageKey(id)) ?? null
  } catch {
    return null
  }
}

function storeDuplicateOperationKey(id: number, key: string | null): void {
  try {
    if (key) globalThis.sessionStorage?.setItem(duplicateOperationStorageKey(id), key)
    else globalThis.sessionStorage?.removeItem(duplicateOperationStorageKey(id))
  } catch {
    // In-memory retry protection still works when browser storage is unavailable.
  }
}

export async function duplicate(id: number): Promise<Account> {
  let idempotencyKey = duplicateOperationKeys.get(id) ?? getStoredDuplicateOperationKey(id)
  if (!idempotencyKey) {
    const requestID = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
    idempotencyKey = `account-duplicate-${id}-${requestID}`
  }
  duplicateOperationKeys.set(id, idempotencyKey)
  storeDuplicateOperationKey(id, idempotencyKey)
  const { data } = await apiClient.post<Account>(`/admin/accounts/${id}/duplicate`, undefined, {
    headers: { 'Idempotency-Key': idempotencyKey }
  })
  duplicateOperationKeys.delete(id)
  storeDuplicateOperationKey(id, null)
  return data
}

export async function deleteAccount(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/accounts/${id}`)
  return data
}

export async function syncCPACapacity(id: number): Promise<CPACapacityStatus> {
  const { data } = await apiClient.post<CPACapacityStatus>(`/admin/accounts/${id}/cpa/sync`)
  return data
}

export async function refreshCredentials(id: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${id}/refresh`)
  return data
}

export async function recoverState(id: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${id}/recover-state`)
  return data
}

export async function resetAccountQuota(id: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${id}/reset-quota`)
  return data
}

export async function bulkUpdate(
  accountIdsOrPayload: number[] | Record<string, unknown>,
  updates?: Record<string, unknown>
): Promise<BulkUpdateResult> {
  const payload = Array.isArray(accountIdsOrPayload)
    ? { account_ids: accountIdsOrPayload, ...(updates ?? {}) }
    : accountIdsOrPayload
  const { data } = await apiClient.post<BulkUpdateResult>('/admin/accounts/bulk-update', payload)
  return data
}

export async function checkMixedChannelRisk(
  payload: CheckMixedChannelRequest
): Promise<CheckMixedChannelResponse> {
  const { data } = await apiClient.post<CheckMixedChannelResponse>(
    '/admin/accounts/check-mixed-channel',
    payload
  )
  return data
}

export async function setSchedulable(id: number, schedulable: boolean): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${id}/schedulable`, { schedulable })
  return data
}

export async function exportData(options?: AccountExportOptions): Promise<AdminDataPayload> {
  const params: Record<string, string> = {}
  if (options?.ids && options.ids.length > 0) {
    params.ids = options.ids.join(',')
  } else if (options?.filters) {
    const { platform, type, status, oauth_quota, group, privacy_mode, search, sort_by, sort_order } = options.filters
    if (platform) params.platform = platform
    if (type) params.type = type
    if (status) params.status = status
    if (oauth_quota) params.oauth_quota = oauth_quota
    if (group) params.group = group
    if (privacy_mode) params.privacy_mode = privacy_mode
    if (search) params.search = search
    if (sort_by) params.sort_by = sort_by
    if (sort_order) params.sort_order = sort_order
  }
  if (options?.includeProxies === false) params.include_proxies = 'false'

  const { data } = await apiClient.get<AdminDataPayload>('/admin/accounts/data', { params })
  return data
}

export async function batchDelete(accountIds: number[]): Promise<BatchOperationResult> {
  const { data } = await apiClient.post<BatchOperationResult>('/admin/accounts/batch-delete', {
    account_ids: accountIds
  })
  return data
}

export async function revertProxyFallback(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(`/admin/accounts/${id}/revert-proxy-fallback`)
  return data
}

export async function batchClearError(accountIds: number[]): Promise<BatchOperationResult> {
  const { data } = await apiClient.post<BatchOperationResult>('/admin/accounts/batch-clear-error', {
    account_ids: accountIds
  })
  return data
}

export async function batchRefresh(accountIds: number[]): Promise<BatchOperationResult> {
  const { data } = await apiClient.post<BatchOperationResult>('/admin/accounts/batch-refresh', {
    account_ids: accountIds
  }, {
    timeout: 120000
  })
  return data
}

export async function setPrivacy(id: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${id}/set-privacy`)
  return data
}

export async function createSparkShadow(parentId: number, payload: SparkShadowCreatePayload): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${parentId}/shadow`, payload)
  return data
}

export async function probeUpstreamBilling(id: number): Promise<UpstreamBillingProbeResult> {
  const { data } = await apiClient.post<UpstreamBillingProbeResult>(`/admin/accounts/${id}/upstream-billing-probe`)
  return data
}

export async function probeUpstreamBillingBatch(accountIds: number[]): Promise<UpstreamBillingProbeResult[]> {
  const { data } = await apiClient.post<{ results: UpstreamBillingProbeResult[] }>(
    '/admin/accounts/upstream-billing-probe/batch',
    { account_ids: accountIds },
    { timeout: 120000 }
  )
  return data.results
}

export async function queryUpstreamQuota(id: number): Promise<UpstreamQuotaQueryResult> {
  const { data } = await apiClient.post<UpstreamQuotaQueryResult>(`/admin/accounts/${id}/upstream-quota/query`)
  return data
}

export async function refreshOpenAIQuotaBatch(accountIds: number[]): Promise<OpenAIQuotaRefreshBatchResult> {
  const { data } = await apiClient.post<OpenAIQuotaRefreshBatchResult>(
    '/admin/openai/accounts/quota/refresh/batch',
    { account_ids: accountIds },
    { timeout: 180000 }
  )
  return data
}

export async function updateOllamaCloudUsageSettings(
  settings: OllamaCloudUsageSettings
): Promise<OllamaCloudUsageSettings> {
  const { data } = await apiClient.put<OllamaCloudUsageSettings>(
    '/admin/accounts/ollama-cloud-usage/settings',
    settings
  )
  return data
}

export async function saveOllamaCloudUsageSession(
  id: number,
  session: string
): Promise<OllamaCloudUsageState> {
  const { data } = await apiClient.put<OllamaCloudUsageState>(
    `/admin/accounts/${id}/ollama-cloud-usage/session`,
    { session }
  )
  return data
}

export async function deleteOllamaCloudUsageSession(id: number): Promise<OllamaCloudUsageState> {
  const { data } = await apiClient.delete<OllamaCloudUsageState>(
    `/admin/accounts/${id}/ollama-cloud-usage/session`
  )
  return data
}

export async function setOllamaCloudUsageAutoRefresh(
  id: number,
  enabled: boolean
): Promise<OllamaCloudUsageState> {
  const { data } = await apiClient.put<OllamaCloudUsageState>(
    `/admin/accounts/${id}/ollama-cloud-usage/auto-refresh`,
    { enabled }
  )
  return data
}

export async function refreshOllamaCloudUsage(id: number): Promise<OllamaCloudUsageState> {
  const { data } = await apiClient.post<OllamaCloudUsageState>(
    `/admin/accounts/${id}/ollama-cloud-usage/refresh`
  )
  return data
}
