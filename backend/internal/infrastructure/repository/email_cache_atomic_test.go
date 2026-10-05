//go:build unit

package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newAtomicEmailCache(t *testing.T) (*emailCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return &emailCache{rdb: rdb}, mr
}

func TestEmailCodeAtomicFailureCapAndLegacyJSON(t *testing.T) {
	for _, notify := range []bool{false, true} {
		t.Run(map[bool]string{false: "registration", true: "notification"}[notify], func(t *testing.T) {
			cache, mr := newAtomicEmailCache(t)
			ctx := context.Background()
			key := verifyCodeKey("user@example.com")
			verify := cache.VerifyVerificationCode
			if notify {
				key, verify = notifyVerifyKey("user@example.com"), cache.VerifyNotifyVerificationCode
			}
			// A JSON value produced by the old binary is usable without migration.
			require.NoError(t, cache.rdb.Set(ctx, key, `{"Code":"123456","Attempts":1,"CreatedAt":"2026-10-05T00:00:00Z","ExpiresAt":"2026-10-05T00:15:00Z"}`, time.Minute).Err())
			var invalid, capped atomic.Int32
			var wg sync.WaitGroup
			for range 50 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					err := verify(ctx, "USER@example.com", "654321", 5)
					if errors.Is(err, service.ErrInvalidVerifyCode) {
						invalid.Add(1)
					}
					if errors.Is(err, service.ErrVerifyCodeMaxAttempts) {
						capped.Add(1)
					}
				}()
			}
			wg.Wait()
			require.EqualValues(t, 3, invalid.Load())
			require.EqualValues(t, 47, capped.Load())
			require.ErrorIs(t, verify(ctx, "user@example.com", "123456", 5), service.ErrVerifyCodeMaxAttempts)
			require.Equal(t, time.Minute, mr.TTL(key), "failed checks must not extend or remove the TTL")
			require.Equal(t, []string{key}, mr.Keys(), "no extra attempt key")
			// Sending a fresh code resets the counter in the original JSON key.
			require.NoError(t, cache.rdb.Set(ctx, key, `{"Code":"999999","Attempts":0}`, time.Minute).Err())
			require.ErrorIs(t, verify(ctx, "user@example.com", "123456", 5), service.ErrInvalidVerifyCode)
			require.NoError(t, verify(ctx, "user@example.com", "999999", 5))
			require.False(t, mr.Exists(key))
		})
	}
}

func TestEmailCodeAtomicSingleUseAndMalformedValues(t *testing.T) {
	cache, mr := newAtomicEmailCache(t)
	ctx := context.Background()
	key := verifyCodeKey("user@example.com")
	for _, raw := range []string{`broken`, `[]`, `{"Code":0}`, `{"Code":""}`} {
		require.NoError(t, cache.rdb.Set(ctx, key, raw, time.Minute).Err())
		require.ErrorIs(t, cache.VerifyVerificationCode(ctx, "user@example.com", "123456", 5), service.ErrInvalidVerifyCode)
	}
	data := &service.VerificationCodeData{Code: "123456", ExpiresAt: time.Now().Add(time.Minute)}
	require.NoError(t, cache.SetVerificationCode(ctx, "user@example.com", data, time.Minute))
	var success atomic.Int32
	var wg sync.WaitGroup
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if cache.VerifyVerificationCode(ctx, "user@example.com", "123456", 5) == nil {
				success.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, success.Load())
	require.False(t, mr.Exists(key))
	require.NoError(t, cache.SetVerificationCode(ctx, "user@example.com", data, time.Minute))
	mr.FastForward(time.Minute)
	require.ErrorIs(t, cache.VerifyVerificationCode(ctx, "user@example.com", "123456", 5), service.ErrInvalidVerifyCode)
}

func TestEmailResetTokenAtomicHashAndLegacyConsumption(t *testing.T) {
	token := strings.Repeat("a", 64)
	digest := sha256.Sum256([]byte(token))
	hash := "sha256:" + hex.EncodeToString(digest[:])
	for _, stored := range []string{token, hash} {
		t.Run(map[bool]string{true: "hash", false: "legacy"}[stored == hash], func(t *testing.T) {
			cache, mr := newAtomicEmailCache(t)
			ctx := context.Background()
			require.NoError(t, cache.SetPasswordResetToken(ctx, "user@example.com", &service.PasswordResetTokenData{Token: stored}, time.Minute))
			if stored == hash {
				raw, err := mr.Get(passwordResetKey("user@example.com"))
				require.NoError(t, err)
				require.NotContains(t, raw, token)
			}
			ok, err := cache.ConsumePasswordResetToken(ctx, "user@example.com", "wrong", "sha256:wrong")
			require.NoError(t, err)
			require.False(t, ok)
			require.True(t, mr.Exists(passwordResetKey("user@example.com")))
			var success atomic.Int32
			var wg sync.WaitGroup
			for range 30 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					ok, err := cache.ConsumePasswordResetToken(ctx, "USER@example.com", token, hash)
					if err == nil && ok {
						success.Add(1)
					}
				}()
			}
			wg.Wait()
			require.EqualValues(t, 1, success.Load())
			require.False(t, mr.Exists(passwordResetKey("user@example.com")))
		})
	}
}
