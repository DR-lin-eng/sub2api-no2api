package egress

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

type staticResolver struct {
	addresses []netip.Addr
	err       error
	network   string
}

func (r *staticResolver) LookupNetIP(_ context.Context, network, _ string) ([]netip.Addr, error) {
	r.network = network
	return r.addresses, r.err
}

func TestResolveIPv6RequestsAAAAOnly(t *testing.T) {
	resolver := &staticResolver{addresses: []netip.Addr{
		netip.MustParseAddr("192.0.2.10"),
		netip.MustParseAddr("2001:db8::10"),
	}}
	addresses, err := resolveIPv6(context.Background(), resolver, "example.test")
	if err != nil {
		t.Fatalf("resolveIPv6() error = %v", err)
	}
	if resolver.network != "ip6" {
		t.Fatalf("LookupNetIP network = %q, want ip6", resolver.network)
	}
	if len(addresses) != 1 || addresses[0].String() != "2001:db8::10" {
		t.Fatalf("resolveIPv6() = %v", addresses)
	}
}

func TestResolveIPv6RejectsIPv4LiteralAndMissingAAAA(t *testing.T) {
	if _, err := resolveIPv6(context.Background(), &staticResolver{}, "192.0.2.10"); !errors.Is(err, ErrIPv6Destination) {
		t.Fatalf("IPv4 literal error = %v", err)
	}
	if _, err := resolveIPv6(context.Background(), &staticResolver{}, "example.test"); !errors.Is(err, ErrIPv6Destination) {
		t.Fatalf("missing AAAA error = %v", err)
	}
}
