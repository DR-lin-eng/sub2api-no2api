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
