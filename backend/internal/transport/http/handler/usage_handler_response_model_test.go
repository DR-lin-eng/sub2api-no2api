package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/pagination"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/usagestats"
	middleware2 "github.com/DR-lin-eng/sub2api-no2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type responseModelChannelRepoStub struct {
	service.ChannelRepository
	channels []service.Channel
}

func (s *responseModelChannelRepoStub) ListAll(context.Context) ([]service.Channel, error) {
	return s.channels, nil
}

func (s *responseModelChannelRepoStub) GetGroupIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}

func (s *responseModelChannelRepoStub) GetGroupPlatforms(context.Context, []int64) (map[int64]string, error) {
	return map[int64]string{}, nil
}

func (s *responseModelChannelRepoStub) ListModelPricing(context.Context, int64) ([]service.ChannelModelPricing, error) {
	return nil, nil
}

type responseModelUsageRepoStub struct {
	service.UsageLogRepository
	logs []service.UsageLog
}

func (s *responseModelUsageRepoStub) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _ usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return s.logs, &pagination.PaginationResult{Total: int64(len(s.logs)), Page: 1, PageSize: 20}, nil
}

func (s *responseModelUsageRepoStub) GetByID(_ context.Context, id int64) (*service.UsageLog, error) {
	for i := range s.logs {
		if s.logs[i].ID == id {
			cp := s.logs[i]
			return &cp, nil
		}
	}
	return nil, service.ErrUsageLogNotFound
}

func TestUsageHandler_ExposesResponseModelOnlyForOptedInActiveChannels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	chExposed := int64(10)
	chHidden := int64(20)
	chInactive := int64(30)
	respModelA := "claude-sonnet-4-5-20250929"
	respModelB := "gpt-5.4-2026-11-01"

	usageRepo := &responseModelUsageRepoStub{
		logs: []service.UsageLog{
			{ID: 1, UserID: 42, APIKeyID: 7, Model: "claude-sonnet-4-5", RequestedModel: "claude-sonnet-4-5", UpstreamResponseModel: &respModelA, ChannelID: &chExposed, CreatedAt: time.Now()},
			{ID: 2, UserID: 42, APIKeyID: 7, Model: "gpt-5.4", RequestedModel: "gpt-5.4", UpstreamResponseModel: &respModelB, ChannelID: &chHidden, CreatedAt: time.Now()},
			{ID: 3, UserID: 42, APIKeyID: 7, Model: "gpt-5.4", RequestedModel: "gpt-5.4", UpstreamResponseModel: &respModelB, ChannelID: &chInactive, CreatedAt: time.Now()},
			{ID: 4, UserID: 42, APIKeyID: 7, Model: "gpt-5.4", RequestedModel: "gpt-5.4", UpstreamResponseModel: &respModelB, CreatedAt: time.Now()},
		},
	}
	channelRepo := &responseModelChannelRepoStub{
		channels: []service.Channel{
			{ID: chExposed, Name: "exposed", Status: service.StatusActive, FeaturesConfig: map[string]any{"expose_response_model": true}},
			{ID: chHidden, Name: "hidden", Status: service.StatusActive, FeaturesConfig: map[string]any{"expose_response_model": false}},
			{ID: chInactive, Name: "inactive", Status: "inactive", FeaturesConfig: map[string]any{"expose_response_model": true}},
		},
	}
	settingService := service.NewSettingService(&usageDetailSettingRepoStub{value: "true"}, nil)
	channelService := service.NewChannelService(channelRepo, nil, nil, nil)
	h := NewUsageHandler(service.NewUsageService(usageRepo, nil, nil, nil), nil, nil, settingService, channelService)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
		c.Next()
	})
	router.GET("/usage", h.List)
	router.GET("/usage/:id", h.GetByID)

	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/usage", nil))
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp struct {
		Data struct {
			Items []struct {
				ID            int64   `json:"id"`
				ResponseModel *string `json:"response_model"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	require.Len(t, listResp.Data.Items, 4)
	require.NotNil(t, listResp.Data.Items[0].ResponseModel)
	require.Equal(t, respModelA, *listResp.Data.Items[0].ResponseModel)
	require.Nil(t, listResp.Data.Items[1].ResponseModel)
	require.Nil(t, listResp.Data.Items[2].ResponseModel)
	require.Nil(t, listResp.Data.Items[3].ResponseModel)

	detailExposed := httptest.NewRecorder()
	router.ServeHTTP(detailExposed, httptest.NewRequest(http.MethodGet, "/usage/1", nil))
	require.Equal(t, http.StatusOK, detailExposed.Code)
	require.Contains(t, detailExposed.Body.String(), `"response_model":"claude-sonnet-4-5-20250929"`)

	detailHidden := httptest.NewRecorder()
	router.ServeHTTP(detailHidden, httptest.NewRequest(http.MethodGet, "/usage/2", nil))
	require.Equal(t, http.StatusOK, detailHidden.Code)
	require.NotContains(t, detailHidden.Body.String(), `"response_model"`)
}
