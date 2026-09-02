package tlsfingerprint

import (
	"strconv"
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

func TestForHTTP1PinsALPNWithoutMutatingProfile(t *testing.T) {
	base := BuiltInCodexRustlsProfile()
	http1 := base.ForHTTP1()

	require.Equal(t, []string{"http/1.1"}, http1.ALPNProtocols)
	require.Equal(t, []string{"h2", "http/1.1"}, base.ALPNProtocols)
	http1.CipherSuites[0]++
	require.NotEqual(t, http1.CipherSuites[0], base.CipherSuites[0])
}

func TestVariantForKeyIsStableAndAccountScoped(t *testing.T) {
	base := BuiltInCodexRustlsProfile()
	first := VariantForKey(base, "account-101")
	second := VariantForKey(base, "account-202")
	repeat := VariantForKey(base, "account-101")

	require.NotEqual(t, WireProfileFingerprint(base), WireProfileFingerprint(first))
	require.NotEqual(t, WireProfileFingerprint(first), WireProfileFingerprint(second))
	require.Equal(t, WireProfileFingerprint(first), WireProfileFingerprint(repeat))
	require.ElementsMatch(t, base.Extensions, first.Extensions)
	require.NotEqual(t, base.Extensions, first.Extensions)
	require.Equal(t, BuiltInCodexRustlsProfile(), base)
}

func TestVariantForKeyKeepsTLS12CertificateFamilies(t *testing.T) {
	for id := 1; id <= 4096; id++ {
		variant := VariantForKey(BuiltInCodexRustlsProfile(), strconv.Itoa(id))
		ecdsa, rsa := tls12CertificateFamilyCounts(variant.CipherSuites)
		require.Positive(t, ecdsa, "account %d lost all ECDSA TLS 1.2 ciphers", id)
		require.Positive(t, rsa, "account %d lost all RSA TLS 1.2 ciphers", id)
	}
}
