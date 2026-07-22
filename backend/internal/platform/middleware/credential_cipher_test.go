package middleware

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCredentialCipherDecryptsEnvelopeWithoutSendingPlaintext(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cipherService := NewCredentialCipher(rdb, nil)

	publicKey := issueCredentialPublicKey(t, cipherService)
	body := encryptCredentialRequest(t, publicKey, "user@example.com", "secret-123", time.Now())
	require.NotContains(t, string(body), "user@example.com")
	require.NotContains(t, string(body), "secret-123")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/login", cipherService.DecryptEnvelope(), func(c *gin.Context) {
		var payload struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		require.NoError(t, c.ShouldBindJSON(&payload))
		require.Equal(t, "user@example.com", payload.Email)
		require.Equal(t, "secret-123", payload.Password)
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestCredentialCipherRequiresMatchingBrowserFlowCookie(t *testing.T) {
	now := time.Date(2026, 7, 18, 3, 0, 0, 0, time.UTC)
	setCredentialTestClock(t, &now)
	cipherService := NewCredentialCipher(nil, nil)
	publicKey, cookie := issueCredentialPublicKeyAndCookie(t, cipherService, false)
	body := encryptCredentialRequest(t, publicKey, "user@example.com", "secret-123", now)

	require.Equal(t, http.StatusForbidden, serveBrowserCredentialRequest(t, cipherService, body, nil))
	require.Equal(t, http.StatusNoContent, serveBrowserCredentialRequest(t, cipherService, body, cookie))

	tampered := *cookie
	tampered.Value += "x"
	require.Equal(t, http.StatusForbidden, serveBrowserCredentialRequest(t, cipherService, body, &tampered))

	var mismatched map[string]any
	require.NoError(t, json.Unmarshal(body, &mismatched))
	envelope, ok := mismatched["credential_envelope"].(map[string]any)
	require.True(t, ok)
	envelope["key_id"] = credentialKeyID(now.Add(credentialKeySlotDuration))
	mismatchedBody, err := json.Marshal(mismatched)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, serveBrowserCredentialRequest(t, cipherService, mismatchedBody, cookie))
}

func TestCredentialCipherRejectsExpiredBrowserFlowCookie(t *testing.T) {
	now := time.Date(2026, 7, 18, 3, 0, 0, 0, time.UTC)
	setCredentialTestClock(t, &now)
	cipherService := NewCredentialCipher(nil, nil)
	publicKey, cookie := issueCredentialPublicKeyAndCookie(t, cipherService, false)

	now = now.Add(credentialBrowserFlowTTL)
	body := encryptCredentialRequest(t, publicKey, "user@example.com", "secret-123", now)
	require.Equal(t, http.StatusForbidden, serveBrowserCredentialRequest(t, cipherService, body, cookie))
}

func TestCredentialCipherBrowserFlowCookieAttributes(t *testing.T) {
	now := time.Date(2026, 7, 18, 3, 0, 0, 0, time.UTC)
	setCredentialTestClock(t, &now)
	publicKey, cookie := issueCredentialPublicKeyAndCookie(t, NewCredentialCipher(nil, nil), true)

	require.Equal(t, now.Add(credentialBrowserFlowTTL).Unix(), publicKey.FlowExpiresAt)
	require.Equal(t, credentialBrowserCookieName, cookie.Name)
	require.Equal(t, credentialBrowserCookiePath, cookie.Path)
	require.Equal(t, int(credentialBrowserFlowTTL/time.Second), cookie.MaxAge)
	require.True(t, cookie.HttpOnly)
	require.True(t, cookie.Secure)
	require.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
}

func TestCredentialCipherBrowserFlowRejectsPlaintextCompatibility(t *testing.T) {
	cipherService := NewCredentialCipher(nil, nil)
	_, cookie := issueCredentialPublicKeyAndCookie(t, cipherService, false)
	body := []byte(`{"email":"user@example.com","password":"secret-123"}`)
	require.Equal(t, http.StatusForbidden, serveBrowserCredentialRequest(t, cipherService, body, cookie))
}

func TestCredentialCipherRejectsExpiredEnvelope(t *testing.T) {
	cipherService := NewCredentialCipher(nil, nil)
	publicKey := issueCredentialPublicKey(t, cipherService)
	body := encryptCredentialRequest(t, publicKey, "user@example.com", "secret-123", time.Now().Add(-credentialEnvelopeMaxAge-time.Minute))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/login", cipherService.DecryptEnvelope(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "INVALID_CREDENTIAL_ENVELOPE")
}

