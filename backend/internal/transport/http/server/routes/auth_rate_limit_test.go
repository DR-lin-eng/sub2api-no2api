package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/platform/middleware"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newAuthRoutesTestRouter(redisClient *redis.Client) *gin.Engine {
	return newAuthRoutesTestRouterWithSettings(redisClient, nil)
}

func newAuthRoutesTestRouterWithSettings(redisClient *redis.Client, settingService *service.SettingService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.NewCredentialAuthIngressLimiter())
	v1 := router.Group("/api/v1")

	RegisterAuthRoutes(
		v1,
		&handler.Handlers{
			Auth:    &handler.AuthHandler{},
			Setting: &handler.SettingHandler{},
		},
		servermiddleware.JWTAuthMiddleware(func(c *gin.Context) {
			c.Next()
		}),
		servermiddleware.AuditLogMiddleware(func(c *gin.Context) {
			c.Next()
		}),
		redisClient,
		nil,
		settingService,
	)

	return router
}

type authRouteSettingRepo struct {
	values map[string]string
}

func (r *authRouteSettingRepo) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}

func (r *authRouteSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (r *authRouteSettingRepo) Set(context.Context, string, string) error { return nil }

func (r *authRouteSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

func (r *authRouteSettingRepo) SetMultiple(context.Context, map[string]string) error { return nil }
func (r *authRouteSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return r.values, nil
}
func (r *authRouteSettingRepo) Delete(context.Context, string) error { return nil }

func newAuthRouteSettingService(values map[string]string) *service.SettingService {
	return service.NewSettingService(&authRouteSettingRepo{values: values}, &config.Config{})
}

func TestAuthRoutesRateLimitFailCloseWhenRedisUnavailable(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
	})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	router := newAuthRoutesTestRouter(rdb)
	paths := []string{
		"/api/v1/auth/login/2fa",
		"/api/v1/auth/send-verify-code",
		"/api/v1/auth/oauth/pending/send-verify-code",
	}

	for _, path := range paths {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.10:12345"

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusTooManyRequests, w.Code, "path=%s", path)
		require.Contains(t, w.Body.String(), "rate limit exceeded", "path=%s", path)
	}
}

func TestCredentialKeyRouteUsesBoundedLocalLimiterWithoutRedisCounter(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	router := newAuthRoutesTestRouter(rdb)
	for index := 0; index < 30; index++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/credential-key", nil)
		req.RemoteAddr = "203.0.113.20:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "request=%d", index+1)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/credential-key", nil)
	req.RemoteAddr = "203.0.113.20:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, "60", w.Header().Get("Retry-After"))

	keys := server.Keys()
	for _, key := range keys {
		require.NotContains(t, key, "rate_limit:auth-credential-key")
	}
}

func TestAuthRoutesRejectSimpleCredentialRequestsWithoutBrowserFlow(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	router := newAuthRoutesTestRouter(rdb)

	for _, path := range []string{"/api/v1/auth/login", "/api/v1/auth/register"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"email":"user@example.com","password":"secret-123"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.30:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusForbidden, w.Code, "path=%s", path)
		require.Contains(t, w.Body.String(), "CREDENTIAL_BROWSER_FLOW_REQUIRED", "path=%s", path)
	}
	for _, key := range server.Keys() {
		require.NotContains(t, key, "rate_limit:auth-login")
		require.NotContains(t, key, "rate_limit:auth-register")
	}
}

func TestAuthRoutesLocalCaptchaDefaultsOff(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	router := newAuthRoutesTestRouterWithSettings(rdb, newAuthRouteSettingService(map[string]string{}))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/captcha", nil))

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "captcha protection is not enabled")
}

func TestAuthRoutesBrowserFlowPrecedesLocalCaptcha(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	router := newAuthRoutesTestRouterWithSettings(rdb, newAuthRouteSettingService(map[string]string{
		service.SettingKeyLocalCaptchaEnabled: "true",
	}))

	body := strings.NewReader(`{"email":"user@example.com","password":"secret-123"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "CREDENTIAL_BROWSER_FLOW_REQUIRED")
}

func TestAuthRoutesTurnstileTakesPriorityOverLocalCaptcha(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	router := newAuthRoutesTestRouterWithSettings(rdb, newAuthRouteSettingService(map[string]string{
		service.SettingKeyLocalCaptchaEnabled: "true",
		service.SettingKeyTurnstileEnabled:    "true",
	}))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/captcha", nil))

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "captcha protection is not enabled")
}

func TestAuthRoutesLocalCaptchaProtectsRecoveryAndPendingOAuthEmail(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	router := newAuthRoutesTestRouterWithSettings(rdb, newAuthRouteSettingService(map[string]string{
		service.SettingKeyLocalCaptchaEnabled: "true",
	}))

	for _, path := range []string{
		"/api/v1/auth/forgot-password",
		"/api/v1/auth/oauth/pending/send-verify-code",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"email":"user@example.com"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code, "path=%s", path)
		require.Contains(t, w.Body.String(), "LOCAL_CAPTCHA_REQUIRED", "path=%s", path)
	}
}
