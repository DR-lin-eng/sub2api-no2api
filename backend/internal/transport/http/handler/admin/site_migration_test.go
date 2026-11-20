package admin

import (
	"github.com/DR-lin-eng/sub2api-no2api/internal/transport/http/server/middleware"

	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/modules/sitemigration"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSiteMigrationOnlyExplicitCredentialAndTLS(t *testing.T) {
	for _, tt := range []struct {
		protocol, mode, profile, key, method, scope string
		want                                        int
	}{
		{"https", "export", "all", "id", "admin_api_key", "admin.read", 403},
		{"https", "export", "all", "id", "jwt", "admin.migration.export", 403},
		{"https", "export", "all", "legacy", "admin_api_key", "admin.migration.export", 403},
		{"https", "export", "all", "id", "admin_api_key", "admin.migration.import", 403},
		{"http", "export", "all", "id", "admin_api_key", "admin.migration.export", 403},
		{"https", "export", "all", "id", "admin_api_key", "admin.migration.users.export", 403},
		{"https", "export", "clone", "id", "admin_api_key", "admin.migration.users.export", 403},
		{"https", "export", "login-quota", "id", "admin_api_key", "admin.migration.users.export", 200},
		{"https", "export", "login", "id", "admin_api_key", "admin.migration.users.export", 200},
		{"https", "export", "clone", "id", "admin_api_key", "admin.migration.export", 200},
	} {
		t.Run(strings.Join([]string{tt.protocol, tt.profile, tt.key, tt.method, tt.scope}, "/"), func(t *testing.T) {
			r := gin.New()
			r.POST("/:mode/:action", func(c *gin.Context) {
				c.Set("auth_method", tt.method)
				c.Set("admin_api_key_id", tt.key)
				c.Set("admin_api_key_scopes", []string{tt.scope})
			}, SiteMigration(sitemigration.Store{}))
			req := httptest.NewRequest(http.MethodPost, tt.protocol+"://example.test/"+tt.mode+"/probe?scope="+tt.profile, nil)
			req.Header.Set("x-api-key", "synthetic key")
			req.Header.Set("X-Forwarded-Proto", "https")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, tt.want, w.Code)
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		})
	}
}

func TestMigrationConsoleRequiresHumanAdministratorOverTLS(t *testing.T) {
	for _, tc := range []struct{ role, method, scheme string }{
		{"admin", "admin_api_key", "https"}, {"admin", "mcp_oauth", "https"}, {"support", "jwt", "https"}, {"user", "jwt", "https"}, {"admin", "jwt", "http"},
	} {
		r := gin.New()
		r.GET("/jobs", func(c *gin.Context) {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
			c.Set(string(middleware.ContextKeyUserRole), tc.role)
			c.Set("auth_method", tc.method)
			c.Next()
		}, SiteMigrationPanel(nil, nil))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", tc.scheme+"://example.test/jobs", nil))
		require.Equal(t, 403, w.Code, tc)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
}
