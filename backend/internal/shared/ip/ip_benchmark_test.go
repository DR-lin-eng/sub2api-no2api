//go:build unit

package ip

import (
	"fmt"
	"testing"
)

// This fixture uses only the pre-existing public API so the same command and
// inputs can measure the pristine, modified and restored implementations.
func benchmarkIPPatterns(kind string, count int) []string {
	patterns := make([]string, count)
	for i := range patterns {
		switch kind {
		case "host_v4":
			patterns[i] = fmt.Sprintf("10.%d.%d.1", i/256, i%256)
		case "cidr_v4":
			patterns[i] = fmt.Sprintf("10.%d.%d.0/24", i/256, i%256)
		case "cidr_v6":
			patterns[i] = fmt.Sprintf("2001:db8:%x::/48", i)
		case "mixed":
			if i%2 == 0 {
				patterns[i] = fmt.Sprintf("10.%d.%d.0/24", i/256, i%256)
			} else {
				patterns[i] = fmt.Sprintf("2001:db8:%x::/48", i)
			}
		}
	}
	return patterns
}

func TestIPRestrictionTransaction(t *testing.T) {
	rules := CompileIPRules(benchmarkIPPatterns("cidr_v4", 4096))
	for _, tc := range []struct {
		input     string
		blacklist bool
		allowed   bool
	}{
		{"10.15.255.42", false, true},
		{"::ffff:10.15.255.42", false, true},
		{"10.15.255.42:8080", false, true},
		{"203.0.113.42", false, false},
		{"10.15.255.42", true, false},
		{"203.0.113.42", true, true},
		{"invalid", false, false},
	} {
		whitelist, blacklist := rules, (*CompiledIPRules)(nil)
		if tc.blacklist {
			whitelist, blacklist = nil, rules
		}
		allowed, reason := CheckIPRestrictionWithCompiledRules(tc.input, whitelist, blacklist)
		t.Logf("input=%q blacklist=%t allowed=%t reason=%q", tc.input, tc.blacklist, allowed, reason)
		if allowed != tc.allowed || (!allowed && reason != "access denied") || (allowed && reason != "") {
			t.Fatalf("unexpected access result for %q", tc.input)
		}
	}
}

func BenchmarkIPRestriction(b *testing.B) {
	for _, count := range []int{4, 64, 4096} {
		for _, kind := range []string{"host_v4", "cidr_v4", "cidr_v6", "mixed"} {
			rules := CompileIPRules(benchmarkIPPatterns(kind, count))
			for _, hit := range []string{"miss", "first", "last"} {
				input := "203.0.113.42"
				if kind == "cidr_v6" {
					input = "2001:db9::1"
				}
				if hit == "first" {
					input = rules.Prefixes[0].Addr().String()
				} else if hit == "last" {
					input = rules.Prefixes[len(rules.Prefixes)-1].Addr().String()
				}
				want := hit != "miss"
				b.Run(fmt.Sprintf("%s/%d/%s", kind, count, hit), func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						allowed, _ := CheckIPRestrictionWithCompiledRules(input, rules, nil)
						if allowed != want {
							b.Fatalf("allowed=%t; want %t", allowed, want)
						}
					}
				})
			}
		}
	}
}

func BenchmarkCompileIPRules(b *testing.B) {
	for _, count := range []int{4, 4096} {
		patterns := benchmarkIPPatterns("mixed", count)
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				CompileIPRules(patterns)
			}
		})
	}
}
