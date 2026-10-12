package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterOAuth2ProviderRoutes installs the public OAuth2 protocol endpoints
// and the JWT-protected resource-owner approval endpoints.
func RegisterOAuth2ProviderRoutes(
	r *gin.Engine,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	panelRateLimiter *middleware.PanelRateLimiter,
	redisClients ...*redis.Client,
) {
	if h == nil || h.OAuth2Provider == nil {
		return
	}
	r.GET("/.well-known/oauth-authorization-server", panelRateLimiter.PublicIP(), h.OAuth2Provider.Discovery)

	var redisClient *redis.Client
	if len(redisClients) > 0 {
		redisClient = redisClients[0]
	}
	credentialLimit := middleware.OAuth2CredentialRateLimit(redisClient)
	oauth := r.Group("/oauth")
	oauth.Use(panelRateLimiter.PublicIP())
	{
		oauth.POST("/token", credentialLimit, h.OAuth2Provider.Token)
		oauth.POST("/revoke", credentialLimit, h.OAuth2Provider.Revoke)
		oauth.GET("/userinfo", h.OAuth2Provider.UserInfo)
	}

	owner := r.Group("/api/v1/oauth2")
	owner.Use(gin.HandlerFunc(jwtAuth))
	owner.Use(panelRateLimiter.Authenticated())
	{
		owner.GET("/authorize", h.OAuth2Provider.AuthorizationPreview)
		owner.POST("/authorize", h.OAuth2Provider.Authorize)
	}
}
