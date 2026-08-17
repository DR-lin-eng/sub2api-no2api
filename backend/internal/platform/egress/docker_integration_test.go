//go:build integration

package egress

import (
	"context"
	"encoding/json"
	"net/http"
	"net/netip"
	"os"
	"testing"
	"time"
)

func TestDockerIPv6SourceIsolation(t *testing.T) {
	target := os.Getenv("IPV6_EGRESS_ECHO_URL")
	sourceA := os.Getenv("IPV6_EGRESS_SOURCE_A")
	sourceB := os.Getenv("IPV6_EGRESS_SOURCE_B")
	if target == "" || sourceA == "" || sourceB == "" {
		t.Skip("Docker IPv6 integration environment is not configured")
	}

	observedA := dockerProbeSource(t, target, IPv6PoolRoute(sourceA, 1, 1, false))
	observedARepeat := dockerProbeSource(t, target, IPv6PoolRoute(sourceA, 1, 1, false))
	observedB := dockerProbeSource(t, target, IPv6PoolRoute(sourceB, 1, 2, false))
	if observedA != netip.MustParseAddr(sourceA) || observedARepeat != observedA {
		t.Fatalf("stable source A = %s then %s, want %s", observedA, observedARepeat, sourceA)
	}
	if observedB != netip.MustParseAddr(sourceB) || observedB == observedA {
		t.Fatalf("rotated source B = %s, source A = %s", observedB, observedA)
	}
}

func dockerProbeSource(t *testing.T, target string, route Route) netip.Addr {
	t.Helper()
	dialContext, err := NewDialContext(route, Policy{IPv6Enabled: true, FreeBind: true}, DialerOptions{
		Timeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{DialContext: dialContext}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var payload struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	observed, err := netip.ParseAddr(payload.IP)
	if err != nil {
		t.Fatalf("echo returned %q: %v", payload.IP, err)
	}
	return observed.Unmap()
}
