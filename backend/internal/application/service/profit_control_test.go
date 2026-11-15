package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/ctxkey"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/timezone"
	"github.com/stretchr/testify/require"
)

func newProfitControlTestGroup(id int64, platform string) *Group {
	return &Group{
		ID:                   id,
		Name:                 "profit-control",
		Platform:             platform,
		RateMultiplier:       1,
		SubscriptionType:     SubscriptionTypeStandard,
		Status:               StatusActive,
		Hydrated:             true,
		ProfitControlEnabled: true,
	}
}

func profitControlTestContext(group *Group, pricingAt time.Time) context.Context {
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)
	ctx = context.WithValue(ctx, tokenRequestPricingAtCtxKey{}, pricingAt)
	return context.WithValue(ctx, tokenRequestBillingGroupCtxKey{}, group)
}

func TestProfitControlConfigValidationAndNormalization(t *testing.T) {
	supported := []string{PlatformOpenAI, PlatformAnthropic, PlatformGemini, PlatformGrok, PlatformAntigravity}
	for _, platform := range supported {
		require.NoError(t, ValidateProfitControlConfig(platform, true, 0.3, 0.05))
		enabled, margin, buffer := NormalizeProfitControlConfig(platform, true, 0.3, 0.05)
		require.True(t, enabled)
		require.InDelta(t, 0.3, margin, 1e-12)
		require.InDelta(t, 0.05, buffer, 1e-12)
	}

	require.Error(t, ValidateProfitControlConfig(PlatformComposite, true, 0.3, 0.05))
	require.Error(t, ValidateProfitControlConfig(PlatformOpenAI, true, math.NaN(), 0))
	require.Error(t, ValidateProfitControlConfig(PlatformOpenAI, true, 0.6, 0.4))
	require.NoError(t, ValidateProfitControlConfig(PlatformComposite, false, math.NaN(), math.Inf(1)))

	enabled, margin, buffer := NormalizeProfitControlConfig(PlatformComposite, true, 0.3, 0.05)
	require.False(t, enabled)
	require.Zero(t, margin)
	require.Zero(t, buffer)

	enabled, margin, buffer = NormalizeProfitControlConfig(PlatformOpenAI, false, -1, math.Inf(1))
	require.False(t, enabled)
	require.Zero(t, margin)
	require.Zero(t, buffer)
}

func TestProfitControlThresholdHelpers(t *testing.T) {
	for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -0.1} {
		require.Zero(t, clampProfitControlThreshold(invalid))
	}
	require.Equal(t, 0.7, clampProfitControlThreshold(0.7))
	require.False(t, profitControlOverThreshold(0.7, 0.7))
	require.False(t, profitControlOverThreshold(0.7+1e-12, 0.7))
	require.True(t, profitControlOverThreshold(0.7001, 0.7))
	require.False(t, profitControlOverThreshold(0, 0))
	require.True(t, profitControlOverThreshold(0.0001, 0))
}

func TestResolveProfitControlGateUsesFrozenPeakAndUserRate(t *testing.T) {
	group := newProfitControlTestGroup(41, PlatformOpenAI)
	group.SubscriptionType = SubscriptionTypeSubscription
	group.PeakRateEnabled = true
	group.PeakStart = "09:00"
	group.PeakEnd = "10:00"
	group.PeakRateMultiplier = 1.5
	group.ProfitMinMargin = 0.2
	group.ProfitSafetyBuffer = 0.1
	pricingAt := time.Date(2026, time.August, 2, 9, 30, 0, 0, timezone.Location())
	ctx := context.WithValue(profitControlTestContext(group, pricingAt), ctxkey.UserID, int64(7))

	resolverCalls := 0
	gate := resolveProfitControlGate(ctx, &group.ID, nil, func(_ context.Context, userID, groupID int64, base float64) float64 {
		resolverCalls++
		require.Equal(t, int64(7), userID)
		require.Equal(t, group.ID, groupID)
		require.Equal(t, group.RateMultiplier, base)
		return 1.6
	})

	require.NotNil(t, gate)
	require.Equal(t, pricingAt, gate.pricingAt)
	require.Equal(t, group.ID, gate.groupID)
	require.Equal(t, PlatformOpenAI, gate.platform)
	require.InDelta(t, 1.6*1.5*(1-0.2-0.1), gate.threshold, 1e-12)
	require.Equal(t, 1, resolverCalls)

	group.ProfitControlEnabled = false
	require.Nil(t, resolveProfitControlGate(ctx, &group.ID, nil, nil))
	group.ProfitControlEnabled = true
	group.Platform = PlatformComposite
	require.Nil(t, resolveProfitControlGate(ctx, &group.ID, nil, nil))
	require.Nil(t, resolveProfitControlGate(context.Background(), &group.ID, nil, nil), "metadata requests have no frozen token pricing instant")
}

