package tlsfingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// BuiltInCodexRustlsProfile returns the stable, conservative subset of the
// aws-lc-rs provider used by the official Codex transport. Rustls randomizes
// extension order per handshake, so this profile describes the provider
// parameters and ALPN rather than claiming a byte-for-byte JA3 match.
func BuiltInCodexRustlsProfile() *Profile {
	return &Profile{
		Name: "Built-in Codex Rustls (aws-lc-rs)",
		CipherSuites: []uint16{
			0x1302, // TLS_AES_256_GCM_SHA384
			0x1301, // TLS_AES_128_GCM_SHA256
			0x1303, // TLS_CHACHA20_POLY1305_SHA256
			0xc02c, // TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
			0xc02b, // TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
			0xcca9, // TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
			0xc030, // TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
			0xc02f, // TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
			0xcca8, // TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
		},
		Curves:              []uint16{29, 23, 24},
		PointFormats:        []uint16{0},
		SignatureAlgorithms: []uint16{0x0503, 0x0403, 0x0603, 0x0807, 0x0806, 0x0805, 0x0804, 0x0601, 0x0501, 0x0401},
		ALPNProtocols:       []string{"h2", "http/1.1"},
		SupportedVersions:   []uint16{0x0304, 0x0303},
		KeyShareGroups:      []uint16{29},
		PSKModes:            []uint16{1},
		Extensions:          []uint16{0, 10, 11, 13, 16, 23, 35, 43, 45, 51},
	}
}

// ForWebSocket returns a copy with HTTP/1.1-only ALPN, which is required by
// the WebSocket upgrade path even when the corresponding HTTP profile offers
// HTTP/2 to reqwest/net/http callers.
func (p *Profile) ForWebSocket() *Profile {
	if p == nil {
		return nil
	}
	copyProfile := *p
	copyProfile.ALPNProtocols = []string{"http/1.1"}
	return &copyProfile
}

// FingerprintKey returns a stable, non-sensitive identity for the effective
// TLS profile. It is used only for connection-pool compatibility and cache
// partitioning; it never contains account credentials or profile contents.
func FingerprintKey(profile *Profile) string {
	if profile == nil {
		return ""
	}

	// Display names are administrative metadata, not wire parameters. Exclude
	// them so renaming a profile does not churn live connection pools.
	canonical := *profile
	canonical.Name = ""
	encoded, err := json.Marshal(&canonical)
	if err != nil {
		// Profile contains only JSON-marshalable scalar and slice fields. Keep a
		// deterministic fallback if that invariant ever changes.
		encoded = []byte(profile.Name)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}
