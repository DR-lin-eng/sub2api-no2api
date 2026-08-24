package tlsfingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// WireProfileSnapshot is a deterministic capture of the profile inputs that
// influence ClientHello construction. It is suitable for golden fixtures and
// regression comparisons; it deliberately does not claim to be a raw TLS
// byte capture or a JA3 equivalent.
type WireProfileSnapshot struct {
	FingerprintKey      string   `json:"fingerprint_key"`
	CipherSuites        []uint16 `json:"cipher_suites,omitempty"`
	Curves              []uint16 `json:"curves,omitempty"`
	PointFormats        []uint16 `json:"point_formats,omitempty"`
	EnableGREASE        bool     `json:"enable_grease"`
	SignatureAlgorithms []uint16 `json:"signature_algorithms,omitempty"`
	ALPNProtocols       []string `json:"alpn_protocols,omitempty"`
	SupportedVersions   []uint16 `json:"supported_versions,omitempty"`
	KeyShareGroups      []uint16 `json:"key_share_groups,omitempty"`
	PSKModes            []uint16 `json:"psk_modes,omitempty"`
	Extensions          []uint16 `json:"extensions,omitempty"`
}

// CaptureWireProfile materializes effective profile values, including the
// built-in defaults, so two callers can compare the actual intended wire
// inputs instead of comparing administrative profile names.
func CaptureWireProfile(profile *Profile) WireProfileSnapshot {
	if profile == nil {
		profile = &Profile{}
	}
	effective := *profile
	if len(effective.CipherSuites) == 0 {
		effective.CipherSuites = append([]uint16(nil), defaultCipherSuites...)
	}
	if len(effective.Curves) == 0 {
		effective.Curves = []uint16{29, 23, 24}
	}
	if len(effective.PointFormats) == 0 {
		effective.PointFormats = []uint16{0}
	}
	if len(effective.SignatureAlgorithms) == 0 {
		effective.SignatureAlgorithms = make([]uint16, len(defaultSignatureAlgorithms))
		for i, value := range defaultSignatureAlgorithms {
			effective.SignatureAlgorithms[i] = uint16(value)
		}
	}
	if len(effective.ALPNProtocols) == 0 {
		effective.ALPNProtocols = []string{"http/1.1"}
	}
	if len(effective.SupportedVersions) == 0 {
		effective.SupportedVersions = []uint16{0x0304, 0x0303}
	}
	if len(effective.KeyShareGroups) == 0 {
		effective.KeyShareGroups = []uint16{29}
	}
	if len(effective.PSKModes) == 0 {
		effective.PSKModes = []uint16{1}
	}
	if len(effective.Extensions) == 0 {
		effective.Extensions = append([]uint16(nil), defaultExtensionOrder...)
	}
	return WireProfileSnapshot{
		FingerprintKey:      FingerprintKey(&effective),
		CipherSuites:        effective.CipherSuites,
		Curves:              effective.Curves,
		PointFormats:        effective.PointFormats,
		EnableGREASE:        effective.EnableGREASE,
		SignatureAlgorithms: effective.SignatureAlgorithms,
		ALPNProtocols:       effective.ALPNProtocols,
		SupportedVersions:   effective.SupportedVersions,
		KeyShareGroups:      effective.KeyShareGroups,
		PSKModes:            effective.PSKModes,
		Extensions:          effective.Extensions,
	}
}

func WireProfileFingerprint(profile *Profile) string {
	snapshot := CaptureWireProfile(profile)
	encoded, _ := json.Marshal(snapshot)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}
