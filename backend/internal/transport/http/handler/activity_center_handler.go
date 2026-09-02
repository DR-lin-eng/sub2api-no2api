package handler

import (
	"strconv"

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

type participateActivityRequest struct {
	PoolID string `json:"pool_id"`
}

func (h *ActivityCenterHandler) ListCampaigns(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "Activity center service not available")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	items, err := h.service.ListVisibleForUser(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.UserActivityCampaign, 0, len(items))
	for i := range items {
		out = append(out, *dto.UserActivityCampaignFromService(&items[i]))
	}
	response.Success(c, out)
}

func (h *ActivityCenterHandler) GetCampaign(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "Activity center service not available")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid activity campaign ID")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	item, err := h.service.GetVisibleForUser(c.Request.Context(), id, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.UserActivityCampaignFromService(item))
}

func (h *ActivityCenterHandler) Participate(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "Activity center service not available")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid activity campaign ID")
		return
	}
	var req participateActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	record, err := h.service.Participate(c.Request.Context(), id, activitycenter.ParticipateInput{
		UserID: subject.UserID,
		PoolID: req.PoolID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ActivityParticipationRecordFromService(record, false))
}

func (h *ActivityCenterHandler) GetCheckinStatus(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid activity campaign ID")
		return
	}
	status, err := h.service.CheckinStatus(c.Request.Context(), id, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ActivityCheckinStatusFromService(status))
}

func (h *ActivityCenterHandler) Checkin(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid activity campaign ID")
		return
	}
	record, status, err := h.service.Checkin(c.Request.Context(), id, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"record": dto.ActivityParticipationRecordFromService(record, false), "status": dto.ActivityCheckinStatusFromService(status)})
}

func (h *ActivityCenterHandler) CheckinLeaderboard(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid activity campaign ID")
		return
	}
	if _, err := h.service.GetVisibleForUser(c.Request.Context(), id, subject.UserID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items, err := h.service.CheckinLeaderboard(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ActivityCheckinLeaderboardFromService(items))
}

func (h *ActivityCenterHandler) ListMyRecords(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "Activity center service not available")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, pageResult, err := h.service.ListUserRecords(c.Request.Context(), subject.UserID, pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.ActivityParticipationRecord, 0, len(items))
	for i := range items {
		out = append(out, *dto.ActivityParticipationRecordFromService(&items[i], false))
	}
	response.Paginated(c, out, pageResult.Total, page, pageSize)
}
