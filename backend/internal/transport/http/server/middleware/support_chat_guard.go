package middleware

import (
	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/response"

	"github.com/gin-gonic/gin"
)

// SupportChatFeatureGuard blocks support chat APIs when the runtime feature
// switch is disabled. The guard is fail-closed so a missing service or a
// settings storage outage cannot expose a newly added privileged surface.
func SupportChatFeatureGuard(settingService *service.SettingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if settingService != nil && settingService.IsSupportChatEnabled(c.Request.Context()) {
			c.Next()
			return
		}
		response.Forbidden(c, "Support chat is disabled.")
		c.Abort()
	}
}
