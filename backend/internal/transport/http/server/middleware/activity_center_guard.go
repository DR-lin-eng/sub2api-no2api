package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"

	"github.com/gin-gonic/gin"
)

func ActivityCenterFeatureGuard(settingService *service.SettingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if settingService != nil && settingService.IsActivityCenterEnabled(c.Request.Context()) {
			c.Next()
			return
		}
		response.Forbidden(c, "Activity center is disabled.")
		c.Abort()
	}
}
