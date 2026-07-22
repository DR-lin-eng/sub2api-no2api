//go:build integration

package routes

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

const authRouteRedisImageTag = "redis:8.4-alpine"

func TestAuthRegisterRateLimitThresholdHitReturns429(t *testing.T) {
	ctx := context.Background()
	rdb := startAuthRouteRedis(t, ctx)

	router := newAuthRoutesTestRouter(rdb)
	const path = "/api/v1/auth/register"
	publicKey, flowCookie := issueAuthRouteCredentialKey(t, router)

	for i := 1; i <= 6; i++ {
		body := encryptAuthRouteCredentialRequest(t, publicKey, time.Now())
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.10:23456"
		req.AddCookie(flowCookie)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if i <= 5 {
			require.Equal(t, http.StatusBadRequest, w.Code, "第 %d 次请求应先进入业务校验", i)
			continue
		}
		require.Equal(t, http.StatusTooManyRequests, w.Code, "第 6 次请求应命中限流")
		require.Contains(t, w.Body.String(), "rate limit exceeded")
	}
}

type authRouteCredentialPublicKey struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	PublicKey string `json:"public_key"`
}

func issueAuthRouteCredentialKey(t *testing.T, router http.Handler) (authRouteCredentialPublicKey, *http.Cookie) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/credential-key", nil)
	req.RemoteAddr = "198.51.100.10:23456"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data authRouteCredentialPublicKey `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, "RSA-OAEP-256+A256GCM", response.Data.Algorithm)
	require.Len(t, w.Result().Cookies(), 1)
	return response.Data, w.Result().Cookies()[0]
}

func encryptAuthRouteCredentialRequest(t *testing.T, public authRouteCredentialPublicKey, issuedAt time.Time) []byte {
	t.Helper()
	publicDER, err := base64.RawStdEncoding.DecodeString(public.PublicKey)
	require.NoError(t, err)
	parsed, err := x509.ParsePKIXPublicKey(publicDER)
	require.NoError(t, err)
	publicKey, ok := parsed.(*rsa.PublicKey)
	require.True(t, ok)

	aesKey := make([]byte, 32)
	_, err = rand.Read(aesKey)
	require.NoError(t, err)
	encryptedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, aesKey, nil)
	require.NoError(t, err)
	block, err := aes.NewCipher(aesKey)
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	iv := make([]byte, gcm.NonceSize())
	_, err = rand.Read(iv)
	require.NoError(t, err)

	plaintext, err := json.Marshal(map[string]any{
		"email":     "not-an-email",
		"password":  "secret-123",
		"issued_at": issuedAt.Unix(),
	})
	require.NoError(t, err)
	ciphertext := gcm.Seal(nil, iv, plaintext, []byte(public.KeyID))
	body, err := json.Marshal(map[string]any{
		"credential_envelope": map[string]any{
			"algorithm":     public.Algorithm,
			"key_id":        public.KeyID,
			"encrypted_key": base64.RawURLEncoding.EncodeToString(encryptedKey),
			"iv":            base64.RawURLEncoding.EncodeToString(iv),
			"ciphertext":    base64.RawURLEncoding.EncodeToString(ciphertext),
		},
	})
	require.NoError(t, err)
	return body
}

func startAuthRouteRedis(t *testing.T, ctx context.Context) *redis.Client {
	t.Helper()
	ensureAuthRouteDockerAvailable(t)

	redisContainer, err := tcredis.Run(ctx, authRouteRedisImageTag)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = redisContainer.Terminate(ctx)
	})

	redisHost, err := redisContainer.Host(ctx)
	require.NoError(t, err)
	redisPort, err := redisContainer.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)

	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", redisHost, redisPort.Int()),
		DB:   0,
	})
	require.NoError(t, rdb.Ping(ctx).Err())
	t.Cleanup(func() {
		_ = rdb.Close()
	})
	return rdb
}

func ensureAuthRouteDockerAvailable(t *testing.T) {
	t.Helper()
	if authRouteDockerAvailable() {
		return
	}
	t.Skip("Docker 未启用，跳过认证限流集成测试")
}

func authRouteDockerAvailable() bool {
	if os.Getenv("DOCKER_HOST") != "" {
		return true
	}

	socketCandidates := []string{
		"/var/run/docker.sock",
		filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "docker.sock"),
		filepath.Join(authRouteUserHomeDir(), ".docker", "run", "docker.sock"),
		filepath.Join(authRouteUserHomeDir(), ".docker", "desktop", "docker.sock"),
		filepath.Join("/run/user", strconv.Itoa(os.Getuid()), "docker.sock"),
	}

	for _, socket := range socketCandidates {
		if socket == "" {
			continue
		}
		if _, err := os.Stat(socket); err == nil {
			return true
		}
	}
	return false
}

func authRouteUserHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
