package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type countTokensAdmissionCache struct {
	*concurrencyCacheMock
	accountWaitAllowed bool
	accountWaitCalls   int
	priorityStatus     service.PriorityAccountAdmissionStatus
	priorityErr        error
	priorityRequests   []service.PriorityAccountAdmissionRequest
}

func (c *countTokensAdmissionCache) IncrementAccountWaitCount(context.Context, int64, int) (bool, error) {
	c.accountWaitCalls++
	return c.accountWaitAllowed, nil
}

func (c *countTokensAdmissionCache) AcquirePriorityAccountSlot(_ context.Context, request service.PriorityAccountAdmissionRequest) (service.PriorityAccountAdmissionStatus, error) {
	c.priorityRequests = append(c.priorityRequests, request)
	return c.priorityStatus, c.priorityErr
}

func (c *countTokensAdmissionCache) CancelPriorityAccountWait(context.Context, int64, string) error {
	return nil
}

func (c *countTokensAdmissionCache) AcquirePriorityUserSlot(context.Context, service.PriorityUserAdmissionRequest) (service.PriorityAccountAdmissionStatus, error) {
	return service.PriorityAccountAdmissionAcquired, nil
}

func (c *countTokensAdmissionCache) CancelPriorityUserWait(context.Context, int64, string) error {
	return nil
}

func newCountTokensAdmissionContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
	return c
}

func countTokensWaitSelection() *service.AccountSelectionResult {
	return &service.AccountSelectionResult{
		Account: &service.Account{ID: 101},
		WaitPlan: &service.AccountWaitPlan{
			AccountID:      101,
			MaxConcurrency: 2,
			MaxWaiting:     7,
			Timeout:        time.Second,
		},
	}
}

func TestAcquireAccountSelectionSlotUsesAcquiredSelection(t *testing.T) {
	cache := &countTokensAdmissionCache{concurrencyCacheMock: &concurrencyCacheMock{}}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)
	c := newCountTokensAdmissionContext(t)
	releaseCalls := 0
	selection := &service.AccountSelectionResult{
		Account:     &service.Account{ID: 101},
		Acquired:    true,
		ReleaseFunc: func() { releaseCalls++ },
	}
	streamStarted := false

	release, err := acquireAccountSelectionSlot(c, helper, selection, false, &streamStarted, nil)
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
	release()
	require.Equal(t, 1, releaseCalls)
	require.Zero(t, cache.accountWaitCalls)
}

func TestAcquireAccountSelectionSlotFeatureOffRetriesBeforeQueue(t *testing.T) {
	cache := &countTokensAdmissionCache{
		concurrencyCacheMock: &concurrencyCacheMock{
			acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
				return true, nil
			},
		},
		accountWaitAllowed: true,
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)
	c := newCountTokensAdmissionContext(t)
	streamStarted := false

	release, err := acquireAccountSelectionSlot(c, helper, countTokensWaitSelection(), false, &streamStarted, nil)
	require.NoError(t, err)
	require.NotNil(t, release)
	require.Zero(t, cache.accountWaitCalls)
	release()
}

func TestAcquireAccountSelectionSlotFeatureOffHonorsWaitLimit(t *testing.T) {
	cache := &countTokensAdmissionCache{
		concurrencyCacheMock: &concurrencyCacheMock{
			acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
				return false, nil
			},
		},
		accountWaitAllowed: false,
	}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)
	c := newCountTokensAdmissionContext(t)
	streamStarted := false

	release, err := acquireAccountSelectionSlot(c, helper, countTokensWaitSelection(), false, &streamStarted, nil)
	require.Nil(t, release)
	var queueFullErr *WaitQueueFullError
	require.ErrorAs(t, err, &queueFullErr)
	require.Equal(t, 1, cache.accountWaitCalls)
}

func TestAcquireAccountSelectionSlotLowTierNeverRegistersWaiter(t *testing.T) {
	cache := &countTokensAdmissionCache{
		concurrencyCacheMock: &concurrencyCacheMock{},
		priorityStatus:       service.PriorityAccountAdmissionRejected,
	}
	concurrency := service.NewConcurrencyService(cache)
	concurrency.SetPriorityAdmissionRuntimeConfig(service.PriorityAdmissionRuntimeConfig{Enabled: true})
	helper := NewConcurrencyHelper(concurrency, SSEPingFormatNone, time.Second)
	c := newCountTokensAdmissionContext(t)
	c.Request = c.Request.WithContext(service.WithRequestSchedulingTier(c.Request.Context(), service.RequestSchedulingTierLow))
	streamStarted := false

	release, err := acquireAccountSelectionSlot(c, helper, countTokensWaitSelection(), false, &streamStarted, nil)
	require.Nil(t, release)
	var queueFullErr *WaitQueueFullError
	require.ErrorAs(t, err, &queueFullErr)
	require.Len(t, cache.priorityRequests, 1)
	require.False(t, cache.priorityRequests[0].Register)
	require.Zero(t, cache.accountWaitCalls)
}

func TestAcquireAccountSelectionSlotPriorityRedisFailureFailsClosed(t *testing.T) {
	cache := &countTokensAdmissionCache{
		concurrencyCacheMock: &concurrencyCacheMock{},
		priorityErr:          errors.New("redis unavailable"),
	}
	concurrency := service.NewConcurrencyService(cache)
	concurrency.SetPriorityAdmissionRuntimeConfig(service.PriorityAdmissionRuntimeConfig{Enabled: true})
	helper := NewConcurrencyHelper(concurrency, SSEPingFormatNone, time.Second)
	c := newCountTokensAdmissionContext(t)
	c.Request = c.Request.WithContext(service.WithRequestSchedulingTier(c.Request.Context(), service.RequestSchedulingTierNormal))
	streamStarted := false

	release, err := acquireAccountSelectionSlot(c, helper, countTokensWaitSelection(), false, &streamStarted, nil)
	require.Nil(t, release)
	require.ErrorIs(t, err, service.ErrPriorityAdmissionUnavailable)
	require.Len(t, cache.priorityRequests, 1)
	require.False(t, cache.priorityRequests[0].Register)
}

func TestGrokCountTokensPriorityAdmissionDisabledPreservesLocalPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/messages/count_tokens",
		strings.NewReader(`{"model":"grok-3","messages":[{"role":"user","content":"hello"}]}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	h := &OpenAIGatewayHandler{}
	h.GrokCountTokens(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"input_tokens"`)
}
