//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildUsageBillingCommand_AccountQuotaUsesAccountBillingCost(t *testing.T) {
	t.Parallel()

	groupID := int64(7)
	accountStatsCost := 4.0
	params := &postUsageBillingParams{
		Cost:                  &CostBreakdown{TotalCost: 2, ActualCost: 6},
		AccountStatsCost:      &accountStatsCost,
		User:                  &User{ID: 1},
		APIKey:                &APIKey{ID: 2, GroupID: &groupID},
		Account:               &Account{ID: 3, Type: AccountTypeAPIKey, Extra: map[string]any{"quota_limit": 100.0}},
		AccountRateMultiplier: 1.25,
	}

	cmd := buildUsageBillingCommand("req-account-cost", nil, params)
	require.NotNil(t, cmd)
	require.InDelta(t, 5.0, cmd.AccountQuotaCost, 1e-12)
}

func TestBuildUsageBillingCommand_AccountQuotaFallsBackToRawCost(t *testing.T) {
	t.Parallel()

	groupID := int64(7)
	params := &postUsageBillingParams{
		Cost:                  &CostBreakdown{TotalCost: 2, ActualCost: 6},
		User:                  &User{ID: 1},
		APIKey:                &APIKey{ID: 2, GroupID: &groupID},
		Account:               &Account{ID: 3, Type: AccountTypeAPIKey, Extra: map[string]any{"quota_limit": 100.0}},
		AccountRateMultiplier: 1.25,
	}

	cmd := buildUsageBillingCommand("req-raw-cost", nil, params)
	require.NotNil(t, cmd)
	require.InDelta(t, 2.5, cmd.AccountQuotaCost, 1e-12)
}

func TestBuildUsageBillingCommand_AccountQuotaCanApplyWhenUserCostIsZero(t *testing.T) {
	t.Parallel()

	groupID := int64(7)
	accountStatsCost := 4.0
	params := &postUsageBillingParams{
		Cost:                  &CostBreakdown{},
		AccountStatsCost:      &accountStatsCost,
		User:                  &User{ID: 1},
		APIKey:                &APIKey{ID: 2, GroupID: &groupID},
		Account:               &Account{ID: 3, Type: AccountTypeAPIKey, Extra: map[string]any{"quota_daily_limit": 100.0}},
		AccountRateMultiplier: 1.25,
	}

	cmd := buildUsageBillingCommand("req-free-user", nil, params)
	require.NotNil(t, cmd)
	require.Zero(t, cmd.BalanceCost)
	require.InDelta(t, 5.0, cmd.AccountQuotaCost, 1e-12)
}
