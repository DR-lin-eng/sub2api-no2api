package middleware

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const (
	credentialEnvelopeAlgorithm = "RSA-OAEP-256+A256GCM"
	credentialKeySlotDuration   = 12 * time.Hour
	credentialKeyDecryptGrace   = 30 * time.Minute
	credentialEnvelopeMaxAge    = 15 * time.Minute
	credentialBrowserFlowTTL    = 15 * time.Minute
	credentialRequestMaxBytes   = 64 << 10
	credentialCiphertextMaxSize = 8 << 10
	credentialRedisPrefix       = "auth_credential:v2:key:"
	credentialBrowserCookieName = "sub2api_auth_flow"
	credentialBrowserCookiePath = "/api/v1/auth"
	credentialBrowserCookieV1   = "v1"
)

var credentialNow = time.Now

// CredentialEnvelope contains a hybrid-encrypted email/password pair. RSA-OAEP
// protects the random AES key and AES-GCM protects the credential payload.
type CredentialEnvelope struct {
	Algorithm    string `json:"algorithm"`
	KeyID        string `json:"key_id"`
	EncryptedKey string `json:"encrypted_key"`
	IV           string `json:"iv"`
	Ciphertext   string `json:"ciphertext"`
}

type credentialEnvelopeRequest struct {
	Envelope *CredentialEnvelope `json:"credential_envelope"`
}

type credentialPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	IssuedAt int64  `json:"issued_at"`
}

type credentialPublicKeyResponse struct {
	Algorithm     string `json:"algorithm"`
	KeyID         string `json:"key_id"`
	PublicKey     string `json:"public_key"`
	ExpiresAt     int64  `json:"expires_at"`
	FlowExpiresAt int64  `json:"flow_expires_at"`
	ServerTime    int64  `json:"server_time"`
}

type credentialDerivedKey struct {
	publicKey string
	macSecret [sha256.Size]byte
}

type credentialKeyRecord struct {
	KeyID            string
	PrivateKey       string
	SlotStartedAt    time.Time
	PublicExpiresAt  time.Time
	DecryptExpiresAt time.Time
}

type credentialKeyStore interface {
	Load(context.Context, string) (string, error)
	Store(context.Context, credentialKeyRecord) (string, bool, error)
	DeleteExpired(context.Context, time.Time) error
}

type postgresCredentialKeyStore struct {
	db *sql.DB
}

func (s *postgresCredentialKeyStore) Load(ctx context.Context, keyID string) (string, error) {
	var privateKey string
	err := s.db.QueryRowContext(ctx, `
		SELECT private_key
		FROM auth_credential_keys
		WHERE key_id = $1
	`, keyID).Scan(&privateKey)
	return privateKey, err
}

func (s *postgresCredentialKeyStore) Store(ctx context.Context, record credentialKeyRecord) (string, bool, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO auth_credential_keys (
			key_id, private_key, slot_started_at, public_expires_at, decrypt_expires_at
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (key_id) DO NOTHING
	`, record.KeyID, record.PrivateKey, record.SlotStartedAt, record.PublicExpiresAt, record.DecryptExpiresAt)
	if err != nil {
		return "", false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return "", false, err
	}
	privateKey, err := s.Load(ctx, record.KeyID)
	return privateKey, rowsAffected == 1, err
}

func (s *postgresCredentialKeyStore) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM auth_credential_keys
		WHERE decrypt_expires_at <= $1
	`, now)
	return err
}

// CredentialCipher manages short-lived browser credential encryption keys.
// PostgreSQL is authoritative; Redis and process memory are disposable caches.
type CredentialCipher struct {
	redis *redis.Client
	store credentialKeyStore

	mu        sync.RWMutex
	keyCache  map[string]*rsa.PrivateKey
	derived   map[string]*credentialDerivedKey
	localKeys map[string]*rsa.PrivateKey
	keyFlight singleflight.Group
}

func NewCredentialCipher(redisClient *redis.Client, db *sql.DB) *CredentialCipher {
	var store credentialKeyStore
	if db != nil {
		store = &postgresCredentialKeyStore{db: db}
	}
	return newCredentialCipherWithStore(redisClient, store)
}

func newCredentialCipherWithStore(redisClient *redis.Client, store credentialKeyStore) *CredentialCipher {
	return &CredentialCipher{
		redis:     redisClient,
		store:     store,
		keyCache:  make(map[string]*rsa.PrivateKey, 2),
		derived:   make(map[string]*credentialDerivedKey, 2),
		localKeys: make(map[string]*rsa.PrivateKey, 2),
	}
}

