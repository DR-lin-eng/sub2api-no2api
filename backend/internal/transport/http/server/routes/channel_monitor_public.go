package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"

	"github.com/gin-gonic/gin"
)

const channelMonitorPublicBatchBodyLimit int64 = 16 * 1024

// RegisterChannelMonitorPublicRoutes 注册渠道状态分享路由。
//
// 挂 OptionalJWT：匿名访问由 handler 根据 channel_monitor_public_share_require_auth
// 决定是否允许；带 token 时按普通用户 JWT 严格校验。
func RegisterChannelMonitorPublicRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	optionalJWT middleware.OptionalJWTAuthMiddleware,
	settingService *service.SettingService,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	monitors := v1.Group("/channel-status-share")
	monitors.Use(panelRateLimiter.PublicIP())
	monitors.Use(gin.HandlerFunc(optionalJWT))
	monitors.Use(middleware.BackendModeUserGuard(settingService))
	{
		monitors.GET("", h.ChannelMonitor.PublicList)
		monitors.POST(
			"/status/batch",
			middleware.RequestBodyLimit(channelMonitorPublicBatchBodyLimit),
			h.ChannelMonitor.PublicGetBatchStatus,
		)
		monitors.GET("/:id/status", h.ChannelMonitor.PublicGetStatus)
	}
}