func TestCredentialCipherSharesKeyAcrossInstances(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	first := issueCredentialPublicKey(t, NewCredentialCipher(rdb, nil))
	second := issueCredentialPublicKey(t, NewCredentialCipher(rdb, nil))
	require.Equal(t, first.KeyID, second.KeyID)
	require.Equal(t, first.PublicKey, second.PublicKey)
}

func TestCredentialCipherKeepsPlaintextCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/login", NewCredentialCipher(nil, nil).DecryptEnvelope(), func(c *gin.Context) {
		var payload map[string]string
		require.NoError(t, c.ShouldBindJSON(&payload))
		require.Equal(t, "user@example.com", payload["email"])
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{"email":"user@example.com","password":"secret-123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
}

type memoryCredentialKeyStore struct {
	mu          sync.Mutex
	keys        map[string]credentialKeyRecord
	storeCalls  int
	deleteCalls int
}

func newMemoryCredentialKeyStore() *memoryCredentialKeyStore {
	return &memoryCredentialKeyStore{keys: make(map[string]credentialKeyRecord)}
}

func (s *memoryCredentialKeyStore) Load(_ context.Context, keyID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.keys[keyID]
	if !ok {
		return "", sql.ErrNoRows
	}
	return record.PrivateKey, nil
}

func (s *memoryCredentialKeyStore) Store(_ context.Context, record credentialKeyRecord) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storeCalls++
	if existing, ok := s.keys[record.KeyID]; ok {
		return existing.PrivateKey, false, nil
	}
	s.keys[record.KeyID] = record
	return record.PrivateKey, true, nil
}

func (s *memoryCredentialKeyStore) DeleteExpired(_ context.Context, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteCalls++
	for keyID, record := range s.keys {
		if !record.DecryptExpiresAt.After(now) {
			delete(s.keys, keyID)
		}
	}
	return nil
}

func TestCredentialCipherRestoresCanonicalKeyAfterRestartAndRedisFlush(t *testing.T) {
	now := time.Date(2026, 7, 18, 3, 0, 0, 0, time.UTC)
	setCredentialTestClock(t, &now)
	store := newMemoryCredentialKeyStore()
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	first := issueCredentialPublicKey(t, newCredentialCipherWithStore(rdb, store))
	server.FlushAll()
	second := issueCredentialPublicKey(t, newCredentialCipherWithStore(rdb, store))

	require.Equal(t, first.KeyID, second.KeyID)
	require.Equal(t, first.PublicKey, second.PublicKey)
	require.True(t, server.Exists(credentialRedisPrefix+first.KeyID))
	require.Len(t, store.keys, 1)
}

func TestCredentialBrowserCookieValidAcrossInstances(t *testing.T) {
	now := time.Date(2026, 7, 18, 3, 0, 0, 0, time.UTC)
	setCredentialTestClock(t, &now)
	store := newMemoryCredentialKeyStore()
	issuer := newCredentialCipherWithStore(nil, store)
	publicKey, cookie := issueCredentialPublicKeyAndCookie(t, issuer, false)
	body := encryptCredentialRequest(t, publicKey, "user@example.com", "secret-123", now)

	validator := newCredentialCipherWithStore(nil, store)
	require.Equal(t, http.StatusNoContent, serveBrowserCredentialRequest(t, validator, body, cookie))
}

func TestCredentialCipherConcurrentInstancesConvergeOnDatabaseKey(t *testing.T) {
	now := time.Date(2026, 7, 18, 3, 0, 0, 0, time.UTC)
	setCredentialTestClock(t, &now)
	store := newMemoryCredentialKeyStore()
	services := []*CredentialCipher{
		newCredentialCipherWithStore(nil, store),
		newCredentialCipherWithStore(nil, store),
	}

	keys := make(chan *rsa.PrivateKey, len(services))
	errs := make(chan error, len(services))
	var wg sync.WaitGroup
	for _, service := range services {
		wg.Add(1)
		go func(cipherService *CredentialCipher) {
			defer wg.Done()
			key, err := cipherService.loadOrCreateKey(context.Background(), credentialKeyID(now))
			keys <- key
			errs <- err
		}(service)
	}
	wg.Wait()
	close(keys)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var modulus string
	for key := range keys {
		require.NotNil(t, key)
		if modulus == "" {
			modulus = key.N.String()
		}
		require.Equal(t, modulus, key.N.String())
	}
	require.Len(t, store.keys, 1)
}

