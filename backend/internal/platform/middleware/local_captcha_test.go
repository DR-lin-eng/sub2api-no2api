package middleware

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newLocalCaptchaTestClient(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client, server
}

func TestLocalCaptchaGenerateStoresDigestAndReturnsPNG(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client, server := newLocalCaptchaTestClient(t)
	captcha := NewLocalCaptcha(client)
	router := gin.New()
	router.GET("/captcha", captcha.Generate(func(context.Context) bool { return true }))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/captcha", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	var envelope struct {
		Data LocalCaptchaResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data.CaptchaID, 32)
	require.True(t, strings.HasPrefix(envelope.Data.ImageData, "data:image/png;base64,"))
	require.Equal(t, int(localCaptchaTTL.Seconds()), envelope.Data.ExpiresIn)
	require.True(t, server.Exists("auth_captcha:"+envelope.Data.CaptchaID))
	require.Equal(t, int64(1), client.ZCard(context.Background(), "auth_captcha:active").Val())
}

func TestLocalCaptchaGenerateEvictsOldestChallengeAtCapacity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client, server := newLocalCaptchaTestClient(t)
	captcha := NewLocalCaptcha(client)
	captcha.maxActive = 2
	router := gin.New()
	router.GET("/captcha", captcha.Generate(func(context.Context) bool { return true }))

	issue := func() string {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/captcha", nil))
		require.Equal(t, http.StatusOK, recorder.Code)
		var envelope struct {
			Data LocalCaptchaResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
		return envelope.Data.CaptchaID
	}

	first := issue()
	second := issue()
	third := issue()

	require.False(t, server.Exists("auth_captcha:"+first))
	require.True(t, server.Exists("auth_captcha:"+second))
	require.True(t, server.Exists("auth_captcha:"+third))
	require.Equal(t, int64(2), client.ZCard(context.Background(), "auth_captcha:active").Val())
}

func TestLocalCaptchaIssueCapsConcurrentChallenges(t *testing.T) {
	client, server := newLocalCaptchaTestClient(t)
	captcha := NewLocalCaptcha(client)
	captcha.maxActive = 32

	const total = 256
	errors := make(chan error, total)
	var wg sync.WaitGroup
	for index := 0; index < total; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			captchaID := fmt.Sprintf("%032x", index)
			created, err := captcha.issue(context.Background(), captchaID, "digest")
			if err != nil {
				errors <- err
				return
			}
			if !created {
				errors <- fmt.Errorf("challenge %s was not created", captchaID)
			}
		}(index)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}

	require.Equal(t, int64(32), client.ZCard(context.Background(), "auth_captcha:active").Val())
	challengeKeys := 0
	for _, key := range server.Keys() {
		if strings.HasPrefix(key, "auth_captcha:") && key != "auth_captcha:active" && key != "auth_captcha:sequence" {
			challengeKeys++
		}
	}
	require.Equal(t, 32, challengeKeys)
}

func TestLocalCaptchaRequireConsumesChallengeAndRestoresBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client, server := newLocalCaptchaTestClient(t)
	captcha := NewLocalCaptcha(client)
	const captchaID = "0123456789abcdef0123456789abcdef"
	const answer = "A7K9P"
	require.NoError(t, server.Set("auth_captcha:"+captchaID, hex.EncodeToString(localCaptchaDigest(captchaID, answer, "192.0.2.1"))))
	require.NoError(t, client.ZAdd(context.Background(), "auth_captcha:active", redis.Z{Score: 1, Member: captchaID}).Err())

	router := gin.New()
	router.POST("/login", captcha.Require(LocalCaptchaRequireOptions{
		Enabled: func(context.Context) bool { return true },
	}), func(c *gin.Context) {
		var payload struct {
			Email string `json:"email"`
		}
		require.NoError(t, c.ShouldBindJSON(&payload))
		require.Equal(t, "user@example.com", payload.Email)
		c.Status(http.StatusNoContent)
	})

	body := []byte(`{"email":"user@example.com","captcha_id":"` + captchaID + `","captcha_code":"a7k9p"}`)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.False(t, server.Exists("auth_captcha:"+captchaID))
	_, err := client.ZScore(context.Background(), "auth_captcha:active", captchaID).Result()
	require.ErrorIs(t, err, redis.Nil)

	replay := httptest.NewRecorder()
	replayReq := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	replayReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(replay, replayReq)
	require.Equal(t, http.StatusBadRequest, replay.Code)
	require.Contains(t, replay.Body.String(), "LOCAL_CAPTCHA_EXPIRED")
}

func TestLocalCaptchaRequireInvalidAnswerIsSingleUse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client, server := newLocalCaptchaTestClient(t)
	captcha := NewLocalCaptcha(client)
	const captchaID = "fedcba9876543210fedcba9876543210"
	require.NoError(t, server.Set("auth_captcha:"+captchaID, hex.EncodeToString(localCaptchaDigest(captchaID, "Q8M4T", "192.0.2.1"))))

	router := gin.New()
	router.POST("/register", captcha.Require(LocalCaptchaRequireOptions{
		Enabled: func(context.Context) bool { return true },
	}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"captcha_id":"`+captchaID+`","captcha_code":"WRONG"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "LOCAL_CAPTCHA_INVALID")
	require.False(t, server.Exists("auth_captcha:"+captchaID))
}

func TestLocalCaptchaRequireBindsChallengeToClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client, server := newLocalCaptchaTestClient(t)
	captcha := NewLocalCaptcha(client)
	const captchaID = "abcdef0123456789abcdef0123456789"
	const answer = "Q8M4T"
	require.NoError(t, server.Set("auth_captcha:"+captchaID, hex.EncodeToString(localCaptchaDigest(captchaID, answer, "203.0.113.8"))))

	router := gin.New()
	router.POST("/login", captcha.Require(LocalCaptchaRequireOptions{
		Enabled: func(context.Context) bool { return true },
	}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"captcha_id":"`+captchaID+`","captcha_code":"`+answer+`"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "LOCAL_CAPTCHA_INVALID")
}

func TestLocalCaptchaRequireDisabledBypassesChallenge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	captcha := NewLocalCaptcha(nil)
	router := gin.New()
	router.POST("/login", captcha.Require(LocalCaptchaRequireOptions{
		Enabled: func(context.Context) bool { return false },
	}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/login", nil))
	require.Equal(t, http.StatusNoContent, recorder.Code)
}