func TestProfitControlVetoFiltersInvalidAndExpensiveAccounts(t *testing.T) {
	ctx := context.WithValue(context.Background(), profitControlGateCtxKey{}, &profitControlGate{threshold: 0.7})

	cheapRate := 0.6
	vetoed, reason := profitControlVetoReason(ctx, &Account{RateMultiplier: &cheapRate})
	require.False(t, vetoed)
	require.Empty(t, reason)

	expensiveRate := 0.8
	vetoed, reason = profitControlVetoReason(ctx, &Account{RateMultiplier: &expensiveRate})
	require.True(t, vetoed)
	require.Equal(t, profitControlFilterReasonThreshold, reason)

	for _, invalid := range []*float64{nil, float64Pointer(-1), float64Pointer(math.NaN()), float64Pointer(math.Inf(1))} {
		vetoed, reason = profitControlVetoReason(ctx, &Account{RateMultiplier: invalid})
		require.True(t, vetoed)
		require.Equal(t, profitControlFilterReasonInvalidRate, reason)
	}
}

func float64Pointer(value float64) *float64 {
	return &value
}

type profitControlSnapshotCache struct {
	SchedulerCache
	account *Account
}

func (c *profitControlSnapshotCache) GetAccount(context.Context, int64) (*Account, error) {
	return c.account, nil
}

func TestProfitControlLatestSnapshotVetoAndSelectionGateReplay(t *testing.T) {
	selectedRate := 0.2
	selected := &Account{ID: 51, RateMultiplier: &selectedRate, UpdatedAt: time.Unix(1, 0)}
	latestRate := 0.8
	latest := &Account{ID: 51, RateMultiplier: &latestRate, UpdatedAt: time.Unix(2, 0)}
	gateCtx := context.WithValue(context.Background(), profitControlGateCtxKey{}, &profitControlGate{groupID: 1, threshold: 0.5})
	snapshot := NewSchedulerSnapshotService(&profitControlSnapshotCache{account: latest}, nil, nil, nil, nil)

	refreshed, vetoed, reason := profitControlVetoLatest(gateCtx, selected, snapshot)
	require.Same(t, latest, refreshed)
	require.True(t, vetoed)
	require.Equal(t, profitControlFilterReasonThreshold, reason)
	require.InDelta(t, 0.2, *selected.RateMultiplier, 1e-12)

	selection := attachSelectionProfitGate(gateCtx, &AccountSelectionResult{})
	replayed := ContextWithSelectionProfitGate(context.Background(), selection)
	vetoed, reason = profitControlVetoReason(replayed, latest)
	require.True(t, vetoed)
	require.Equal(t, profitControlFilterReasonThreshold, reason)
}

type profitControlGatewayCache struct {
	bindings map[string]int64
	missErr  error
}

func (c *profitControlGatewayCache) GetSessionAccountID(_ context.Context, _ int64, sessionHash string) (int64, error) {
	if accountID, ok := c.bindings[sessionHash]; ok {
		return accountID, nil
	}
	return 0, c.missErr
}

func (c *profitControlGatewayCache) SetSessionAccountID(_ context.Context, _ int64, sessionHash string, accountID int64, _ time.Duration) error {
	if c.bindings == nil {
		c.bindings = make(map[string]int64)
	}
	c.bindings[sessionHash] = accountID
	return nil
}

func (*profitControlGatewayCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (c *profitControlGatewayCache) DeleteSessionAccountID(_ context.Context, _ int64, sessionHash string) error {
	delete(c.bindings, sessionHash)
	return nil
}

func TestGatewayProfitControlDelaysStickyBindingUntilAdmission(t *testing.T) {
	groupID := int64(61)
	gateCtx := context.WithValue(context.Background(), profitControlGateCtxKey{}, &profitControlGate{groupID: groupID, threshold: 0.5})

	cache := &profitControlGatewayCache{bindings: map[string]int64{"sticky": 1}}
	svc := &GatewayService{cache: cache}
	require.NoError(t, svc.bindGatewayStickySessionDuringSelection(gateCtx, &groupID, "sticky", 2))
	require.Equal(t, int64(1), cache.bindings["sticky"])
	require.NoError(t, svc.BindStickySessionAfterProfitAdmission(gateCtx, &groupID, "sticky", 2))
	require.Equal(t, int64(1), cache.bindings["sticky"], "a fallback must not replace an existing sticky owner")

	missCache := &profitControlGatewayCache{bindings: map[string]int64{}, missErr: ErrStickySessionNotFound}
	missService := &GatewayService{cache: missCache}
	require.NoError(t, missService.BindStickySessionAfterProfitAdmission(gateCtx, &groupID, "fresh", 2))
	require.Equal(t, int64(2), missCache.bindings["fresh"])

	plainCache := &profitControlGatewayCache{bindings: map[string]int64{}}
	plainService := &GatewayService{cache: plainCache}
	require.NoError(t, plainService.bindGatewayStickySessionDuringSelection(context.Background(), &groupID, "plain", 3))
	require.Equal(t, int64(3), plainCache.bindings["plain"])
}
