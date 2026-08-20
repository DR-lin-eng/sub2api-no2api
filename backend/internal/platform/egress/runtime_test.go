package egress

import "testing"

func TestRuntimeEnabledOverrideControlsPolicy(t *testing.T) {
	ClearRuntimeEnabledOverride()
	t.Cleanup(ClearRuntimeEnabledOverride)
	if runtimeEnabled(false) {
		t.Fatal("unset override unexpectedly enabled IPv6")
	}
	SetRuntimeEnabled(true)
	if !runtimeEnabled(false) {
		t.Fatal("enabled override was ignored")
	}
	SetRuntimeEnabled(false)
	if runtimeEnabled(true) {
		t.Fatal("disabled override was ignored")
	}
}
