package admin

import (
	"context"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
)

// AccountQualityHandler exposes quality monitoring independently from the
// account health inspection policy.
type AccountQualityHandler struct {
	qualityService *service.AccountQualityMonitoringService
}

func NewAccountQualityHandler(qualityService *service.AccountQualityMonitoringService) *AccountQualityHandler {
	return &AccountQualityHandler{qualityService: qualityService}
}

func (h *AccountQualityHandler) Overview(c *gin.Context) {
	if h == nil || h.qualityService == nil {
		response.ErrorFrom(c, service.ErrAccountInspectionUnavailable)
		return
	}
	page, pageSize := parseQualityPagination(c)
	overview, err := h.qualityService.GetOverview(c.Request.Context(), service.AccountInspectionListFilter{Page: page, PageSize: pageSize, Status: strings.TrimSpace(c.Query("status")), Type: strings.TrimSpace(c.Query("type")), Search: strings.TrimSpace(c.Query("search"))})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}

func (h *AccountQualityHandler) UpdateSettings(c *gin.Context) {
	if h == nil || h.qualityService == nil {
		response.ErrorFrom(c, service.ErrAccountInspectionUnavailable)
		return
	}
	var req service.AccountQualitySettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	settings, err := h.qualityService.UpdateSettings(c.Request.Context(), &req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *AccountQualityHandler) Run(c *gin.Context) {
	if h == nil || h.qualityService == nil {
		response.ErrorFrom(c, service.ErrAccountInspectionUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), service.AccountQualityRunTimeout)
	defer cancel()
	if _, err := h.qualityService.RunNow(ctx, "manual"); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	overview, err := h.qualityService.GetOverview(ctx, service.AccountInspectionListFilter{Page: 1, PageSize: 50})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}

func parseQualityPagination(c *gin.Context) (int, int) {
	page, pageSize := 1, 50
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
