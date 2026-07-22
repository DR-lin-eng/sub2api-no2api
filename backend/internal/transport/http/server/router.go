package server

import (
	"context"
	"database/sql"
	"log"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	coremiddleware "github.com/Wei-Shaw/sub2api/internal/platform/middleware"
	clientip "github.com/Wei-Shaw/sub2api/internal/shared/ip"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/routes"
	"github.com/Wei-Shaw/sub2api/internal/transport/webassets"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const cspOriginsRefreshTimeout = 5 * time.Second

// SetupRouter 配置路由器中间件和路由
func SetupRouter(
	r *gin.Engine,
	handlers *handler.Handlers,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
	apiKeyAuth middleware2.APIKeyAuthMiddleware,
	auditLog middleware2.AuditLogMiddleware,
	stepUpAuth middleware2.StepUpAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	opsService *service.OpsService,
	settingService *service.SettingService,
	compositeResolver *service.CompositeRouteResolver,
	clientIPResolver *clientip.Resolver,
	cfg *config.Config,
	redisClient *redis.Client,
	db *sql.DB,
) *gin.Engine {
	middleware2.SetIngressRejectRecorder(opsService)
	// 缓存动态 CSP origin，避免每个静态资源请求都读取设置。
	var cachedFrameOrigins atomic.Pointer[[]string]
	var cachedConnectOrigins atomic.Pointer[[]string]
	emptyOrigins := []string{}
	cachedFrameOrigins.Store(&emptyOrigins)
	cachedConnectOrigins.Store(&emptyOrigins)

	refreshCSPOrigins := func() {
		ctx, cancel := context.WithTimeout(context.Background(), cspOriginsRefreshTimeout)
		defer cancel()
		frameOrigins, err := settingService.GetFrameSrcOrigins(ctx)
		if err != nil {
			// 获取失败时保留已有缓存，避免 CSP 来源被意外清空。
			return
		}
		connectOrigins, err := settingService.GetConnectSrcOrigins(ctx)
		if err != nil {
			return
		}
		cachedFrameOrigins.Store(&frameOrigins)
		cachedConnectOrigins.Store(&connectOrigins)
	}
	refreshCSPOrigins() // 启动时初始化

	// 应用中间件
	r.Use(clientIPResolver.Middleware())
	r.Use(coremiddleware.NewCredentialAuthIngressLimiter())
	r.Use(middleware2.RequestLogger())
	// 将客户端 IP + UA 注入 request context，供 token 签发/会话绑定/审计日志统一读取。
	r.Use(middleware2.SessionBindingContext(nil))
	r.Use(middleware2.Logger())
	r.Use(middleware2.CORS(cfg.CORS))
	r.Use(middleware2.SecurityHeaders(cfg.Security.CSP, func() []string {
		if p := cachedFrameOrigins.Load(); p != nil {
			return *p
		}
		return nil
	}, func() []string {
		if p := cachedConnectOrigins.Load(); p != nil {
			return *p
		}
		return nil
	}))
	r.Use(middleware2.ServerTiming(cfg.Server.EnableServerTiming))

	// Serve embedded frontend with settings injection if available
	if web.HasEmbeddedFrontend() {
		frontendServer, err := web.NewFrontendServer(settingService)
		if err != nil {
			log.Printf("Warning: Failed to create frontend server with settings injection: %v, using legacy mode", err)
			r.Use(web.ServeEmbeddedFrontend())
			settingService.SetOnUpdateCallback(refreshCSPOrigins)
		} else {
			// Register combined callback: invalidate HTML cache + refresh CSP origins.
			settingService.SetOnUpdateCallback(func() {
				frontendServer.InvalidateCache()
				refreshCSPOrigins()
			})
			r.Use(frontendServer.Middleware())
		}
	} else {
		settingService.SetOnUpdateCallback(refreshCSPOrigins)
	}

	// 注册路由
	registerRoutes(r, handlers, jwtAuth, adminAuth, apiKeyAuth, auditLog, stepUpAuth, apiKeyService, subscriptionService, opsService, settingService, compositeResolver, cfg, redisClient, db)

	return r
}

// registerRoutes 注册所有 HTTP 路由
func registerRoutes(
	r *gin.Engine,
	h *handler.Handlers,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
	apiKeyAuth middleware2.APIKeyAuthMiddleware,
	auditLog middleware2.AuditLogMiddleware,
	stepUpAuth middleware2.StepUpAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	opsService *service.OpsService,
	settingService *service.SettingService,
	compositeResolver *service.CompositeRouteResolver,
	cfg *config.Config,
	redisClient *redis.Client,
	db *sql.DB,
) {
	// 通用路由（健康检查、状态等）
	routes.RegisterCommonRoutes(r)

	// API v1
	v1 := r.Group("/api/v1")

	// 注册各模块路由
	routes.RegisterAuthRoutes(v1, h, jwtAuth, auditLog, redisClient, db, settingService)
	routes.RegisterUserRoutes(v1, h, jwtAuth, auditLog, settingService)
	routes.RegisterAdminRoutes(v1, h, adminAuth, auditLog, stepUpAuth, settingService)
	routes.RegisterGatewayRoutes(r, h, apiKeyAuth, apiKeyService, subscriptionService, opsService, settingService, compositeResolver, cfg)
	routes.RegisterPaymentRoutes(v1, h.Payment, h.PaymentWebhook, h.Admin.Payment, jwtAuth, adminAuth, auditLog, settingService)

	handler.RegisterPageRoutes(v1, cfg.Pricing.DataDir, gin.HandlerFunc(jwtAuth), gin.HandlerFunc(adminAuth), settingService)
}
