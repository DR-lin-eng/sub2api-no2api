package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	platformmiddleware "github.com/Wei-Shaw/sub2api/internal/platform/middleware"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type panelRateLimitStubRepo struct {
	mu     sync.Mutex
	values map[string]string
}

func (r *panelRateLimitStubRepo) Get(_ context.Context, key string) (*service.Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return nil, service.ErrSettingNotFound
	}
	return &service.Setting{Key: key, Value: value}, nil
}

func (r *panelRateLimitStubRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (r *panelRateLimitStubRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *panelRateLimitStubRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (r *panelRateLimitStubRepo) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		if err := r.Set(context.Background(), key, value); err != nil {
			return err
		}
	}
	return nil
}

func (r *panelRateLimitStubRepo) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *panelRateLimitStubRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
}

type fakePanelAllower struct {
	mu         sync.Mutex
	counts     map[string]int64
	batchCalls int
	allowCalls int
	err        error
}

func (f *fakePanelAllower) Allow(_ context.Context, key string, limit int, _ time.Duration) (platformmiddleware.AllowResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.allowCalls++
	if f.err != nil {
		return platformmiddleware.AllowResult{}, f.err
	}
	if f.counts == nil {
		f.counts = make(map[string]int64)
	}
	f.counts[key]++
	count := f.counts[key]
	return platformmiddleware.AllowResult{Allowed: count <= int64(limit), Count: count, RetryAfter: 30 * time.Second}, nil
}

func (f *fakePanelAllower) AllowMany(_ context.Context, rules []platformmiddleware.RateLimitRule, _ time.Duration) (platformmiddleware.AllowManyResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batchCalls++
	if f.err != nil {
		return platformmiddleware.AllowManyResult{}, f.err
	}
	if f.counts == nil {
		f.counts = make(map[string]int64)
	}
	result := platformmiddleware.AllowManyResult{Allowed: true, Counts: make([]int64, len(rules))}
	blocked := false
	for i, rule := range rules {
		if blocked {
			continue
		}
		f.counts[rule.Key]++
		result.Counts[i] = f.counts[rule.Key]
		if result.Counts[i] > int64(rule.Limit) {
			result.Allowed = false
			result.RetryAfter = 30 * time.Second
			blocked = true
		}
	}
	return result, nil
}

func newPanelRateLimitTestService(t *testing.T, settings service.PanelRateLimitSettings) *service.SettingService {
	t.Helper()
	svc := service.NewSettingService(&panelRateLimitStubRepo{}, &config.Config{})
	require.NoError(t, svc.SetPanelRateLimitSettings(context.Background(), &settings))
	return svc
}

