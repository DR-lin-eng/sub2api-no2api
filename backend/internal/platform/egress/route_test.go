package egress

import (
	"errors"
	"testing"
)

func TestApplyPolicyInheritedIPv6UsesKillSwitch(t *testing.T) {
	route := IPv6PoolRoute("2001:db8::10", 4, 2, true)
	effective, err := ApplyPolicy(route, Policy{})
	if err != nil {
		t.Fatalf("ApplyPolicy() error = %v", err)
	}
	if effective.Mode != ModeDirect || !effective.Inherited {
		t.Fatalf("ApplyPolicy() = %#v, want inherited direct", effective)
	}
}

func TestApplyPolicyExplicitIPv6FailsClosed(t *testing.T) {
	_, err := ApplyPolicy(IPv6PoolRoute("2001:db8::10", 4, 2, false), Policy{})
	if !errors.Is(err, ErrIPv6Disabled) {
		t.Fatalf("ApplyPolicy() error = %v, want %v", err, ErrIPv6Disabled)
	}
}

func TestRouteValidationRejectsIncompleteSelections(t *testing.T) {
	tests := []Route{
		{Mode: ModeExternalProxy},
		{Mode: ModeIPv6Pool, SourceIPv6: "192.0.2.10", PoolID: 1, BindingVersion: 1},
		{Mode: ModeIPv6Pool, SourceIPv6: "2001:db8::1", PoolID: 0, BindingVersion: 1},
		{Mode: ModeInherit},
	}
	for _, route := range tests {
		if err := route.Validate(); err == nil {
			t.Fatalf("Validate(%#v) unexpectedly succeeded", route)
		}
	}
}
