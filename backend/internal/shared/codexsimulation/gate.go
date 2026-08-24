// Package codexsimulation contains process-wide runtime gates shared by the
// admin control plane and low-level Codex transport adapters.
package codexsimulation

import "sync/atomic"

var cLevelEnabled atomic.Bool

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
