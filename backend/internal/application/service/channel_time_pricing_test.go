package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChannelTimePricingMultiplierAtUsesTimezoneAndHalfOpenPeriods(t *testing.T) {
	config := &ChannelTimePricing{
		Timezone: "Asia/Shanghai",
		Periods: []ChannelTimePricingPeriod{
			{StartTime: "09:00", EndTime: "18:00", Multiplier: 1.5},
		},
	}
	require.NoError(t, ValidateChannelTimePricing(config))

	inside := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	boundary := time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC)
	require.InDelta(t, 1.5, config.MultiplierAt(inside), 1e-9)
	require.InDelta(t, 1.0, config.MultiplierAt(boundary), 1e-9)
}

func TestChannelTimePricingRejectsOverlapAndInvalidTimezone(t *testing.T) {
	require.Error(t, ValidateChannelTimePricing(&ChannelTimePricing{
		Timezone: "Asia/Shanghai",
		Periods: []ChannelTimePricingPeriod{
			{StartTime: "09:00", EndTime: "12:00", Multiplier: 1.2},
			{StartTime: "11:00", EndTime: "13:00", Multiplier: 1.3},
		},
	}))
	require.Error(t, ValidateChannelTimePricing(&ChannelTimePricing{
		Timezone: "not/a/timezone",
		Periods:  []ChannelTimePricingPeriod{{StartTime: "09:00", EndTime: "10:00", Multiplier: 1.2}},
	}))
}

func TestBillingAppliesChannelTimeMultiplierToAllTokenCostComponents(t *testing.T) {
	input := &CostBreakdown{
		InputCost: 1, OutputCost: 2, CacheCreationCost: 3, CacheReadCost: 4,
		TotalCost: 10, ActualCost: 10,
	}
	applyCostBreakdownMultiplier(input, 1.5)
	require.Equal(t, 1.5, input.InputCost)
	require.Equal(t, 3.0, input.OutputCost)
	require.Equal(t, 4.5, input.CacheCreationCost)
	require.Equal(t, 6.0, input.CacheReadCost)
	require.Equal(t, 15.0, input.TotalCost)
	require.Equal(t, 15.0, input.ActualCost)
}
