package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type usageBillingQueueAdminStub struct {
	snapshot    *service.UsageBillingQueueSnapshot
	jobs        *service.UsageBillingQueueJobList
	deadLetters *service.UsageBillingDeadLetterList
	retry       service.UsageBillingJobRetry
	replay      service.UsageBillingDeadLetterReplay
	retryErr    error
	replayErr   error
}

func (s *usageBillingQueueAdminStub) GetUsageBillingQueueSnapshot(context.Context) (*service.UsageBillingQueueSnapshot, error) {
	return s.snapshot, nil
}

func (s *usageBillingQueueAdminStub) ListUsageBillingQueueJobs(context.Context, service.UsageBillingQueueJobFilter) (*service.UsageBillingQueueJobList, error) {
	return s.jobs, nil
}

func (s *usageBillingQueueAdminStub) ListUsageBillingDeadLetters(context.Context, service.UsageBillingQueueJobFilter) (*service.UsageBillingDeadLetterList, error) {
	return s.deadLetters, nil
}

func (s *usageBillingQueueAdminStub) RetryUsageBillingQueueJob(_ context.Context, input service.UsageBillingJobRetry) error {
	s.retry = input
	return s.retryErr
}

func (s *usageBillingQueueAdminStub) ReplayUsageBillingDeadLetter(_ context.Context, input service.UsageBillingDeadLetterReplay) error {
	s.replay = input
	return s.replayErr
}

func newUsageBillingQueueAdminHandler(stub service.UsageBillingQueueAdmin) *OpsHandler {
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	ops.SetUsageBillingQueueAdmin(stub)
	return NewOpsHandler(ops)
}

func usageBillingQueueAuthContext(c *gin.Context) {
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
}

func TestOpsUsageBillingQueueRetryRequiresAuditedReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &usageBillingQueueAdminStub{}
	router := gin.New()
	router.POST("/jobs/:id/retry", func(c *gin.Context) {
		usageBillingQueueAuthContext(c)
		newUsageBillingQueueAdminHandler(stub).RetryUsageBillingQueueJob(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/jobs/7/retry", strings.NewReader(`{"reason":"operator review"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(7), stub.retry.JobID)
	require.Equal(t, int64(42), stub.retry.OperatorID)
	require.Equal(t, "operator review", stub.retry.Reason)
}

func TestOpsUsageBillingQueueRetryMapsMissingReasonToBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/jobs/:id/retry", func(c *gin.Context) {
		usageBillingQueueAuthContext(c)
		newUsageBillingQueueAdminHandler(&usageBillingQueueAdminStub{}).RetryUsageBillingQueueJob(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/jobs/7/retry", strings.NewReader(`{"reason":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestOpsUsageBillingQueueReplayMapsConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &usageBillingQueueAdminStub{replayErr: service.ErrUsageBillingRequestConflict}
	router := gin.New()
	router.POST("/dead-letters/:id/replay", func(c *gin.Context) {
		usageBillingQueueAuthContext(c)
		newUsageBillingQueueAdminHandler(stub).ReplayUsageBillingDeadLetter(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/dead-letters/9/replay", strings.NewReader(`{"reason":"reconcile after deploy"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestOpsUsageBillingQueueUnavailableIs503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/summary", NewOpsHandler(service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)).GetUsageBillingQueueSnapshot)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/summary", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
