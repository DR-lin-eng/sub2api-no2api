package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeAccountOAuthQuotaFilter(t *testing.T) {
	tests := map[string]string{
		"exhausted":    AccountOAuthQuotaFilterExhausted,
		"has_quota":    AccountOAuthQuotaFilterHasQuota,
		"with_reset":   AccountOAuthQuotaFilterWithReset,
		"5h_exhausted": AccountOAuthQuotaFilter5hExhausted,
		"7d_exhausted": AccountOAuthQuotaFilter7dExhausted,
	}
	for input, expected := range tests {
		input, expected := input, expected
		t.Run(input, func(t *testing.T) {
			got, err := NormalizeAccountOAuthQuotaFilter(input)
			require.NoError(t, err)
			require.Equal(t, expected, got)
		})
	}

	_, err := NormalizeAccountOAuthQuotaFilter("unknown")
	require.Error(t, err)
}
