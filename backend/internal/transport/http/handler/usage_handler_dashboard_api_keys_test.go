package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDashboardAPIKeyUsageRange(t *testing.T) {
	start, end, err := dashboardAPIKeyUsageRange(BatchAPIKeysUsageRequest{
		StartDate: "2026-07-01",
		EndDate:   "2026-07-07",
		Timezone:  "America/New_York",
	})
	require.NoError(t, err)
	require.Equal(t, "2026-07-01T00:00:00-04:00", start.Format(time.RFC3339))
	require.Equal(t, "2026-07-08T00:00:00-04:00", end.Format(time.RFC3339))
}

func TestDashboardAPIKeyUsageRangeValidation(t *testing.T) {
	tests := []struct {
		name string
		req  BatchAPIKeysUsageRequest
	}{
		{name: "missing end", req: BatchAPIKeysUsageRequest{StartDate: "2026-07-01"}},
		{name: "invalid date", req: BatchAPIKeysUsageRequest{StartDate: "July 1", EndDate: "2026-07-02"}},
		{name: "reversed", req: BatchAPIKeysUsageRequest{StartDate: "2026-07-03", EndDate: "2026-07-02"}},
		{name: "too large", req: BatchAPIKeysUsageRequest{StartDate: "2025-01-01", EndDate: "2026-07-02"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := dashboardAPIKeyUsageRange(tt.req)
			require.Error(t, err)
		})
	}
}

func TestDashboardAPIKeyUsageRangeLegacyDefault(t *testing.T) {
	start, end, err := dashboardAPIKeyUsageRange(BatchAPIKeysUsageRequest{})
	require.NoError(t, err)
	require.True(t, start.IsZero())
	require.True(t, end.IsZero())
}

func TestDashboardAPIKeyUsageRangeRejectsInvalidTimezone(t *testing.T) {
	_, _, err := dashboardAPIKeyUsageRange(BatchAPIKeysUsageRequest{
		StartDate: "2026-07-01", EndDate: "2026-07-02", Timezone: "Not/A_Timezone",
	})
	require.EqualError(t, err, `invalid timezone "Not/A_Timezone"`)
}

func TestDashboardAPIKeyUsageRangeAcrossDSTBoundary(t *testing.T) {
	start, end, err := dashboardAPIKeyUsageRange(BatchAPIKeysUsageRequest{
		StartDate: "2026-03-07", EndDate: "2026-03-08", Timezone: "America/New_York",
	})
	require.NoError(t, err)
	require.Equal(t, "2026-03-07T00:00:00-05:00", start.Format(time.RFC3339))
	require.Equal(t, "2026-03-09T00:00:00-04:00", end.Format(time.RFC3339))
	require.Equal(t, 47*time.Hour, end.Sub(start))
}
