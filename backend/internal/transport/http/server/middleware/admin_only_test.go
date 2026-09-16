package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminSessionOnlyRejectsAdminAPIKeyAndDelegatedRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		role       string
		authMethod string
		want       int
	}{
		{name: "administrator jwt", role: service.RoleAdmin, authMethod: service.AuditAuthMethodJWT, want: http.StatusNoContent},
		{name: "administrator api key", role: service.RoleAdmin, authMethod: service.AuditAuthMethodAdminAPIKey, want: http.StatusForbidden},
		{name: "delegated jwt", role: "settings_staff", authMethod: service.AuditAuthMethodJWT, want: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(ContextKeyUserRole), tt.role)
				c.Set("auth_method", tt.authMethod)
				c.Next()
			})
			router.PUT("/api/v1/admin/oauth2-provider", AdminSessionOnly(), func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/v1/admin/oauth2-provider", nil))
			require.Equal(t, tt.want, recorder.Code)
		})
	}
}
