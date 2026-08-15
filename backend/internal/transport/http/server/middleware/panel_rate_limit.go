package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	platformmiddleware "github.com/Wei-Shaw/sub2api/internal/platform/middleware"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	panelRateLimitWindow          = time.Minute
	panelRateLimitRedisBackoff    = 2 * time.Second
	panelRateLimitFailureLogEvery = 30 * time.Second
)

var panelHeavyPathPrefixes = [...]string{
	"/api/v1/usage",
	"/api/v1/admin/usage",
	"/api/v1/admin/dashboard",
	"/api/v1/admin/ops",
	"/api/v1/admin/payment",
	"/api/v1/chat",
	"/api/v1/admin/chat",
}

type panelRateLimitAllower interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (platformmiddleware.AllowResult, error)
	AllowMany(ctx context.Context, rules []platformmiddleware.RateLimitRule, window time.Duration) (platformmiddleware.AllowManyResult, error)
}

// PanelRateLimiter protects browser-facing APIs. Authenticated requests are
// keyed by user ID; public settings requests use a routable client IP. Redis
// failures fail open and briefly open a local circuit to avoid retrying Redis
// on every request while it is unavailable.
type PanelRateLimiter struct {
	limiter        panelRateLimitAllower
	settingService *service.SettingService
	redisRetryAt   atomic.Int64
	lastFailureLog atomic.Int64
}

func NewPanelRateLimiter(redisClient *redis.Client, settingService *service.SettingService) *PanelRateLimiter {
	return &PanelRateLimiter{
		limiter:        platformmiddleware.NewRateLimiter(redisClient),
		settingService: settingService,
	}
}

// Authenticated applies the global bucket to every authenticated panel route
// and adds the heavy bucket based on Gin's registered route template. Both
// buckets are checked by one Redis EVAL.
func (p *PanelRateLimiter) Authenticated() gin.HandlerFunc {
	return func(c *gin.Context) {
		if p == nil || p.limiter == nil || p.settingService == nil {
			c.Next()
			return
		}
		settings := p.settingService.GetPanelRateLimitSettingsCached(c.Request.Context())
		if !settings.Enabled {
			c.Next()
			return
		}
		if p.redisCircuitOpen() {
			c.Next()
			return
		}

		subject, ok := GetAuthSubjectFromContext(c)
		if !ok || subject.UserID <= 0 {
			c.Next()
			return
		}
		if settings.ExemptAdmin {
			if role, hasRole := GetUserRoleFromContext(c); hasRole && role == service.RoleAdmin {
				c.Next()
				return
			}
		}

		userID := strconv.FormatInt(subject.UserID, 10)
		rules := make([]platformmiddleware.RateLimitRule, 0, 2)
		if settings.UserRPM > 0 {
			rules = append(rules, platformmiddleware.RateLimitRule{
				Key:   "panel:global:user:" + userID,
				Limit: settings.UserRPM,
			})
		}
		if settings.HeavyRPM > 0 && isPanelHeavyPath(c.FullPath()) {
			rules = append(rules, platformmiddleware.RateLimitRule{
				Key:   "panel:heavy:user:" + userID,
				Limit: settings.HeavyRPM,
			})
		}
		if len(rules) == 0 {
			c.Next()
			return
		}

		result, err := p.limiter.AllowMany(c.Request.Context(), rules, panelRateLimitWindow)
		if err != nil {
			p.recordRedisFailure("authenticated", err)
			c.Next()
			return
		}
		p.redisRetryAt.Store(0)
		if !result.Allowed {
			abortPanelRateLimited(c, result.RetryAfter)
			return
		}
		c.Next()
	}
}

// PublicIP limits unauthenticated settings endpoints. Private and loopback
// addresses are skipped so a reverse proxy's internal address cannot collapse
// all clients into one bucket.
func (p *PanelRateLimiter) PublicIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		if p == nil || p.limiter == nil || p.settingService == nil {
			c.Next()
			return
		}
		settings := p.settingService.GetPanelRateLimitSettingsCached(c.Request.Context())
		if !settings.Enabled || settings.PublicIPRPM <= 0 || p.redisCircuitOpen() {
			c.Next()
			return
		}
		clientIP := SecurityClientIP(c)
		if !isPubliclyRoutableClientIP(clientIP) {
			c.Next()
			return
		}

		result, err := p.limiter.Allow(c.Request.Context(), "panel:public:ip:"+clientIP, settings.PublicIPRPM, panelRateLimitWindow)
		if err != nil {
			p.recordRedisFailure("public", err)
			c.Next()
			return
		}
		p.redisRetryAt.Store(0)
		if !result.Allowed {
			abortPanelRateLimited(c, result.RetryAfter)
			return
		}
		c.Next()
	}
}

func (p *PanelRateLimiter) redisCircuitOpen() bool {
	return p != nil && time.Now().UnixNano() < p.redisRetryAt.Load()
}

func (p *PanelRateLimiter) recordRedisFailure(scope string, err error) {
	now := time.Now().UnixNano()
	p.redisRetryAt.Store(time.Unix(0, now).Add(panelRateLimitRedisBackoff).UnixNano())
	for {
		last := p.lastFailureLog.Load()
		if last != 0 && now-last < panelRateLimitFailureLogEvery.Nanoseconds() {
			return
		}
		if p.lastFailureLog.CompareAndSwap(last, now) {
			slog.Warn("panel rate limit Redis check failed; allowing request",
				"scope", scope,
				"error", err,
			)
			return
		}
	}
}

func isPanelHeavyPath(path string) bool {
	for _, prefix := range panelHeavyPathPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	switch path {
	case "/api/v1/user/api-keys/:id/usage/daily",
		"/api/v1/admin/users/:id/usage",
		"/api/v1/admin/groups/usage-summary",
		"/api/v1/admin/accounts/:id/usage",
		"/api/v1/admin/accounts/:id/ollama-cloud-usage":
		return true
	default:
		return false
	}
}

func isPubliclyRoutableClientIP(clientIP string) bool {
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return ip.IsGlobalUnicast()
}

func abortPanelRateLimited(c *gin.Context, retryAfter time.Duration) {
	if retryAfter <= 0 {
		retryAfter = panelRateLimitWindow
	}
	seconds := int64(retryAfter / time.Second)
	if retryAfter%time.Second > 0 {
		seconds++
	}
	c.Header("Retry-After", strconv.FormatInt(seconds, 10))
	AbortWithError(c, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests, please slow down and try again later")
}
