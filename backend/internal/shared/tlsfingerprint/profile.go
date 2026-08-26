package tlsfingerprint

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strconv"
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

// VariantForKey returns a deterministic account-scoped wire variant of a
// profile. The variant keeps every TLS 1.3 cipher and a compatible TLS 1.2
// subset, then changes offer ordering for ciphers, curves, signatures, and
// extensions. The extension permutation mirrors Rustls' randomized ClientHello
// ordering while remaining stable for one local account.
// Those changes alter the JA3 inputs without putting an account identifier or
// credential into the ClientHello. The original profile and all of its slices
// remain untouched.
//
// An explicit administrator profile remains the base profile; callers should
// use this helper only for modes that request per-account variation.
func VariantForKey(profile *Profile, key string) *Profile {
	if profile == nil || key == "" {
		return profile
	}

	variant := cloneProfile(profile)
	// Materialize the fields whose empty values are filled by the dialer so a
	// minimal administrator profile can still receive an account-specific wire
	// variant.
	if len(variant.CipherSuites) == 0 {
		variant.CipherSuites = append([]uint16(nil), defaultCipherSuites...)
	}
	if len(variant.Curves) == 0 {
		variant.Curves = []uint16{29, 23, 24}
	}
	if len(variant.SignatureAlgorithms) == 0 {
		variant.SignatureAlgorithms = make([]uint16, len(defaultSignatureAlgorithms))
		for i, value := range defaultSignatureAlgorithms {
			variant.SignatureAlgorithms[i] = uint16(value)
		}
	}
	if len(variant.Extensions) == 0 {
		variant.Extensions = append([]uint16(nil), defaultExtensionOrder...)
	}
	variant.CipherSuites = keyedCipherSubset(variant.CipherSuites, key)
	variant.CipherSuites = keyedPermutation(variant.CipherSuites, key, "cipher_suites")
	variant.Curves = keyedPermutation(variant.Curves, key, "curves")
	variant.SignatureAlgorithms = keyedPermutation(variant.SignatureAlgorithms, key, "signature_algorithms")
	variant.Extensions = keyedPermutation(variant.Extensions, key, "extensions")
	return variant
}

func keyedCipherSubset(values []uint16, key string) []uint16 {
	nonTLS13 := make([]uint16, 0, len(values))
	for _, value := range values {
		if isVariantTLS12Cipher(value) {
			nonTLS13 = append(nonTLS13, value)
		}
	}
	// Keep at least three TLS 1.2 options in addition to every TLS 1.3
	// cipher, so variation does not remove broad OpenAI endpoint compatibility.
	maxDrop := len(nonTLS13) - 3
	if maxDrop <= 0 {
		return append([]uint16(nil), values...)
	}
	if maxDrop > 3 {
		maxDrop = 3
	}

	digest := sha256.Sum256([]byte("sub2api:tls-fingerprint-variant:v1\x00" + key + "\x00cipher_set"))
	dropCount := 1 + int(digest[0])%maxDrop
	dropOrder := keyedPermutation(nonTLS13, key, "cipher_set")
	dropped := make(map[uint16]struct{}, dropCount)
	ecdsaCount, rsaCount := tls12CertificateFamilyCounts(nonTLS13)
	for _, value := range dropOrder {
		if len(dropped) == dropCount {
			break
		}
		if isTLS12ECDSACipher(value) && ecdsaCount <= 1 {
			continue
		}
		if isTLS12RSACipher(value) && rsaCount <= 1 {
			continue
		}
		dropped[value] = struct{}{}
		if isTLS12ECDSACipher(value) {
			ecdsaCount--
		}
		if isTLS12RSACipher(value) {
			rsaCount--
		}
	}

	result := make([]uint16, 0, len(values)-dropCount)
	for _, value := range values {
		if _, shouldDrop := dropped[value]; !shouldDrop {
			result = append(result, value)
		}
	}
	return result
}

func isVariantTLS12Cipher(value uint16) bool {
	return isTLS12ECDSACipher(value) || isTLS12RSACipher(value)
}

func tls12CertificateFamilyCounts(values []uint16) (ecdsa, rsa int) {
	for _, value := range values {
		if isTLS12ECDSACipher(value) {
			ecdsa++
		}
		if isTLS12RSACipher(value) {
			rsa++
		}
	}
	return ecdsa, rsa
}

func isTLS12ECDSACipher(value uint16) bool {
	switch value {
	case 0xc02b, 0xc02c, 0xcca9:
		return true
	default:
		return false
	}
}

func isTLS12RSACipher(value uint16) bool {
	switch value {
	case 0xc02f, 0xc030, 0xcca8:
		return true
	default:
		return false
	}
}

func cloneProfile(profile *Profile) *Profile {
	clone := *profile
	clone.CipherSuites = append([]uint16(nil), profile.CipherSuites...)
	clone.Curves = append([]uint16(nil), profile.Curves...)
	clone.PointFormats = append([]uint16(nil), profile.PointFormats...)
	clone.SignatureAlgorithms = append([]uint16(nil), profile.SignatureAlgorithms...)
	clone.ALPNProtocols = append([]string(nil), profile.ALPNProtocols...)
	clone.SupportedVersions = append([]uint16(nil), profile.SupportedVersions...)
	clone.KeyShareGroups = append([]uint16(nil), profile.KeyShareGroups...)
	clone.PSKModes = append([]uint16(nil), profile.PSKModes...)
	clone.Extensions = append([]uint16(nil), profile.Extensions...)
	return &clone
}

func keyedPermutation(values []uint16, key, field string) []uint16 {
	permuted := append([]uint16(nil), values...)
	if len(permuted) < 2 {
		return permuted
	}

	for i := len(permuted) - 1; i > 0; i-- {
		digest := sha256.Sum256([]byte(
			"sub2api:tls-fingerprint-variant:v1\x00" + key + "\x00" + field + "\x00" + strconv.Itoa(i),
		))
		j := int(binary.LittleEndian.Uint64(digest[:8]) % uint64(i+1))
		permuted[i], permuted[j] = permuted[j], permuted[i]
	}

	// Avoid returning the unmodified order for a key whose permutation happens
	// to be the identity. Duplicate values are skipped so the swap is visible.
	if sameUint16Order(permuted, values) {
		for i := 1; i < len(permuted); i++ {
			if permuted[i] != permuted[0] {
				permuted[0], permuted[i] = permuted[i], permuted[0]
				break
			}
		}
	}
	return permuted
}

func sameUint16Order(left, right []uint16) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