// PublicKey issues the current browser credential-encryption public key.
func (m *CredentialCipher) PublicKey(c *gin.Context) {
	now := credentialNow().UTC()
	keyID := credentialKeyID(now)
	privateKey, err := m.loadOrCreateKey(c.Request.Context(), keyID)
	if err != nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "credential encryption is unavailable", "CREDENTIAL_ENCRYPTION_UNAVAILABLE", nil)
		return
	}

	derived, err := m.derivedKey(keyID, privateKey)
	if err != nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "credential encryption is unavailable", "CREDENTIAL_ENCRYPTION_UNAVAILABLE", nil)
		return
	}
	flowExpiresAt := now.Add(credentialBrowserFlowTTL)
	flowCookie, err := buildCredentialBrowserCookie(derived.macSecret, keyID, flowExpiresAt)
	if err != nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "credential encryption is unavailable", "CREDENTIAL_ENCRYPTION_UNAVAILABLE", nil)
		return
	}
	setCredentialBrowserCookie(c, flowCookie, flowExpiresAt)

	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	response.Success(c, credentialPublicKeyResponse{
		Algorithm:     credentialEnvelopeAlgorithm,
		KeyID:         keyID,
		PublicKey:     derived.publicKey,
		ExpiresAt:     credentialKeySlotStart(now).Add(credentialKeySlotDuration).Unix(),
		FlowExpiresAt: flowExpiresAt.Unix(),
		ServerTime:    now.Unix(),
	})
}

// DecryptEnvelope replaces an encrypted credential envelope with email and
// password fields for existing handlers. Plaintext requests remain accepted for
// API compatibility, while the bundled web client always uses an envelope.
func (m *CredentialCipher) DecryptEnvelope() gin.HandlerFunc {
	return m.decryptEnvelope(false)
}

// RequireBrowserFlow decrypts credentials only when the request includes the
// short-lived HttpOnly cookie issued with the matching public key.
func (m *CredentialCipher) RequireBrowserFlow() gin.HandlerFunc {
	return m.decryptEnvelope(true)
}

func (m *CredentialCipher) decryptEnvelope(requireBrowserFlow bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, ok := readCredentialRequestBody(c)
		if !ok {
			return
		}

		var request credentialEnvelopeRequest
		if err := json.Unmarshal(body, &request); err != nil {
			abortCredentialEnvelope(c, "invalid request body")
			return
		}
		if request.Envelope == nil {
			if requireBrowserFlow {
				abortCredentialBrowserFlow(c)
				return
			}
			c.Next()
			return
		}
		if requireBrowserFlow {
			flowKeyID, err := m.verifyCredentialBrowserCookie(c.Request.Context(), c, credentialNow().UTC())
			if err != nil || flowKeyID != request.Envelope.KeyID {
				clearCredentialBrowserCookie(c)
				abortCredentialBrowserFlow(c)
				return
			}
		}

		credentials, err := m.decrypt(c.Request.Context(), request.Envelope)
		if err != nil {
			abortCredentialEnvelope(c, "invalid or expired credential envelope")
			return
		}

		var payload map[string]json.RawMessage
		if err := json.Unmarshal(body, &payload); err != nil {
			abortCredentialEnvelope(c, "invalid request body")
			return
		}
		delete(payload, "credential_envelope")
		payload["email"], _ = json.Marshal(credentials.Email)
		payload["password"], _ = json.Marshal(credentials.Password)

		decryptedBody, err := json.Marshal(payload)
		if err != nil {
			abortCredentialEnvelope(c, "invalid credential envelope")
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(decryptedBody))
		c.Request.ContentLength = int64(len(decryptedBody))
		c.Next()
	}
}

