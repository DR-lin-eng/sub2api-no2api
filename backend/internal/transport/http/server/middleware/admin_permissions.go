package middleware

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
)

// AdminPermissionMiddleware enforces the permission group attached to a JWT user.
// The legacy admin role keeps full access; custom roles are evaluated per admin route.
func AdminPermissionMiddleware(_ *service.SettingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := GetUserRoleFromContext(c)
		path := strings.TrimSuffix(c.Request.URL.Path, "/")
		if role == service.RoleAdmin || path == "/api/v1/admin/compliance" || path == "/api/v1/admin/compliance/accept" {
			c.Next()
			return
		}
		required := requiredAdminPermission(c.Request.Method, c.Request.URL.Path)
		if required != "" && hasAdminPermission(c, required) {
			c.Next()
			return
		}
		if required == "" {
			required = "admin.only"
		}
		response.Error(c, http.StatusForbidden, "Permission required: "+required)
		c.Abort()
	}
}

func SetAdminPermissions(c *gin.Context, permissions []string) {
	if c == nil {
		return
	}
	c.Set(string(ContextKeyAdminPermissions), append([]string(nil), permissions...))
}

func GetAdminPermissionsFromContext(c *gin.Context) []string {
	if c == nil {
		return nil
	}
	value, exists := c.Get(string(ContextKeyAdminPermissions))
	if !exists {
		return nil
	}
	permissions, _ := value.([]string)
	return append([]string(nil), permissions...)
}

func hasAdminPermission(c *gin.Context, required string) bool {
	for _, permission := range GetAdminPermissionsFromContext(c) {
		if permission == "*" || permission == required {
			return true
		}
	}
	return false
}

func requiredAdminPermission(method, path string) string {
	path = strings.Trim(strings.TrimPrefix(path, "/api/v1/admin/"), "/")
	parts := strings.Split(path, "/")
	read := method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
	switch parts[0] {
	case "chat":
		if strings.HasSuffix(path, "/balance-transfers") {
			return service.PermissionSupportTransfer
		}
		if read {
			return service.PermissionSupportRead
		}
		return service.PermissionSupportWrite
	case "users":
		if path == "users/permission-groups" {
			return service.PermissionUsersManage
		}
		if strings.HasSuffix(path, "/api-keys") || strings.HasSuffix(path, "/auth-identities") {
			return service.PermissionUsersCredentials
		}
		if strings.HasSuffix(path, "/basic") {
			return service.PermissionUsersReadBasic
		}
		if strings.Contains(path, "/balance") || strings.Contains(path, "/platform-quotas") || strings.HasSuffix(path, "/rpm-status") || strings.HasSuffix(path, "/batch-limits") || strings.HasSuffix(path, "/batch-concurrency") || strings.HasSuffix(path, "/replace-group") {
			return service.PermissionUsersBilling
		}
		return service.PermissionUsersManage
	case "dashboard":
		return service.PermissionDashboardRead
	case "settings":
		// Delegated settings access must not mint global keys or change its own role grants.
		if strings.HasPrefix(path, "settings/admin-api-key") || path == "settings/permission-groups" {
			return ""
		}
		return service.PermissionSettingsManage
	case "groups":
		return service.PermissionGroupsManage
	case "accounts", "account-inspection", "proxies", "egress":
		return service.PermissionAccountsManage
	default:
		return ""
	}
}
