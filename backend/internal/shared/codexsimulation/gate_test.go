package codexsimulation

import "testing"

func TestCLevelGateRoundTrip(t *testing.T) {
	SetCLevelEnabled(false)
	if CLevelEnabled() {
		t.Fatal("C-level gate should start disabled")
	}
	SetCLevelEnabled(true)
	if !CLevelEnabled() {
		t.Fatal("C-level gate should report enabled")
	}
	SetCLevelEnabled(false)
}

func TestExperimentalTransportGateRequiresCLevel(t *testing.T) {
	SetCLevelEnabled(false)
	SetExperimentalTransportEnabled(true)
	t.Cleanup(func() {
		SetCLevelEnabled(false)
		SetExperimentalTransportEnabled(false)
	})
	if CodexExperimentalTransportEnabled() {
		t.Fatal("experimental transport must remain inactive while C-level simulation is off")
	}
	SetCLevelEnabled(true)
	if !CodexExperimentalTransportEnabled() {
		t.Fatal("experimental transport should activate when both switches are on")
	}
}
