// Package codexsimulation contains process-wide runtime gates shared by the
// admin control plane and low-level Codex transport adapters.
package codexsimulation

import "sync/atomic"

var cLevelEnabled atomic.Bool
var prewarmContinuationEnabled atomic.Bool
var experimentalTransportEnabled atomic.Bool

// SetCLevelEnabled updates the administrator-controlled C-level transport
// simulation switch. The setting service is the authoritative writer.
func SetCLevelEnabled(enabled bool) {
	cLevelEnabled.Store(enabled)
}

// CLevelEnabled reports the current C-level transport simulation state.
// Request adapters use an atomic read and never query the database.
func CLevelEnabled() bool {
	return cLevelEnabled.Load()
}

// SetExperimentalTransportEnabled updates the opt-in comparison switch for
// plugin-derived transport behavior. The effective gate also requires C-level
// simulation, so enabling this flag alone never changes request transport.
func SetExperimentalTransportEnabled(enabled bool) {
	experimentalTransportEnabled.Store(enabled)
}

// ExperimentalTransportEnabled reports the persisted comparison switch.
func ExperimentalTransportEnabled() bool {
	return experimentalTransportEnabled.Load()
}

// CodexExperimentalTransportEnabled is the request-path gate used by shared
// HTTP/TLS adapters. Both administrator switches must be on.
func CodexExperimentalTransportEnabled() bool {
	return CLevelEnabled() && ExperimentalTransportEnabled()
}

// SetPrewarmContinuationEnabled updates the administrator-controlled global
// Codex account prewarm switch. Account and repository scheduling paths use
// this process-local value so the switch is enforced without a database read
// on request hot paths.
func SetPrewarmContinuationEnabled(enabled bool) {
	prewarmContinuationEnabled.Store(enabled)
}

// PrewarmContinuationEnabled reports the current global Codex account prewarm
// switch.
func PrewarmContinuationEnabled() bool {
	return prewarmContinuationEnabled.Load()
}