func newPanelTestRouter(path string, middleware gin.HandlerFunc, userID int64, role string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if userID > 0 {
		router.Use(func(c *gin.Context) {
			setAuthSubject(c, userID, 1, service.RequestSchedulingTierNormal)
			c.Set(string(ContextKeyUserRole), role)
			c.Next()
		})
	}
	router.Use(middleware)
	router.GET(path, func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

func performPanelRequest(router http.Handler, path, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remoteAddr
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestPanelRateLimiterAuthenticatedUsesUserBucket(t *testing.T) {
	allower := &fakePanelAllower{}
	p := &PanelRateLimiter{
		limiter: allower,
		settingService: newPanelRateLimitTestService(t, service.PanelRateLimitSettings{
			Enabled: true, UserRPM: 2, HeavyRPM: 1,
		}),
	}
	router := newPanelTestRouter("/api/v1/user/profile", p.Authenticated(), 7, service.RoleUser)
	require.Equal(t, http.StatusOK, performPanelRequest(router, "/api/v1/user/profile", "127.0.0.1:1").Code)
	require.Equal(t, http.StatusOK, performPanelRequest(router, "/api/v1/user/profile", "203.0.113.8:1").Code)
	limited := performPanelRequest(router, "/api/v1/user/profile", "198.51.100.8:1")
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	require.Equal(t, "30", limited.Header().Get("Retry-After"))
	require.Equal(t, int64(3), allower.counts["panel:global:user:7"])
}

func TestPanelRateLimiterHeavyUsesOneBatchForBothBuckets(t *testing.T) {
	allower := &fakePanelAllower{}
	p := &PanelRateLimiter{
		limiter: allower,
		settingService: newPanelRateLimitTestService(t, service.PanelRateLimitSettings{
			Enabled: true, UserRPM: 100, HeavyRPM: 1,
		}),
	}
	path := "/api/v1/usage/dashboard/stats"
	router := newPanelTestRouter(path, p.Authenticated(), 9, service.RoleUser)
	require.Equal(t, http.StatusOK, performPanelRequest(router, path, "127.0.0.1:1").Code)
	require.Equal(t, http.StatusTooManyRequests, performPanelRequest(router, path, "127.0.0.1:1").Code)

	allower.mu.Lock()
	defer allower.mu.Unlock()
	require.Equal(t, 2, allower.batchCalls, "each request must use one batch, not one call per bucket")
	require.Equal(t, int64(2), allower.counts["panel:global:user:9"])
	require.Equal(t, int64(2), allower.counts["panel:heavy:user:9"])
}

func TestPanelRateLimiterGlobalRejectionDoesNotConsumeHeavyBucket(t *testing.T) {
	allower := &fakePanelAllower{}
	p := &PanelRateLimiter{
		limiter: allower,
		settingService: newPanelRateLimitTestService(t, service.PanelRateLimitSettings{
			Enabled: true, UserRPM: 1, HeavyRPM: 100,
		}),
	}
	path := "/api/v1/usage/dashboard/stats"
	router := newPanelTestRouter(path, p.Authenticated(), 10, service.RoleUser)
	require.Equal(t, http.StatusOK, performPanelRequest(router, path, "127.0.0.1:1").Code)
	require.Equal(t, http.StatusTooManyRequests, performPanelRequest(router, path, "127.0.0.1:1").Code)

	allower.mu.Lock()
	defer allower.mu.Unlock()
	require.Equal(t, int64(2), allower.counts["panel:global:user:10"])
	require.Equal(t, int64(1), allower.counts["panel:heavy:user:10"])
}

func TestPanelRateLimiterDisabledDoesNotTouchRedis(t *testing.T) {
	allower := &fakePanelAllower{}
	p := &PanelRateLimiter{
		limiter: allower,
		settingService: newPanelRateLimitTestService(t, service.PanelRateLimitSettings{
			Enabled: false, UserRPM: 1, HeavyRPM: 1, PublicIPRPM: 1,
		}),
	}
	path := "/api/v1/usage"
	router := newPanelTestRouter(path, p.Authenticated(), 3, service.RoleUser)
	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusOK, performPanelRequest(router, path, "203.0.113.9:1").Code)
	}
	require.Zero(t, allower.batchCalls)
	require.Zero(t, allower.allowCalls)
}

func TestPanelRateLimiterAdminExemption(t *testing.T) {
	allower := &fakePanelAllower{}
	p := &PanelRateLimiter{
		limiter: allower,
		settingService: newPanelRateLimitTestService(t, service.PanelRateLimitSettings{
			Enabled: true, UserRPM: 1, HeavyRPM: 1, ExemptAdmin: true,
		}),
	}
	path := "/api/v1/admin/dashboard/stats"
	router := newPanelTestRouter(path, p.Authenticated(), 11, service.RoleAdmin)
	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusOK, performPanelRequest(router, path, "127.0.0.1:1").Code)
	}
	require.Zero(t, allower.batchCalls)
}

func TestPanelRateLimiterRedisFailureUsesShortCircuitBackoff(t *testing.T) {
	allower := &fakePanelAllower{err: errors.New("redis unavailable")}
	p := &PanelRateLimiter{
		limiter: allower,
		settingService: newPanelRateLimitTestService(t, service.PanelRateLimitSettings{
			Enabled: true, UserRPM: 1,
		}),
	}
	path := "/api/v1/user/profile"
	router := newPanelTestRouter(path, p.Authenticated(), 5, service.RoleUser)
	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusOK, performPanelRequest(router, path, "127.0.0.1:1").Code)
	}
	require.Equal(t, 1, allower.batchCalls)
}

func TestPanelRateLimiterPublicIP(t *testing.T) {
	allower := &fakePanelAllower{}
	p := &PanelRateLimiter{
		limiter: allower,
		settingService: newPanelRateLimitTestService(t, service.PanelRateLimitSettings{
			Enabled: true, PublicIPRPM: 1,
		}),
	}
	path := "/api/v1/settings/public"
	router := newPanelTestRouter(path, p.PublicIP(), 0, "")
	require.Equal(t, http.StatusOK, performPanelRequest(router, path, "203.0.113.9:1").Code)
	require.Equal(t, http.StatusTooManyRequests, performPanelRequest(router, path, "203.0.113.9:1").Code)
	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusOK, performPanelRequest(router, path, "127.0.0.1:1").Code)
	}
}

func TestPanelHeavyPathCoverage(t *testing.T) {
	for _, path := range []string{
		"/api/v1/usage/dashboard/stats",
		"/api/v1/user/api-keys/:id/usage/daily",
		"/api/v1/admin/usage/stats",
		"/api/v1/admin/dashboard/snapshot-v2",
		"/api/v1/admin/ops/dashboard/overview",
		"/api/v1/admin/payment/dashboard",
		"/api/v1/admin/payment/orders",
		"/api/v1/chat/messages",
		"/api/v1/admin/chat/conversations/:id/messages",
	} {
		require.Truef(t, isPanelHeavyPath(path), "expected heavy route %s", path)
	}
	for _, path := range []string{
		"/api/v1/user/profile",
		"/api/v1/auth/me",
		"/api/v1/payment/webhook/stripe",
	} {
		require.Falsef(t, isPanelHeavyPath(path), "expected light/public route %s", path)
	}
}

