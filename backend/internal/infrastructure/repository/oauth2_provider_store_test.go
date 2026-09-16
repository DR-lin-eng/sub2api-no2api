//go:build unit

package repository

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOAuth2ProviderStoreConsumesAuthorizationCodeOnceAndExpiresTokens(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewOAuth2ProviderStore(client)
	ctx := context.Background()

	code := &service.OAuth2AuthorizationCodeData{
		ClientID:     "client-1",
		UserID:       42,
		RedirectURI:  "https://client.example.test/callback",
		Scopes:       []string{"profile"},
		TokenVersion: 7,
		IssuedAt:     time.Now().UTC(),
	}
	require.NoError(t, store.StoreAuthorizationCode(ctx, "code-hash", code, time.Minute))
	const contenders = 12
	var wait sync.WaitGroup
	wait.Add(contenders)
	results := make(chan error, contenders)
	for range contenders {
		go func() {
			defer wait.Done()
			consumed, err := store.ConsumeAuthorizationCode(ctx, "code-hash")
			if err == nil && (consumed.ClientID != code.ClientID || consumed.UserID != code.UserID) {
				err = errors.New("consumed authorization code payload mismatch")
			}
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	successes := 0
	notFound := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, service.ErrOAuth2GrantNotFound):
			notFound++
		default:
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, contenders-1, notFound)

	token := &service.OAuth2AccessTokenData{
		ClientID:  "client-1",
		UserID:    42,
		Scopes:    []string{"profile"},
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	require.NoError(t, store.StoreAccessToken(ctx, "token-hash", token, time.Minute))
	loaded, err := store.GetAccessToken(ctx, "token-hash")
	require.NoError(t, err)
	require.Equal(t, token.ClientID, loaded.ClientID)

	server.FastForward(time.Minute + time.Second)
	_, err = store.GetAccessToken(ctx, "token-hash")
	require.True(t, errors.Is(err, service.ErrOAuth2GrantNotFound))
}
