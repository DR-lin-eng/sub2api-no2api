package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type batchQuotaRepoStub struct {
	service.UserPlatformQuotaRepository
	requested []int64
}

func (s *batchQuotaRepoStub) ListByUsers(_ context.Context, userIDs []int64) (map[int64][]service.UserPlatformQuotaRecord, error) {
	s.requested = append([]int64(nil), userIDs...)
	return map[int64][]service.UserPlatformQuotaRecord{
		7: {{UserID: 7, Platform: service.PlatformOpenAI, DailyUsageUSD: 1.25}},
	}, nil
}

func TestUserHandlerGetBatchPlatformQuotasDeduplicatesIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &batchQuotaRepoStub{}
	handler := NewUserHandler(newStubAdminService(), nil, repo, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/admin/users/platform-quotas/batch", handler.GetBatchUserPlatformQuotas)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/users/platform-quotas/batch", bytes.NewBufferString(`{"user_ids":[7,8,7]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []int64{7, 8}, repo.requested)
	var payload struct {
		Data struct {
			PlatformQuotas map[int64][]map[string]any `json:"platform_quotas"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Len(t, payload.Data.PlatformQuotas[7], 1)
	require.Empty(t, payload.Data.PlatformQuotas[8])
}

func TestAccountHandlerGetBatchSummariesDeduplicatesIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAccountHandler(newStubAdminService(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/admin/accounts/summaries/batch", handler.GetBatchSummaries)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/accounts/summaries/batch", bytes.NewBufferString(`{"account_ids":[3,4,3]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Data struct {
			Items []accountSummaryResponse `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, []accountSummaryResponse{{ID: 3, Name: "account"}, {ID: 4, Name: "account"}}, payload.Data.Items)
}