func TestPanelRateLimiterAdminPaymentAndOpsRoutesConsumeHeavyBucket(t *testing.T) {
	for _, path := range []string{
		"/api/v1/admin/payment/dashboard",
		"/api/v1/admin/ops/dashboard/overview",
	} {
		t.Run(path, func(t *testing.T) {
			allower := &fakePanelAllower{}
			p := &PanelRateLimiter{
				limiter: allower,
				settingService: newPanelRateLimitTestService(t, service.PanelRateLimitSettings{
					Enabled: true, UserRPM: 100, HeavyRPM: 10,
				}),
			}
			router := newPanelTestRouter(path, p.Authenticated(), 21, service.RoleAdmin)
			require.Equal(t, http.StatusOK, performPanelRequest(router, path, "127.0.0.1:1").Code)

			allower.mu.Lock()
			defer allower.mu.Unlock()
			require.Equal(t, 1, allower.batchCalls)
			require.Equal(t, int64(1), allower.counts["panel:global:user:21"])
			require.Equal(t, int64(1), allower.counts["panel:heavy:user:21"])
		})
	}
}

func TestIsPubliclyRoutableClientIP(t *testing.T) {
	require.True(t, isPubliclyRoutableClientIP("203.0.113.9"))
	require.True(t, isPubliclyRoutableClientIP("2001:db8::1"))
	for _, value := range []string{"127.0.0.1", "::1", "10.1.2.3", "172.16.0.1", "192.168.0.1", "169.254.1.1", "fe80::1", "fc00::1", "0.0.0.0", "", "not-an-ip"} {
		require.Falsef(t, isPubliclyRoutableClientIP(value), "expected non-routable address %q", value)
	}
}

func BenchmarkPanelRateLimiterDisabledFastPath(b *testing.B) {
	benchmarkPanelRateLimiter(b, service.PanelRateLimitSettings{
		Enabled: false, UserRPM: 100, HeavyRPM: 20,
	}, &fakePanelAllower{}, false)
}

func BenchmarkPanelRateLimiterEnabledRedisSuccess(b *testing.B) {
	miniRedis, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(miniRedis.Close)
	redisClient := redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
	b.Cleanup(func() { _ = redisClient.Close() })

	settings := service.PanelRateLimitSettings{Enabled: true, UserRPM: 100000, HeavyRPM: 100000}
	svc := service.NewSettingService(&panelRateLimitStubRepo{}, &config.Config{})
	if err := svc.SetPanelRateLimitSettings(context.Background(), &settings); err != nil {
		b.Fatal(err)
	}
	p := NewPanelRateLimiter(redisClient, svc)
	path := "/api/v1/usage/dashboard/stats"
	router := newPanelTestRouter(path, p.Authenticated(), 77, service.RoleUser)
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.RemoteAddr = "127.0.0.1:1"
	recorder := httptest.NewRecorder()

	// Prime SCRIPT LOAD and exclude it from the steady-state measurement.
	router.ServeHTTP(recorder, request)
	miniRedis.FlushAll()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i > 0 && i%90000 == 0 {
			b.StopTimer()
			miniRedis.FlushAll()
			b.StartTimer()
		}
		recorder.Body.Reset()
		recorder.Code = http.StatusOK
		router.ServeHTTP(recorder, request)
	}
}

func BenchmarkPanelRateLimiterRedisBackoffFastPath(b *testing.B) {
	benchmarkPanelRateLimiter(b, service.PanelRateLimitSettings{
		Enabled: true, UserRPM: 100, HeavyRPM: 20,
	}, &fakePanelAllower{err: errors.New("redis unavailable")}, true)
}

func benchmarkPanelRateLimiter(b *testing.B, settings service.PanelRateLimitSettings, allower *fakePanelAllower, primeBackoff bool) {
	b.Helper()
	svc := service.NewSettingService(&panelRateLimitStubRepo{}, &config.Config{})
	if err := svc.SetPanelRateLimitSettings(context.Background(), &settings); err != nil {
		b.Fatal(err)
	}
	p := &PanelRateLimiter{limiter: allower, settingService: svc}
	path := "/api/v1/usage/dashboard/stats"
	router := newPanelTestRouter(path, p.Authenticated(), 77, service.RoleUser)
	if primeBackoff {
		response := performPanelRequest(router, path, "127.0.0.1:1")
		if response.Code != http.StatusOK {
			b.Fatalf("prime request status = %d", response.Code)
		}
		// Keep the benchmark focused on the circuit-open fast path even when a
		// long benchmark run exceeds the production two-second retry interval.
		p.redisRetryAt.Store(time.Now().Add(time.Hour).UnixNano())
	}

	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.RemoteAddr = "127.0.0.1:1"
	recorder := httptest.NewRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recorder.Body.Reset()
		recorder.Code = http.StatusOK
		router.ServeHTTP(recorder, request)
	}
}
