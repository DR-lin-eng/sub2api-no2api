package egress

import "sync/atomic"

// runtimeEnabledOverride is intentionally tri-state. A value of -1 means that
// the application has not loaded its persisted control-plane setting yet, so
// callers keep using their static configuration value during bootstrap.
var runtimeEnabledOverride atomic.Int32

func init() {
	runtimeEnabledOverride.Store(-1)
}

// SetRuntimeEnabled applies the administrator-controlled IPv6 switch to every
// route policy in the process. The switch is process-local and is refreshed by
// the egress service when a persisted setting changes.
func SetRuntimeEnabled(enabled bool) {
	if enabled {
		runtimeEnabledOverride.Store(1)
		return
	}
	runtimeEnabledOverride.Store(0)
}

// ClearRuntimeEnabledOverride restores the pre-control-plane behavior. It is
// primarily useful during shutdown and isolated tests.
func ClearRuntimeEnabledOverride() {
	runtimeEnabledOverride.Store(-1)
}

func runtimeEnabled(configured bool) bool {
	value := runtimeEnabledOverride.Load()
	if value < 0 {
		return configured
	}
	return value == 1
}
