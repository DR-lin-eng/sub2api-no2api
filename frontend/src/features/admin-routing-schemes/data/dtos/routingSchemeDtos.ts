import type { GroupPlatform, SubscriptionType } from '@/types/group'

export type RoutingSchemeStatus = 'active' | 'disabled'

/** Trigger kinds stay string-based so new policies add no migration. */
export type RoutingRuleTriggerKind = 'ttft' | 'error_class' | 'error_rate'
export type RoutingRuleActionKind = 'failover' | 'cooldown' | 'weight_zero'

export type RoutingRuleErrorClass =
  | 'timeout'
  | 'http_429'
  | 'http_5xx'
  | 'auth'
  | 'model_unavailable'

export type RoutingRuleTrigger =
  | {
      kind: 'ttft'
      /** First-token threshold in seconds. */
      threshold_seconds: number
      /** Minimum samples in the fixed 15-minute window before the rule fires. */
      consecutive: number
    }
  | {
      kind: 'error_class'
      classes: RoutingRuleErrorClass[]
    }
  | {
      kind: 'error_rate'
      failure_percent: number
      min_requests: number
    }

export type RoutingRuleAction =
  | { kind: 'failover'; cooldown_seconds: number }
  | { kind: 'cooldown'; cooldown_seconds: number }
  | { kind: 'weight_zero'; cooldown_seconds: number }

export interface RoutingSchemeRule {
  id: string
  enabled: boolean
  trigger: RoutingRuleTrigger
  action: RoutingRuleAction
}

export interface RoutingSchemeItem {
  group_id: number
  rules: RoutingSchemeRule[]
  /** Read-only projections returned by the admin API. */
  group_name?: string
  platform?: GroupPlatform
  rate_multiplier?: number
  position?: number
  /** Backend-computed composite score and rolling metrics. */
  score?: number
  p95_ttft_ms?: number
  failure_rate?: number
  failure_count?: number
  sample_count?: number
  cold_start?: boolean
}

export interface RoutingScheme {
  id: number
  name: string
  description: string
  platform: GroupPlatform
  status: RoutingSchemeStatus
  sort_order: number
  /** Scheme-level score weights (0-100). Only their ratio matters. */
  ttft_weight: number
  error_weight: number
  items: RoutingSchemeItem[]
  created_at: string
  updated_at: string
}

export interface RoutingSchemeDraft {
  name: string
  description: string
  platform: GroupPlatform
  status: RoutingSchemeStatus
  sort_order: number
  ttft_weight: number
  error_weight: number
  items: RoutingSchemeItem[]
}

/** Minimal group shape shared by the admin and user preview data sources. */
export interface RoutingSchemeGroupOption {
  id: number
  name: string
  platform: GroupPlatform
  status?: string
  subscription_type?: SubscriptionType
  rate_multiplier?: number
  description?: string | null
}
