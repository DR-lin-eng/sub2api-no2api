//go:build unit

package repository

import (
	"testing"

	dbent "github.com/DR-lin-eng/sub2api-no2api/ent"
	"github.com/stretchr/testify/require"
)

func TestGroupEntityToServiceCorruptModelPricingFallsBackSafely(t *testing.T) {
	group := groupEntityToService(&dbent.Group{
		ID:                        42,
		Name:                      "legacy-corrupt-pricing",
		LongContextPricingEnabled: true,
		ModelPricing:              []byte(`{"not":"an array"}`),
	})

	require.NotNil(t, group)
	require.Empty(t, group.ModelPricing)
	require.True(t, group.LongContextPricingEnabled)
}
