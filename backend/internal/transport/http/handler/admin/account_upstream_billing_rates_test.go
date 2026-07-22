package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type upstreamBillingRatesAdminService struct {
	service.AdminService
	items   []service.UpstreamBillingRateSnapshotItem
	total   int64
	filters service.UpstreamBillingRateListFilters
}

func (s *upstreamBillingRatesAdminService) ListUpstreamBillingRateSnapshots(
	_ context.Context,
	_, _ int,
	filters service.UpstreamBillingRateListFilters,
	_, _ string,
) ([]service.UpstreamBillingRateSnapshotItem, int64, error) {
	s.filters = filters
	return s.items, s.total, nil
}

func setupUpstreamBillingRatesRouter(adminService service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	handler := NewAccountHandler(adminService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/admin/accounts/upstream-billing-rates", handler.GetUpstreamBillingRates)
	return router
}

func TestAccountHandlerGetUpstreamBillingRatesReturnsCompactPayloadAndETag(t *testing.T) {
	now := time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC)
	snapshotService := &upstreamBillingRatesAdminService{
		total: 2,
		items: []service.UpstreamBillingRateSnapshotItem{
			{AccountID: 9, Snapshot: &service.UpstreamBillingProbeSnapshot{
				Status: service.UpstreamBillingProbeStatusOK, LastAttemptAt: now, NextProbeAt: now.Add(time.Hour),
			}},
			{AccountID: 4},
		},
	}
	router := setupUpstreamBillingRatesRouter(snapshotService)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/accounts/upstream-billing-rates?page=1&page_size=20&platform=openai&type=apikey&sort_by=upstream_billing_rate&sort_order=desc", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, service.PlatformOpenAI, snapshotService.filters.Platform)
	require.Equal(t, service.AccountTypeAPIKey, snapshotService.filters.AccountType)
	etag := recorder.Header().Get("ETag")
	require.NotEmpty(t, etag)
	var envelope struct {
		Data upstreamBillingRatesResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, int64(2), envelope.Data.Total)
	require.Equal(t, []int64{9, 4}, []int64{envelope.Data.Items[0].AccountID, envelope.Data.Items[1].AccountID})
	require.NotContains(t, recorder.Body.String(), "credentials")

	notModified := httptest.NewRecorder()
	notModifiedRequest := httptest.NewRequest(http.MethodGet, request.URL.String(), nil)
	notModifiedRequest.Header.Set("If-None-Match", etag)
	router.ServeHTTP(notModified, notModifiedRequest)
	require.Equal(t, http.StatusNotModified, notModified.Code)
}

func TestAccountHandlerGetUpstreamBillingRatesRejectsInvalidGroup(t *testing.T) {
	router := setupUpstreamBillingRatesRouter(&upstreamBillingRatesAdminService{})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/accounts/upstream-billing-rates?group=not-a-number", nil))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestAccountHandlerGetUpstreamBillingRatesRequiresProjectionService(t *testing.T) {
	router := setupUpstreamBillingRatesRouter(&stubAdminService{})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/accounts/upstream-billing-rates", nil))
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}
