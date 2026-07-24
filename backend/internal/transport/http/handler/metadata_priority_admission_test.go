package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type metadataPriorityCache struct {
	helperConcurrencyCacheStub

	eventMu         sync.Mutex
	events          []string
	userStatus      service.PriorityAccountAdmissionStatus
	accountStatuses []service.PriorityAccountAdmissionStatus
	userErr         error
	accountErr      error
}

func (s *metadataPriorityCache) AcquirePriorityUserSlot(_ context.Context, request service.PriorityUserAdmissionRequest) (service.PriorityAccountAdmissionStatus, error) {
	s.recordEvent("user:acquire")
	if s.userErr != nil {
		return service.PriorityAccountAdmissionRejected, s.userErr
	}
	return s.userStatus, nil
}

func (s *metadataPriorityCache) AcquirePriorityAccountSlot(_ context.Context, request service.PriorityAccountAdmissionRequest) (service.PriorityAccountAdmissionStatus, error) {
	s.recordEvent(fmt.Sprintf("account:%d:acquire", request.AccountID))
	if s.accountErr != nil {
		return service.PriorityAccountAdmissionRejected, s.accountErr
	}
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	if len(s.accountStatuses) == 0 {
		return service.PriorityAccountAdmissionRejected, nil
	}
	status := s.accountStatuses[0]
	s.accountStatuses = s.accountStatuses[1:]
	return status, nil
}

func (s *metadataPriorityCache) CancelPriorityUserWait(_ context.Context, _ int64, _ string) error {
	s.recordEvent("user:cancel")
	return nil
}

func (s *metadataPriorityCache) CancelPriorityAccountWait(_ context.Context, accountID int64, _ string) error {
	s.recordEvent(fmt.Sprintf("account:%d:cancel", accountID))
	return nil
}

func (s *metadataPriorityCache) ReleaseUserSlot(ctx context.Context, userID int64, requestID string) error {
	s.recordEvent("user:release")
	return s.helperConcurrencyCacheStub.ReleaseUserSlot(ctx, userID, requestID)
}

func (s *metadataPriorityCache) ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error {
	s.recordEvent(fmt.Sprintf("account:%d:release", accountID))
	return s.helperConcurrencyCacheStub.ReleaseAccountSlot(ctx, accountID, requestID)
}

func (s *metadataPriorityCache) recordEvent(event string) {
	s.eventMu.Lock()
	s.events = append(s.events, event)
	s.eventMu.Unlock()
}

func (s *metadataPriorityCache) eventSnapshot() []string {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	return append([]string(nil), s.events...)
}

func newMetadataConcurrency(cache *metadataPriorityCache, enabled bool) *ConcurrencyHelper {
	concurrencyService := service.NewConcurrencyService(cache)
	concurrencyService.SetPriorityAdmissionRuntimeConfig(service.PriorityAdmissionRuntimeConfig{
		Enabled:                 enabled,
		PendingLimitPerInstance: 256,
		PendingBytesPerInstance: 256 << 20,
	})
	return NewConcurrencyHelper(concurrencyService, SSEPingFormatNone, time.Millisecond)
}

func performCodexMetadataRequest(
	t *testing.T,
	handler *OpenAIGatewayHandler,
	groupID int64,
	tier service.RequestSchedulingTier,
	includeSubject bool,
) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/backend-api/codex/models?client_version=0.144.0", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI},
	})
	if includeSubject {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{
			UserID:         91,
			Concurrency:    1,
			SchedulingTier: tier,
		})
	}

	handler.CodexModels(c)
	return recorder
}

func TestCodexModelsPriorityAdmissionDisabledPreservesLegacyPath(t *testing.T) {
	handler, upstream, groupID := newCodexModelsFailoverTestHandler(http.StatusOK)
	upstream.firstBody = `{"models":[{"slug":"gpt-5.6-sol"}]}`
	cache := &metadataPriorityCache{userStatus: service.PriorityAccountAdmissionAcquired}
	handler.concurrencyHelper = newMetadataConcurrency(cache, false)

	recorder := performCodexMetadataRequest(t, handler, groupID, service.RequestSchedulingTierLow, false)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []int64{1}, upstream.calls())
	require.Empty(t, cache.eventSnapshot())
	require.Zero(t, cache.userAcquireCalls)
	require.Zero(t, cache.accountAcquireCalls)
}

