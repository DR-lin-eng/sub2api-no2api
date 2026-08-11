package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/shared/claude"
)

type stubIdentityCache struct {
	fingerprint *Fingerprint
	setCalls    int
	lastSet     *Fingerprint
}

func (s *stubIdentityCache) GetFingerprint(_ context.Context, _ int64) (*Fingerprint, error) {
	if s.fingerprint == nil {
		return nil, nil
	}
	clone := *s.fingerprint
	return &clone, nil
}

func (s *stubIdentityCache) SetFingerprint(_ context.Context, _ int64, fp *Fingerprint) error {
	s.setCalls++
	clone := *fp
	s.lastSet = &clone
	s.fingerprint = &clone
	return nil
}

func (s *stubIdentityCache) GetMaskedSessionID(_ context.Context, _ int64) (string, error) {
	return "", nil
}

func (s *stubIdentityCache) SetMaskedSessionID(_ context.Context, _ int64, _ string) error {
	return nil
}

func headersWithUA(ua string) http.Header {
	headers := http.Header{}
	if ua != "" {
		headers.Set("User-Agent", ua)
	}
	return headers
}

func TestIsAcceptableFingerprintUserAgent(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want bool
	}{
		{name: "official CLI", ua: "claude-cli/2.1.220 (external, cli)", want: true},
		{name: "official CLI without metadata", ua: "claude-cli/2.1.220", want: true},
		{name: "next major within skew", ua: "claude-cli/4.0.0 (external, cli)", want: true},
		{name: "other product", ua: "some-sdk/1.2.3 (node)", want: true},
		{name: "local suffix", ua: "claude-cli/999.0.0-local (undefined, cli)", want: false},
		{name: "development suffix", ua: "claude-cli/2.1.220-dev (external, cli)", want: false},
		{name: "build suffix", ua: "claude-cli/2.1.220+build1 (external, cli)", want: false},
		{name: "sentinel major", ua: "claude-cli/999.0.0 (external, cli)", want: false},
		{name: "empty", ua: "", want: false},
		{name: "missing version", ua: "claude-cli (external, cli)", want: false},
		{name: "short version", ua: "claude-cli/2.1 (external, cli)", want: false},
		{name: "browser UA", ua: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", want: false},
		{name: "leading junk", ua: "x claude-cli/2.1.220 (external, cli)", want: false},
		{name: "newline", ua: "claude-cli/2.1.220 (external)\nInjected: value", want: false},
		{name: "too long", ua: "claude-cli/2.1.220 (" + strings.Repeat("a", 300) + ")", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isAcceptableFingerprintUserAgent(tt.ua))
		})
	}
}

func TestGetOrCreateFingerprintRejectsMalformedUserAgentOnCreate(t *testing.T) {
	cache := &stubIdentityCache{}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(),
		1,
		headersWithUA("claude-cli/999.0.0-local (undefined, cli)"),
	)

	require.NoError(t, err)
	require.Equal(t, defaultFingerprint.UserAgent, fp.UserAgent)
	require.NotContains(t, cache.lastSet.UserAgent, "999.0.0")
}

func TestGetOrCreateFingerprintRejectsSentinelVersionOnUpgrade(t *testing.T) {
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent: "claude-cli/2.1.22 (external, cli)",
		ClientID:  "cid-1",
		UpdatedAt: time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(),
		1,
		headersWithUA("claude-cli/999.0.0-local (undefined, cli)"),
	)

	require.NoError(t, err)
	require.Equal(t, "claude-cli/2.1.22 (external, cli)", fp.UserAgent)
	require.Zero(t, cache.setCalls)
}

func TestGetOrCreateFingerprintStillUpgradesOnValidNewerVersion(t *testing.T) {
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent: "claude-cli/2.1.22 (external, cli)",
		ClientID:  "cid-1",
		UpdatedAt: time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	newUA := "claude-cli/2.1.223 (external, cli)"
	fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, headersWithUA(newUA))

	require.NoError(t, err)
	require.Equal(t, newUA, fp.UserAgent)
	require.Equal(t, 1, cache.setCalls)
}

func TestGetOrCreateFingerprintAcceptsValidUserAgentOnCreate(t *testing.T) {
	cache := &stubIdentityCache{}
	svc := NewIdentityService(cache)

	ua := "claude-cli/" + claude.CLICurrentVersion + " (external, cli)"
	fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, headersWithUA(ua))

	require.NoError(t, err)
	require.Equal(t, ua, fp.UserAgent)
	require.NotEmpty(t, fp.ClientID)
	require.Equal(t, 1, cache.setCalls)
}

func TestDefaultFingerprintUserAgentIsAcceptable(t *testing.T) {
	require.True(t, isAcceptableFingerprintUserAgent(defaultFingerprint.UserAgent))
}

func TestGetOrCreateFingerprintHealsPoisonedCacheUsingValidClientUA(t *testing.T) {
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent: "claude-cli/999.0.0-local (undefined, cli)",
		ClientID:  "cid-1",
		UpdatedAt: time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	realUA := "claude-cli/2.1.22 (external, cli)"
	fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, headersWithUA(realUA))

	require.NoError(t, err)
	require.Equal(t, realUA, fp.UserAgent)
	require.Equal(t, 1, cache.setCalls)
	require.NotContains(t, cache.lastSet.UserAgent, "999.0.0")
	require.Equal(t, "cid-1", fp.ClientID)
}

func TestGetOrCreateFingerprintHealsPoisonedCacheWithoutValidClientUA(t *testing.T) {
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent: "claude-cli/999.0.0-local (undefined, cli)",
		ClientID:  "cid-1",
		UpdatedAt: time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(),
		1,
		headersWithUA("claude-cli/999.0.0-local (undefined, cli)"),
	)

	require.NoError(t, err)
	require.Equal(t, defaultFingerprint.UserAgent, fp.UserAgent)
	require.Equal(t, 1, cache.setCalls)
}

func TestGetOrCreateFingerprintDoesNotRewriteHealthyCache(t *testing.T) {
	cache := &stubIdentityCache{fingerprint: &Fingerprint{
		UserAgent: "claude-cli/2.1.220 (external, cli)",
		ClientID:  "cid-1",
		UpdatedAt: time.Now().Unix(),
	}}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(
		context.Background(),
		1,
		headersWithUA("claude-cli/2.1.22 (external, cli)"),
	)

	require.NoError(t, err)
	require.Equal(t, "claude-cli/2.1.220 (external, cli)", fp.UserAgent)
	require.Zero(t, cache.setCalls)
}

func TestGetOrCreateFingerprintMissingUserAgentKeepsDefault(t *testing.T) {
	cache := &stubIdentityCache{}
	svc := NewIdentityService(cache)

	fp, err := svc.GetOrCreateFingerprint(context.Background(), 1, http.Header{})

	require.NoError(t, err)
	require.Equal(t, defaultFingerprint.UserAgent, fp.UserAgent)
}

var benchmarkFingerprintUserAgentAccepted bool

func BenchmarkIsAcceptableFingerprintUserAgent(b *testing.B) {
	for _, ua := range []string{
		"claude-cli/2.1.220 (external, cli)",
		"claude-cli/999.0.0-local (undefined, cli)",
		"some-sdk/1.2.3 (node)",
	} {
		b.Run(ua, func(b *testing.B) {
			b.ReportAllocs()
			accepted := false
			for b.Loop() {
				accepted = isAcceptableFingerprintUserAgent(ua)
			}
			benchmarkFingerprintUserAgentAccepted = accepted
		})
	}
}
