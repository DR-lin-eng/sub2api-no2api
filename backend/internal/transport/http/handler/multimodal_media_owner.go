package handler

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
)

func (h *OpenAIGatewayHandler) ResolveMultimodalVideoTaskOwner(ctx context.Context, apiKey *service.APIKey, requestID string) (int64, error) {
	if h == nil || h.gatewayService == nil {
		return 0, nil
	}
	return h.gatewayService.ResolveMultimodalVideoTaskOwner(ctx, apiKey, requestID)
}

// DispatchCompositeMultimodalVideoLookup uses only an authenticated task owner.
// Unmatched requests retain the existing Grok composite lookup path.
func (h *OpenAIGatewayHandler) DispatchCompositeMultimodalVideoLookup(c *gin.Context, content bool) bool {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformComposite {
		return false
	}
	requestID := c.Param("request_id")
	accountID, checked := middleware2.GetMultimodalVideoTaskAccount(c, apiKey, requestID)
	if !checked {
		var err error
		accountID, err = h.ResolveMultimodalVideoTaskOwner(c.Request.Context(), apiKey, requestID)
		if err != nil {
			h.errorResponse(c, http.StatusBadGateway, "billing_service_error", "Failed to verify video task ownership, please retry later")
			return true
		}
		middleware2.SetMultimodalVideoTaskAccount(c, apiKey, requestID, accountID)
	}
	if accountID <= 0 {
		return false
	}
	if content {
		h.MultimodalVideoContent(c)
	} else {
		h.MultimodalVideoStatus(c)
	}
	return true
}