func (m *CredentialCipher) verifyCredentialBrowserCookie(ctx context.Context, c *gin.Context, now time.Time) (string, error) {
	cookie, err := c.Request.Cookie(credentialBrowserCookieName)
	if err != nil || cookie.Value == "" {
		return "", errors.New("credential browser cookie is missing")
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 5 || parts[0] != credentialBrowserCookieV1 {
		return "", errors.New("credential browser cookie is invalid")
	}
	keyID := parts[1]
	expiresUnix, err := strconv.ParseInt(parts[2], 36, 64)
	if err != nil || !now.Before(time.Unix(expiresUnix, 0)) || !credentialKeyIDAllowed(keyID, now) {
		return "", errors.New("credential browser cookie is expired")
	}
	if _, err := decodeCredentialBase64(parts[3], 32); err != nil {
		return "", errors.New("credential browser cookie is invalid")
	}
	providedMAC, err := decodeCredentialBase64(parts[4], sha256.Size)
	if err != nil || len(providedMAC) != sha256.Size {
		return "", errors.New("credential browser cookie is invalid")
	}
	privateKey, err := m.loadKey(ctx, keyID)
	if err != nil {
		return "", err
	}
	derived, err := m.derivedKey(keyID, privateKey)
	if err != nil {
		return "", err
	}
	expectedMAC := credentialBrowserCookieMAC(derived.macSecret, strings.Join(parts[:4], "."))
	if !hmac.Equal(providedMAC, expectedMAC) {
		return "", errors.New("credential browser cookie signature is invalid")
	}
	return keyID, nil
}

func buildCredentialBrowserCookie(macSecret [sha256.Size]byte, keyID string, expiresAt time.Time) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	payload := strings.Join([]string{
		credentialBrowserCookieV1,
		keyID,
		strconv.FormatInt(expiresAt.Unix(), 36),
		base64.RawURLEncoding.EncodeToString(nonce),
	}, ".")
	mac := credentialBrowserCookieMAC(macSecret, payload)
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac), nil
}

func credentialBrowserCookieMAC(macSecret [sha256.Size]byte, payload string) []byte {
	mac := hmac.New(sha256.New, macSecret[:])
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func setCredentialBrowserCookie(c *gin.Context, value string, expiresAt time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     credentialBrowserCookieName,
		Value:    value,
		Path:     credentialBrowserCookiePath,
		Expires:  expiresAt,
		MaxAge:   int(credentialBrowserFlowTTL / time.Second),
		HttpOnly: true,
		Secure:   credentialRequestIsHTTPS(c),
		SameSite: http.SameSiteStrictMode,
	})
}

func clearCredentialBrowserCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     credentialBrowserCookieName,
		Value:    "",
		Path:     credentialBrowserCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   credentialRequestIsHTTPS(c),
		SameSite: http.SameSiteStrictMode,
	})
}

func credentialRequestIsHTTPS(c *gin.Context) bool {
	if c != nil && c.Request != nil && c.Request.TLS != nil {
		return true
	}
	return c != nil && strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
}

func (m *CredentialCipher) decrypt(ctx context.Context, envelope *CredentialEnvelope) (*credentialPayload, error) {
	if envelope == nil || envelope.Algorithm != credentialEnvelopeAlgorithm {
		return nil, errors.New("unsupported credential envelope")
	}
	now := credentialNow().UTC()
	if !credentialKeyIDAllowed(envelope.KeyID, now) {
		return nil, errors.New("credential key expired")
	}

	privateKey, err := m.loadKey(ctx, envelope.KeyID)
	if err != nil {
		return nil, err
	}
	encryptedKey, err := decodeCredentialBase64(envelope.EncryptedKey, 1024)
	if err != nil {
		return nil, err
	}
	iv, err := decodeCredentialBase64(envelope.IV, 32)
	if err != nil || len(iv) != 12 {
		return nil, errors.New("invalid credential iv")
	}
	ciphertext, err := decodeCredentialBase64(envelope.Ciphertext, credentialCiphertextMaxSize)
	if err != nil {
		return nil, err
	}

	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, encryptedKey, nil)
	if err != nil || len(aesKey) != 32 {
		return nil, errors.New("invalid encrypted credential key")
	}
	defer zeroBytes(aesKey)

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, iv, ciphertext, []byte(envelope.KeyID))
	if err != nil {
		return nil, errors.New("invalid credential ciphertext")
	}
	defer zeroBytes(plaintext)

	var credentials credentialPayload
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return nil, errors.New("invalid credential payload")
	}
	issuedAt := time.Unix(credentials.IssuedAt, 0)
	if credentials.Email == "" || credentials.Password == "" ||
		issuedAt.Before(now.Add(-credentialEnvelopeMaxAge)) || issuedAt.After(now.Add(time.Minute)) {
		return nil, errors.New("expired credential payload")
	}
	return &credentials, nil
}