func TestMetadataPriorityAdmissionCapturesEnabledRequestSnapshot(t *testing.T) {
	cache := &metadataPriorityCache{userStatus: service.PriorityAccountAdmissionAcquired}
	helper := newMetadataConcurrency(cache, true)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/backend-api/codex/models", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{
		UserID:         91,
		Concurrency:    1,
		SchedulingTier: service.RequestSchedulingTierPriority,
	})

	require.True(t, metadataPriorityAdmissionEnabled(c, helper))
	helper.concurrencyService.SetPriorityAdmissionRuntimeConfig(service.PriorityAdmissionRuntimeConfig{Enabled: false})
	require.True(t, helper.concurrencyService.PriorityAdmissionEnabledForRequest(c.Request.Context()))
}

func TestCodexModelsPriorityAdmissionReleasesAccountBeforeFailover(t *testing.T) {
	handler, upstream, groupID := newCodexModelsFailoverTestHandler(http.StatusServiceUnavailable)
	cache := &metadataPriorityCache{
		userStatus: service.PriorityAccountAdmissionAcquired,
		accountStatuses: []service.PriorityAccountAdmissionStatus{
			service.PriorityAccountAdmissionAcquired,
			service.PriorityAccountAdmissionAcquired,
		},
	}
	handler.concurrencyHelper = newMetadataConcurrency(cache, true)

	recorder := performCodexMetadataRequest(t, handler, groupID, service.RequestSchedulingTierPriority, true)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []int64{1, 2}, upstream.calls())
	require.Equal(t, []string{
		"user:acquire",
		"account:1:acquire",
		"account:1:release",
		"account:2:acquire",
		"account:2:release",
		"user:release",
	}, cache.eventSnapshot())
}

func TestCodexModelsLowTierBusyAccountReturnsGeneric429(t *testing.T) {
	handler, upstream, groupID := newCodexModelsFailoverTestHandler(http.StatusOK)
	cache := &metadataPriorityCache{
		userStatus:      service.PriorityAccountAdmissionAcquired,
		accountStatuses: []service.PriorityAccountAdmissionStatus{service.PriorityAccountAdmissionRejected},
	}
	handler.concurrencyHelper = newMetadataConcurrency(cache, true)

	recorder := performCodexMetadataRequest(t, handler, groupID, service.RequestSchedulingTierLow, true)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Empty(t, upstream.calls())
	require.Contains(t, recorder.Body.String(), `"type":"rate_limit_error"`)
	require.Contains(t, recorder.Body.String(), "Too many pending requests")
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "tier")
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "low")
}

func TestCodexModelsPriorityAdmissionRedisFailureReturns503(t *testing.T) {
	handler, upstream, groupID := newCodexModelsFailoverTestHandler(http.StatusOK)
	cache := &metadataPriorityCache{userErr: errors.New("redis unavailable")}
	handler.concurrencyHelper = newMetadataConcurrency(cache, true)

	recorder := performCodexMetadataRequest(t, handler, groupID, service.RequestSchedulingTierNormal, true)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Empty(t, upstream.calls())
	require.Contains(t, recorder.Body.String(), "Service temporarily unavailable")
}

type geminiMetadataAccountRepo struct {
	service.AccountRepository
	account service.Account
}

func (r geminiMetadataAccountRepo) ListSchedulableByPlatforms(_ context.Context, _ []string) ([]service.Account, error) {
	return []service.Account{r.account}, nil
}

type geminiMetadataHTTPUpstream struct {
	service.HTTPUpstream
	mu    sync.Mutex
	paths []string
}

