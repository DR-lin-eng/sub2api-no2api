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
    return { kind: 'error_class', classes: ['timeout', 'http_5xx'] }
  }
  if (kind === 'error_rate') {
    return { kind: 'error_rate', failure_percent: 30, min_requests: 10 }
  }
  return { kind: 'ttft', threshold_seconds: 5, consecutive: 3 }
}

export function defaultAction(kind: RoutingRuleActionKind): RoutingRuleAction {
  if (kind === 'cooldown') {
    return { kind: 'cooldown', cooldown_seconds: 120 }
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