func (m *CredentialCipher) loadOrCreateKey(ctx context.Context, keyID string) (*rsa.PrivateKey, error) {
	if key := m.cachedKey(keyID); key != nil {
		return key, nil
	}

	value, err, _ := m.keyFlight.Do(keyID, func() (any, error) {
		if key := m.cachedKey(keyID); key != nil {
			return key, nil
		}
		if key, err := m.readRedisKey(ctx, keyID); err == nil {
			m.rememberKey(keyID, key)
			return key, nil
		}
		if m.store == nil {
			return m.loadOrCreateEphemeralKey(ctx, keyID)
		}

		encoded, err := m.store.Load(ctx, keyID)
		if err == nil {
			return m.cachePersistedKey(ctx, keyID, encoded)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}

		generated, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, err
		}
		candidate, err := marshalCredentialPrivateKey(generated)
		if err != nil {
			return nil, err
		}
		slotStartedAt, err := credentialKeySlotFromID(keyID)
		if err != nil {
			return nil, err
		}
		publicExpiresAt := slotStartedAt.Add(credentialKeySlotDuration)
		encoded, inserted, err := m.store.Store(ctx, credentialKeyRecord{
			KeyID:            keyID,
			PrivateKey:       candidate,
			SlotStartedAt:    slotStartedAt,
			PublicExpiresAt:  publicExpiresAt,
			DecryptExpiresAt: publicExpiresAt.Add(credentialKeyDecryptGrace),
		})
		if err != nil {
			return nil, err
		}
		if inserted {
			_ = m.store.DeleteExpired(ctx, credentialNow().UTC())
		}
		return m.cachePersistedKey(ctx, keyID, encoded)
	})
	if err != nil {
		return nil, err
	}
	key, ok := value.(*rsa.PrivateKey)
	if !ok || key == nil {
		return nil, errors.New("credential key cache returned an invalid key")
	}
	return key, nil
}

func (m *CredentialCipher) loadKey(ctx context.Context, keyID string) (*rsa.PrivateKey, error) {
	if key := m.cachedKey(keyID); key != nil {
		return key, nil
	}
	value, err, _ := m.keyFlight.Do(keyID, func() (any, error) {
		if key := m.cachedKey(keyID); key != nil {
			return key, nil
		}
		if key, err := m.readRedisKey(ctx, keyID); err == nil {
			m.rememberKey(keyID, key)
			return key, nil
		}
		if m.store != nil {
			encoded, err := m.store.Load(ctx, keyID)
			if err != nil {
				return nil, err
			}
			return m.cachePersistedKey(ctx, keyID, encoded)
		}
		m.mu.RLock()
		key := m.localKeys[keyID]
		m.mu.RUnlock()
		if key == nil {
			return nil, errors.New("credential key not found")
		}
		m.rememberKey(keyID, key)
		return key, nil
	})
	if err != nil {
		return nil, err
	}
	key, ok := value.(*rsa.PrivateKey)
	if !ok || key == nil {
		return nil, errors.New("credential key cache returned an invalid key")
	}
	return key, nil
}

func (m *CredentialCipher) loadOrCreateEphemeralKey(ctx context.Context, keyID string) (*rsa.PrivateKey, error) {
	if m.redis != nil {
		generated, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, err
		}
		encoded, err := marshalCredentialPrivateKey(generated)
		if err != nil {
			return nil, err
		}
		created, err := m.redis.SetNX(ctx, credentialRedisPrefix+keyID, encoded, credentialRedisTTL(keyID)).Result()
		if err == nil && !created {
			generated, err = m.readRedisKey(ctx, keyID)
		}
		if err == nil {
			m.rememberKey(keyID, generated)
			return generated, nil
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	key := m.localKeys[keyID]
	if key == nil {
		generated, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, err
		}
		key = generated
		m.localKeys[keyID] = key
		m.pruneLocalKeysLocked(credentialNow().UTC())
	}
	m.keyCache[keyID] = key
	return key, nil
}

func (m *CredentialCipher) cachePersistedKey(ctx context.Context, keyID, encoded string) (*rsa.PrivateKey, error) {
	key, err := parseCredentialPrivateKey(encoded)
	if err != nil {
		return nil, err
	}
	if m.redis != nil {
		_ = m.redis.Set(ctx, credentialRedisPrefix+keyID, encoded, credentialRedisTTL(keyID)).Err()
	}
	m.rememberKey(keyID, key)
	return key, nil
}

func (m *CredentialCipher) readRedisKey(ctx context.Context, keyID string) (*rsa.PrivateKey, error) {
	if m.redis == nil {
		return nil, redis.Nil
	}
	encoded, err := m.redis.Get(ctx, credentialRedisPrefix+keyID).Result()
	if err != nil {
		return nil, err
	}
	return parseCredentialPrivateKey(encoded)
}

func (m *CredentialCipher) cachedKey(keyID string) *rsa.PrivateKey {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.keyCache[keyID]
}

