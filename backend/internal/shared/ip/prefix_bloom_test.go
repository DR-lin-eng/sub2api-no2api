//go:build unit

package ip

import (
	"fmt"
	"math/rand/v2"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrefixBloomAllNetworkLengths(t *testing.T) {
	for _, raw := range []string{"192.0.2.129", "2001:db8:1234:5678:abcd::1234"} {
		addr := netip.MustParseAddr(raw)
		for length := 0; length <= addr.BitLen(); length++ {
			t.Run(fmt.Sprintf("%s/%d", raw, length), func(t *testing.T) {
				// Include non-canonical host bits to verify network masking.
				pattern := fmt.Sprintf("%s/%d", raw, length)
				patterns := make([]string, 64)
				for i := range patterns {
					patterns[i] = pattern
				}
				rules := CompileIPRules(patterns)
				require.NotNil(t, rules.bloom)
				require.True(t, rules.bloom.mayContain(addr), "member must never be filtered out")
				allowed, reason := CheckIPRestrictionWithCompiledRules(raw, rules, nil)
				require.True(t, allowed)
				require.Empty(t, reason)
			})
		}
	}
}

func TestPrefixBloomMatchesLinearRules(t *testing.T) {
	rules := CompileIPRules(benchmarkIPPatterns("mixed", 512))
	require.NotNil(t, rules.bloom)
	rng := rand.New(rand.NewPCG(7, 11))
	addresses := []netip.Addr{netip.Addr{}, netip.MustParseAddr("fe80::1%eth0")}
	for _, prefix := range rules.Prefixes {
		addresses = append(addresses, prefix.Addr())
	}
	for i := 0; i < 2000; i++ {
		var raw [16]byte
		for j := range raw {
			raw[j] = byte(rng.Uint32())
		}
		addresses = append(addresses, netip.AddrFrom16(raw), netip.AddrFrom4([4]byte{raw[0], raw[1], raw[2], raw[3]}))
	}
	for _, addr := range addresses {
		want := false
		for _, prefix := range rules.Prefixes {
			if prefix.Contains(addr) {
				want = true
				break
			}
		}
		require.Equal(t, want, matchesCompiledRules(addr, rules), addr.String())
		if want {
			require.True(t, rules.bloom.mayContain(addr), addr.String())
		}
	}
}

func TestPrefixBloomFalsePositiveStillUsesExactRules(t *testing.T) {
	rules := CompileIPRules(benchmarkIPPatterns("host_v4", 64))
	require.NotNil(t, rules.bloom)
	// Saturate the unpublished test snapshot to force a false positive.
	for i := range rules.bloom.words {
		rules.bloom.words[i] = ^uint64(0)
	}
	clientIP := "203.0.113.99"
	require.True(t, rules.bloom.mayContain(netip.MustParseAddr(clientIP)))
	allowed, reason := CheckIPRestrictionWithCompiledRules(clientIP, rules, nil)
	require.False(t, allowed, "false positive must not bypass the whitelist")
	require.Equal(t, "access denied", reason)
	allowed, reason = CheckIPRestrictionWithCompiledRules(clientIP, nil, rules)
	require.True(t, allowed, "false positive must not add a blacklist match")
	require.Empty(t, reason)
}

func TestPrefixBloomFallbackAndRestrictions(t *testing.T) {
	for _, patterns := range [][]string{nil, {"invalid"}, benchmarkIPPatterns("host_v4", 4)} {
		require.Nil(t, CompileIPRules(patterns).bloom)
	}
	diverse := make([]string, 64)
	for i := range diverse {
		diverse[i] = fmt.Sprintf("2001:db8::/%d", i+1)
	}
	require.Nil(t, CompileIPRules(diverse).bloom, "many prefix lengths should retain the linear path")

	patterns := benchmarkIPPatterns("host_v4", 64)
	patterns = append(patterns, " ::ffff:192.0.2.128/120 ", "2001:db8:1::abcd/64", "invalid", "")
	rules := CompileIPRules(patterns)
	require.NotNil(t, rules.bloom)
	for _, raw := range []string{"192.0.2.255", "::ffff:192.0.2.255", "192.0.2.255:8080", "2001:db8:1::ffff"} {
		allowed, _ := CheckIPRestrictionWithCompiledRules(raw, rules, nil)
		require.True(t, allowed, raw)
		allowed, reason := CheckIPRestrictionWithCompiledRules(raw, rules, rules)
		require.False(t, allowed, "blacklist must take priority: %s", raw)
		require.Equal(t, "access denied", reason)
	}
	for _, raw := range []string{"192.0.3.0", "2001:db8:2::1", "", "invalid", "fe80::1%eth0"} {
		allowed, reason := CheckIPRestrictionWithCompiledRules(raw, rules, nil)
		require.False(t, allowed, raw)
		require.Equal(t, "access denied", reason)
	}
	// Callers that construct the exported struct directly still work.
	manual := &CompiledIPRules{Prefixes: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, PatternCount: 1}
	allowed, _ := CheckIPRestrictionWithCompiledRules("10.1.2.3", manual, nil)
	require.True(t, allowed)
}

func TestPrefixBloomConcurrentRestrictionChecks(t *testing.T) {
	rules := CompileIPRules(benchmarkIPPatterns("cidr_v6", 512))
	require.NotNil(t, rules.bloom)
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			for n := 0; n < 100; n++ {
				allowed, _ := CheckIPRestrictionWithCompiledRules("2001:db8:1ff::1", rules, nil)
				require.True(t, allowed)
				allowed, _ = CheckIPRestrictionWithCompiledRules("2001:db9::1", rules, nil)
				require.False(t, allowed)
			}
		})
	}
}
