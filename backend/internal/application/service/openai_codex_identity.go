package service

import (
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/google/uuid"
)

const (
	codexUpstreamMinVersion  = "0.144.0"
	codexClientVersionMaxLen = 64
)

var codexClientVersionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+){1,3}(-[0-9A-Za-z.]+)?$`)

func NormalizeCodexClientVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || len(version) > codexClientVersionMaxLen || !codexClientVersionPattern.MatchString(version) {
		return ""
	}
	return version
}

func buildCodexCLIUserAgent(version string) string {
	if version = NormalizeCodexClientVersion(version); version == "" {
		return codexCLIUserAgent
	}
	return openai.CodexCLIOriginator + "/" + version + codexCLIUserAgentSuffix
}

var codexIdentityEnforcement = func() *atomic.Bool {
	v := &atomic.Bool{}
	v.Store(true)
	return v
}()

func SetCodexIdentityEnforcementEnabled(enabled bool) {
	codexIdentityEnforcement.Store(enabled)
}

// SetCodexOriginatorNormalizationEnabled is retained for source compatibility
// with the previous rollback switch. Identity enforcement supersedes the
// narrower originator-only normalization behavior.
func SetCodexOriginatorNormalizationEnabled(enabled bool) {
	SetCodexIdentityEnforcementEnabled(enabled)
}

type codexCanonicalUserAgentResolverHolder struct {
	resolve func() string
}

// The gateway calls this on every OAuth request. An atomic resolver snapshot
// avoids adding a process-wide RWMutex to the request hot path.
var codexCanonicalUAResolver atomic.Pointer[codexCanonicalUserAgentResolverHolder]

func SetCodexCanonicalUserAgentResolver(resolver func() string) {
	if resolver == nil {
		codexCanonicalUAResolver.Store(nil)
		return
	}
	codexCanonicalUAResolver.Store(&codexCanonicalUserAgentResolverHolder{resolve: resolver})
}

// CodexCanonicalUserAgent returns the current canonical Codex client identity
// without reading persistent settings on the request path.
func CodexCanonicalUserAgent() string {
	return resolveCodexOutboundIdentity("").userAgent
}

// CodexCanonicalAuthIdentity returns the paired identity used by auth.openai.com
// token exchange, refresh, and whoami requests. The credential surface does not
// send the inference-only version header.
func CodexCanonicalAuthIdentity() (userAgent, originator string) {
	identity := resolveCodexOutboundIdentity("")
	return identity.userAgent, identity.originator
}

// ApplyCodexCanonicalAuthIdentity applies the credential-surface identity pair.
func ApplyCodexCanonicalAuthIdentity(h http.Header) {
	if h == nil {
		return
	}
	userAgent, originator := CodexCanonicalAuthIdentity()
	h.Set("user-agent", userAgent)
	h.Set("originator", originator)
}

// CodexCanonicalClientVersion returns the version paired with the canonical UA.
func CodexCanonicalClientVersion() string {
	return resolveCodexOutboundIdentity("").version
}

func codexCanonicalUserAgent() string {
	if holder := codexCanonicalUAResolver.Load(); holder != nil && holder.resolve != nil {
		if ua := strings.TrimSpace(holder.resolve()); ua != "" {
			return ua
		}
	}
	return codexCLIUserAgent
}

type codexOutboundIdentity struct {
	userAgent  string
	originator string
	version    string
}

func resolveCodexOutboundIdentity(candidateUA string) codexOutboundIdentity {
	canonical := codexCanonicalUserAgent()
	ua := strings.TrimSpace(candidateUA)
	if ua == "" {
		ua = canonical
	}
	originator, pairedUA, ok := openai.PairCodexClientIdentity(ua)
	if !ok {
		if originator, pairedUA, ok = openai.PairCodexClientIdentity(canonical); !ok {
			originator, pairedUA = openai.CodexCLIOriginator, codexCLIUserAgent
		}
	}
	version := codexClientVersionFromUA(canonical)
	if rebuilt := openai.SetCodexUserAgentVersion(pairedUA, version); rebuilt != "" {
		pairedUA = rebuilt
	}
	identity := codexOutboundIdentity{userAgent: pairedUA, originator: originator, version: version}
	// The upstream scheduler currently puts codex-tui in a load-shed bucket.
	// Normalize only while the canonical identity policy is enabled; the
	// existing disable switch must continue to provide a genuine rollback path.
	if codexIdentityEnforcement.Load() {
		identity.originator, identity.userAgent, _ = openai.NormalizeCodexClientIdentityToCLI(
			identity.originator,
			identity.userAgent,
		)
	}
	return identity
}

func codexClientVersionFromUA(ua string) string {
	version := NormalizeCodexClientVersion(openai.CodexUserAgentVersion(ua))
	if version == "" || CompareVersions(version, codexUpstreamMinVersion) < 0 {
		return codexCLIVersion
	}
	return version
}

func ensureCodexIdentityHeaders(h http.Header) {
	if h == nil {
		return
	}
	identity := resolveCodexOutboundIdentity("")
	if strings.TrimSpace(h.Get("user-agent")) == "" {
		h.Set("user-agent", identity.userAgent)
	}
	if strings.TrimSpace(h.Get("originator")) == "" {
		h.Set("originator", identity.originator)
	}
	if strings.TrimSpace(h.Get("version")) == "" {
		h.Set("version", identity.version)
	}
	h.Set("OpenAI-Beta", "responses=experimental")
}

func applyOpenAICodexProbeHeaders(h http.Header) {
	if h == nil {
		return
	}
	ensureCodexIdentityHeaders(h)
	h.Set("X-Codex-Window-ID", uuid.NewString())
}

func enforceCodexIdentityHeaders(h http.Header) {
	enforceCodexIdentityHeadersWithUA(h, "")
}

func enforceCodexIdentityHeadersWithUA(h http.Header, overrideUA string) {
	if h == nil || h.Get("originator") == "" {
		return
	}
	if !codexIdentityEnforcement.Load() {
		pairCodexIdentityHeaders(h)
		return
	}
	identity := resolveCodexOutboundIdentity(overrideUA)
	h.Set("user-agent", identity.userAgent)
	h.Set("originator", identity.originator)
	h.Set("version", identity.version)
}

func pairCodexIdentityHeaders(h http.Header) {
	originator, pairedUA, ok := openai.PairCodexClientIdentity(h.Get("user-agent"))
	if !ok {
		identity := resolveCodexOutboundIdentity("")
		originator, pairedUA = identity.originator, identity.userAgent
		h.Set("version", identity.version)
	}
	h.Set("user-agent", pairedUA)
	h.Set("originator", originator)
	if v := strings.TrimSpace(h.Get("version")); v != "" && CompareVersions(v, codexUpstreamMinVersion) < 0 {
		h.Set("version", resolveCodexOutboundIdentity("").version)
	}
}
