package payment

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// AES256KeySize is the required key length (in bytes) for AES-256-GCM.
const AES256KeySize = 32

// Encrypt encrypts plaintext using AES-256-GCM with the given 32-byte key.
// The output format is "iv:authTag:ciphertext" where each component is base64-encoded,
// matching the Node.js crypto.ts format for cross-compatibility.
//
// Provider configs use this format at rest. The random nonce makes repeated
// saves of the same JSON produce independent ciphertexts.
func Encrypt(plaintext string, key []byte) (string, error) {
	if len(key) != AES256KeySize {
		return "", fmt.Errorf("encryption key must be %d bytes, got %d", AES256KeySize, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize()) // 12 bytes for GCM
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	// Seal appends the ciphertext + auth tag
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// Split sealed into ciphertext and auth tag (last 16 bytes)
	tagSize := gcm.Overhead()
	ciphertext := sealed[:len(sealed)-tagSize]
	authTag := sealed[len(sealed)-tagSize:]

	// Format: iv:authTag:ciphertext (all base64)
	return fmt.Sprintf("%s:%s:%s",
		base64.StdEncoding.EncodeToString(nonce),
		base64.StdEncoding.EncodeToString(authTag),
		base64.StdEncoding.EncodeToString(ciphertext),
	), nil
}

// Decrypt decrypts a ciphertext string produced by Encrypt.
// The input format is "iv:authTag:ciphertext" where each component is base64-encoded.
//
// Decrypt accepts the provider-config ciphertext format and is also used to
// read records written before the current encryption path was restored.
func Decrypt(ciphertext string, key []byte) (string, error) {
	if len(key) != AES256KeySize {
		return "", fmt.Errorf("encryption key must be %d bytes, got %d", AES256KeySize, len(key))
	}

	parts := strings.SplitN(ciphertext, ":", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid ciphertext format: expected iv:authTag:ciphertext")
	}

	nonce, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode IV: %w", err)
	}

	authTag, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode auth tag: %w", err)
	}

	encrypted, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	if len(nonce) != gcm.NonceSize() || len(authTag) != gcm.Overhead() {
		return "", fmt.Errorf("invalid ciphertext nonce or authentication tag length")
	}

	// Reconstruct the sealed data: ciphertext + authTag
	sealed := append(encrypted, authTag...)

	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// DecodeProviderConfig preserves legacy plaintext reads while failing closed
// for malformed ciphertext or unavailable keys. It never treats unreadable
// secrets as an empty editable config.
func DecodeProviderConfig(stored string, key []byte) (map[string]string, error) {
	if strings.TrimSpace(stored) == "" {
		return nil, nil
	}
	var cfg map[string]string
	if err := json.Unmarshal([]byte(stored), &cfg); err == nil {
		return cfg, nil
	}
	plaintext, err := Decrypt(stored, key)
	if err != nil {
		return nil, fmt.Errorf("decrypt provider config: %w", err)
	}
	if err := json.Unmarshal([]byte(plaintext), &cfg); err != nil {
		return nil, fmt.Errorf("decode provider config: %w", err)
	}
	return cfg, nil
}
