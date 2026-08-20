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
		{http.MethodGet, "/api/v1/admin/account-inspection", []string{service.AdminAPIKeyScopeAccountsRead}, true},
		{http.MethodPost, "/api/v1/admin/account-inspection/run", []string{service.AdminAPIKeyScopeAccountsRead}, false},
		{http.MethodPost, "/api/v1/admin/account-inspection/run", []string{service.AdminAPIKeyScopeAccountsWrite}, true},
		{http.MethodGet, "/api/v1/admin/ops/concurrency", []string{service.AdminAPIKeyScopeRead}, true},
		{http.MethodPost, "/api/v1/admin/redeem-codes/export-generated", []string{service.AdminAPIKeyScopeRead}, true},
		{http.MethodGet, "/api/v1/admin/chat/conversations", []string{service.AdminAPIKeyScopeRead}, false},
		{http.MethodPost, "/api/v1/admin/chat/conversations/1/messages", []string{service.AdminAPIKeyScopeWrite}, false},
		{http.MethodGet, "/api/v1/admin/chat/ws", []string{service.AdminAPIKeyScopeRead}, false},
	}
	for _, check := range checks {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		parsed, err := url.Parse(check.path)
		require.NoError(t, err)
		c.Request = &http.Request{Method: check.method, URL: parsed}
		require.Equal(t, check.allow, adminAPIKeyRequestAllowed(c, check.scopes), "%s %s", check.method, check.path)
	}
}

func TestGeneratedRedeemExportRequiresAdminAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(adminAuth(nil, nil, nil, nil))
	router.POST("/api/v1/admin/redeem-codes/export-generated", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/redeem-codes/export-generated", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Contains(t, recorder.Body.String(), "UNAUTHORIZED")
}

func TestAdminAPIKeyCannotReachAnySupportChatEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checks := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/admin/chat/conversations"},
		{http.MethodGet, "/api/v1/admin/chat/unread-count"},
		{http.MethodGet, "/api/v1/admin/chat/image-library"},
		{http.MethodPost, "/api/v1/admin/chat/image-library"},
		{http.MethodDelete, "/api/v1/admin/chat/image-library/1"},
		{http.MethodGet, "/api/v1/admin/chat/stickers"},
		{http.MethodPost, "/api/v1/admin/chat/stickers"},
		{http.MethodDelete, "/api/v1/admin/chat/stickers/1"},
		{http.MethodGet, "/api/v1/admin/chat/assets/1"},
		{http.MethodGet, "/api/v1/admin/chat/conversations/1/messages"},
		{http.MethodPost, "/api/v1/admin/chat/conversations/1/messages"},
		{http.MethodPost, "/api/v1/admin/chat/conversations/1/assets"},
		{http.MethodPost, "/api/v1/admin/chat/conversations/1/balance-transfers"},
		{http.MethodPost, "/api/v1/admin/chat/conversations/1/read"},
		{http.MethodGet, "/api/v1/admin/chat/quick-replies"},
		{http.MethodPost, "/api/v1/admin/chat/quick-replies"},
		{http.MethodPost, "/api/v1/admin/chat/quick-replies/import"},
		{http.MethodPost, "/api/v1/admin/chat/quick-replies/reorder"},
		{http.MethodPut, "/api/v1/admin/chat/quick-replies/1"},
		{http.MethodDelete, "/api/v1/admin/chat/quick-replies/1"},
		{http.MethodGet, "/api/v1/admin/chat/ws"},
	}
	for _, check := range checks {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		parsed, err := url.Parse(check.path)
		require.NoError(t, err)
		c.Request = &http.Request{Method: check.method, URL: parsed}
		require.Falsef(t, adminAPIKeyRequestAllowed(c, []string{
			service.AdminAPIKeyScopeRead,
			service.AdminAPIKeyScopeWrite,
		}), "%s %s must require a human admin JWT", check.method, check.path)
	}
}
