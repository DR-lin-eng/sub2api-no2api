package middleware

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestRequiredAdminPermissionOpenAIUsage(t *testing.T) {
	require.Equal(t, service.PermissionAccountsManage, requiredAdminPermission(http.MethodGet, "/api/v1/admin/openai/accounts/41/usage/ws"))
	require.Equal(t, service.PermissionAccountsManage, requiredAdminPermission(http.MethodPost, "/api/v1/admin/openai/accounts/41/usage/query"))
	for _, path := range []string{"/api/v1/admin/openai/accounts/41/reset-quota", "/api/v1/admin/openai/accounts/41/quota/refresh", "/api/v1/admin/openai/refresh-token", "/api/v1/admin/openai/accounts/41/usage/anything"} {
		require.Empty(t, requiredAdminPermission(http.MethodPost, path), "other OpenAI administrative actions keep their existing permissions")
	}
}
