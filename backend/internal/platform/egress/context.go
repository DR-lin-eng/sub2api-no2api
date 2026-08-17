package egress

import "context"

type contextRouteKey struct{}

type ContextRoute struct {
	Route     Route
	Policy    Policy
	AccountID int64
}

func WithContextRoute(ctx context.Context, route Route, policy Policy) context.Context {
	return WithContextAccountRoute(ctx, route, policy, 0)
}

func WithContextAccountRoute(ctx context.Context, route Route, policy Policy, accountID int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextRouteKey{}, ContextRoute{Route: route, Policy: policy, AccountID: accountID})
}

func FromContext(ctx context.Context) (ContextRoute, bool) {
	if ctx == nil {
		return ContextRoute{}, false
	}
	value, ok := ctx.Value(contextRouteKey{}).(ContextRoute)
	return value, ok
}
