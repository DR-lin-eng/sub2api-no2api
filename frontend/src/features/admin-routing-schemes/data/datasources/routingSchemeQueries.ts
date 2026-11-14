import { apiClient } from '@/core/networks/client'
import type { GroupPlatform } from '@/types/group'
import { ROUTING_RULE_ERROR_CLASSES } from '../routingRuleCatalog'
import type {
  RoutingScheme,
  RoutingSchemeDraft,
  RoutingSchemeItem,
  RoutingSchemeRule,
  RoutingSchemeStatus,
} from '../dtos/routingSchemeDtos'

interface RoutingSchemeItemDto {
  group_id: number
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
  ttft_weight?: number
  error_weight?: number
  items: RoutingSchemeItemDto[]
  created_at: string
  updated_at: string
}

// weight_zero was removed because it behaved exactly like cooldown; fold any
// value saved before that into cooldown so the editor never shows a blank action.
// empty_stream is no longer reported by the metrics repository, so legacy rules
// that only watched it could never fire and are dropped.
function toRule(rule: RoutingSchemeRule): RoutingSchemeRule | null {
  let next = rule
  if (next.action?.kind === 'weight_zero') {
    next = { ...next, action: { ...next.action, kind: 'cooldown' } }
  }
  if (next.trigger?.kind === 'error_class') {
    const classes = (next.trigger.classes ?? [])
      .filter(className => ROUTING_RULE_ERROR_CLASSES.includes(className))
    if (classes.length === 0) return null
    next = { ...next, trigger: { ...next.trigger, classes } }
  }
  return next
}

function toScheme(dto: RoutingSchemeDto): RoutingScheme {
  return {
    id: dto.id,
    name: dto.name,
    description: dto.description ?? '',
    platform: dto.platform,
    status: dto.status === 'disabled' ? 'disabled' : 'active',
    sort_order: dto.sort_order ?? 0,
    ttft_weight: dto.ttft_weight ?? 40,
    error_weight: dto.error_weight ?? 60,
    items: (dto.items ?? []).map((item): RoutingSchemeItem => ({
      group_id: item.group_id,
      rules: (item.rules ?? [])
        .map(toRule)
        .filter((rule): rule is RoutingSchemeRule => rule !== null),
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
    ttft_weight: draft.ttft_weight,
    error_weight: draft.error_weight,
    items: draft.items.map(item => ({
      group_id: item.group_id,
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
 * exists so even the cold-start value and the weighting come from the backend.
 */
export async function listGroupScores(
  groupIDs: number[],
  ttftWeight?: number,
  errorWeight?: number,
): Promise<RoutingGroupScoreDto[]> {
  if (groupIDs.length === 0) return []
  const { data } = await apiClient.get<RoutingGroupScoreDto[]>('/admin/routing-schemes/group-scores', {
    params: {
      group_ids: groupIDs.join(','),
      ...(ttftWeight != null ? { ttft_weight: ttftWeight } : {}),
      ...(errorWeight != null ? { error_weight: errorWeight } : {}),
    },
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
  /** successes / requests, already normalized by the backend. */
  success_rate: number
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
