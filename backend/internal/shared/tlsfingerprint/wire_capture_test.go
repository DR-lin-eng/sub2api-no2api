package tlsfingerprint

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCaptureWireProfileUsesEffectiveDefaults(t *testing.T) {
	snapshot := CaptureWireProfile(BuiltInCodexRustlsProfile())
	require.Equal(t, []string{"h2", "http/1.1"}, snapshot.ALPNProtocols)
	require.Equal(t, []uint16{29}, snapshot.KeyShareGroups)
	require.Equal(t, []uint16{0, 10, 11, 13, 16, 23, 35, 43, 45, 51}, snapshot.Extensions)
	require.Len(t, snapshot.FingerprintKey, 64)
	require.Len(t, WireProfileFingerprint(BuiltInCodexRustlsProfile()), 64)
}

func TestWireProfileFingerprintChangesWhenWireInputChanges(t *testing.T) {
	first := WireProfileFingerprint(BuiltInCodexRustlsProfile())
	profile := BuiltInCodexRustlsProfile()
	profile.ALPNProtocols = []string{"http/1.1"}
	second := WireProfileFingerprint(profile)
	require.NotEqual(t, first, second)
}
