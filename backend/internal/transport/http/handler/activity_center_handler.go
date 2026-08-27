package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/modules/activitycenter"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/dto"

	"github.com/gin-gonic/gin"
)

type ActivityCenterHandler struct {
	service *activitycenter.Service
}

func NewActivityCenterHandler(service *activitycenter.Service) *ActivityCenterHandler {
	return &ActivityCenterHandler{service: service}
}

func (h *ActivityCenterHandler) ListCampaigns(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "Activity center service not available")
		return
	}
	items, err := h.service.ListVisible(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.ActivityCampaign, 0, len(items))
	for i := range items {
		out = append(out, *dto.ActivityCampaignFromService(&items[i]))
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

	item, err := h.service.GetVisible(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ActivityCampaignFromService(item))
}
