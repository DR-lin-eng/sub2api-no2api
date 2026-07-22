package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type ownedProgressSubscriptionRepo struct {
	UserSubscriptionRepository
	sub          *UserSubscription
	wantID       int64
	wantUserID   int64
	ownedLookups int
	err          error
}

func (r *ownedProgressSubscriptionRepo) GetByIDAndUserID(_ context.Context, id, userID int64) (*UserSubscription, error) {
	r.ownedLookups++
	if r.err != nil {
		return nil, r.err
	}
	if id != r.wantID || userID != r.wantUserID || r.sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	return r.sub, nil
}

func TestGetSubscriptionProgressForUserUsesOwnedLookup(t *testing.T) {
	dailyLimit := 10.0
	windowStart := time.Now().Add(-time.Hour)
	repo := &ownedProgressSubscriptionRepo{
		wantID:     17,
		wantUserID: 42,
		sub: &UserSubscription{
			ID:               17,
			UserID:           42,
			GroupID:          9,
			ExpiresAt:        time.Now().Add(24 * time.Hour),
			DailyWindowStart: &windowStart,
			DailyUsageUSD:    2,
			Group: &Group{
				ID:            9,
				Name:          "owned",
				DailyLimitUSD: &dailyLimit,
			},
		},
	}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)

	progress, err := svc.GetSubscriptionProgressForUser(context.Background(), 17, 42)
	require.NoError(t, err)
	require.Equal(t, int64(17), progress.ID)
	require.Equal(t, 1, repo.ownedLookups)

	_, err = svc.GetSubscriptionProgressForUser(context.Background(), 17, 99)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	require.Equal(t, 2, repo.ownedLookups)

	dbErr := errors.New("database unavailable")
	repo.err = dbErr
	_, err = svc.GetSubscriptionProgressForUser(context.Background(), 17, 42)
	require.ErrorIs(t, err, dbErr)
}
