package egress

import (
	"net/netip"
	"testing"
)

func TestAllocatorIsDeterministicAndStaysInsidePrefix(t *testing.T) {
	allocator := NewAllocator("0123456789abcdef0123456789abcdef")
	pool := Pool{ID: 7, CIDR: "2001:db8:42::/64", AllocationVersion: 3}
	first, err := allocator.Address(pool, 99, 1, 0)
	if err != nil {
		t.Fatalf("Address() error = %v", err)
	}
	second, err := allocator.Address(pool, 99, 1, 0)
	if err != nil {
		t.Fatalf("Address() second error = %v", err)
	}
	if first != second {
		t.Fatalf("deterministic allocation mismatch: %s != %s", first, second)
	}
	prefix := netip.MustParsePrefix(pool.CIDR)
	if !prefix.Contains(netip.MustParseAddr(first)) {
		t.Fatalf("allocated address %s is outside %s", first, prefix)
	}

	rotated, err := allocator.Address(pool, 99, 2, 0)
	if err != nil {
		t.Fatalf("rotated Address() error = %v", err)
	}
	if rotated == first {
		t.Fatalf("rotation retained address %s", first)
	}
}

func TestValidatePoolCIDRRejectsIPv4AndTinyPools(t *testing.T) {
	for _, raw := range []string{"192.0.2.0/24", "2001:db8::/124", "not-a-prefix"} {
		if _, err := ValidatePoolCIDR(raw); err == nil {
			t.Fatalf("ValidatePoolCIDR(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestPoolCapacityExcludesPrefixBaseAddress(t *testing.T) {
	capacity, err := PoolCapacity("2001:db8::/64")
	if err != nil || capacity != "18446744073709551615" {
		t.Fatalf("PoolCapacity(/64) = %q, %v", capacity, err)
	}
	capacity, err = PoolCapacity("2001:db8::/120")
	if err != nil || capacity != "255" {
		t.Fatalf("PoolCapacity(/120) = %q, %v", capacity, err)
	}
}
