//go:build integration

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAILocationPostgresFirstWriteWinsAndSurvivesStaleEdit(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	first := newAccountRepositoryWithSQL(client, integrationDB, nil)
	second := newAccountRepositoryWithSQL(client, integrationDB, nil)
	account := &service.Account{Name: "location-fixture", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture"}, Extra: map[string]any{"keep": true}, Status: service.StatusActive, Concurrency: 1, Schedulable: true}
	require.NoError(t, first.Create(ctx, account))
	t.Cleanup(func() { _ = first.Delete(ctx, account.ID) })
	stale, err := first.GetByID(ctx, account.ID)
	require.NoError(t, err)
	var wg sync.WaitGroup
	type result struct {
		offset service.OpenAILocationOffset
		err    error
	}
	results := make(chan result, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := first
			if i%2 == 0 {
				r = second
			}
			offset, err := r.GetOrCreateOpenAILocationOffset(ctx, account.ID, service.OpenAILocationOffset{NorthMeters: float64(i + 1), EastMeters: 100})
			results <- result{offset, err}
		}(i)
	}
	wg.Wait()
	close(results)
	var winner service.OpenAILocationOffset
	for result := range results {
		require.NoError(t, result.err)
		if winner.NorthMeters == 0 {
			winner = result.offset
		}
		require.Equal(t, winner, result.offset)
	}
	stale.Name = "stale location edit"
	require.NoError(t, second.Update(ctx, stale))
	// A new repository models a restart with no process-local location state.
	restarted := newAccountRepositoryWithSQL(client, integrationDB, nil)
	stored, err := restarted.GetOrCreateOpenAILocationOffset(ctx, account.ID, service.OpenAILocationOffset{EastMeters: 499})
	require.NoError(t, err)
	require.Equal(t, winner, stored)
	loaded, err := first.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, true, loaded.Extra["keep"])

	_, err = client.ExecContext(ctx, `UPDATE accounts SET extra = extra || '{"openai_location_persistent_offset":{"north_meters":"invalid"}}'::jsonb WHERE id=$1`, account.ID)
	require.NoError(t, err)
	stored, err = first.GetOrCreateOpenAILocationOffset(ctx, account.ID, service.OpenAILocationOffset{NorthMeters: 3, EastMeters: 4})
	require.NoError(t, err)
	require.Equal(t, service.OpenAILocationOffset{NorthMeters: 3, EastMeters: 4}, stored)
}

func TestOpenAILocationRedisFixedTTLAndReplicaConvergence(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	first, second := &gatewayCache{rdb: rdb}, &gatewayCache{rdb: rdb}
	value, remaining, err := first.GetOrCreateOpenAILocation(ctx, "account-fixture", "1,2,3", 10*time.Minute)
	require.NoError(t, err)
	require.Equal(t, "1,2,3", value)
	require.Positive(t, remaining)
	require.NoError(t, rdb.PExpire(ctx, "openai:location:account-fixture", time.Minute).Err())
	value, remaining, err = second.GetOrCreateOpenAILocation(ctx, "account-fixture", "4,5,20", 10*time.Minute)
	require.NoError(t, err)
	require.Equal(t, "1,2,3", value)
	require.LessOrEqual(t, remaining, time.Minute)
	require.NoError(t, rdb.Del(ctx, "openai:location:account-fixture").Err())
	value, _, err = second.GetOrCreateOpenAILocation(ctx, "account-fixture", "4,5,20", 10*time.Minute)
	require.NoError(t, err)
	require.Equal(t, "4,5,20", value)
}
