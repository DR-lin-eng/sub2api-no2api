//go:build unit

package payment

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPaymentMinorUnitConversionRejectsOverflow(t *testing.T) {
	for _, test := range []struct{ amount, currency string }{
		{"92233720368547758.08", "CNY"},
		{"184467440737095516.17", "CNY"}, // previously wrapped to one cent
		{"-92233720368547758.09", "CNY"},
		{"9223372036854775808", "JPY"},
		{"9223372036854775.808", "KWD"},
	} {
		_, err := AmountToMinorUnit(test.amount, test.currency)
		require.ErrorContains(t, err, "range")
	}
	for _, test := range []struct {
		amount string
		want   int64
	}{
		{"92233720368547758.07", math.MaxInt64},
		{"-92233720368547758.08", math.MinInt64},
		{"0", 0},
	} {
		got, err := AmountToMinorUnit(test.amount, "CNY")
		require.NoError(t, err)
		require.Equal(t, test.want, got)
	}
}
