package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
)

// PreviewFromCPA returns metadata only; credentials stay on the server.
func (h *AccountHandler) PreviewFromCPA(c *gin.Context) {
	var input service.CPAConnectionInput
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid CPA preview request")
		return
	}
	result, err := h.cpaSyncService.Preview(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// SyncFromCPA imports a bounded selection, revalidating live CPA health first.
func (h *AccountHandler) SyncFromCPA(c *gin.Context) {
	var input service.CPASyncInput
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid CPA sync request")
		return
	}
	result, err := h.cpaSyncService.Sync(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