func (m *CredentialCipher) derivedKey(keyID string, privateKey *rsa.PrivateKey) (*credentialDerivedKey, error) {
	m.mu.RLock()
	cached := m.derived[keyID]
	m.mu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, err
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	derived := &credentialDerivedKey{
		publicKey: base64.RawStdEncoding.EncodeToString(publicDER),
		macSecret: sha256.Sum256(privateDER),
	}
	zeroBytes(privateDER)

	m.mu.Lock()
	if existing := m.derived[keyID]; existing != nil {
		m.mu.Unlock()
		return existing, nil
	}
	m.derived[keyID] = derived
	for cachedID := range m.derived {
		if cachedID != keyID && len(m.derived) > 2 {
			delete(m.derived, cachedID)
		}
	}
	m.mu.Unlock()
	return derived, nil
}

func (m *CredentialCipher) rememberKey(keyID string, key *rsa.PrivateKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keyCache[keyID] = key
	for cachedID := range m.keyCache {
		if cachedID != keyID && len(m.keyCache) > 2 {
			delete(m.keyCache, cachedID)
		}
	}
}

func (m *CredentialCipher) pruneLocalKeysLocked(now time.Time) {
	for keyID := range m.localKeys {
		if !credentialKeyIDAllowed(keyID, now) {
			delete(m.localKeys, keyID)
		}
	}
}

func readCredentialRequestBody(c *gin.Context) ([]byte, bool) {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		abortCredentialEnvelope(c, "invalid request body")
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, credentialRequestMaxBytes+1))
	_ = c.Request.Body.Close()
	if err != nil {
		abortCredentialEnvelope(c, "invalid request body")
		return nil, false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if len(body) > credentialRequestMaxBytes {
		response.Error(c, http.StatusRequestEntityTooLarge, "request body is too large")
		c.Abort()
		return nil, false
	}
	return body, true
}

func abortCredentialEnvelope(c *gin.Context, message string) {
	if c == nil {
		return
	}
	response.ErrorWithDetails(c, http.StatusBadRequest, message, "INVALID_CREDENTIAL_ENVELOPE", nil)
	c.Abort()
}

func abortCredentialBrowserFlow(c *gin.Context) {
	response.ErrorWithDetails(c, http.StatusForbidden, "browser credential flow is required", "CREDENTIAL_BROWSER_FLOW_REQUIRED", nil)
	c.Abort()
}

func credentialKeySlotStart(now time.Time) time.Time {
	seconds := int64(credentialKeySlotDuration / time.Second)
	return time.Unix((now.Unix()/seconds)*seconds, 0).UTC()
}

func credentialKeyID(now time.Time) string {
	return strconv.FormatInt(credentialKeySlotStart(now).Unix(), 36)
}

func credentialKeyIDAllowed(keyID string, now time.Time) bool {
	slotStartedAt, err := credentialKeySlotFromID(keyID)
	if err != nil || now.Before(slotStartedAt) {
		return false
	}
	return now.Before(slotStartedAt.Add(credentialKeySlotDuration + credentialKeyDecryptGrace))
}

func credentialKeySlotFromID(keyID string) (time.Time, error) {
	unixSeconds, err := strconv.ParseInt(keyID, 36, 64)
	if err != nil {
		return time.Time{}, errors.New("invalid credential key id")
	}
	slotStartedAt := time.Unix(unixSeconds, 0).UTC()
	if credentialKeyID(slotStartedAt) != keyID {
		return time.Time{}, errors.New("invalid credential key id")
	}
	return slotStartedAt, nil
}

func credentialRedisTTL(keyID string) time.Duration {
	slotStartedAt, err := credentialKeySlotFromID(keyID)
	if err != nil {
		return credentialKeySlotDuration + credentialKeyDecryptGrace
	}
	ttl := slotStartedAt.Add(credentialKeySlotDuration + credentialKeyDecryptGrace).Sub(credentialNow().UTC())
	if ttl <= 0 {
		return time.Second
	}
	return ttl
}

func marshalCredentialPrivateKey(key *rsa.PrivateKey) (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(der), nil
}

func parseCredentialPrivateKey(encoded string) (*rsa.PrivateKey, error) {
	der, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("credential key is %T, not RSA", parsed)
	}
	return key, key.Validate()
}

func decodeCredentialBase64(value string, maxBytes int) ([]byte, error) {
	if value == "" || base64.RawURLEncoding.DecodedLen(len(value)) > maxBytes {
		return nil, errors.New("invalid credential envelope encoding")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) > maxBytes {
		return nil, errors.New("invalid credential envelope encoding")
	}
	return decoded, nil
}

func zeroBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
