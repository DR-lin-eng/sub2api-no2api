package routes

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterOAuth2ProviderRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{OAuth2Provider: handler.NewOAuth2ProviderHandler(nil)}
	jwtAuth := middleware.JWTAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterOAuth2ProviderRoutes(router, handlers, jwtAuth, &middleware.PanelRateLimiter{})

	wanted := map[string]bool{
		"GET /.well-known/oauth-authorization-server": false,
		"POST /oauth/token":                           false,
		"POST /oauth/revoke":                          false,
		"GET /oauth/userinfo":                         false,
		"GET /api/v1/oauth2/authorize":                false,
		"POST /api/v1/oauth2/authorize":               false,
	}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := wanted[key]; ok {
			wanted[key] = true
		}
	}
	for route, registered := range wanted {
		require.True(t, registered, route)
	}
}
