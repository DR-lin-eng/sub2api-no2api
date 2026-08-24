package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type openAIQuotaCacheRepo struct {
	AccountRepository
	updates map[int64]map[string]any
	err     error
}

func (r *openAIQuotaCacheRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	if r.err != nil {
		return r.err
	}
	if r.updates == nil {
		r.updates = make(map[int64]map[string]any)
	}
	r.updates[id] = updates
	return nil
}

func TestCacheResetCreditsSnapshot(t *testing.T) {
	t.Run("complete snapshot is persisted", func(t *testing.T) {
		repo := &openAIQuotaCacheRepo{}
		svc := &OpenAIQuotaService{accountRepo: repo}
		credits := &OpenAIRateLimitResetCredits{
			AvailableCount: 1,
			Credits:        []OpenAIRateLimitResetCreditDetail{{ExpiresAt: "2026-08-04T00:00:00Z"}},
		}

		require.NoError(t, svc.CacheResetCreditsSnapshot(context.Background(), 42, credits))
		require.Equal(t, credits, repo.updates[42][openAIQuotaResetCreditsKey])
	})

	t.Run("zero count allows an empty expiration list", func(t *testing.T) {
		repo := &openAIQuotaCacheRepo{}
		svc := &OpenAIQuotaService{accountRepo: repo}
		credits := &OpenAIRateLimitResetCredits{AvailableCount: 0}

		require.NoError(t, svc.CacheResetCreditsSnapshot(context.Background(), 42, credits))
		require.Equal(t, credits, repo.updates[42][openAIQuotaResetCreditsKey])
	})

	for _, tc := range []struct {
		name    string
		credits *OpenAIRateLimitResetCredits
	}{
		{name: "nil snapshot", credits: nil},
		{name: "positive count without expirations", credits: &OpenAIRateLimitResetCredits{AvailableCount: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &openAIQuotaCacheRepo{}
			svc := &OpenAIQuotaService{accountRepo: repo}

			require.Error(t, svc.CacheResetCreditsSnapshot(context.Background(), 42, tc.credits))
			require.Empty(t, repo.updates)
		})
	}

	t.Run("repository error is preserved as cause", func(t *testing.T) {
		repoErr := errors.New("database unavailable")
		repo := &openAIQuotaCacheRepo{err: repoErr}
		svc := &OpenAIQuotaService{accountRepo: repo}

		err := svc.CacheResetCreditsSnapshot(context.Background(), 42, &OpenAIRateLimitResetCredits{
			AvailableCount: 1,
			Credits:        []OpenAIRateLimitResetCreditDetail{{ExpiresAt: "2026-08-04T00:00:00Z"}},
		})

		require.ErrorIs(t, err, repoErr)
	})
}

func TestCacheRateLimitSnapshot(t *testing.T) {
	t.Run("persists only the rate-limit fields", func(t *testing.T) {
		repo := &openAIQuotaCacheRepo{}
		svc := &OpenAIQuotaService{accountRepo: repo}
		usage := &OpenAIQuotaUsage{
			FetchedAt: 123,
			RateLimitsByLimitID: map[string]OpenAIAppServerRateLimitBucket{
				"codex": {
					LimitID: "codex",
					Primary: &OpenAIAppServerRateLimitWindow{UsedPercent: 77, WindowDurationMins: 10080, ResetsAt: 1730947200},
				},
			},
			ServerTokenUsage: &OpenAIServerTokenUsage{Summary: OpenAITokenUsageSummary{LifetimeTokens: int64Ptr(999)}},
		}

		require.NoError(t, svc.CacheRateLimitSnapshot(context.Background(), 42, usage))
		got, ok := repo.updates[42][openAIQuotaRateLimitSnapshotKey].(*OpenAIRateLimitSnapshot)
		require.True(t, ok)
		require.Equal(t, int64(123), got.FetchedAt)
		require.Equal(t, float64(77), got.RateLimitsByLimitID["codex"].Primary.UsedPercent)
	})

	t.Run("clears an old snapshot when no bucket is returned", func(t *testing.T) {
		repo := &openAIQuotaCacheRepo{}
		svc := &OpenAIQuotaService{accountRepo: repo}

		require.NoError(t, svc.CacheRateLimitSnapshot(context.Background(), 42, &OpenAIQuotaUsage{}))
		value, exists := repo.updates[42][openAIQuotaRateLimitSnapshotKey]
		require.True(t, exists)
		require.Nil(t, value)
	})

	t.Run("wraps repository errors", func(t *testing.T) {
		repoErr := errors.New("database unavailable")
		repo := &openAIQuotaCacheRepo{err: repoErr}
		svc := &OpenAIQuotaService{accountRepo: repo}

		err := svc.CacheRateLimitSnapshot(context.Background(), 42, &OpenAIQuotaUsage{
			RateLimitsByLimitID: map[string]OpenAIAppServerRateLimitBucket{
				"codex": {LimitID: "codex"},
			},
		})
		require.ErrorIs(t, err, repoErr)
	})
}
