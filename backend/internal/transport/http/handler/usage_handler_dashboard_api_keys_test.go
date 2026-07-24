package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/usagestats"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dashboardAPIKeyUsageRepoStub struct {
	service.UsageLogRepository
	stats map[int64]*usagestats.BatchAPIKeyUsageStats
}

func (s *dashboardAPIKeyUsageRepoStub) GetBatchAPIKeyUsageStats(context.Context, []int64, time.Time, time.Time) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	return s.stats, nil
}

type dashboardAPIKeyOwnershipRepoStub struct {
	service.APIKeyRepository
}

func (s *dashboardAPIKeyOwnershipRepoStub) VerifyOwnership(_ context.Context, _ int64, apiKeyIDs []int64) ([]int64, error) {
	return apiKeyIDs, nil
}

type dashboardPendingUsageReaderStub struct {
	costs map[int64]float64
	err   error
}

func (s *dashboardPendingUsageReaderStub) GetPendingAPIKeyUsageCosts(context.Context, []int64) (map[int64]float64, error) {
	return s.costs, s.err
}

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

func TestDashboardAPIKeysUsageExposesPendingCosts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	usageSvc := service.NewUsageService(&dashboardAPIKeyUsageRepoStub{stats: map[int64]*usagestats.BatchAPIKeyUsageStats{
		7: {APIKeyID: 7, TodayActualCost: 1, TotalActualCost: 2},
	}}, nil, nil, nil)
	apiKeySvc := service.NewAPIKeyService(&dashboardAPIKeyOwnershipRepoStub{}, nil, nil, nil, nil, nil, nil)
	apiKeySvc.SetPendingUsageReader(&dashboardPendingUsageReaderStub{costs: map[int64]float64{7: 0.75}})
	handler := NewUsageHandler(usageSvc, apiKeySvc, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
		c.Next()
	})
	router.POST("/usage/dashboard/api-keys-usage", handler.DashboardAPIKeysUsage)

	req := httptest.NewRequest(http.MethodPost, "/usage/dashboard/api-keys-usage", bytes.NewBufferString(`{"api_key_ids":[7]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data struct {
			Stats                 map[string]usagestats.BatchAPIKeyUsageStats `json:"stats"`
			PendingUsageAvailable bool                                        `json:"pending_usage_available"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.True(t, body.Data.PendingUsageAvailable)
	require.InDelta(t, 0.75, body.Data.Stats["7"].PendingActualCost, 1e-9)
	require.InDelta(t, 1, body.Data.Stats["7"].TodayActualCost, 1e-9)
}
