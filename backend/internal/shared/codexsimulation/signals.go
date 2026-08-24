package codexsimulation

import (
	"net/http"
	"strings"
)

const (
	AttestationHeader    = "x-oai-attestation"
	ResidencyHeader      = "x-openai-internal-codex-residency"
	HostDeviceKindHeader = "x-codex-host-device-kind"
)

// PlatformSignals contains only values supplied by a trusted platform
// provider. The package never synthesizes hardware or residency evidence.
type PlatformSignals struct {
	Attestation    string
	Residency      string
	HostDeviceKind string
}

type PlatformSignalsProvider interface {
	Signals() (PlatformSignals, bool)
}

// StripUntrustedPlatformSignals removes caller-provided proof/capability
// headers before a request crosses into another virtual OAuth principal.
func StripUntrustedPlatformSignals(headers http.Header) {
	if headers == nil {
		return
	}
	for _, key := range []string{AttestationHeader, ResidencyHeader, HostDeviceKindHeader} {
		headers.Del(key)
		delete(headers, key)
	}
}

// ApplyTrustedPlatformSignals applies only a provider-approved, non-empty
// projection. Callers should invoke StripUntrustedPlatformSignals first.
func ApplyTrustedPlatformSignals(headers http.Header, provider PlatformSignalsProvider) {
	if headers == nil || provider == nil {
		return
	}
	signals, ok := provider.Signals()
	if !ok {
		return
	}
	if value := strings.TrimSpace(signals.Attestation); value != "" {
		headers.Set(AttestationHeader, value)
	}
	if value := strings.TrimSpace(signals.Residency); value != "" {
		headers.Set(ResidencyHeader, value)
	}
	if value := strings.TrimSpace(signals.HostDeviceKind); value != "" {
		headers.Set(HostDeviceKindHeader, value)
	}
}
