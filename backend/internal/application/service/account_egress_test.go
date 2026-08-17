package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	moduleegress "github.com/Wei-Shaw/sub2api/internal/modules/egress"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
)

type legacyEgressUpstreamStub struct {
	proxyURL string
	calls    int
}

func (s *legacyEgressUpstreamStub) Do(_ *http.Request, proxyURL string, _ int64, _ int) (*http.Response, error) {
	s.calls++
	s.proxyURL = proxyURL
	return nil, nil
}

func (s *legacyEgressUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, concurrency)
}

func TestWithEgressRouteContextPreservesInvalidExplicitSelection(t *testing.T) {
	route := platformegress.ExternalProxyRoute("")
	ctx := withEgressRouteContext(context.Background(), route, nil)
	routed, ok := platformegress.FromContext(ctx)
	if !ok || routed.Route.Mode != platformegress.ModeExternalProxy {
		t.Fatalf("context route = %#v, %v", routed, ok)
	}
	if _, err := platformegress.ApplyPolicy(routed.Route, routed.Policy); err == nil {
		t.Fatal("invalid explicit external proxy selection unexpectedly became direct")
	}
}

func TestAntigravityClientExplicitIPv6DisabledFailsClosed(t *testing.T) {
	ctx := platformegress.WithContextRoute(
		context.Background(),
		platformegress.IPv6PoolRoute("2001:db8::40", 4, 1, false),
		platformegress.Policy{},
	)
	if _, err := newAntigravityClient(ctx, ""); !errors.Is(err, platformegress.ErrIPv6Disabled) {
		t.Fatalf("antigravity client error = %v", err)
	}
}

func TestAccountEgressRouteProxyRemainsAuthoritative(t *testing.T) {
	proxyID := int64(9)
	account := &Account{
		ProxyID:    &proxyID,
		Proxy:      &Proxy{Protocol: "http", Host: "proxy.example", Port: 8080},
		EgressMode: platformegress.ModeIPv6Pool,
		EgressBinding: &moduleegress.Binding{
			PoolID:     4,
			SourceIPv6: "2001:db8::10",
			Status:     moduleegress.BindingStatusActive,
			PoolStatus: moduleegress.PoolStatusActive,
			Version:    2,
		},
	}

	route := account.EgressRoute()
	if route.Mode != platformegress.ModeExternalProxy || route.ProxyURL != "http://proxy.example:8080" {
		t.Fatalf("EgressRoute() = %#v, want existing external proxy", route)
	}
}

func TestAccountEgressRouteInheritedMissingBindingFailsClosedWhenEnabled(t *testing.T) {
	route := (&Account{EgressMode: platformegress.ModeInherit}).EgressRoute()
	if route.Mode != platformegress.ModeIPv6Pool || !route.Inherited {
		t.Fatalf("EgressRoute() = %#v, want incomplete inherited IPv6 route", route)
	}

	effective, err := platformegress.ApplyPolicy(route, platformegress.Policy{})
	if err != nil || effective.Mode != platformegress.ModeDirect || !effective.Inherited {
		t.Fatalf("disabled ApplyPolicy() = %#v, %v; want inherited direct", effective, err)
	}
	if _, err := platformegress.ApplyPolicy(route, platformegress.Policy{IPv6Enabled: true}); err == nil {
		t.Fatal("enabled ApplyPolicy() unexpectedly accepted an inherited account without a binding")
	}
}

func TestAccountEgressRouteUsesStableBindingIdentity(t *testing.T) {
	account := &Account{
		EgressMode: platformegress.ModeIPv6Pool,
		EgressBinding: &moduleegress.Binding{
			PoolID:     5,
			SourceIPv6: "2001:db8:1::55",
			Status:     moduleegress.BindingStatusActive,
			PoolStatus: moduleegress.PoolStatusActive,
			Version:    7,
		},
	}
	route := account.EgressRoute()
	if route.Mode != platformegress.ModeIPv6Pool || route.SourceIPv6 != "2001:db8:1::55" || route.PoolID != 5 || route.BindingVersion != 7 || route.Inherited {
		t.Fatalf("EgressRoute() = %#v", route)
	}
}

func TestLegacyHTTPUpstreamFailsClosedForIPv6AccountRoute(t *testing.T) {
	account := &Account{
		ID:         61,
		EgressMode: platformegress.ModeIPv6Pool,
		EgressBinding: &moduleegress.Binding{
			PoolID:     6,
			SourceIPv6: "2001:db8:6::61",
			Status:     moduleegress.BindingStatusActive,
			PoolStatus: moduleegress.PoolStatusActive,
			Version:    1,
		},
	}
	upstream := &legacyEgressUpstreamStub{}
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := doAccountHTTPUpstream(upstream, req, "", account); !errors.Is(err, platformegress.ErrIPv6Unsupported) {
		t.Fatalf("Do() error = %v", err)
	}
	if _, err := doAccountHTTPUpstreamWithTLS(upstream, req, "", account, nil); !errors.Is(err, platformegress.ErrIPv6Unsupported) {
		t.Fatalf("DoWithTLS() error = %v", err)
	}
	if upstream.calls != 0 {
		t.Fatalf("legacy upstream was called %d times", upstream.calls)
	}
}

func TestLegacyHTTPUpstreamPreservesExternalProxy(t *testing.T) {
	upstream := &legacyEgressUpstreamStub{}
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL := "http://proxy.example:8080"
	if _, err := doAccountHTTPUpstream(upstream, req, proxyURL, &Account{ID: 62}); err != nil {
		t.Fatal(err)
	}
	if upstream.proxyURL != proxyURL {
		t.Fatalf("legacy upstream proxy = %q, want %q", upstream.proxyURL, proxyURL)
	}
	if upstream.calls != 1 {
		t.Fatalf("legacy upstream calls = %d, want 1", upstream.calls)
	}
}

func TestLegacyHTTPUpstreamPreservesImplicitInheritedDirectCompatibility(t *testing.T) {
	upstream := &legacyEgressUpstreamStub{}
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doAccountHTTPUpstream(upstream, req, "", &Account{ID: 63}); err != nil {
		t.Fatal(err)
	}
	if upstream.calls != 1 || upstream.proxyURL != "" {
		t.Fatalf("legacy inherited direct = calls %d, proxy %q", upstream.calls, upstream.proxyURL)
	}
}
