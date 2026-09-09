package domain

// GroupModelAllowlist is the persisted group model policy. When enabled it
// constrains both model catalog responses and gateway request admission.
type GroupModelAllowlist struct {
	Enabled bool     `json:"enabled"`
	Models  []string `json:"models,omitempty"`
}
