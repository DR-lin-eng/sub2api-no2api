package domain

// APIKeyGroupBinding persists one ordered group candidate for an API key.
// A nil MaxRateMultiplier means the candidate has no rate protection ceiling.
type APIKeyGroupBinding struct {
	GroupID           int64    `json:"group_id"`
	MaxRateMultiplier *float64 `json:"max_rate_multiplier,omitempty"`
}
