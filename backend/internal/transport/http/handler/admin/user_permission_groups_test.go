package admin

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestManagedUserGuardBlocksStaffTargetsAndPreservesBatchBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &UserHandler{adminService: &stubAdminService{users: []service.User{{ID: 1, Role: "admin"}, {ID: 2, Role: "user"}, {ID: 3, Role: "support"}}}}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(string(middleware.ContextKeyUserRole), "support"); c.Next() })
	r.POST("/api/v1/admin/users/:id/auth-identities", h.RequireManagedUser, func(c *gin.Context) { c.Status(204) })
	r.DELETE("/api/v1/admin/users/:id", h.RequireManagedUser, func(c *gin.Context) { c.Status(204) })
	r.POST("/api/v1/admin/users/batch-limits", h.RequireManagedUser, func(c *gin.Context) {
		b, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		c.Data(200, "application/json", b)
	})
	for _, tt := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/api/v1/admin/users/1/auth-identities", `{}`, 403},
		{"DELETE", "/api/v1/admin/users/3", ``, 403},
		{"DELETE", "/api/v1/admin/users/2", ``, 204},
		{"POST", "/api/v1/admin/users/batch-limits", `{"all":true}`, 403},
		{"POST", "/api/v1/admin/users/batch-limits", `{"user_ids":[2,1],"concurrency":2}`, 403},
		{"POST", "/api/v1/admin/users/batch-limits", `{"user_ids":[2],"concurrency":2}`, 200},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		require.Equal(t, tt.status, w.Code, tt.path)
		if tt.status == 200 {
			require.JSONEq(t, tt.body, w.Body.String())
		}
	}
}

func TestDelegatedUserFieldsNeedAdditionalPermissions(t *testing.T) {
	for _, tt := range []struct {
		permissions                []string
		credentials, billing, want bool
	}{
		{nil, false, false, true}, {nil, true, false, false}, {nil, false, true, false},
		{[]string{service.PermissionUsersCredentials}, true, false, true},
		{[]string{service.PermissionUsersBilling}, false, true, true},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(string(middleware.ContextKeyUserRole), "custom")
		middleware.SetAdminPermissions(c, tt.permissions)
		require.Equal(t, tt.want, delegatedUserFieldsAllowed(c, tt.credentials, tt.billing))
	}
}

func TestSupportBasicProfileIsAllowlisted(t *testing.T) {
	b, err := json.Marshal(BasicUser{})
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(b, &fields))
	require.Len(t, fields, 13)
	for _, field := range []string{"api_keys", "password", "auth_identities", "balance_notify_extra_emails", "subscriptions", "group_rates"} {
		require.NotContains(t, fields, field)
	}
}
