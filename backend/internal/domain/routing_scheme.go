package domain

// Routing scheme status values.
const (
	RoutingSchemeStatusActive   = "active"
	RoutingSchemeStatusDisabled = "disabled"
)

// Routing scheme sort modes. They decide the per-request candidate order.
const (
	RoutingSortScore      = "score"
	RoutingSortTTFT       = "ttft"
	RoutingSortErrorCount = "error_count"
)

// Routing rule trigger kinds.
const (
	RoutingTriggerTTFT       = "ttft"
	RoutingTriggerErrorClass = "error_class"
	RoutingTriggerErrorRate  = "error_rate"
)

// Routing rule action kinds.
const (
	RoutingActionFailover   = "failover"
	RoutingActionCooldown   = "cooldown"
	RoutingActionWeightZero = "weight_zero"
)

// RoutingSchemeRule is a declarative failover rule attached to one scheme item.
// Trigger uses a flat shape so new trigger kinds can add optional fields without
// breaking persisted JSON.
type RoutingSchemeRule struct {
	ID      string                   `json:"id"`
	Enabled bool                     `json:"enabled"`
	Trigger RoutingSchemeRuleTrigger `json:"trigger"`
	Action  RoutingSchemeRuleAction  `json:"action"`
}

type RoutingSchemeRuleTrigger struct {
	Kind             string   `json:"kind"`
	ThresholdSeconds float64  `json:"threshold_seconds,omitempty"`
	WindowSeconds    int      `json:"window_seconds,omitempty"`
	Consecutive      int      `json:"consecutive,omitempty"`
	Classes          []string `json:"classes,omitempty"`
	FailurePercent   float64  `json:"failure_percent,omitempty"`
	MinRequests      int      `json:"min_requests,omitempty"`
}

type RoutingSchemeRuleAction struct {
	Kind            string `json:"kind"`
	CooldownSeconds int    `json:"cooldown_seconds"`
}
