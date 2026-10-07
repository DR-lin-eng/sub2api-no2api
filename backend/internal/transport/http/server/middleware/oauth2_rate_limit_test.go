package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOAuth2FailureLimiterSharesTokenAndRevokeBudget(t *testing.T) {
	store := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: store.Addr()})
	defer func() { _ = client.Close() }()
	router := gin.New()
	calls := 0
	for _, path := range []string{"/oauth/token", "/oauth/revoke"} {
		router.POST(path, OAuth2CredentialRateLimit(client), func(c *gin.Context) { calls++; c.JSON(401, gin.H{"error": "invalid_client"}) })
	}
	for i := 0; i < 20; i++ {
		path := "/oauth/token"
		if i%2 == 1 {
			path = "/oauth/revoke"
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.RemoteAddr = "10.1.2.3:1234"
		req.Header.Set("X-Forwarded-For", "8.8.8.8")
		router.ServeHTTP(rec, req)
		require.Equal(t, 401, rec.Code)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", nil)
	req.RemoteAddr = "10.1.2.3:4567"
	router.ServeHTTP(rec, req)
	require.Equal(t, 429, rec.Code)
	require.Equal(t, 20, calls)
	require.JSONEq(t, `{"error":"slow_down","error_description":"OAuth2 credential request limit reached; retry later"}`, rec.Body.String())
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	store.FastForward(time.Minute)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, 401, rec.Code)
}

func TestOAuth2SuccessDoesNotConsumeFailureBudget(t *testing.T) {
	store := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: store.Addr()})
	defer func() { _ = client.Close() }()
	router := gin.New()
	router.POST("/oauth/token", OAuth2CredentialRateLimit(client), func(c *gin.Context) { c.JSON(200, gin.H{"access_token": "test-token"}) })
	for i := 0; i < 100; i++ {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest("POST", "/oauth/token", nil))
		require.Equal(t, 200, rec.Code)
	}
	require.False(t, store.Exists("rate_limit:oauth2:failures:192.0.2.1"))
}

func TestOAuth2FailureLimiterFailsClosedWithoutRedis(t *testing.T) {
	router := gin.New()
	called := false
	router.POST("/oauth/token", OAuth2CredentialRateLimit(nil), func(c *gin.Context) { called = true })
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("POST", "/oauth/token", nil))
	require.Equal(t, 503, rec.Code)
	require.False(t, called)
	require.Contains(t, rec.Body.String(), `"error":"temporarily_unavailable"`)
}
