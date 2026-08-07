package service

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func usageBillingDecimalPlaces(v float64) int32 {
	return -decimal.NewFromFloat(v).Exponent()
}

func TestUsageBillingCommandQuantizesBalanceAndQuotaIdentically(t *testing.T) {
	const actualCost = 0.000078125
	cmd := &UsageBillingCommand{
		RequestID:             "req-5229",
		UserID:                1,
		APIKeyID:              2,
		AccountID:             3,
		BalanceCost:           actualCost,
		APIKeyQuotaCost:       actualCost,
		UserPlatformQuotaCost: actualCost,
		QuotaPlatform:         "openai",
	}

	cmd.Normalize()

	require.Equal(t, cmd.BalanceCost, cmd.APIKeyQuotaCost)
	require.Equal(t, cmd.BalanceCost, cmd.UserPlatformQuotaCost)
	require.LessOrEqual(t, usageBillingDecimalPlaces(cmd.BalanceCost), int32(UsageBillingMonetaryScale))
}

func TestQuantizeUsageBillingAmountBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   float64
	}{
		{name: "below_half", in: 0.000078120},
		{name: "just_below_half", in: 0.000078124},
		{name: "exact_half", in: 0.000078125},
		{name: "just_above_half", in: 0.000078126},
		{name: "long_tail", in: 0.0000781234567},
		{name: "already_quantized", in: 0.00007813},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := QuantizeUsageBillingAmount(tc.in)
			want, _ := decimal.NewFromFloat(tc.in).Round(UsageBillingMonetaryScale).Float64()
			require.Equal(t, want, got)
			require.LessOrEqual(t, math.Abs(got-tc.in), 5e-9)
		})
	}
}

func TestUsageBillingNormalizeKeepsFingerprintDerivedFromRawAmounts(t *testing.T) {
	newCommand := func() *UsageBillingCommand {
		return &UsageBillingCommand{
			RequestID:             "req-5229-fingerprint",
			UserID:                1,
			APIKeyID:              2,
			AccountID:             3,
			BalanceCost:           0.000078125,
			APIKeyQuotaCost:       0.000078125,
			QuotaPlatform:         "openai",
			UserPlatformQuotaCost: 0.000078125,
		}
	}

	cmd := newCommand()
	want := buildUsageBillingFingerprint(newCommand())
	cmd.Normalize()

	require.Equal(t, want, cmd.RequestFingerprint)
}

func TestBuildUsageBillingCommandDefersFingerprintUntilPlatformQuotaIsKnown(t *testing.T) {
	const actualCost = 0.000078125
	groupID := int64(7)
	params := &postUsageBillingParams{
		Cost:    &CostBreakdown{TotalCost: actualCost, ActualCost: actualCost},
		User:    &User{ID: 1},
		APIKey:  &APIKey{ID: 2, GroupID: &groupID},
		Account: &Account{ID: 3},
	}

	cmd := buildUsageBillingCommand("req-platform-quota-upgrade", nil, params)
	require.NotNil(t, cmd)
	require.Empty(t, cmd.RequestFingerprint)
	require.Equal(t, actualCost, cmd.BalanceCost)

	cmd.QuotaPlatform = "openai"
	cmd.UserPlatformQuotaCost = actualCost
	wantFingerprint := buildUsageBillingFingerprint(cmd)
	cmd.Normalize()

	require.Equal(t, wantFingerprint, cmd.RequestFingerprint)
	require.Equal(t, QuantizeUsageBillingAmount(actualCost), cmd.BalanceCost)
	require.Equal(t, QuantizeUsageBillingAmount(actualCost), cmd.UserPlatformQuotaCost)
}

func TestQuantizeUsageBillingAmountHandlesSpecialAndNegativeValues(t *testing.T) {
	require.Equal(t, 0.0, QuantizeUsageBillingAmount(0))
	require.True(t, math.IsNaN(QuantizeUsageBillingAmount(math.NaN())))
	require.True(t, math.IsInf(QuantizeUsageBillingAmount(math.Inf(1)), 1))
	require.Equal(t, -QuantizeUsageBillingAmount(0.000078125), QuantizeUsageBillingAmount(-0.000078125))
}
