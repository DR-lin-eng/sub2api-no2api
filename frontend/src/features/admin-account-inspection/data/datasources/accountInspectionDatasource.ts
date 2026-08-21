import { apiClient } from '@/core/networks/client'
import type {
  AccountInspectionOverview,
  AccountInspectionQuery,
  AccountInspectionResult,
  AccountInspectionSettings,
} from '../dtos/accountInspectionDtos'

const defaultAccountInspectionSettings: AccountInspectionSettings = {
  enabled: false,
  interval_minutes: 60,
  auto_disable: true,
  lookback_minutes: 60,
  min_requests: 1,
  ttft_threshold_ms: 30_000,
  success_rate_threshold: 0.6,
  oauth_auto_disable: true,
  api_key_auto_disable: true,
  oauth_min_requests: 1,
  api_key_min_requests: 1,
  oauth_ttft_threshold_ms: 30_000,
  api_key_ttft_threshold_ms: 30_000,
  oauth_success_rate_threshold: 0.6,
  api_key_success_rate_threshold: 0.6,
  oauth_quota_check_enabled: true,
  api_key_quota_check_enabled: true,
  api_key_min_cache_hit_rate: 0,
  api_key_max_rate_multiplier: 0,
  api_key_min_remaining_quota: 0,
  protected_account_ids: [],
}

export function normalizeAccountInspectionSettings(
  raw?: Partial<AccountInspectionSettings> | null,
): AccountInspectionSettings {
  const source = raw ?? {}
  const autoDisable = source.auto_disable ?? defaultAccountInspectionSettings.auto_disable
  const minRequests = source.min_requests ?? defaultAccountInspectionSettings.min_requests
  const ttftThresholdMs = source.ttft_threshold_ms ?? defaultAccountInspectionSettings.ttft_threshold_ms
  const successRateThreshold = source.success_rate_threshold ?? defaultAccountInspectionSettings.success_rate_threshold
  return {
    ...defaultAccountInspectionSettings,
    ...source,
    auto_disable: autoDisable,
    min_requests: minRequests,
    ttft_threshold_ms: ttftThresholdMs,
    success_rate_threshold: successRateThreshold,
    oauth_auto_disable: source.oauth_auto_disable ?? autoDisable,
    api_key_auto_disable: source.api_key_auto_disable ?? autoDisable,
    oauth_min_requests: source.oauth_min_requests ?? minRequests,
    api_key_min_requests: source.api_key_min_requests ?? minRequests,
    oauth_ttft_threshold_ms: source.oauth_ttft_threshold_ms ?? ttftThresholdMs,
    api_key_ttft_threshold_ms: source.api_key_ttft_threshold_ms ?? ttftThresholdMs,
    oauth_success_rate_threshold: source.oauth_success_rate_threshold ?? successRateThreshold,
    api_key_success_rate_threshold: source.api_key_success_rate_threshold ?? successRateThreshold,
    protected_account_ids: Array.isArray(source.protected_account_ids)
      ? [...new Set(source.protected_account_ids.filter((id): id is number => Number.isInteger(id) && id > 0))].sort((left, right) => left - right)
      : [],
  }
}

function normalizeAccountInspectionResult(result: AccountInspectionResult): AccountInspectionResult {
  return {
    ...result,
    // Older snapshots omit `reasons` for healthy accounts. Keep the UI DTO stable
    // while mixed-version backends or previously persisted snapshots exist.
    reasons: Array.isArray(result.reasons)
      ? result.reasons.filter((reason): reason is string => typeof reason === 'string')
      : [],
  }
}

function normalizeAccountInspectionOverview(data: AccountInspectionOverview): AccountInspectionOverview {
  if (!data?.results || !Array.isArray(data.results.items)) return data

  return {
    ...data,
    settings: normalizeAccountInspectionSettings(data?.settings),
    results: {
      ...data.results,
      items: data.results.items.map(normalizeAccountInspectionResult),
    },
  }
}

export async function getOverview(query: AccountInspectionQuery = {}): Promise<AccountInspectionOverview> {
  const { data } = await apiClient.get<AccountInspectionOverview>('/admin/account-inspection', {
    params: query,
  })
  return normalizeAccountInspectionOverview(data)
}

export async function updateSettings(settings: AccountInspectionSettings): Promise<AccountInspectionSettings> {
  const { data } = await apiClient.put<AccountInspectionSettings>('/admin/account-inspection/settings', settings)
  return data
}

export async function runInspection(): Promise<AccountInspectionOverview> {
  const { data } = await apiClient.post<AccountInspectionOverview>('/admin/account-inspection/run')
  return normalizeAccountInspectionOverview(data)
}
