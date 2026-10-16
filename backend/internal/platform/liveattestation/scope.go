package liveattestation

import (
	"context"
	"errors"
)

var ErrScopeMismatch = errors.New("attestation provider does not match request identity")

// RequestScope is an immutable, server-owned routing receipt. These fields are
// internal context, never additional wire fields or a claim about hardware.
type RequestScope struct {
	Key            string
	InstallationID string
	ThreadID       string
}

type requestScopeContextKey struct{}

func WithRequestScope(ctx context.Context, scope RequestScope) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestScopeContextKey{}, scope)
}

func RequestScopeFromContext(ctx context.Context) (RequestScope, bool) {
	if ctx == nil {
		return RequestScope{}, false
	}
	scope, ok := ctx.Value(requestScopeContextKey{}).(RequestScope)
	return scope, ok && scope.Key != "" && scope.InstallationID != ""
}

// ScopedProvider selects an account/installation-specific local runtime view.
// It must not mutate the original provider. Existing providers still receive
// the immutable RequestScope in context without changing their interface.
type ScopedProvider interface {
	ForScope(string) Provider
}

func ProviderForScope(provider Provider, scopeKey string) Provider {
	if scoped, ok := provider.(ScopedProvider); ok {
		return scoped.ForScope(scopeKey)
	}
	return provider
}

func validateProviderScope(ctx context.Context, scopeKey string) error {
	if scopeKey == "" {
		return nil
	}
	scope, ok := RequestScopeFromContext(ctx)
	if !ok || scope.Key != scopeKey {
		return ErrScopeMismatch
	}
	return nil
}
