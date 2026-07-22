package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminAPIKeyScopePolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checks := []struct {
		method string
		path   string
		scopes []string
		allow  bool
	}{
		{http.MethodGet, "/api/v1/admin/users", []string{service.AdminAPIKeyScopeUsersRead}, true},
		{http.MethodPost, "/api/v1/admin/users", []string{service.AdminAPIKeyScopeUsersRead}, false},
		{http.MethodPost, "/api/v1/admin/users", []string{service.AdminAPIKeyScopeUsersWrite}, true},
		{http.MethodGet, "/api/v1/admin/settings/admin-api-keys", []string{service.AdminAPIKeyScopeSettingsRead}, true},
		{http.MethodDelete, "/api/v1/admin/settings/admin-api-keys/id", []string{service.AdminAPIKeyScopeSettingsRead}, false},
		{http.MethodGet, "/api/v1/admin/accounts/data", []string{service.AdminAPIKeyScopeRead}, false},
		{http.MethodGet, "/api/v1/admin/ops/concurrency", []string{service.AdminAPIKeyScopeRead}, true},
	}
	for _, check := range checks {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		parsed, err := url.Parse(check.path)
		require.NoError(t, err)
		c.Request = &http.Request{Method: check.method, URL: parsed}
		require.Equal(t, check.allow, adminAPIKeyRequestAllowed(c, check.scopes), "%s %s", check.method, check.path)
	}
}
