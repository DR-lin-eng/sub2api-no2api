package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
)

// BatchSyncUpstreamModels persists live models for each selected account.
func (h *AccountHandler) BatchSyncUpstreamModels(c *gin.Context) {
	var req struct {
		AccountIDs []int64 `json:"account_ids" binding:"required,min=1,max=100,dive,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Provide between 1 and 100 positive account IDs")
		return
	}
	if h.accountTestService == nil {
		response.InternalError(c, "Account test service is not configured")
		return
	}
	result, err := h.accountTestService.BatchSyncUpstreamModels(c.Request.Context(), req.AccountIDs)
	if err != nil {
		// Validation is handled above; these errors indicate missing server wiring.
		response.InternalError(c, "Upstream model sync is not configured")
		return
	}
	response.Success(c, result)
}
