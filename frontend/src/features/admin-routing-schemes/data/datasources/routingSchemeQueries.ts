import { apiClient } from '@/core/networks/client'
import type { GroupPlatform } from '@/types/group'
import type {
  RoutingScheme,
  RoutingSchemeDraft,
  RoutingSchemeItem,
  RoutingSchemeRule,
  RoutingSchemeSortMode,
  RoutingSchemeStatus,
} from '../dtos/routingSchemeDtos'

interface RoutingSchemeItemDto {
  group_id: number
  weight: number
  rules: RoutingSchemeRule[]
  group_name?: string
  platform?: GroupPlatform
  rate_multiplier?: number
  position?: number
  score?: number
  p95_ttft_ms?: number
  failure_rate?: number
  failure_count?: number
  sample_count?: number
  cold_start?: boolean
}

interface RoutingSchemeDto {
  id: number
  name: string
  description: string
  platform: GroupPlatform
  status: RoutingSchemeStatus
  sort_order: number
  sort_mode: RoutingSchemeSortMode
  items: RoutingSchemeItemDto[]
  created_at: string
  updated_at: string
}

// weight_zero was removed because it behaved exactly like cooldown; fold any
// value saved before that into cooldown so the editor never shows a blank action.
function toRule(rule: RoutingSchemeRule): RoutingSchemeRule {
  if (rule.action?.kind !== 'weight_zero') return rule
  return { ...rule, action: { ...rule.action, kind: 'cooldown' } }
}

function toScheme(dto: RoutingSchemeDto): RoutingScheme {
  return {
    id: dto.id,
    name: dto.name,
    description: dto.description ?? '',
    platform: dto.platform,
    status: dto.status === 'disabled' ? 'disabled' : 'active',
    sort_order: dto.sort_order ?? 0,
    sort_mode: dto.sort_mode ?? 'score',
    items: (dto.items ?? []).map((item): RoutingSchemeItem => ({
      group_id: item.group_id,
      weight: item.weight,
      rules: (item.rules ?? []).map(toRule),
      group_name: item.group_name,
      platform: item.platform,
      rate_multiplier: item.rate_multiplier,
      position: item.position,
      score: item.score,
      p95_ttft_ms: item.p95_ttft_ms,
      failure_rate: item.failure_rate,
      failure_count: item.failure_count,
      sample_count: item.sample_count,
      cold_start: item.cold_start,
    })),
    created_at: dto.created_at,
    updated_at: dto.updated_at,
  }
}

function toPayload(draft: RoutingSchemeDraft) {
  return {
    name: draft.name,
    description: draft.description,
    platform: draft.platform,
    status: draft.status,
    sort_order: draft.sort_order,
    sort_mode: draft.sort_mode,
    items: draft.items.map(item => ({
      group_id: item.group_id,
      weight: item.weight,
      rules: item.rules,
    })),
  }
}

export interface RoutingGroupScoreDto {
  group_id: number
  score: number
  p95_ttft_ms: number
  failure_rate: number
  failure_count: number
  sample_count: number
  cold_start: boolean
}

/**
 * Read-only scores for arbitrary groups. The editor calls this before a scheme
 * exists so even the cold-start value comes from the backend.
 */
export async function listGroupScores(groupIDs: number[]): Promise<RoutingGroupScoreDto[]> {
  if (groupIDs.length === 0) return []
  const { data } = await apiClient.get<RoutingGroupScoreDto[]>('/admin/routing-schemes/group-scores', {
    params: { group_ids: groupIDs.join(',') },
  })
  return data ?? []
}

export async function listRoutingSchemes(options?: {
  platform?: string
  includeDisabled?: boolean
  signal?: AbortSignal
}): Promise<RoutingScheme[]> {
  const { data } = await apiClient.get<RoutingSchemeDto[]>('/admin/routing-schemes', {
    params: {
      ...(options?.platform ? { platform: options.platform } : {}),
      ...(options?.includeDisabled ? { include_disabled: true } : {}),
    },
    signal: options?.signal,
  })
  return (data ?? []).map(toScheme)
}

export async function createRoutingScheme(draft: RoutingSchemeDraft): Promise<RoutingScheme> {
  const { data } = await apiClient.post<RoutingSchemeDto>('/admin/routing-schemes', toPayload(draft))
  return toScheme(data)
}

export async function updateRoutingScheme(id: number, draft: RoutingSchemeDraft): Promise<RoutingScheme> {
  const { data } = await apiClient.put<RoutingSchemeDto>(`/admin/routing-schemes/${id}`, toPayload(draft))
  return toScheme(data)
}

export async function deleteRoutingScheme(id: number): Promise<void> {
  await apiClient.delete(`/admin/routing-schemes/${id}`)
}

export type RoutingSchemeAnalyticsWindow = '24h' | '7d' | '30d'

export interface RoutingSchemeAnalyticsGroupDto {
  group_id: number
  group_name: string
  platform: GroupPlatform
  rate_multiplier: number
  requests: number
  successes: number
  failures: number
  p95_ttft_ms: number
  input_tokens: number
  output_tokens: number
  cost: number
  request_share: number
  cost_share: number
}

export interface RoutingSchemeAnalyticsDto {
  scheme_id: number
  window: RoutingSchemeAnalyticsWindow
  since: string
  requests: number
  cost: number
  input_tokens: number
  output_tokens: number
  groups: RoutingSchemeAnalyticsGroupDto[]
}

export async function getRoutingSchemeAnalytics(
  id: number,
  window: RoutingSchemeAnalyticsWindow,
): Promise<RoutingSchemeAnalyticsDto> {
  const { data } = await apiClient.get<RoutingSchemeAnalyticsDto>(`/admin/routing-schemes/${id}/analytics`, {
    params: { window },
  })
  return data
}
