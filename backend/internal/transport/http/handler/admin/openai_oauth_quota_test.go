//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIQuotaWorkflowStub struct {
	resetResult *service.OpenAIQuotaResetResult
	resetErr    error
	queryResult *service.OpenAIQuotaUsage
	queryErr    error
	cacheErr    error
	resetCalls  int
	queryCalls  int
	cacheCalls  int
	queryCtxErr error
	cacheCtxErr error
}

func (s *openAIQuotaWorkflowStub) ResetCredit(context.Context, int64) (*service.OpenAIQuotaResetResult, error) {
	s.resetCalls++
	return s.resetResult, s.resetErr
}

func (s *openAIQuotaWorkflowStub) QueryUsage(ctx context.Context, _ int64) (*service.OpenAIQuotaUsage, error) {
	s.queryCalls++
	s.queryCtxErr = ctx.Err()
	return s.queryResult, s.queryErr
}

func (s *openAIQuotaWorkflowStub) QueryUsageWithServerUsage(ctx context.Context, accountID int64) (*service.OpenAIQuotaUsage, error) {
	return s.QueryUsage(ctx, accountID)
}

func (s *openAIQuotaWorkflowStub) CacheResetCreditsSnapshot(ctx context.Context, _ int64, _ *service.OpenAIRateLimitResetCredits) error {
	s.cacheCalls++
	s.cacheCtxErr = ctx.Err()
	return s.cacheErr
}

type openAIAccountStateRecovererStub struct {
	err         error
	calls       int
	accountID   int64
	lastOptions service.AccountRecoveryOptions
	lastCtxErr  error
}

func (s *openAIAccountStateRecovererStub) RecoverAccountState(ctx context.Context, accountID int64, options service.AccountRecoveryOptions) (*service.SuccessfulTestRecoveryResult, error) {
	s.calls++
	s.accountID = accountID
	s.lastOptions = options
	s.lastCtxErr = ctx.Err()
	return &service.SuccessfulTestRecoveryResult{}, s.err
}

type openAIResetAdminServiceStub struct {
	service.AdminService
	account *service.Account
	err     error
	calls   int
}

func (s *openAIResetAdminServiceStub) GetAccount(context.Context, int64) (*service.Account, error) {
	s.calls++
	return s.account, s.err
}

type openAIQuotaResetEnvelope struct {
	Code int                      `json:"code"`
	Data openAIQuotaResetResponse `json:"data"`
}

type openAIQuotaRefreshEnvelope struct {
	Code int                        `json:"code"`
	Data openAIQuotaRefreshResponse `json:"data"`
}

func performOpenAIQuotaResetRequest(t *testing.T, handler *OpenAIOAuthHandler, ctx context.Context) (int, openAIQuotaResetEnvelope) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/admin/openai/accounts/:id/reset-quota", handler.ResetQuota)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/accounts/42/reset-quota", nil)
	if ctx != nil {
		request = request.WithContext(ctx)
	}
	router.ServeHTTP(recorder, request)

	var envelope openAIQuotaResetEnvelope
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	return recorder.Code, envelope
}

func performOpenAIQuotaRefreshRequest(t *testing.T, handler *OpenAIOAuthHandler) (int, openAIQuotaRefreshEnvelope) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/admin/openai/accounts/:id/quota/refresh", handler.RefreshQuota)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/accounts/42/quota/refresh", nil)
	router.ServeHTTP(recorder, request)

	var envelope openAIQuotaRefreshEnvelope
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	return recorder.Code, envelope
}

func successfulOpenAIQuotaWorkflowStub() *openAIQuotaWorkflowStub {
	return &openAIQuotaWorkflowStub{
		resetResult: &service.OpenAIQuotaResetResult{Code: "success", WindowsReset: 1},
		queryResult: &service.OpenAIQuotaUsage{
			FetchedAt: 123,
			RateLimitResetCredits: &service.OpenAIRateLimitResetCredits{
				AvailableCount: 0,
				Credits:        []service.OpenAIRateLimitResetCreditDetail{},
			},
		},
	}
}

func recoveredOpenAIAccountStub() *openAIResetAdminServiceStub {
	return &openAIResetAdminServiceStub{account: &service.Account{
		ID:          42,
		Name:        "recovered",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: false,
	}}
}

func TestOpenAIResetQuotaRecoversStateAndReturnsFreshData(t *testing.T) {
	quota := successfulOpenAIQuotaWorkflowStub()
	recoverer := &openAIAccountStateRecovererStub{}
	adminService := recoveredOpenAIAccountStub()
	handler := &OpenAIOAuthHandler{
		adminService:     adminService,
		quotaService:     quota,
		rateLimitService: recoverer,
	}

	status, envelope := performOpenAIQuotaResetRequest(t, handler, nil)

	require.Equal(t, http.StatusOK, status)
	require.Empty(t, envelope.Data.WarningCode)
	require.True(t, envelope.Data.AccountStateRecovered)
	require.True(t, envelope.Data.CacheRefreshed)
	require.NotNil(t, envelope.Data.Quota)
	require.NotNil(t, envelope.Data.Account)
	require.False(t, envelope.Data.Account.Schedulable)
	require.Equal(t, int64(42), recoverer.accountID)
	require.True(t, recoverer.lastOptions.InvalidateToken)
	require.Equal(t, 1, quota.resetCalls)
	require.Equal(t, 1, quota.queryCalls)
	require.Equal(t, 1, quota.cacheCalls)
	require.Equal(t, 1, adminService.calls)
}

