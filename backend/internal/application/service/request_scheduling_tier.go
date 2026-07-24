package service

import "context"

// RequestSchedulingTier controls request admission order when priority
// admission is enabled. Lower numeric values represent higher priority.
type RequestSchedulingTier int16

const (
	RequestSchedulingTierPriority RequestSchedulingTier = 0
	RequestSchedulingTierNormal   RequestSchedulingTier = 1
	RequestSchedulingTierLow      RequestSchedulingTier = 2
)

func (t RequestSchedulingTier) Valid() bool {
	return t >= RequestSchedulingTierPriority && t <= RequestSchedulingTierLow
}

func NormalizeRequestSchedulingTier(t RequestSchedulingTier) RequestSchedulingTier {
	if !t.Valid() {
		return RequestSchedulingTierNormal
	}
	return t
}

type requestSchedulingTierContextKey struct{}

func WithRequestSchedulingTier(ctx context.Context, tier RequestSchedulingTier) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestSchedulingTierContextKey{}, NormalizeRequestSchedulingTier(tier))
}

func RequestSchedulingTierFromContextOK(ctx context.Context) (RequestSchedulingTier, bool) {
	if ctx == nil {
		return RequestSchedulingTierNormal, false
	}
	tier, ok := ctx.Value(requestSchedulingTierContextKey{}).(RequestSchedulingTier)
	if !ok || !tier.Valid() {
		return RequestSchedulingTierNormal, false
	}
	return tier, true
}

func RequestSchedulingTierFromContext(ctx context.Context) RequestSchedulingTier {
	tier, _ := RequestSchedulingTierFromContextOK(ctx)
	return tier
}
