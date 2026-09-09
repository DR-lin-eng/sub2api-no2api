package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequiredAdminPermissionSeparatesBasicUserInfoAndSensitiveUserRoutes(t *testing.T) {
	tests := []struct{ method, path, want string }{
		{http.MethodGet, "/api/v1/admin/chat/conversations", service.PermissionSupportRead},
		{http.MethodPost, "/api/v1/admin/chat/conversations/1/messages", service.PermissionSupportWrite},
		{http.MethodPost, "/api/v1/admin/chat/conversations/1/balance-transfers", service.PermissionSupportTransfer},
		{http.MethodGet, "/api/v1/admin/users/1/basic", service.PermissionUsersReadBasic},
		{http.MethodGet, "/api/v1/admin/users/1/api-keys", service.PermissionUsersCredentials},
		{http.MethodPost, "/api/v1/admin/users/1/balance", service.PermissionUsersBilling},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			require.Equal(t, tt.want, requiredAdminPermission(tt.method, tt.path))
		})
	}
}

func TestAdminPermissionMiddlewareDeniesTransferToSupportRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyUserRole), "support")
		SetAdminPermissions(c, []string{service.PermissionSupportRead, service.PermissionSupportWrite})
		c.Next()
	})
	router.Use(AdminPermissionMiddleware(nil))
	router.POST("/api/v1/admin/chat/conversations/:id/balance-transfers", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/chat/conversations/1/balance-transfers", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestDelegatedPermissionsFailClosed(t *testing.T) {
	tests := []struct{ method, path, want string }{
		{"POST", "/api/v1/admin/users/1/platform-quotas/reset", service.PermissionUsersBilling},
		{"POST", "/api/v1/admin/users/platform-quotas/batch", service.PermissionUsersBilling},
		{"POST", "/api/v1/admin/users/batch-concurrency", service.PermissionUsersBilling},
		{"POST", "/api/v1/admin/settings/admin-api-keys", ""},
		{"PUT", "/api/v1/admin/settings/permission-groups", ""},
		{"GET", "/api/v1/admin/ops/dashboard", ""},
		{"GET", "/api/v1/admin/settings-unassigned", ""},
		{"GET", "/api/v1/admin/groups/1/api-keys", service.PermissionGroupsManage},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, requiredAdminPermission(tt.method, tt.path), tt.path)
	}
}

func TestDelegatedSettingsCannotModifyRoleGrantsOrMintAdministratorKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyUserRole), "settings_staff")
		SetAdminPermissions(c, []string{service.PermissionSettingsManage})
		c.Next()
	})
	r.Use(AdminPermissionMiddleware(nil))
	for _, path := range []string{"/api/v1/admin/settings/permission-groups", "/api/v1/admin/settings/admin-api-keys", "/api/v1/admin/compliance-extra"} {
		r.PUT(path, func(c *gin.Context) { c.Status(204) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("PUT", path, nil))
		require.Equal(t, 403, w.Code, path)
	}
}
