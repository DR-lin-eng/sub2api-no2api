package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
)

type ClusterHandler struct {
	service *service.ClusterService
}

func NewClusterHandler(clusterService *service.ClusterService) *ClusterHandler {
	return &ClusterHandler{service: clusterService}
}

// GetStatus returns node health, resolved deployment configuration, and recent
// renewable task leases for the dedicated multi-instance administration page.
func (h *ClusterHandler) GetStatus(c *gin.Context) {
	status, err := h.service.GetStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}
