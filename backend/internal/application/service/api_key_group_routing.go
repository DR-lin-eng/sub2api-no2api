package service

import (
	"context"
	"math"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
)

type apiKeyGroupRoutingState struct {
	mu            sync.RWMutex
	apiKey        *APIKey
	selectedIndex int
}

type apiKeyGroupRoutingAttemptKey struct{}

// WithAPIKeyGroupRouting installs request-local routing state and selects the
// first currently eligible binding for the pre-selection authorization checks.
func WithAPIKeyGroupRouting(ctx context.Context, apiKey *APIKey) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if apiKey == nil || len(apiKey.GroupBindings) == 0 {
		return ctx
	}
	state := &apiKeyGroupRoutingState{apiKey: apiKey, selectedIndex: -1}
	for i := range apiKey.GroupBindings {
		if apiKeyGroupBindingEligible(apiKey, &apiKey.GroupBindings[i]) {
			state.selectedIndex = i
			applyAPIKeyRoutingBinding(apiKey, &apiKey.GroupBindings[i])
			break
		}
	}
	return context.WithValue(ctx, ctxkey.APIKeyGroupRouting, state)
}

func apiKeyGroupBindingEligible(apiKey *APIKey, binding *APIKeyGroupBinding) bool {
	if apiKey == nil || binding == nil || binding.Group == nil || !binding.Group.IsActive() {
		return false
	}
	if apiKey.User != nil && !binding.Group.IsSubscriptionType() &&
		!apiKey.User.CanBindGroup(binding.GroupID, binding.Group.IsExclusive) {
		return false
	}
	if binding.MaxRateMultiplier == nil {
		return true
	}
	rate := binding.EffectiveRateMultiplier
	return !math.IsNaN(rate) && !math.IsInf(rate, 0) && rate <= *binding.MaxRateMultiplier+1e-12
}

func applyAPIKeyRoutingBinding(apiKey *APIKey, binding *APIKeyGroupBinding) {
	if apiKey == nil || binding == nil {
		return
	}
	groupID := binding.GroupID
	if apiKey.GroupID == nil {
		apiKey.GroupID = &groupID
	} else {
		*apiKey.GroupID = groupID
	}
	apiKey.Group = binding.Group
}

func apiKeyGroupRoutingCandidates(ctx context.Context, groupID *int64) ([]APIKeyGroupBinding, bool) {
	if ctx == nil || ctx.Value(apiKeyGroupRoutingAttemptKey{}) != nil {
		return nil, false
	}
	state, ok := ctx.Value(ctxkey.APIKeyGroupRouting).(*apiKeyGroupRoutingState)
	if !ok || state == nil || state.apiKey == nil || len(state.apiKey.GroupBindings) == 0 {
		return nil, false
	}
	state.mu.RLock()
	defer state.mu.RUnlock()
	if groupID != nil && state.apiKey.GroupID != nil && *groupID != *state.apiKey.GroupID {
		return nil, false
	}
	start := state.selectedIndex
	if start < 0 {
		start = 0
	}
	candidates := make([]APIKeyGroupBinding, 0, len(state.apiKey.GroupBindings)-start)
	for i := start; i < len(state.apiKey.GroupBindings); i++ {
		binding := state.apiKey.GroupBindings[i]
		if apiKeyGroupBindingEligible(state.apiKey, &binding) {
			candidates = append(candidates, binding)
		}
	}
	return candidates, true
}

func withAPIKeyGroupRoutingAttempt(ctx context.Context, binding APIKeyGroupBinding) context.Context {
	ctx = context.WithValue(ctx, apiKeyGroupRoutingAttemptKey{}, binding.GroupID)
	if IsGroupContextValid(binding.Group) {
		ctx = context.WithValue(ctx, ctxkey.Group, binding.Group)
		if _, ok := tokenRequestPricingAtFromContext(ctx); ok {
			ctx = context.WithValue(ctx, tokenRequestBillingGroupCtxKey{}, binding.Group)
		}
	}
	return ctx
}

func markAPIKeyGroupRoutingSelected(ctx context.Context, groupID int64) {
	if ctx == nil || groupID <= 0 {
		return
	}
	state, ok := ctx.Value(ctxkey.APIKeyGroupRouting).(*apiKeyGroupRoutingState)
	if !ok || state == nil || state.apiKey == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	for i := range state.apiKey.GroupBindings {
		if state.apiKey.GroupBindings[i].GroupID == groupID {
			state.selectedIndex = i
			applyAPIKeyRoutingBinding(state.apiKey, &state.apiKey.GroupBindings[i])
			return
		}
	}
}

func selectedAPIKeyRoutingGroup(ctx context.Context, groupID int64) *Group {
	if ctx == nil || groupID <= 0 {
		return nil
	}
	state, ok := ctx.Value(ctxkey.APIKeyGroupRouting).(*apiKeyGroupRoutingState)
	if !ok || state == nil || state.apiKey == nil {
		return nil
	}
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.apiKey.GroupID == nil || *state.apiKey.GroupID != groupID {
		return nil
	}
	return state.apiKey.Group
}

func currentAPIKeyRoutingGroup(ctx context.Context) *Group {
	if ctx == nil {
		return nil
	}
	state, ok := ctx.Value(ctxkey.APIKeyGroupRouting).(*apiKeyGroupRoutingState)
	if !ok || state == nil || state.apiKey == nil {
		return nil
	}
	state.mu.RLock()
	defer state.mu.RUnlock()
	if IsGroupContextValid(state.apiKey.Group) {
		return state.apiKey.Group
	}
	return nil
}

// EligibleAPIKeyGroupBindings returns the request-visible routing candidates in
// their configured order. It is used by metadata endpoints such as /v1/models.
func EligibleAPIKeyGroupBindings(apiKey *APIKey) []APIKeyGroupBinding {
	if apiKey == nil {
		return nil
	}
	out := make([]APIKeyGroupBinding, 0, len(apiKey.GroupBindings))
	for i := range apiKey.GroupBindings {
		if apiKeyGroupBindingEligible(apiKey, &apiKey.GroupBindings[i]) {
			out = append(out, apiKey.GroupBindings[i])
		}
	}
	return out
}
