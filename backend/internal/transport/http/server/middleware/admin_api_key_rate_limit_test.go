package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminAPIKeyRateLimitTransaction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	allower := &fakePanelAllower{}
	p := &PanelRateLimiter{limiter: allower, settingService: newPanelRateLimitTestService(t, service.PanelRateLimitSettings{
		Enabled: true, UserRPM: 1, HeavyRPM: 1, ExemptAdmin: true,
	})}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		setAuthSubject(c, 7, 1, service.RequestSchedulingTierNormal)
		c.Set(string(ContextKeyUserRole), service.RoleAdmin)
		if id := c.GetHeader("Fixture-Key-ID"); id != "" {
			c.Set("auth_method", "admin_api_key")
			c.Set("admin_api_key_id", id)
		}
		c.Next()
	})
	router.Use(p.Authenticated())
	router.GET("/api/v1/admin/usage", func(c *gin.Context) { c.Status(http.StatusOK) })
	for _, tc := range []struct {
		id   string
		want int
	}{{"key-a", 200}, {"key-b", 200}, {"key-a", 429}, {"", 200}, {"", 200}} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage", nil)
		r.Header.Set("Fixture-Key-ID", tc.id)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		t.Logf("key=%q status=%d", tc.id, w.Code)
		require.Equal(t, tc.want, w.Code)
	}
}
