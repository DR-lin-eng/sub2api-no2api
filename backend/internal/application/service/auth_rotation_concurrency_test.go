package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type rotationRaceCache struct {
	RefreshTokenCache
	data         RefreshTokenData
	readers      atomic.Int32
	allRead      chan struct{}
	participants int32
	consumed     atomic.Bool
	stored       atomic.Int32
	consumeErr   error
}

func (c *rotationRaceCache) GetRefreshToken(context.Context, string) (*RefreshTokenData, error) {
	if c.readers.Add(1) == c.participants {
		close(c.allRead)
	}
	<-c.allRead
	copy := c.data
	return &copy, nil
}

func (c *rotationRaceCache) ConsumeRefreshToken(context.Context, string) (*RefreshTokenData, error) {
	if c.consumeErr != nil {
		return nil, c.consumeErr
	}
	if !c.consumed.CompareAndSwap(false, true) {
		return nil, ErrRefreshTokenNotFound
	}
	copy := c.data
	return &copy, nil
}

func (c *rotationRaceCache) StoreRefreshToken(context.Context, string, *RefreshTokenData, time.Duration) error {
	c.stored.Add(1)
	return nil
}
func (*rotationRaceCache) AddToUserTokenSet(context.Context, int64, string, time.Duration) error {
	return nil
}
func (*rotationRaceCache) AddToFamilyTokenSet(context.Context, string, string, time.Duration) error {
	return nil
}

type rotationRaceUsers struct {
	UserRepository
	user User
}

func (r *rotationRaceUsers) GetByID(context.Context, int64) (*User, error) {
	copy := r.user
	return &copy, nil
}

func TestRefreshTokenRotationConcurrentReadersIssueOnePair(t *testing.T) {
	const callers = 16
	user := User{ID: 1, Email: "rotation@example.test", Role: RoleUser, Status: StatusActive}
	cache := &rotationRaceCache{
		data:    RefreshTokenData{UserID: user.ID, TokenVersion: resolvedTokenVersion(&user), FamilyID: "session", ExpiresAt: time.Now().Add(time.Hour)},
		allRead: make(chan struct{}), participants: callers,
	}
	svc := &AuthService{userRepo: &rotationRaceUsers{user: user}, refreshTokenCache: cache,
		cfg: &config.Config{JWT: config.JWTConfig{Secret: "fixture-only-signing-secret", ExpireHour: 1, RefreshTokenExpireDays: 30}}}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			pair, err := svc.RefreshTokenPair(context.Background(), "rt_original")
			if err == nil {
				winners.Add(1)
				require.NotEmpty(t, pair.RefreshToken)
				require.NotEmpty(t, pair.AccessToken)
			} else {
				require.ErrorIs(t, err, ErrRefreshTokenInvalid)
			}
		})
	}
	wg.Wait()
	require.Equal(t, int32(1), winners.Load())
	require.Equal(t, int32(1), cache.stored.Load())
}

func TestRefreshTokenRotationFailsClosedWhenConsumeIsUnavailable(t *testing.T) {
	user := User{ID: 1, Email: "rotation@example.test", Status: StatusActive}
	cache := &rotationRaceCache{
		data:    RefreshTokenData{UserID: user.ID, TokenVersion: resolvedTokenVersion(&user), ExpiresAt: time.Now().Add(time.Hour)},
		allRead: make(chan struct{}), participants: 1, consumeErr: errors.New("redis unavailable"),
	}
	svc := &AuthService{userRepo: &rotationRaceUsers{user: user}, refreshTokenCache: cache}
	pair, err := svc.RefreshTokenPair(context.Background(), "rt_original")
	require.Nil(t, pair)
	require.ErrorIs(t, err, ErrServiceUnavailable)
	require.Zero(t, cache.stored.Load())
}
