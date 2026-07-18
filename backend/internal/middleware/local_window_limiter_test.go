package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBoundedLocalWindowLimiterLimitsAndResets(t *testing.T) {
	now := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	limiter := NewBoundedLocalWindowLimiter(2, 100, time.Minute, 64)
	limiter.now = func() time.Time { return now }

	require.True(t, limiter.allow("203.0.113.1"))
	require.True(t, limiter.allow("203.0.113.1"))
	require.False(t, limiter.allow("203.0.113.1"))

	now = now.Add(time.Minute)
	require.True(t, limiter.allow("203.0.113.1"))
}

func TestBoundedLocalWindowLimiterCapsTrackedSources(t *testing.T) {
	limiter := NewBoundedLocalWindowLimiter(1, 10_000, time.Minute, 64)
	for index := 0; index < 10_000; index++ {
		limiter.allow(fmt.Sprintf("198.51.100.%d", index))
	}

	tracked := 0
	totalSlots := 0
	for index := range limiter.shards {
		totalSlots += len(limiter.shards[index].entries)
		for _, entry := range limiter.shards[index].entries {
			if entry.occupied {
				tracked++
			}
		}
	}
	require.Equal(t, 64, totalSlots)
	require.LessOrEqual(t, tracked, 64)
}

func TestBoundedLocalWindowLimiterStopsBeforeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewBoundedLocalWindowLimiter(1, 100, time.Minute, 64)
	router := gin.New()
	router.Use(limiter.Limit())
	router.GET("/key", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/key", nil)
		req.RemoteAddr = "203.0.113.10:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	require.Equal(t, http.StatusNoContent, request().Code)
	limited := request()
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	require.Equal(t, "60", limited.Header().Get("Retry-After"))
}

func BenchmarkBoundedLocalWindowLimiterParallel(b *testing.B) {
	limiter := NewBoundedLocalWindowLimiter(int(^uint32(0)>>1), int(^uint32(0)>>1), time.Minute, 4096)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = limiter.allow("203.0.113.10")
		}
	})
}

func TestBoundedLocalWindowLimiterEnforcesGlobalLimit(t *testing.T) {
	limiter := NewBoundedLocalWindowLimiter(10, 3, time.Minute, 64)
	require.True(t, limiter.allow("203.0.113.1"))
	require.True(t, limiter.allow("203.0.113.2"))
	require.True(t, limiter.allow("203.0.113.3"))
	require.False(t, limiter.allow("203.0.113.4"))
}

func TestCredentialAuthIngressLimiterCapsConcurrentSubmissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(NewCredentialAuthIngressLimiter())
	entered := make(chan struct{}, credentialSubmitMaxInFlight)
	release := make(chan struct{})
	router.POST(credentialLoginPath, func(c *gin.Context) {
		entered <- struct{}{}
		<-release
		c.Status(http.StatusNoContent)
	})

	var wg sync.WaitGroup
	for index := 0; index < credentialSubmitMaxInFlight; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, credentialLoginPath, nil)
			req.RemoteAddr = fmt.Sprintf("198.51.100.%d:1234", index+1)
			router.ServeHTTP(httptest.NewRecorder(), req)
		}(index)
	}
	for range credentialSubmitMaxInFlight {
		<-entered
	}

	req := httptest.NewRequest(http.MethodPost, credentialLoginPath, nil)
	req.RemoteAddr = "203.0.113.250:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, "1", w.Header().Get("Retry-After"))

	close(release)
	wg.Wait()
}
