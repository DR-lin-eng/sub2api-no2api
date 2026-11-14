package service

import (
	"context"
	"encoding/binary"
	"hash/fnv"
	"math"
	"math/rand"
	"sort"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
)

type apiKeyGroupRoutingState struct {
	mu     sync.RWMutex
	apiKey *APIKey
	// orderedBindings is the request-visible candidate order. Scheme bindings
	// are ordered by weighted rendezvous seeded with the session hash; legacy
	// bindings keep their configured priority order.
	ordered         bool
	orderedBindings []APIKeyGroupBinding
	selectedIndex   int
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

// orderRoutingBindings builds the request-visible candidate order for scheme
// keys. Each binding draws -ln(unitHash(seed, groupID)) / weight; sorting the
// keys ascending yields a weighted rendezvous order. Seeding with the session
// hash keeps one conversation on the same group while different sessions spread
// by weight. Bindings without a weight keep their configured priority order.
func orderRoutingBindings(apiKey *APIKey, sessionHash string) []APIKeyGroupBinding {
	if apiKey == nil {
		return nil
	}
	bindings := apiKey.GroupBindings
	if len(bindings) < 2 {
		return bindings
	}
	weighted := false
	for i := range bindings {
		if bindings[i].Weight > 0 {
			weighted = true
			break
		}
	}
	if !weighted {
		return bindings
	}
	seed := routingOrderSeed(apiKey, sessionHash)
	type weightedEntry struct {
		binding APIKeyGroupBinding
		key     float64
	}
	entries := make([]weightedEntry, len(bindings))
	for i := range bindings {
		weight := bindings[i].Weight
		if weight <= 0 {
			weight = 1
		}
		entries[i] = weightedEntry{
			binding: bindings[i],
			key:     -math.Log(routingUnitHash(seed, bindings[i].GroupID)) / float64(weight),
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].key < entries[j].key
	})
	ordered := make([]APIKeyGroupBinding, len(entries))
	for i := range entries {
		ordered[i] = entries[i].binding
	}
	return ordered
}

// routingOrderSeed derives the rendezvous seed. A session hash makes the order
// stable across requests and instances; without one, each request re-draws.
func routingOrderSeed(apiKey *APIKey, sessionHash string) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(apiKey.ID))
	_, _ = h.Write(buf[:])
	var schemeID int64
	if apiKey.RoutingSchemeID != nil {
		schemeID = *apiKey.RoutingSchemeID
	}
	binary.LittleEndian.PutUint64(buf[:], uint64(schemeID))
	_, _ = h.Write(buf[:])
	if sessionHash == "" {
		binary.LittleEndian.PutUint64(buf[:], rand.Uint64())
		_, _ = h.Write(buf[:])
	} else {
		_, _ = h.Write([]byte(sessionHash))
	}
	return h.Sum64()
}

// routingUnitHash maps (seed, groupID) into (0, 1].
func routingUnitHash(seed uint64, groupID int64) float64 {
	h := fnv.New64a()
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[:8], seed)
	binary.LittleEndian.PutUint64(buf[8:], uint64(groupID))
	_, _ = h.Write(buf[:])
	return float64((h.Sum64()>>11)+1) / float64(uint64(1)<<53)
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

func apiKeyGroupRoutingCandidates(ctx context.Context, groupID *int64, sessionHash string) ([]APIKeyGroupBinding, bool) {
	if ctx == nil || ctx.Value(apiKeyGroupRoutingAttemptKey{}) != nil {
		return nil, false
	}
	state, ok := ctx.Value(ctxkey.APIKeyGroupRouting).(*apiKeyGroupRoutingState)
	if !ok || state == nil || state.apiKey == nil || len(state.apiKey.GroupBindings) == 0 {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if groupID != nil && state.apiKey.GroupID != nil && *groupID != *state.apiKey.GroupID {
		return nil, false
	}
	if !state.ordered {
		state.orderedBindings = orderRoutingBindings(state.apiKey, sessionHash)
		state.ordered = true
		// The preselected binding was only for early auth checks; actual
		// scheduling starts from the session-stable order.
		state.selectedIndex = -1
		for i := range state.orderedBindings {
			if apiKeyGroupBindingEligible(state.apiKey, &state.orderedBindings[i]) {
				state.selectedIndex = i
				break
			}
		}
	}
	if len(state.orderedBindings) == 0 {
		return nil, false
	}
	start := state.selectedIndex
	if start < 0 {
		start = 0
	}
	candidates := make([]APIKeyGroupBinding, 0, len(state.orderedBindings)-start)
	for i := start; i < len(state.orderedBindings); i++ {
		binding := state.orderedBindings[i]
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
	bindings := state.apiKey.GroupBindings
	if len(state.orderedBindings) > 0 {
		bindings = state.orderedBindings
	}
	for i := range bindings {
		if bindings[i].GroupID == groupID {
			state.selectedIndex = i
			applyAPIKeyRoutingBinding(state.apiKey, &bindings[i])
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
