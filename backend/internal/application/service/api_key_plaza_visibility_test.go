//go:build unit

package service

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

type plazaVisibilityUserRepo struct{ UserRepository }

func (plazaVisibilityUserRepo) GetByID(context.Context, int64) (*User, error) {
	return &User{AllowedGroups: []int64{1, 2}}, nil
}

type plazaVisibilitySubRepo struct {
	UserSubscriptionRepository
	err error
}

func (r plazaVisibilitySubRepo) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return []UserSubscription{{GroupID: 2}, {GroupID: 3}}, r.err
}
func TestPlazaVisibilityIncludesActiveSubscriptionsAndPreservesExplicitGrants(t *testing.T) {
	s := &APIKeyService{userRepo: plazaVisibilityUserRepo{}, userSubRepo: plazaVisibilitySubRepo{}}
	got, err := s.GetUserAllowedGroupIDSet(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, map[int64]struct{}{1: {}, 2: {}, 3: {}}, got)
	s.userSubRepo = plazaVisibilitySubRepo{err: errors.New("db unavailable")}
	_, err = s.GetUserAllowedGroupIDSet(context.Background(), 1)
	require.Error(t, err)
}
