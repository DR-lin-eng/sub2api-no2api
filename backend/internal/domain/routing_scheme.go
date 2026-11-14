package domain

// Routing scheme status values.
const (
	RoutingSchemeStatusActive   = "active"
	RoutingSchemeStatusDisabled = "disabled"
)

// Scheme-level score weights. The overall score is a weighted average of the
// TTFT score and the error score, so each scheme can trade speed for stability.
// Both weights are 0..100 and only their ratio matters.
const (
	RoutingSchemeDefaultTTFTWeight  = 40
	RoutingSchemeDefaultErrorWeight = 60
	RoutingSchemeMaxScoreWeight     = 100
)

// Routing rule trigger kinds.
const (
	RoutingTriggerTTFT       = "ttft"
	RoutingTriggerErrorClass = "error_class"
	RoutingTriggerErrorRate  = "error_rate"
)

// Routing rule action kinds.
const (
	RoutingActionFailover = "failover"
	RoutingActionCooldown = "cooldown"
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
	Kind             string  `json:"kind"`
	ThresholdSeconds float64 `json:"threshold_seconds,omitempty"`
	// Consecutive is the minimum number of samples in the fixed 15-minute
	// window before an item-level rule may fire. It is not a literal
	// "consecutive requests" counter: the metrics are window aggregates.
	Consecutive    int      `json:"consecutive,omitempty"`
	Classes        []string `json:"classes,omitempty"`
	FailurePercent float64  `json:"failure_percent,omitempty"`
	MinRequests    int      `json:"min_requests,omitempty"`
}

type RoutingSchemeRuleAction struct {
	Kind            string `json:"kind"`
	CooldownSeconds int    `json:"cooldown_seconds"`
}
