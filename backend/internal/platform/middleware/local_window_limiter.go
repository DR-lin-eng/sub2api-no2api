package middleware

import (
	"hash/maphash"
	"net/http"
	"strconv"
	"sync"
	"time"

	clientip "github.com/Wei-Shaw/sub2api/internal/shared/ip"
	"github.com/gin-gonic/gin"
)

const localWindowLimiterShardCount = 64

const (
	credentialAuthLocalMaxSources = 4096 // About 64 KiB of fixed entry storage per limiter.
	credentialKeyGlobalLimit      = 6000 // Per instance and minute.
	credentialSubmitGlobalLimit   = 3000 // Per instance and minute.
	credentialKeyPath             = "/api/v1/auth/credential-key"
	credentialLoginPath           = "/api/v1/auth/login"
	credentialRegisterPath        = "/api/v1/auth/register"
	credentialKeyMaxInFlight      = 128
	credentialSubmitMaxInFlight   = 64
)

type localWindowLimiterShard struct {
	mu       sync.Mutex
	windowID int64
	entries  []localWindowLimiterEntry
}

type localWindowLimiterEntry struct {
	key      uint64
	count    uint32
	occupied bool
}

// BoundedLocalWindowLimiter sheds request floods before they reach shared
// infrastructure. Its fixed sharded slots reset every window and never grow.
type BoundedLocalWindowLimiter struct {
	limit           uint32
	globalLimit     uint32
	window          time.Duration
	entriesPerShard int
	seed            maphash.Seed
	now             func() time.Time
	globalMu        sync.Mutex
	globalWindowID  int64
	globalCount     uint32
	shards          [localWindowLimiterShardCount]localWindowLimiterShard
}

func NewBoundedLocalWindowLimiter(limit int, globalLimit int, window time.Duration, maxEntries int) *BoundedLocalWindowLimiter {
	if limit < 1 {
		limit = 1
	}
	if globalLimit < 1 {
		globalLimit = 1
	}
	if window < time.Second {
		window = time.Second
	}
	if maxEntries < localWindowLimiterShardCount {
		maxEntries = localWindowLimiterShardCount
	}
	limiter := &BoundedLocalWindowLimiter{
		limit:           uint32(limit),
		globalLimit:     uint32(globalLimit),
		window:          window,
		entriesPerShard: (maxEntries + localWindowLimiterShardCount - 1) / localWindowLimiterShardCount,
		seed:            maphash.MakeSeed(),
		now:             time.Now,
	}
	for index := range limiter.shards {
		limiter.shards[index].entries = make([]localWindowLimiterEntry, limiter.entriesPerShard)
	}
	return limiter
}

// NewCredentialAuthIngressLimiter protects the public credential endpoints
// before request logging, audit capture, Redis rate limiting, or RSA work.
func NewCredentialAuthIngressLimiter() gin.HandlerFunc {
	keyLimiter := NewBoundedLocalWindowLimiter(30, credentialKeyGlobalLimit, time.Minute, credentialAuthLocalMaxSources)
	submitLimiter := NewBoundedLocalWindowLimiter(30, credentialSubmitGlobalLimit, time.Minute, credentialAuthLocalMaxSources)
	keyHandler := keyLimiter.Limit()
	submitHandler := submitLimiter.Limit()
	keySlots := make(chan struct{}, credentialKeyMaxInFlight)
	submitSlots := make(chan struct{}, credentialSubmitMaxInFlight)
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		switch {
		case c.Request.Method == http.MethodGet && path == credentialKeyPath:
			limitCredentialConcurrency(c, keySlots, keyHandler)
		case c.Request.Method == http.MethodPost && (path == credentialLoginPath || path == credentialRegisterPath):
			limitCredentialConcurrency(c, submitSlots, submitHandler)
		default:
			c.Next()
		}
	}
}

func limitCredentialConcurrency(c *gin.Context, slots chan struct{}, next gin.HandlerFunc) {
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
		next(c)
	default:
		abortLocalWindowLimit(c, time.Second)
	}
}

func (l *BoundedLocalWindowLimiter) Limit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.allow(clientip.GetClientIP(c)) {
			abortLocalWindowLimit(c, l.window)
			return
		}
		c.Next()
	}
}

func abortLocalWindowLimit(c *gin.Context, retryAfter time.Duration) {
	seconds := int64(retryAfter / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	c.Header("Retry-After", strconv.FormatInt(seconds, 10))
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error":   "rate limit exceeded",
		"message": "Too many requests, please try again later",
	})
}

func (l *BoundedLocalWindowLimiter) allow(clientKey string) bool {
	hashedKey := maphash.String(l.seed, clientKey)
	shard := &l.shards[hashedKey&(localWindowLimiterShardCount-1)]
	windowID := l.now().UnixNano() / l.window.Nanoseconds()

	shard.mu.Lock()
	defer shard.mu.Unlock()
	if shard.windowID != windowID {
		clear(shard.entries)
		shard.windowID = windowID
	}
	start := int((hashedKey >> 6) % uint64(len(shard.entries)))
	for offset := range shard.entries {
		entry := &shard.entries[(start+offset)%len(shard.entries)]
		if !entry.occupied {
			entry.key = hashedKey
			entry.count = 1
			entry.occupied = true
			return l.allowGlobal(windowID)
		}
		if entry.key == hashedKey {
			if entry.count >= l.limit {
				return false
			}
			entry.count++
			return l.allowGlobal(windowID)
		}
	}
	return false
}

func (l *BoundedLocalWindowLimiter) allowGlobal(windowID int64) bool {
	l.globalMu.Lock()
	defer l.globalMu.Unlock()
	if l.globalWindowID != windowID {
		l.globalWindowID = windowID
		l.globalCount = 0
	}
	if l.globalCount >= l.globalLimit {
		return false
	}
	l.globalCount++
	return true
}
