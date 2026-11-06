package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeAccountOAuthPlanFilter(t *testing.T) {
	tests := map[string]string{
		"free":       AccountOAuthPlanFilterFree,
		"pro-5x":     AccountOAuthPlanFilterPro5x,
		"PRO_20X":    AccountOAuthPlanFilterPro20x,
		"pro 50x":    AccountOAuthPlanFilterPro50x,
		"chatgptpro": AccountOAuthPlanFilterPro,
		"team":       AccountOAuthPlanFilterTeam,
		"plus":       AccountOAuthPlanFilterPlus,
	}
	for input, want := range tests {
		got, err := NormalizeAccountOAuthPlanFilter(input)
		require.NoError(t, err, input)
		require.Equal(t, want, got, input)
	}

	_, err := NormalizeAccountOAuthPlanFilter("unknown-tier")
	require.Error(t, err)
}
