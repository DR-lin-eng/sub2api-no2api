package admin

import (
	"context"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
)

type ClusterHandler struct {
	service        *service.ClusterService
	releaseService *service.ClusterReleaseService
}

func NewClusterHandler(clusterService *service.ClusterService, releaseService *service.ClusterReleaseService) *ClusterHandler {
	return &ClusterHandler{service: clusterService, releaseService: releaseService}
}

// GetStatus returns node health, resolved deployment configuration, and recent
// renewable task leases for the dedicated multi-instance administration page.
func (h *ClusterHandler) GetStatus(c *gin.Context) {
	status, err := h.service.GetStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	release, err := h.releaseService.GetOverview(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	status.Release = release
	response.Success(c, status)
}

func (h *ClusterHandler) RenameNode(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.service.RenameNode(c.Request.Context(), c.Param("node_id"), req.Name); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"node_id": c.Param("node_id"), "name": strings.TrimSpace(req.Name)})
}

func (h *ClusterHandler) CreateRollout(c *gin.Context) {
	var req struct {
		TargetVersion string `json:"target_version"`
		Confirm       bool   `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirm {
		response.Error(c, http.StatusBadRequest, "confirmation required")
		return
	}
	var actorID int64
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		actorID = subject.UserID
	}
	rollout, err := h.releaseService.CreateRollout(c.Request.Context(), service.CreateClusterRolloutInput{
		TargetVersion: req.TargetVersion,
		CreatedBy:     actorID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, rollout)
}

func (h *ClusterHandler) GetRollout(c *gin.Context) {
	rollout, err := h.releaseService.GetRollout(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rollout)
}

func (h *ClusterHandler) PauseRollout(c *gin.Context) {
	h.mutateRollout(c, h.releaseService.PauseRollout)
}

func (h *ClusterHandler) ResumeRollout(c *gin.Context) {
	h.mutateRollout(c, h.releaseService.ResumeRollout)
}

func (h *ClusterHandler) CancelRollout(c *gin.Context) {
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirm {
		response.Error(c, http.StatusBadRequest, "confirmation required")
		return
	}
	h.mutateRollout(c, h.releaseService.CancelRollout)
}

func (h *ClusterHandler) ConfirmRollout(c *gin.Context) {
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirm {
		response.Error(c, http.StatusBadRequest, "confirmation required")
		return
	}
	h.mutateRollout(c, h.releaseService.ConfirmRollout)
}

func (h *ClusterHandler) RetryTarget(c *gin.Context) {
	if err := h.releaseService.RetryTarget(c.Request.Context(), c.Param("id"), c.Param("node_id")); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	rollout, err := h.releaseService.GetRollout(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rollout)
}

func (h *ClusterHandler) mutateRollout(c *gin.Context, mutate func(context.Context, string) error) {
	if err := mutate(c.Request.Context(), c.Param("id")); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	rollout, err := h.releaseService.GetRollout(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rollout)
}
