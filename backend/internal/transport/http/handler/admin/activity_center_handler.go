package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/activitycenter"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/dto"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"

	"github.com/gin-gonic/gin"
)

type ActivityCenterHandler struct {
	service *activitycenter.Service
}

func NewActivityCenterHandler(service *activitycenter.Service) *ActivityCenterHandler {
	return &ActivityCenterHandler{service: service}
}

type CreateActivityCampaignRequest struct {
	Title      string `json:"title" binding:"required"`
	Subtitle   string `json:"subtitle"`
	BannerURL  string `json:"banner_url"`
	BannerHTML string `json:"banner_html"`
	Type       string `json:"type" binding:"omitempty,oneof=lottery inflate redeem custom"`
	RefID      string `json:"ref_id"`
	ConfigJSON string `json:"config_json"`
	Status     string `json:"status" binding:"omitempty,oneof=draft active archived"`
	StartsAt   *int64 `json:"starts_at"`
	EndsAt     *int64 `json:"ends_at"`
	SortOrder  int    `json:"sort_order"`
	Content    string `json:"content"`
}

type UpdateActivityCampaignRequest struct {
	Title      *string `json:"title"`
	Subtitle   *string `json:"subtitle"`
	BannerURL  *string `json:"banner_url"`
	BannerHTML *string `json:"banner_html"`
	Type       *string `json:"type" binding:"omitempty,oneof=lottery inflate redeem custom"`
	RefID      *string `json:"ref_id"`
	ConfigJSON *string `json:"config_json"`
	Status     *string `json:"status" binding:"omitempty,oneof=draft active archived"`
	StartsAt   *int64  `json:"starts_at"`
	EndsAt     *int64  `json:"ends_at"`
	SortOrder  *int    `json:"sort_order"`
	Content    *string `json:"content"`
}

func (h *ActivityCenterHandler) ListCampaigns(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "Activity center service not available")
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}
	filters := activitycenter.ListFilters{
		Status: strings.TrimSpace(c.Query("status")),
		Type:   strings.TrimSpace(c.Query("type")),
		Search: strings.TrimSpace(c.Query("search")),
	}

	items, pageResult, err := h.service.List(c.Request.Context(), params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.ActivityCampaign, 0, len(items))
	for i := range items {
		item := dto.ActivityCampaignFromService(&items[i])
		stats, err := h.service.PrizeStockStats(c.Request.Context(), &items[i])
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		item.PrizeStockStats = dto.ActivityPrizeStockStatsFromService(stats)
		out = append(out, *item)
	}
	response.Paginated(c, out, pageResult.Total, page, pageSize)
}

func (h *ActivityCenterHandler) GetCampaign(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "Activity center service not available")
		return
	}
	id, ok := parseActivityCampaignID(c)
	if !ok {
		return
	}
	item, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := dto.ActivityCampaignFromService(item)
	stats, err := h.service.PrizeStockStats(c.Request.Context(), item)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out.PrizeStockStats = dto.ActivityPrizeStockStatsFromService(stats)
	response.Success(c, out)
}

func (h *ActivityCenterHandler) CreateCampaign(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "Activity center service not available")
		return
	}
	var req CreateActivityCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	input := &activitycenter.CreateInput{
		Title:      req.Title,
		Subtitle:   req.Subtitle,
		BannerURL:  req.BannerURL,
		BannerHTML: req.BannerHTML,
		Type:       req.Type,
		RefID:      req.RefID,
		ConfigJSON: req.ConfigJSON,
		Status:     req.Status,
		SortOrder:  req.SortOrder,
		Content:    req.Content,
		ActorID:    &subject.UserID,
	}
	if req.StartsAt != nil && *req.StartsAt > 0 {
		t := time.Unix(*req.StartsAt, 0)
		input.StartsAt = &t
	}
	if req.EndsAt != nil && *req.EndsAt > 0 {
		t := time.Unix(*req.EndsAt, 0)
		input.EndsAt = &t
	}

	created, err := h.service.Create(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := dto.ActivityCampaignFromService(created)
	stats, err := h.service.PrizeStockStats(c.Request.Context(), created)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out.PrizeStockStats = dto.ActivityPrizeStockStatsFromService(stats)
	response.Success(c, out)
}

func (h *ActivityCenterHandler) UpdateCampaign(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "Activity center service not available")
		return
	}
	id, ok := parseActivityCampaignID(c)
	if !ok {
		return
	}

	var req UpdateActivityCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	input := &activitycenter.UpdateInput{
		Title:      req.Title,
		Subtitle:   req.Subtitle,
		BannerURL:  req.BannerURL,
		BannerHTML: req.BannerHTML,
		Type:       req.Type,
		RefID:      req.RefID,
		ConfigJSON: req.ConfigJSON,
		Status:     req.Status,
		SortOrder:  req.SortOrder,
		Content:    req.Content,
	}
	if req.StartsAt != nil {
		if *req.StartsAt == 0 {
			var cleared *time.Time
			input.StartsAt = &cleared
		} else {
			t := time.Unix(*req.StartsAt, 0)
			ptr := &t
			input.StartsAt = &ptr
		}
	}
	if req.EndsAt != nil {
		if *req.EndsAt == 0 {
			var cleared *time.Time
			input.EndsAt = &cleared
		} else {
			t := time.Unix(*req.EndsAt, 0)
			ptr := &t
			input.EndsAt = &ptr
		}
	}

	updated, err := h.service.Update(c.Request.Context(), id, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := dto.ActivityCampaignFromService(updated)
	stats, err := h.service.PrizeStockStats(c.Request.Context(), updated)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out.PrizeStockStats = dto.ActivityPrizeStockStatsFromService(stats)
	response.Success(c, out)
}

func (h *ActivityCenterHandler) DeleteCampaign(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "Activity center service not available")
		return
	}
	id, ok := parseActivityCampaignID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Activity campaign deleted successfully"})
}

func (h *ActivityCenterHandler) ListRecords(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "Activity center service not available")
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}
	filters := activitycenter.RecordFilters{
		Type:   strings.TrimSpace(c.Query("type")),
		Search: strings.TrimSpace(c.Query("search")),
	}
	if raw := strings.TrimSpace(c.Query("campaign_id")); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
			filters.CampaignID = id
		}
	}
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
			filters.UserID = id
		}
	}

	items, pageResult, err := h.service.ListRecords(c.Request.Context(), params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.ActivityParticipationRecord, 0, len(items))
	for i := range items {
		out = append(out, *dto.ActivityParticipationRecordFromService(&items[i], true))
	}
	response.Paginated(c, out, pageResult.Total, page, pageSize)
}

func parseActivityCampaignID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid activity campaign ID")
		return 0, false
	}
	return id, true
}