func TestCredentialCipherStopsIssuingExpiredKeyAtSlotBoundary(t *testing.T) {
	now := time.Date(2026, 7, 18, 11, 59, 59, 0, time.UTC)
	setCredentialTestClock(t, &now)
	store := newMemoryCredentialKeyStore()
	cipherService := newCredentialCipherWithStore(nil, store)

	beforeBoundary := issueCredentialPublicKey(t, cipherService)
	now = time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	afterBoundary := issueCredentialPublicKey(t, cipherService)

	require.NotEqual(t, beforeBoundary.KeyID, afterBoundary.KeyID)
	require.NotEqual(t, beforeBoundary.PublicKey, afterBoundary.PublicKey)
	require.Equal(t, now.Add(credentialKeySlotDuration).Unix(), afterBoundary.ExpiresAt)
}

func TestCredentialCipherAcceptsExpiredPublicKeyOnlyDuringDecryptGrace(t *testing.T) {
	slotStart := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	now := slotStart.Add(time.Hour)
	setCredentialTestClock(t, &now)
	cipherService := NewCredentialCipher(nil, nil)
	publicKey := issueCredentialPublicKey(t, cipherService)

	now = slotStart.Add(credentialKeySlotDuration + credentialKeyDecryptGrace - time.Second)
	validBody := encryptCredentialRequest(t, publicKey, "user@example.com", "secret-123", now)
	require.Equal(t, http.StatusNoContent, serveCredentialRequest(t, cipherService, validBody))

	now = slotStart.Add(credentialKeySlotDuration + credentialKeyDecryptGrace)
	expiredBody := encryptCredentialRequest(t, publicKey, "user@example.com", "secret-123", now)
	require.Equal(t, http.StatusBadRequest, serveCredentialRequest(t, cipherService, expiredBody))
}

func setCredentialTestClock(t *testing.T, now *time.Time) {
	t.Helper()
	previous := credentialNow
	credentialNow = func() time.Time { return *now }
	t.Cleanup(func() { credentialNow = previous })
}

func serveCredentialRequest(t *testing.T, cipherService *CredentialCipher, body []byte) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/login", cipherService.DecryptEnvelope(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

func serveBrowserCredentialRequest(t *testing.T, cipherService *CredentialCipher, body []byte, cookie *http.Cookie) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/login", cipherService.RequireBrowserFlow(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

func issueCredentialPublicKey(t *testing.T, cipherService *CredentialCipher) credentialPublicKeyResponse {
	publicKey, _ := issueCredentialPublicKeyAndCookie(t, cipherService, false)
	return publicKey
}

func issueCredentialPublicKeyAndCookie(t *testing.T, cipherService *CredentialCipher, secure bool) (credentialPublicKeyResponse, *http.Cookie) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/credential-key", nil)
	if secure {
		c.Request.Header.Set("X-Forwarded-Proto", "https")
	}
	cipherService.PublicKey(c)
	require.Equal(t, http.StatusOK, w.Code)

	var envelope struct {
		Data credentialPublicKeyResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Equal(t, credentialEnvelopeAlgorithm, envelope.Data.Algorithm)
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	return envelope.Data, cookies[0]
}

func encryptCredentialRequest(t *testing.T, public credentialPublicKeyResponse, email, password string, issuedAt time.Time) []byte {
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

	plaintext, err := json.Marshal(credentialPayload{
		Email:    email,
		Password: password,
		IssuedAt: issuedAt.Unix(),
	})
	require.NoError(t, err)
	ciphertext := gcm.Seal(nil, iv, plaintext, []byte(public.KeyID))
	body, err := json.Marshal(map[string]any{
		"turnstile_token": "captcha-token",
		"credential_envelope": CredentialEnvelope{
			Algorithm:    credentialEnvelopeAlgorithm,
			KeyID:        public.KeyID,
			EncryptedKey: base64.RawURLEncoding.EncodeToString(encryptedKey),
			IV:           base64.RawURLEncoding.EncodeToString(iv),
			Ciphertext:   base64.RawURLEncoding.EncodeToString(ciphertext),
		},
	})
	require.NoError(t, err)
	return body
}

func BenchmarkCredentialCipherPublicKeyWarm(b *testing.B) {
	gin.SetMode(gin.TestMode)
	cipherService := NewCredentialCipher(nil, nil)
	warmRecorder := httptest.NewRecorder()
	warmContext, _ := gin.CreateTestContext(warmRecorder)
	warmContext.Request = httptest.NewRequest(http.MethodGet, "/credential-key", nil)
	cipherService.PublicKey(warmContext)
	if warmRecorder.Code != http.StatusOK {
		b.Fatalf("warm credential key status=%d", warmRecorder.Code)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/credential-key", nil)
		cipherService.PublicKey(c)
		if w.Code != http.StatusOK {
			b.Fatalf("credential key status=%d", w.Code)
		}
	}
}
