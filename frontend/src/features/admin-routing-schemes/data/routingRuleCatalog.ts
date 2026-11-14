import type {
  RoutingRuleAction,
  RoutingRuleActionKind,
  RoutingRuleErrorClass,
  RoutingRuleTrigger,
  RoutingRuleTriggerKind,
  RoutingSchemeRule,
} from './dtos/routingSchemeDtos'

/**
 * UI catalog for routing policy rules. New trigger/action kinds only need an
 * entry here plus backend evaluation; the editor renders from this list.
 */
export const ROUTING_RULE_TRIGGER_KINDS: RoutingRuleTriggerKind[] = [
  'ttft',
  'error_class',
  'error_rate',
]

// weight_zero duplicates the cooldown behavior in the backend, so it is not
// offered as a separate choice.
export const ROUTING_RULE_ACTION_KINDS: RoutingRuleActionKind[] = [
  'failover',
  'cooldown',
]

export const ROUTING_RULE_ERROR_CLASSES: RoutingRuleErrorClass[] = [
  'timeout',
  'http_429',
  'http_5xx',
  'empty_stream',
  'auth',
  'model_unavailable',
]

let ruleSequence = 0

export function createRuleID(): string {
  ruleSequence += 1
  return `rule-${Date.now().toString(36)}-${ruleSequence}`
}

export function defaultTrigger(kind: RoutingRuleTriggerKind): RoutingRuleTrigger {
  if (kind === 'error_class') {
    return { kind: 'error_class', classes: ['timeout', 'http_5xx', 'empty_stream'] }
  }
  if (kind === 'error_rate') {
    return { kind: 'error_rate', failure_percent: 30, window_seconds: 300, min_requests: 10 }
  }
  return { kind: 'ttft', threshold_seconds: 5, window_seconds: 120, consecutive: 3 }
}

export function defaultAction(kind: RoutingRuleActionKind): RoutingRuleAction {
  if (kind === 'cooldown') {
    return { kind: 'cooldown', cooldown_seconds: 120 }
  }
  if (kind === 'weight_zero') {
    return { kind: 'weight_zero', cooldown_seconds: 180 }
  }
  return { kind: 'failover', cooldown_seconds: 60 }
}

export function createRouteRule(): RoutingSchemeRule {
  return {
    id: createRuleID(),
    enabled: true,
    trigger: defaultTrigger('ttft'),
    action: defaultAction('failover'),
  }
}

const isPositiveNumber = (value: unknown): value is number =>
  typeof value === 'number' && Number.isFinite(value) && value > 0

/** Defensive hydration for preview storage that may predate the rules field. */
export function normalizeRule(value: unknown): RoutingSchemeRule | null {
  if (!value || typeof value !== 'object') return null
  const rule = value as Partial<RoutingSchemeRule>
  const trigger = rule.trigger as Partial<RoutingRuleTrigger> | undefined
  const action = rule.action as Partial<RoutingRuleAction> | undefined
  if (!trigger || !action) return null

  let normalizedTrigger: RoutingRuleTrigger
  if (trigger.kind === 'ttft') {
    normalizedTrigger = {
      kind: 'ttft',
      threshold_seconds: isPositiveNumber(trigger.threshold_seconds) ? trigger.threshold_seconds : 5,
      window_seconds: isPositiveNumber(trigger.window_seconds) ? trigger.window_seconds : 120,
      consecutive: isPositiveNumber(trigger.consecutive) ? trigger.consecutive : 3,
    }
  } else if (trigger.kind === 'error_class') {
    const classes = Array.isArray(trigger.classes)
      ? trigger.classes.filter((item): item is RoutingRuleErrorClass =>
        ROUTING_RULE_ERROR_CLASSES.includes(item as RoutingRuleErrorClass))
      : []
    normalizedTrigger = {
      kind: 'error_class',
      classes: classes.length > 0 ? classes : ['timeout'],
    }
  } else if (trigger.kind === 'error_rate') {
    normalizedTrigger = {
      kind: 'error_rate',
      failure_percent: isPositiveNumber(trigger.failure_percent) ? trigger.failure_percent : 30,
      window_seconds: isPositiveNumber(trigger.window_seconds) ? trigger.window_seconds : 300,
      min_requests: isPositiveNumber(trigger.min_requests) ? trigger.min_requests : 10,
    }
  } else {
    return null
  }

  if (!ROUTING_RULE_ACTION_KINDS.includes(action.kind as RoutingRuleActionKind)) return null
  const normalizedAction: RoutingRuleAction = {
    kind: action.kind as RoutingRuleActionKind,
    cooldown_seconds: isPositiveNumber(action.cooldown_seconds) ? action.cooldown_seconds : 60,
  }

  return {
    id: typeof rule.id === 'string' && rule.id ? rule.id : createRuleID(),
    enabled: rule.enabled !== false,
    trigger: normalizedTrigger,
    action: normalizedAction,
  }
}

export function normalizeRules(value: unknown): RoutingSchemeRule[] {
  if (!Array.isArray(value)) return []
  return value
    .map(item => normalizeRule(item))
    .filter((item): item is RoutingSchemeRule => item !== null)
}