func TestOpenAIResetQuotaRecoveryFailureStopsPostProcessing(t *testing.T) {
	quota := successfulOpenAIQuotaWorkflowStub()
	recoverer := &openAIAccountStateRecovererStub{err: errors.New("recovery failed")}
	handler := &OpenAIOAuthHandler{
		adminService:     recoveredOpenAIAccountStub(),
		quotaService:     quota,
		rateLimitService: recoverer,
	}

	status, envelope := performOpenAIQuotaResetRequest(t, handler, nil)

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, openAIQuotaResetWarningAccountRecoveryFailed, envelope.Data.WarningCode)
	require.False(t, envelope.Data.AccountStateRecovered)
	require.Zero(t, quota.queryCalls)
	require.Zero(t, quota.cacheCalls)
}

func TestOpenAIResetQuotaCacheFailureStillReturnsRecoveredAccount(t *testing.T) {
	quota := successfulOpenAIQuotaWorkflowStub()
	quota.cacheErr = errors.New("cache write failed")
	handler := &OpenAIOAuthHandler{
		adminService:     recoveredOpenAIAccountStub(),
		quotaService:     quota,
		rateLimitService: &openAIAccountStateRecovererStub{},
	}

	status, envelope := performOpenAIQuotaResetRequest(t, handler, nil)

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, openAIQuotaResetWarningCacheRefreshFailed, envelope.Data.WarningCode)
	require.True(t, envelope.Data.AccountStateRecovered)
	require.False(t, envelope.Data.CacheRefreshed)
	require.Nil(t, envelope.Data.Quota)
	require.NotNil(t, envelope.Data.Account)
}

func TestOpenAIResetQuotaPostProcessingSurvivesClientCancellation(t *testing.T) {
	quota := successfulOpenAIQuotaWorkflowStub()
	recoverer := &openAIAccountStateRecovererStub{}
	handler := &OpenAIOAuthHandler{
		adminService:     recoveredOpenAIAccountStub(),
		quotaService:     quota,
		rateLimitService: recoverer,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	status, _ := performOpenAIQuotaResetRequest(t, handler, ctx)

	require.Equal(t, http.StatusOK, status)
	require.NoError(t, recoverer.lastCtxErr)
	require.NoError(t, quota.queryCtxErr)
	require.NoError(t, quota.cacheCtxErr)
}

func TestOpenAIRefreshQuotaPersistsSnapshotWithoutHidingReadFailures(t *testing.T) {
	t.Run("persisted", func(t *testing.T) {
		quota := successfulOpenAIQuotaWorkflowStub()
		quota.queryResult.RateLimitsByLimitID = map[string]service.OpenAIAppServerRateLimitBucket{
			"codex_other": {
				LimitID: "codex_other",
				Primary: &service.OpenAIAppServerRateLimitWindow{
					UsedPercent:        42,
					WindowDurationMins: 60,
					ResetsAt:           1730950800,
				},
			},
		}
		quota.queryResult.ServerTokenUsage = &service.OpenAIServerTokenUsage{
			Summary: service.OpenAITokenUsageSummary{LifetimeTokens: ptrInt64(1234)},
		}
		handler := &OpenAIOAuthHandler{quotaService: quota}

		status, envelope := performOpenAIQuotaRefreshRequest(t, handler)

		require.Equal(t, http.StatusOK, status)
		require.True(t, envelope.Data.CachePersisted)
		require.Equal(t, int64(123), envelope.Data.FetchedAt)
		require.Equal(t, float64(42), envelope.Data.RateLimitsByLimitID["codex_other"].Primary.UsedPercent)
		require.Equal(t, int64(1234), *envelope.Data.ServerTokenUsage.Summary.LifetimeTokens)
	})

	t.Run("persist failure is partial success", func(t *testing.T) {
		quota := successfulOpenAIQuotaWorkflowStub()
		quota.cacheErr = errors.New("expiration details unavailable")
		quota.queryResult.FetchedAt = 456
		handler := &OpenAIOAuthHandler{quotaService: quota}

		status, envelope := performOpenAIQuotaRefreshRequest(t, handler)

		require.Equal(t, http.StatusOK, status)
		require.False(t, envelope.Data.CachePersisted)
		require.Equal(t, int64(456), envelope.Data.FetchedAt)
	})

	t.Run("empty result is rejected", func(t *testing.T) {
		quota := successfulOpenAIQuotaWorkflowStub()
		quota.queryResult = nil
		handler := &OpenAIOAuthHandler{quotaService: quota}

		status, _ := performOpenAIQuotaRefreshRequest(t, handler)

		require.Equal(t, http.StatusInternalServerError, status)
		require.Zero(t, quota.cacheCalls)
	})
}

func TestNewOpenAIOAuthHandlerKeepsNilQuotaCapabilitiesGuarded(t *testing.T) {
	handler := NewOpenAIOAuthHandler(nil, newStubAdminService(), nil, nil)
	require.Nil(t, handler.quotaService)
	require.Nil(t, handler.rateLimitService)
}
