package service

import (
	"context"
	"testing"
)

func TestRequestSchedulingTierValidationAndContext(t *testing.T) {
	for _, tier := range []RequestSchedulingTier{
		RequestSchedulingTierPriority,
		RequestSchedulingTierNormal,
		RequestSchedulingTierLow,
	} {
		if !tier.Valid() {
			t.Fatalf("expected tier %d to be valid", tier)
		}
		ctx := WithRequestSchedulingTier(context.Background(), tier)
		got, ok := RequestSchedulingTierFromContextOK(ctx)
		if !ok || got != tier {
			t.Fatalf("expected tier %d round trip, got %d, %v", tier, got, ok)
		}
	}

	if got, ok := RequestSchedulingTierFromContextOK(context.Background()); ok || got != RequestSchedulingTierNormal {
		t.Fatalf("absent tier must use normal without an explicit marker, got %d, %v", got, ok)
	}
	if got := NormalizeRequestSchedulingTier(99); got != RequestSchedulingTierNormal {
		t.Fatalf("invalid tier must normalize to normal, got %d", got)
	}
}
