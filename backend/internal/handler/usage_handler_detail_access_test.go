package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type usageDetailRepoStub struct {
	service.UsageLogRepository
	record *service.UsageLog
	calls  int
}

func (s *usageDetailRepoStub) GetByID(context.Context, int64) (*service.UsageLog, error) {
	s.calls++
	return s.record, nil
}

type usageDetailSettingRepoStub struct {
	service.SettingRepository
	value string
	err   error
}

func (s *usageDetailSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return map[string]string{service.SettingKeyAllowUserViewUsageDetails: s.value}, nil
}

func newUsageDetailTestRouter(usageRepo *usageDetailRepoStub, settingService *service.SettingService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewUsageHandler(service.NewUsageService(usageRepo, nil, nil, nil), nil, nil, settingService)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
		c.Next()
	})
	router.GET("/usage/:id", h.GetByID)
	return router
}

func TestUsageDetailAccessDefaultsToDisabledAndFailsClosed(t *testing.T) {
	tests := []struct {
		name           string
		settingService *service.SettingService
	}{
		{name: "setting service unavailable"},
		{
			name: "setting missing",
			settingService: service.NewSettingService(
				&usageDetailSettingRepoStub{},
				&config.Config{},
			),
		},
		{
			name: "setting read failure",
			settingService: service.NewSettingService(
				&usageDetailSettingRepoStub{err: errors.New("database unavailable")},
				&config.Config{},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usageRepo := &usageDetailRepoStub{record: &service.UsageLog{ID: 7, UserID: 42}}
			router := newUsageDetailTestRouter(usageRepo, tt.settingService)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/usage/7", nil))

			require.Equal(t, http.StatusForbidden, rec.Code)
			require.Zero(t, usageRepo.calls, "disabled detail access must not query the usage record")
		})
	}
}

func TestUsageDetailAccessAllowsOwnerWhenExplicitlyEnabled(t *testing.T) {
	usageRepo := &usageDetailRepoStub{record: &service.UsageLog{ID: 7, UserID: 42, RequestID: "req_detail"}}
	settingService := service.NewSettingService(
		&usageDetailSettingRepoStub{value: "true"},
		&config.Config{},
	)
	router := newUsageDetailTestRouter(usageRepo, settingService)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/usage/7", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, usageRepo.calls)
	require.Contains(t, rec.Body.String(), `"request_id":"req_detail"`)
}

func TestUsageDetailAccessStillRejectsAnotherUsersRecordWhenEnabled(t *testing.T) {
	usageRepo := &usageDetailRepoStub{record: &service.UsageLog{ID: 7, UserID: 99}}
	settingService := service.NewSettingService(
		&usageDetailSettingRepoStub{value: "true"},
		&config.Config{},
	)
	router := newUsageDetailTestRouter(usageRepo, settingService)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/usage/7", nil))

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, 1, usageRepo.calls)
}
