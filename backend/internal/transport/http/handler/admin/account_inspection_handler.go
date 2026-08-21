package admin

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
)

// AccountInspectionHandler exposes the admin account-inspection controls.
type AccountInspectionHandler struct {
	inspectionService *service.AccountInspectionService
}

func NewAccountInspectionHandler(inspectionService *service.AccountInspectionService) *AccountInspectionHandler {
	return &AccountInspectionHandler{inspectionService: inspectionService}
}

// AccountInspectionSettings returns the current settings and latest run page.
// GET /api/v1/admin/account-inspection
func (h *AccountInspectionHandler) AccountInspectionSettings(c *gin.Context) {
	if h == nil || h.inspectionService == nil {
		response.ErrorFrom(c, service.ErrAccountInspectionUnavailable)
		return
	}
	page, pageSize := parseInspectionPagination(c)
	overview, err := h.inspectionService.GetOverview(c.Request.Context(), service.AccountInspectionListFilter{
		Page: page, PageSize: pageSize,
		Status: strings.TrimSpace(c.Query("status")),
		Type:   strings.TrimSpace(c.Query("type")),
		Search: strings.TrimSpace(c.Query("search")),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}

// UpdateAccountInspectionSettings updates the global inspection policy.
// PUT /api/v1/admin/account-inspection/settings
func (h *AccountInspectionHandler) UpdateAccountInspectionSettings(c *gin.Context) {
	if h == nil || h.inspectionService == nil {
		response.ErrorFrom(c, service.ErrAccountInspectionUnavailable)
		return
	}
	// Seed from the saved policy so lightweight/older clients can update one
	// field without resetting thresholds or the protection list they do not
	// know about yet.
	req, err := h.inspectionService.GetSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	settings, err := h.inspectionService.UpdateSettings(c.Request.Context(), &req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

// RunAccountInspection triggers one bounded inspection using the saved policy.
// POST /api/v1/admin/account-inspection/run
func (h *AccountInspectionHandler) RunAccountInspection(c *gin.Context) {
	if h == nil || h.inspectionService == nil {
		response.ErrorFrom(c, service.ErrAccountInspectionUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	if _, err := h.inspectionService.RunNow(ctx, "manual"); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	overview, err := h.inspectionService.GetOverview(ctx, service.AccountInspectionListFilter{Page: 1, PageSize: 50})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}

func parseInspectionPagination(c *gin.Context) (int, int) {
	page := 1
	pageSize := 50
	if raw := c.Query("page"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			pageSize = parsed
		}
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}
