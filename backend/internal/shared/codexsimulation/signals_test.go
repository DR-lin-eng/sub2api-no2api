package codexsimulation

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type testSignalsProvider struct{}

func (testSignalsProvider) Signals() (PlatformSignals, bool) {
	return PlatformSignals{Attestation: "trusted", Residency: "us", HostDeviceKind: "mac_mini"}, true
}

func TestPlatformSignalProjectionRequiresTrustedProvider(t *testing.T) {
	headers := make(http.Header)
	headers.Set(AttestationHeader, "caller-proof")
	headers.Set(ResidencyHeader, "caller-region")
	headers.Set(HostDeviceKindHeader, "caller-device")
	StripUntrustedPlatformSignals(headers)
	ApplyTrustedPlatformSignals(headers, nil)
	require.Empty(t, headers)
	ApplyTrustedPlatformSignals(headers, testSignalsProvider{})
	require.Equal(t, "trusted", headers.Get(AttestationHeader))
	require.Equal(t, "us", headers.Get(ResidencyHeader))
	require.Equal(t, "mac_mini", headers.Get(HostDeviceKindHeader))
}
