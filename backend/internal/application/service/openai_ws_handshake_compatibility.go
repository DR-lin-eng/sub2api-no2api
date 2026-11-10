package service

import (
	"crypto/sha256"
	"net/http"
	"sort"
	"strings"

	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
)

// Handshake identity is immutable for the life of an upstream socket.
type openAIWSHandshakeCompatibilityKey struct {
	targetURL        string
	userAgent        string
	originator       string
	version          string
	installationID   string
	windowID         string
	betaHeader       string
	betaFeatures     string
	routingOverride  string
	chatGPTAccountID string
	fedRAMP          string
	responsesLite    string
	subagent         string
	guardian         string
	memgen           string
	timingMetrics    string
	locationHash     [sha256.Size]byte
	authHash         [sha256.Size]byte
	principalHash    [sha256.Size]byte
	fingerprintKey   string
	turnStateHash    [sha256.Size]byte
	tlsProfileKey    string
	egressRouteKey   string
}

func openAIWSAcquireCompatibility(req openAIWSAcquireRequest) openAIWSHandshakeCompatibilityKey {
	compatibility := normalizeOpenAIWSHandshakeCompatibility(req.Headers)
	// A nil header set is used by lightweight pool callers/tests that seed a
	// connection without a real handshake. Keep that legacy zero key reusable;
	// all production WS paths provide the final handshake headers, so routed URL
	// and auth identity still partition real sockets.
	if req.Headers != nil && strings.TrimSpace(req.Headers.Get("Authorization")) != "" {
		compatibility.targetURL = stringsTrim(req.WSURL)
		compatibility.authHash = openAIWSAuthorizationCompatibilityHash(req.Headers)
		compatibility.principalHash = openAIWSPrincipalCompatibilityHash(req.Account)
	}
	compatibility.tlsProfileKey = tlsfingerprint.FingerprintKey(req.TLSProfile)
	route := openAIWSAcquireRoute(req)
	if route.Mode == platformegress.ModeIPv6Pool {
		compatibility.egressRouteKey = route.CacheKey()
	}
	return compatibility
}

func normalizeOpenAIWSBetaFeatures(headers http.Header) string {
	features := make(map[string]struct{})
	for name, values := range headers {
		if !strings.EqualFold(strings.TrimSpace(name), "x-codex-beta-features") {
			continue
		}
		for _, value := range values {
			for _, feature := range strings.Split(value, ",") {
				if feature = strings.TrimSpace(feature); feature != "" {
					features[feature] = struct{}{}
				}
			}
		}
	}
	if len(features) == 0 {
		return ""
	}
	normalized := make([]string, 0, len(features))
	for feature := range features {
		normalized = append(normalized, feature)
	}
	sort.Strings(normalized)
	return strings.Join(normalized, ",")
}

func normalizeOpenAIWSHandshakeCompatibility(headers http.Header) openAIWSHandshakeCompatibilityKey {
	return openAIWSHandshakeCompatibilityKey{
		userAgent:        headers.Get("User-Agent"),
		originator:       headers.Get("originator"),
		version:          headers.Get("version"),
		installationID:   headers.Get("x-codex-installation-id"),
		windowID:         headers.Get("x-codex-window-id"),
		betaHeader:       headers.Get("OpenAI-Beta"),
		betaFeatures:     normalizeOpenAIWSBetaFeatures(headers),
		routingOverride:  headers.Get(openAIAccountRoutingOverrideHeader),
		chatGPTAccountID: headers.Get("chatgpt-account-id"),
		fedRAMP:          headers.Get("x-openai-fedramp"),
		responsesLite:    headers.Get(responsesLiteHeaderKey),
		subagent:         headers.Get("x-openai-subagent"),
		guardian:         headers.Get("x-codex-guardian"),
		memgen:           headers.Get("x-openai-memgen-request"),
		timingMetrics:    headers.Get("x-responsesapi-include-timing-metrics"),
		locationHash:     openAIWSLocationCompatibilityHash(headers),
		fingerprintKey:   headers.Get(codexFingerprintWSKeyHeader),
		turnStateHash:    openAIWSTurnStateCompatibilityHash(headers),
	}
}

func openAIWSLocationCompatibilityHash(headers http.Header) [sha256.Size]byte {
	if value := headers.Get("OAI-Location"); value != "" {
		return sha256.Sum256([]byte(value))
	}
	return [sha256.Size]byte{}
}

func openAIWSAuthorizationCompatibilityHash(headers http.Header) [sha256.Size]byte {
	if headers == nil {
		return [sha256.Size]byte{}
	}
	value := strings.TrimSpace(headers.Get("Authorization"))
	if value == "" {
		return [sha256.Size]byte{}
	}
	return sha256.Sum256([]byte(value))
}

// openAIWSPrincipalCompatibilityHash keeps a durable socket bound to the
// account/user identity that produced its handshake. In addition to the bearer
// hash, bind the selected account/user snapshot: opaque tokens may stay the
// same while workspace selection or a local credential binding changes.
func openAIWSPrincipalCompatibilityHash(account *Account) [sha256.Size]byte {
	if account == nil || !account.IsOpenAIOAuth() {
		return [sha256.Size]byte{}
	}
	principal := strings.Join([]string{
		strings.TrimSpace(account.CodexVirtualClientKey()),
		strings.TrimSpace(account.GetChatGPTAccountID()),
		strings.TrimSpace(account.GetChatGPTUserID()),
		openAIOAuthAccountPlanClass(account),
	}, "\x00")
	if strings.Trim(principal, "\x00") == "" {
		return [sha256.Size]byte{}
	}
	return sha256.Sum256([]byte(principal))
}

func openAIWSTurnStateCompatibilityHash(headers http.Header) [sha256.Size]byte {
	state := strings.TrimSpace(headers.Get(openAIWSTurnStateHeader))
	if state == "" {
		return [sha256.Size]byte{}
	}
	return sha256.Sum256([]byte(state))
}