func (u *geminiMetadataHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.paths = append(u.paths, req.URL.Path)
	u.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"models":[]}`)),
	}, nil
}

func (u *geminiMetadataHTTPUpstream) pathSnapshot() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.paths...)
}

func newGeminiMetadataHandler(cache *metadataPriorityCache, enabled bool) (*GatewayHandler, *geminiMetadataHTTPUpstream) {
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.Scheduling.FallbackWaitTimeout = time.Second
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 4
	account := service.Account{
		ID:          73,
		Name:        "gemini-metadata",
		Platform:    service.PlatformGemini,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "gemini-key",
			"base_url": "https://generativelanguage.googleapis.com",
		},
	}
	upstream := &geminiMetadataHTTPUpstream{}
	compatService := service.NewGeminiMessagesCompatService(
		geminiMetadataAccountRepo{account: account},
		nil,
		nil,
		nil,
		nil,
		nil,
		upstream,
		nil,
		cfg,
	)
	return &GatewayHandler{
		geminiCompatService: compatService,
		concurrencyHelper:   newMetadataConcurrency(cache, enabled),
		cfg:                 cfg,
	}, upstream
}

func performGeminiMetadataRequest(
	t *testing.T,
	handler *GatewayHandler,
	path string,
	model string,
	tier service.RequestSchedulingTier,
	includeSubject bool,
) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{Platform: service.PlatformGemini},
	})
	if includeSubject {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{
			UserID:         92,
			Concurrency:    1,
			SchedulingTier: tier,
		})
	}
	if model != "" {
		c.Params = gin.Params{{Key: "model", Value: model}}
		handler.GeminiV1BetaGetModel(c)
	} else {
		handler.GeminiV1BetaListModels(c)
	}
	return recorder
}

func TestGeminiMetadataPriorityAdmissionDisabledPreservesLegacyPath(t *testing.T) {
	cache := &metadataPriorityCache{userStatus: service.PriorityAccountAdmissionAcquired}
	handler, upstream := newGeminiMetadataHandler(cache, false)

	recorder := performGeminiMetadataRequest(t, handler, "/v1beta/models", "", service.RequestSchedulingTierLow, false)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []string{"/v1beta/models"}, upstream.pathSnapshot())
	require.Empty(t, cache.eventSnapshot())
	require.Zero(t, cache.userAcquireCalls)
	require.Zero(t, cache.accountAcquireCalls)
}

func TestGeminiMetadataPriorityAdmissionAcquiresUserThenAccount(t *testing.T) {
	cache := &metadataPriorityCache{
		userStatus:      service.PriorityAccountAdmissionAcquired,
		accountStatuses: []service.PriorityAccountAdmissionStatus{service.PriorityAccountAdmissionAcquired},
	}
	handler, upstream := newGeminiMetadataHandler(cache, true)

	recorder := performGeminiMetadataRequest(t, handler, "/v1beta/models", "", service.RequestSchedulingTierNormal, true)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []string{"/v1beta/models"}, upstream.pathSnapshot())
	require.Equal(t, []string{
		"user:acquire",
		"account:73:acquire",
		"account:73:release",
		"user:release",
	}, cache.eventSnapshot())
}

func TestGeminiMetadataLowTierBusyAccountReturnsGoogle429(t *testing.T) {
	cache := &metadataPriorityCache{
		userStatus:      service.PriorityAccountAdmissionAcquired,
		accountStatuses: []service.PriorityAccountAdmissionStatus{service.PriorityAccountAdmissionRejected},
	}
	handler, upstream := newGeminiMetadataHandler(cache, true)

	recorder := performGeminiMetadataRequest(t, handler, "/v1beta/models/gemini-2.5-pro", "gemini-2.5-pro", service.RequestSchedulingTierLow, true)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Empty(t, upstream.pathSnapshot())
	require.Contains(t, recorder.Body.String(), `"code":429`)
	require.Contains(t, recorder.Body.String(), `"status":"RESOURCE_EXHAUSTED"`)
	require.Contains(t, recorder.Body.String(), "Too many pending requests")
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "tier")
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "low")
}

func TestGeminiMetadataPriorityAdmissionRedisFailureReturnsGoogle503(t *testing.T) {
	cache := &metadataPriorityCache{userErr: errors.New("redis unavailable")}
	handler, upstream := newGeminiMetadataHandler(cache, true)

	recorder := performGeminiMetadataRequest(t, handler, "/v1beta/models", "", service.RequestSchedulingTierPriority, true)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Empty(t, upstream.pathSnapshot())
	require.Contains(t, recorder.Body.String(), `"code":503`)
	require.Contains(t, recorder.Body.String(), "Service temporarily unavailable")
}

func TestGeminiNativePriorityAdmissionRedisFailureReturnsGoogle503(t *testing.T) {
	cache := &metadataPriorityCache{userErr: errors.New("redis unavailable")}
	groupID := int64(73)
	handler := &GatewayHandler{
		gatewayService:    &service.GatewayService{},
		concurrencyHelper: newMetadataConcurrency(cache, true),
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", strings.NewReader(`{"contents":[]}`))
	c.Params = gin.Params{{Key: "modelAction", Value: "/gemini-2.5-pro:generateContent"}}
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      81,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, Platform: service.PlatformGemini},
		User:    &service.User{ID: 92},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{
		UserID:         92,
		Concurrency:    1,
		SchedulingTier: service.RequestSchedulingTierNormal,
	})

	handler.GeminiV1BetaModels(c)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"code":503`)
	require.Contains(t, recorder.Body.String(), "Service temporarily unavailable")
}
