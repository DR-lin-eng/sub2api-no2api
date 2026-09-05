package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

type activityGateRepo struct {
	SettingRepository
	value string
}

func (r activityGateRepo) GetValue(context.Context, string) (string, error) { return r.value, nil }
func TestActivityInflationDisabledPreservesRedemption(t *testing.T) {
	for _, value := range []string{"", "false"} {
		t.Run(value, func(t *testing.T) {
			// A nil activity service proves no activity repository is consulted while disabled.
			resolver := &activityCenterInflationResolver{settings: &SettingService{settingRepo: activityGateRepo{value: value}}}
			amount, id, _, _, pct, err := resolver.ResolveInflatedBalance(context.Background(), 1, 100)
			require.NoError(t, err)
			require.Equal(t, 100.0, amount)
			require.Zero(t, id)
			require.Zero(t, pct)
		})
	}
}
