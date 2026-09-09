package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
)

type updatePermissionGroupsRequest struct {
	Groups []service.PermissionGroup `json:"groups" binding:"required"`
}

// GetPermissionGroups returns the configurable admin permission catalog and groups.
// GET /api/v1/admin/settings/permission-groups
func (h *SettingHandler) GetPermissionGroups(c *gin.Context) {
	groups, err := h.settingService.GetPermissionGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"groups":      groups,
		"permissions": service.PermissionDefinitions(),
	})
}

// UpdatePermissionGroups replaces the permission group document after validation.
// PUT /api/v1/admin/settings/permission-groups
func (h *SettingHandler) UpdatePermissionGroups(c *gin.Context) {
	var req updatePermissionGroupsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	groups, err := h.settingService.UpdatePermissionGroups(c.Request.Context(), req.Groups)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, gin.H{
		"groups":      groups,
		"permissions": service.PermissionDefinitions(),
	})
}
