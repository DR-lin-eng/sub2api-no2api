package egress

import (
	"testing"
)

func TestDerivePoolPrefixFromAddress(t *testing.T) {
	got, err := DerivePoolPrefix("2001:db8:1234:5678::42", 64)
	if err != nil {
		t.Fatalf("DerivePoolPrefix() error = %v", err)
	}
	if got.String() != "2001:db8:1234:5678::/64" {
		t.Fatalf("DerivePoolPrefix() = %s", got)
	}
}

func TestDerivePoolPrefixNarrowsBroadProviderPrefix(t *testing.T) {
	got, err := DerivePoolPrefix("2001:db8:1234:5678::42/56", 64)
	if err != nil {
		t.Fatalf("DerivePoolPrefix() error = %v", err)
	}
	if got.String() != "2001:db8:1234:5678::/64" {
		t.Fatalf("DerivePoolPrefix() = %s", got)
	}
}

func TestDerivePoolPrefixDoesNotBroadenExplicitHostPrefix(t *testing.T) {
	got, err := DerivePoolPrefix("2001:db8:1234:5678::/120", 64)
	if err != nil {
		t.Fatalf("DerivePoolPrefix() error = %v", err)
	}
	if got.String() != "2001:db8:1234:5678::/120" {
		t.Fatalf("DerivePoolPrefix() = %s", got)
	}
}

func TestDerivePoolPrefixRejectsBroadPrefix(t *testing.T) {
	if _, err := DerivePoolPrefix("2001:db8::/40", 64); err == nil {
		t.Fatal("DerivePoolPrefix() accepted a prefix broader than /48")
	}
}

func TestTunnelInterfaceClassification(t *testing.T) {
	for _, name := range []string{"he-sub2api", "sit0", "ip6tnl0", "foo-6in4"} {
		if !isTunnelInterface(name) {
			t.Fatalf("isTunnelInterface(%q) = false", name)
		}
	}
	if isTunnelInterface("eth0") {
		t.Fatal("isTunnelInterface(eth0) = true")
	}
}
