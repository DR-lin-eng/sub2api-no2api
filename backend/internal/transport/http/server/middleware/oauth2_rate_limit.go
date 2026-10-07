package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	ratelimit "github.com/Wei-Shaw/sub2api/internal/platform/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const oauth2CredentialFailureLimit = 20
const oauth2CredentialIngressLimit = 300

// Token and revocation endpoints share failure counters so an attacker cannot
// alternate endpoints to reset its budget. A coarse ingress bucket also bounds
// database work for successful or malformed requests. Both use the resolved
// peer address, including private peers, and fail closed when Redis fails.
func OAuth2CredentialRateLimit(client *redis.Client) gin.HandlerFunc {
	limiter := ratelimit.NewRateLimiter(client)
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if client == nil {
			abortOAuth2Limit(c, http.StatusServiceUnavailable, "temporarily_unavailable")
			return
		}
		ip := SecurityClientIP(c)
		key := "rate_limit:oauth2:failures:" + ip
		failures, err := client.Get(ctx, key).Int()
		if err != nil && err != redis.Nil {
			abortOAuth2Limit(c, http.StatusServiceUnavailable, "temporarily_unavailable")
			return
		}
		if failures >= oauth2CredentialFailureLimit {
			abortOAuth2Limit(c, http.StatusTooManyRequests, "slow_down")
			return
		}
		allowed, err := limiter.Allow(ctx, "oauth2:ingress:"+ip, oauth2CredentialIngressLimit, time.Minute)
		if err != nil {
			abortOAuth2Limit(c, http.StatusServiceUnavailable, "temporarily_unavailable")
			return
		}
		if !allowed.Allowed {
			abortOAuth2Limit(c, http.StatusTooManyRequests, "slow_down")
			return
		}
		cancel()
		c.Next()
		status := c.Writer.Status()
		if status >= 400 && status < 500 {
			// Allow's Lua script increments and repairs TTL atomically across nodes.
			failureCtx, finish := context.WithTimeout(context.Background(), 2*time.Second)
			defer finish()
			_, _ = limiter.Allow(failureCtx, "oauth2:failures:"+ip, oauth2CredentialFailureLimit, time.Minute)
		}
	}
}

func abortOAuth2Limit(c *gin.Context, status int, code string) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Retry-After", strconv.Itoa(60))
	c.AbortWithStatusJSON(status, gin.H{"error": code, "error_description": "OAuth2 credential request limit reached; retry later"})
}
