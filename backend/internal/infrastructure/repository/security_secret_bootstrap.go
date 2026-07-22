package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/securitysecret"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
)

const (
	securitySecretKeyJWT        = "jwt_secret"
	securitySecretKeyTOTP       = "totp_encryption_key"
	securitySecretReadRetryMax  = 5
	securitySecretReadRetryWait = 10 * time.Millisecond
)

var readRandomBytes = rand.Read

func ensureBootstrapSecrets(ctx context.Context, client *ent.Client, cfg *config.Config) error {
	if client == nil {
		return fmt.Errorf("nil ent client")
	}
	if cfg == nil {
		return fmt.Errorf("nil config")
	}

	if err := ensureJWTSecret(ctx, client, cfg); err != nil {
		return err
	}
	if err := ensureTOTPEncryptionKey(ctx, client, cfg); err != nil {
		return err
	}
	return nil
}

func ensureJWTSecret(ctx context.Context, client *ent.Client, cfg *config.Config) error {
	cfg.JWT.Secret = strings.TrimSpace(cfg.JWT.Secret)
	if cfg.JWT.Secret != "" {
		storedSecret, err := createSecuritySecretIfAbsent(ctx, client, securitySecretKeyJWT, cfg.JWT.Secret)
		if err != nil {
			return fmt.Errorf("persist jwt secret: %w", err)
		}
		if storedSecret != cfg.JWT.Secret {
			log.Println("Warning: configured JWT secret mismatches persisted value; using persisted secret for cross-instance consistency.")
		}
		cfg.JWT.Secret = storedSecret
		return nil
	}

	secret, created, err := getOrCreateGeneratedSecuritySecret(ctx, client, securitySecretKeyJWT, 32)
	if err != nil {
		return fmt.Errorf("ensure jwt secret: %w", err)
	}
	cfg.JWT.Secret = secret

	if created {
		log.Println("Warning: JWT secret auto-generated and persisted to database. Consider rotating to a managed secret for production.")
	}
	return nil
}

func ensureTOTPEncryptionKey(ctx context.Context, client *ent.Client, cfg *config.Config) error {
	configuredKey := strings.TrimSpace(cfg.Totp.EncryptionKey)
	var (
		storedKey string
		created   bool
		err       error
	)
	if cfg.Totp.EncryptionKeyConfigured {
		if err := validateTOTPEncryptionKey(configuredKey); err != nil {
			return fmt.Errorf("invalid configured totp encryption key: %w", err)
		}
		storedKey, err = createSecuritySecretIfAbsent(ctx, client, securitySecretKeyTOTP, configuredKey)
		if err != nil {
			return fmt.Errorf("persist totp encryption key: %w", err)
		}
		if storedKey != configuredKey {
			log.Println("Warning: configured TOTP encryption key mismatches persisted value; using persisted key for cross-instance consistency.")
		}
	} else {
		storedKey, created, err = getOrCreateGeneratedSecuritySecret(ctx, client, securitySecretKeyTOTP, 32)
		if err != nil {
			return fmt.Errorf("ensure totp encryption key: %w", err)
		}
	}
	if err := validateTOTPEncryptionKey(storedKey); err != nil {
		return fmt.Errorf("invalid persisted totp encryption key: %w", err)
	}
	cfg.Totp.EncryptionKey = storedKey
	// A database-managed key is stable across replicas and is safe to enable.
	cfg.Totp.EncryptionKeyConfigured = true
	if created {
		log.Println("Warning: TOTP encryption key auto-generated and persisted to database. Consider rotating to a managed secret for production.")
	}
	return nil
}

func validateTOTPEncryptionKey(value string) error {
	decoded, err := hex.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("must be hex encoded: %w", err)
	}
	if len(decoded) != 32 {
		return fmt.Errorf("must decode to exactly 32 bytes, got %d", len(decoded))
	}
	return nil
}

func getOrCreateGeneratedSecuritySecret(ctx context.Context, client *ent.Client, key string, byteLength int) (string, bool, error) {
	existing, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(key)).Only(ctx)
	if err == nil {
		value := strings.TrimSpace(existing.Value)
		if len([]byte(value)) < 32 {
			return "", false, fmt.Errorf("stored secret %q must be at least 32 bytes", key)
		}
		return value, false, nil
	}
	if !ent.IsNotFound(err) {
		return "", false, err
	}

	generated, err := generateHexSecret(byteLength)
	if err != nil {
		return "", false, err
	}

	if err := client.SecuritySecret.Create().
		SetKey(key).
		SetValue(generated).
		OnConflictColumns(securitysecret.FieldKey).
		DoNothing().
		Exec(ctx); err != nil {
		if !isSQLNoRowsError(err) {
			return "", false, err
		}
	}

	stored, err := querySecuritySecretWithRetry(ctx, client, key)
	if err != nil {
		return "", false, err
	}
	value := strings.TrimSpace(stored.Value)
	if len([]byte(value)) < 32 {
		return "", false, fmt.Errorf("stored secret %q must be at least 32 bytes", key)
	}
	return value, value == generated, nil
}

func createSecuritySecretIfAbsent(ctx context.Context, client *ent.Client, key, value string) (string, error) {
	value = strings.TrimSpace(value)
	if len([]byte(value)) < 32 {
		return "", fmt.Errorf("secret %q must be at least 32 bytes", key)
	}

	if err := client.SecuritySecret.Create().
		SetKey(key).
		SetValue(value).
		OnConflictColumns(securitysecret.FieldKey).
		DoNothing().
		Exec(ctx); err != nil {
		if !isSQLNoRowsError(err) {
			return "", err
		}
	}

	stored, err := querySecuritySecretWithRetry(ctx, client, key)
	if err != nil {
		return "", err
	}
	storedValue := strings.TrimSpace(stored.Value)
	if len([]byte(storedValue)) < 32 {
		return "", fmt.Errorf("stored secret %q must be at least 32 bytes", key)
	}
	return storedValue, nil
}

func querySecuritySecretWithRetry(ctx context.Context, client *ent.Client, key string) (*ent.SecuritySecret, error) {
	var lastErr error
	for attempt := 0; attempt <= securitySecretReadRetryMax; attempt++ {
		stored, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(key)).Only(ctx)
		if err == nil {
			return stored, nil
		}
		if !isSecretNotFoundError(err) {
			return nil, err
		}
		lastErr = err
		if attempt == securitySecretReadRetryMax {
			break
		}

		timer := time.NewTimer(securitySecretReadRetryWait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func isSecretNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return ent.IsNotFound(err) || isSQLNoRowsError(err)
}

func isSQLNoRowsError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows in result set")
}

func generateHexSecret(byteLength int) (string, error) {
	if byteLength <= 0 {
		byteLength = 32
	}
	buf := make([]byte, byteLength)
	if _, err := readRandomBytes(buf); err != nil {
		return "", fmt.Errorf("generate random secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
