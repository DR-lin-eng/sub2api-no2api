package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
)

func respondUsageBillingQueueError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrUsageBillingQueueUnavailable):
		response.Error(c, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, service.ErrUsageBillingQueueJobNotFound),
		errors.Is(err, service.ErrUsageBillingDeadLetterNotFound):
		response.Error(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrUsageBillingAdminReasonRequired),
		errors.Is(err, service.ErrUsageBillingDeadLetterInvalid),
		errors.Is(err, service.ErrUsageBillingQueueFilterInvalid):
		response.Error(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrUsageBillingRequestConflict):
		response.Error(c, http.StatusConflict, err.Error())
	default:
		response.ErrorFrom(c, err)
	}
}

// GetUsageBillingQueueSnapshot returns durable settlement and overlay-cleanup
// health. It is intentionally a single aggregate query plus one small error
// classification query so opening the Ops panel does not scan job payloads.
// GET /api/v1/admin/ops/usage-billing/summary
func (h *OpsHandler) GetUsageBillingQueueSnapshot(c *gin.Context) {
	if h == nil || h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	snapshot, err := h.opsService.GetUsageBillingQueueSnapshot(c.Request.Context())
	if err != nil {
		respondUsageBillingQueueError(c, err)
		return
	}
	response.Success(c, snapshot)
}

// ListUsageBillingQueueJobs lists unsettled, cleanup-pending, or reconcile
// required jobs without exposing payloads or account credentials.
// GET /api/v1/admin/ops/usage-billing/jobs
func (h *OpsHandler) ListUsageBillingQueueJobs(c *gin.Context) {
	if h == nil || h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	page, pageSize := response.ParsePagination(c)
	filter := service.UsageBillingQueueJobFilter{
		State:    strings.TrimSpace(c.Query("state")),
		Query:    strings.TrimSpace(c.Query("q")),
		Page:     page,
		PageSize: pageSize,
	}
	result, err := h.opsService.ListUsageBillingQueueJobs(c.Request.Context(), filter)
	if err != nil {
		respondUsageBillingQueueError(c, err)
		return
	}
	response.Paginated(c, result.Jobs, result.Total, result.Page, result.PageSize)
}

// ListUsageBillingDeadLetters lists permanently invalid jobs. Payloads remain
// hidden; replay is a separate audited action.
// GET /api/v1/admin/ops/usage-billing/dead-letters
func (h *OpsHandler) ListUsageBillingDeadLetters(c *gin.Context) {
	if h == nil || h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	page, pageSize := response.ParsePagination(c)
	result, err := h.opsService.ListUsageBillingDeadLetters(c.Request.Context(), service.UsageBillingQueueJobFilter{
		Query:    strings.TrimSpace(c.Query("q")),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		respondUsageBillingQueueError(c, err)
		return
	}
	response.Paginated(c, result.DeadLetters, result.Total, result.Page, result.PageSize)
}

type usageBillingAdminActionRequest struct {
	Reason string `json:"reason"`
}

func usageBillingOperatorID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	return subject.UserID, ok && subject.UserID > 0
}

// RetryUsageBillingQueueJob makes an available_at-now retry request and writes
// an audit row in the same PostgreSQL transaction.
// POST /api/v1/admin/ops/usage-billing/jobs/:id/retry
func (h *OpsHandler) RetryUsageBillingQueueJob(c *gin.Context) {
	if h == nil || h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	operatorID, ok := usageBillingOperatorID(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	jobID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || jobID <= 0 {
		response.BadRequest(c, "Invalid job id")
		return
	}
	var req usageBillingAdminActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		response.BadRequest(c, "reason is required")
		return
	}
	if err := h.opsService.RetryUsageBillingQueueJob(c.Request.Context(), service.UsageBillingJobRetry{
		JobID:      jobID,
		OperatorID: operatorID,
		Reason:     req.Reason,
	}); err != nil {
		respondUsageBillingQueueError(c, err)
		return
	}
	response.Success(c, gin.H{"retried": true, "job_id": jobID})
}

// ReplayUsageBillingDeadLetter re-enqueues a dead-letter payload and records
// the operator and reason transactionally.
// POST /api/v1/admin/ops/usage-billing/dead-letters/:id/replay
func (h *OpsHandler) ReplayUsageBillingDeadLetter(c *gin.Context) {
	if h == nil || h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	operatorID, ok := usageBillingOperatorID(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	deadLetterID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || deadLetterID <= 0 {
		response.BadRequest(c, "Invalid dead letter id")
		return
	}
	var req usageBillingAdminActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		response.BadRequest(c, "reason is required")
		return
	}
	if err := h.opsService.ReplayUsageBillingDeadLetter(c.Request.Context(), service.UsageBillingDeadLetterReplay{
		DeadLetterID: deadLetterID,
		OperatorID:   operatorID,
		Reason:       req.Reason,
	}); err != nil {
		respondUsageBillingQueueError(c, err)
		return
	}
	response.Success(c, gin.H{"replayed": true, "dead_letter_id": deadLetterID})
}
