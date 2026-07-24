//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newDurableBillingQueueIntegrationRepo() *queuedUsageBillingRepository {
	return &queuedUsageBillingRepository{
		direct:         &usageBillingRepository{db: integrationDB},
		db:             integrationDB,
		rdb:            integrationRedis,
		consumerCount:  2,
		readBatchSize:  128,
		pollInterval:   10 * time.Millisecond,
		commandTimeout: 15 * time.Second,
		maxRetryDelay:  time.Second,
		enqueueCh:      make(chan usageBillingEnqueueRequest, usageBillingEnqueueQueueSize),
		wakeCh:         make(chan struct{}, 2),
	}
}

func resetDurableBillingQueueTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), `
		TRUNCATE usage_billing_jobs, usage_billing_dead_letters, usage_billing_dedup RESTART IDENTITY
	`)
	require.NoError(t, err)
	require.NoError(t, integrationRedis.FlushDB(context.Background()).Err())
}

func TestDurableUsageBillingQueueSurvivesRedisLoss(t *testing.T) {
	resetDurableBillingQueueTables(t)
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("durable-billing-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-durable-" + uuid.NewString(),
		Name:   "durable",
		Quota:  100,
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "durable-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})
	cmd := service.UsageBillingCommand{
		RequestID:             uuid.NewString(),
		APIKeyID:              apiKey.ID,
		UserID:                user.ID,
		AccountID:             account.ID,
		AccountType:           service.AccountTypeAPIKey,
		BalanceCost:           1.25,
		APIKeyQuotaCost:       1.25,
		QuotaPlatform:         service.PlatformOpenAI,
		UserPlatformQuotaCost: 1.25,
	}
	cmd.Normalize()
	payload, err := json.Marshal(&cmd)
	require.NoError(t, err)
	input, err := json.Marshal([]usageBillingBatchInput{{
		RequestID:          cmd.RequestID,
		APIKeyID:           cmd.APIKeyID,
		RequestFingerprint: cmd.RequestFingerprint,
		Payload:            payload,
	}})
	require.NoError(t, err)

	repo := newDurableBillingQueueIntegrationRepo()
	statuses, err := repo.insertEnqueueBatch(ctx, input)
	require.NoError(t, err)
	require.Equal(t, usageBillingEnqueueInserted, statuses[usageBillingRequestKey(cmd.RequestID, cmd.APIKeyID)].status)
	repo.reconcilePendingOverlay(&cmd)

	// Simulate total Redis data loss before PostgreSQL settlement.
	require.NoError(t, integrationRedis.FlushDB(ctx).Err())
	repo.recoverPendingOverlays()
	require.InDelta(t, 1.25, mustRedisFloat(t, integrationRedis, usageBillingPendingBalanceKey(user.ID)), 1e-9)
	require.InDelta(t, 1.25, mustRedisFloat(t, integrationRedis, usageBillingPendingAPIKeyUsageKey(apiKey.ID)), 1e-9)
	processed, err := repo.processJobBatch(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	var balance, quotaUsed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used FROM api_keys WHERE id = $1", apiKey.ID).Scan(&quotaUsed))
	require.InDelta(t, 98.75, balance, 1e-9)
	require.InDelta(t, 1.25, quotaUsed, 1e-9)
	var platformUsage float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT daily_usage_usd FROM user_platform_quotas
		WHERE user_id = $1 AND platform = $2 AND deleted_at IS NULL
	`, user.ID, service.PlatformOpenAI).Scan(&platformUsage))
	require.InDelta(t, 1.25, platformUsage, 1e-9)

	var jobs, dedup int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_jobs").Scan(&jobs))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2", cmd.RequestID, cmd.APIKeyID).Scan(&dedup))
	require.Zero(t, jobs)
	require.Equal(t, 1, dedup)
	require.Zero(t, mustRedisFloat(t, integrationRedis, usageBillingPendingAPIKeyUsageKey(apiKey.ID)))

	statuses, err = repo.insertEnqueueBatch(ctx, input)
	require.NoError(t, err)
	require.Equal(t, usageBillingEnqueueApplied, statuses[usageBillingRequestKey(cmd.RequestID, cmd.APIKeyID)].status)
}

func TestDurableUsageBillingQueueRetriesOverlayCompletionAfterPostgresCommit(t *testing.T) {
	resetDurableBillingQueueTables(t)
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("durable-billing-overlay-retry-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-durable-overlay-retry-" + uuid.NewString(),
		Name:   "durable-overlay-retry",
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "durable-overlay-retry-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})
	cmd := service.UsageBillingCommand{
		RequestID:       "durable-overlay-retry-" + uuid.NewString(),
		APIKeyID:        apiKey.ID,
		UserID:          user.ID,
		AccountID:       account.ID,
		AccountType:     service.AccountTypeAPIKey,
		BalanceCost:     1.25,
		APIKeyQuotaCost: 1.25,
	}
	cmd.Normalize()
	payload, err := json.Marshal(&cmd)
	require.NoError(t, err)
	input, err := json.Marshal([]usageBillingBatchInput{{
		RequestID:          cmd.RequestID,
		APIKeyID:           cmd.APIKeyID,
		RequestFingerprint: cmd.RequestFingerprint,
		Payload:            payload,
	}})
	require.NoError(t, err)

	repo := newDurableBillingQueueIntegrationRepo()
	statuses, err := repo.insertEnqueueBatch(ctx, input)
	require.NoError(t, err)
	require.Equal(t, usageBillingEnqueueInserted, statuses[usageBillingRequestKey(cmd.RequestID, cmd.APIKeyID)].status)
	repo.reconcilePendingOverlay(&cmd)
	require.InDelta(t, 1.25, mustRedisFloat(t, integrationRedis, usageBillingPendingAPIKeyUsageKey(apiKey.ID)), 1e-9)

	workingRedis := repo.rdb
	brokenRedis := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  25 * time.Millisecond,
		ReadTimeout:  25 * time.Millisecond,
		WriteTimeout: 25 * time.Millisecond,
		MaxRetries:   0,
	})
	repo.rdb = brokenRedis
	processed, err := repo.processJobBatch(ctx)
	require.Equal(t, 1, processed)
	require.Error(t, err)
	require.NoError(t, brokenRedis.Close())
	repo.rdb = workingRedis

	var balance float64
	var jobs int
	var settledAt *time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 98.75, balance, 1e-9)
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_jobs").Scan(&jobs))
	require.Equal(t, 1, jobs)
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT settled_at FROM usage_billing_jobs WHERE request_id = $1", cmd.RequestID).Scan(&settledAt))
	require.NotNil(t, settledAt)

	// Startup reconciliation treats settled jobs as completed and removes any
	// stale Redis overlay left by the failed post-commit cleanup.
	repo.recoverPendingOverlays()
	require.Zero(t, mustRedisFloat(t, integrationRedis, usageBillingPendingAPIKeyUsageKey(apiKey.ID)))
	_, err = integrationDB.ExecContext(ctx, "UPDATE usage_billing_jobs SET available_at = NOW()")
	require.NoError(t, err)
	processed, err = repo.processJobBatch(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_jobs").Scan(&jobs))
	require.Zero(t, jobs)
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 98.75, balance, 1e-9)
}

func TestDurableUsageBillingQueueConcurrentConsumersApplyExactlyOnce(t *testing.T) {
	resetDurableBillingQueueTables(t)
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("durable-billing-load-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      10000,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-durable-load-" + uuid.NewString(),
		Name:   "durable-load",
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "durable-load-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})

	const jobCount = 1000
	inputs := make([]usageBillingBatchInput, 0, jobCount)
	for i := 0; i < jobCount; i++ {
		cmd := service.UsageBillingCommand{
			RequestID:   fmt.Sprintf("durable-load-%s-%d", uuid.NewString(), i),
			APIKeyID:    apiKey.ID,
			UserID:      user.ID,
			AccountID:   account.ID,
			AccountType: service.AccountTypeAPIKey,
			BalanceCost: 0.01,
		}
		cmd.Normalize()
		payload, err := json.Marshal(&cmd)
		require.NoError(t, err)
		inputs = append(inputs, usageBillingBatchInput{
			RequestID:          cmd.RequestID,
			APIKeyID:           cmd.APIKeyID,
			RequestFingerprint: cmd.RequestFingerprint,
			Payload:            payload,
		})
	}
	batchJSON, err := json.Marshal(inputs)
	require.NoError(t, err)
	repoA := newDurableBillingQueueIntegrationRepo()
	statuses, err := repoA.insertEnqueueBatch(ctx, batchJSON)
	require.NoError(t, err)
	require.Len(t, statuses, jobCount)
	repoB := newDurableBillingQueueIntegrationRepo()

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for _, repo := range []*queuedUsageBillingRepository{repoA, repoB} {
		wg.Add(1)
		go func(repo *queuedUsageBillingRepository) {
			defer wg.Done()
			for {
				processed, processErr := repo.processJobBatch(ctx)
				if processErr != nil {
					errCh <- processErr
					return
				}
				if processed == 0 {
					return
				}
			}
		}(repo)
	}
	wg.Wait()
	close(errCh)
	for processErr := range errCh {
		require.NoError(t, processErr)
	}

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 9990, balance, 1e-7)
	var jobs, dedup int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_jobs").Scan(&jobs))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE api_key_id = $1", apiKey.ID).Scan(&dedup))
	require.Zero(t, jobs)
	require.Equal(t, jobCount, dedup)
}

func TestDurableUsageBillingQueueConcurrent50000(t *testing.T) {
	if os.Getenv("SUB2API_RUN_50K_BILLING_TEST") != "1" {
		t.Skip("set SUB2API_RUN_50K_BILLING_TEST=1 to run the 50k billing stress test")
	}

	resetDurableBillingQueueTables(t)
	client := testEntClient(t)
	const (
		jobCount         = 50_000
		userCount        = 3_000
		primaryHotJobs   = 2_000
		secondaryHotJobs = 200
		initialBalance   = 100_000.0
		costPerJob       = 0.001
	)
	type hotUserTarget struct {
		user    *service.User
		apiKey  *service.APIKey
		account *service.Account
		jobs    int
	}
	targets := make([]hotUserTarget, userCount)
	for i := range targets {
		user := mustCreateUser(t, client, &service.User{
			Email:        fmt.Sprintf("durable-billing-50k-%d-%d@example.com", time.Now().UnixNano(), i),
			PasswordHash: "hash",
			Balance:      initialBalance,
		})
		targets[i] = hotUserTarget{
			user: user,
			apiKey: mustCreateApiKey(t, client, &service.APIKey{
				UserID: user.ID,
				Key:    fmt.Sprintf("sk-durable-50k-%d-%s", i, uuid.NewString()),
				Name:   fmt.Sprintf("durable-50k-%d", i),
			}),
			account: mustCreateAccount(t, client, &service.Account{
				Name: fmt.Sprintf("durable-50k-account-%d-%s", i, uuid.NewString()),
				Type: service.AccountTypeAPIKey,
			}),
		}
	}

	prefix := uuid.NewString()
	commands := make([]service.UsageBillingCommand, jobCount)
	for i := range commands {
		targetIndex := 0
		switch {
		case i < primaryHotJobs:
			targetIndex = 0
		case i < primaryHotJobs+secondaryHotJobs:
			targetIndex = 1
		default:
			targetIndex = 2 + (i-primaryHotJobs-secondaryHotJobs)%(userCount-2)
		}
		target := &targets[targetIndex]
		target.jobs++
		commands[i] = service.UsageBillingCommand{
			RequestID:   fmt.Sprintf("durable-50k-%s-%d", prefix, i),
			APIKeyID:    target.apiKey.ID,
			UserID:      target.user.ID,
			AccountID:   target.account.ID,
			AccountType: service.AccountTypeAPIKey,
			BalanceCost: costPerJob,
		}
	}

	repo := newDurableBillingQueueIntegrationRepo()
	repo.consumerCount = 4
	repo.wakeCh = make(chan struct{}, repo.consumerCount)
	repo.start()
	defer repo.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	start := make(chan struct{})
	var ready sync.WaitGroup
	var done sync.WaitGroup
	var failures atomic.Int64
	firstErr := make(chan error, 1)
	ready.Add(jobCount)
	done.Add(jobCount)
	for i := range commands {
		go func(cmd *service.UsageBillingCommand) {
			defer done.Done()
			ready.Done()
			<-start
			if _, err := repo.Apply(ctx, cmd); err != nil {
				failures.Add(1)
				select {
				case firstErr <- err:
				default:
				}
			}
		}(&commands[i])
	}

	ready.Wait()
	startedAt := time.Now()
	close(start)
	done.Wait()
	enqueueElapsed := time.Since(startedAt)
	if failures.Load() != 0 {
		t.Fatalf("%d of %d billing submissions failed: %v", failures.Load(), jobCount, <-firstErr)
	}

	drainStartedAt := time.Now()
	var jobs, dedup int
	for {
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
			SELECT
				(SELECT COUNT(*) FROM usage_billing_jobs),
				(SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id LIKE $1)
		`, fmt.Sprintf("durable-50k-%s-%%", prefix)).Scan(&jobs, &dedup))
		if jobs == 0 && dedup == jobCount {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("billing queue did not drain: jobs=%d dedup=%d: %v", jobs, dedup, ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	drainElapsed := time.Since(drainStartedAt)
	repo.Stop()

	for _, target := range targets {
		var balance float64
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", target.user.ID).Scan(&balance))
		require.InDelta(t, initialBalance-float64(target.jobs)*costPerJob, balance, 1e-6)
	}
	require.Zero(t, jobs)
	require.Equal(t, jobCount, dedup)
	pendingBalance := 0.0
	for {
		pendingBalance = 0
		for _, target := range targets {
			pending, pendingErr := integrationRedis.Get(ctx, usageBillingPendingBalanceKey(target.user.ID)).Float64()
			if errors.Is(pendingErr, redis.Nil) {
				pending = 0
			} else {
				require.NoError(t, pendingErr)
			}
			pendingBalance += pending
		}
		if pendingBalance == 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("billing Redis overlay did not settle: pending_balance=%f", pendingBalance)
		case <-time.After(50 * time.Millisecond):
		}
	}
	activeMarkers := 0
	var cursor uint64
	for {
		keys, next, scanErr := integrationRedis.Scan(ctx, cursor, usageBillingOverlayPrefix+"*", 1000).Result()
		require.NoError(t, scanErr)
		activeMarkers += len(keys)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	require.Zero(t, pendingBalance)
	for _, target := range targets {
		require.Zero(t, mustRedisFloat(t, integrationRedis, usageBillingPendingAPIKeyUsageKey(target.apiKey.ID)))
	}
	require.Zero(t, activeMarkers)
	t.Logf(
		"jobs=%d concurrent=%d enqueue_elapsed=%s enqueue_rate=%.0f jobs/s drain_elapsed=%s balance=%.3f dedup=%d pending_balance=%.3f active_overlays=%d",
		jobCount,
		jobCount,
		enqueueElapsed,
		float64(jobCount)/enqueueElapsed.Seconds(),
		drainElapsed,
		initialBalance*float64(len(targets))-jobCount*costPerJob,
		dedup,
		pendingBalance,
		activeMarkers,
	)
}

func BenchmarkDurableUsageBillingQueueEnqueue(b *testing.B) {
	_, err := integrationDB.ExecContext(context.Background(), `
		TRUNCATE usage_billing_jobs, usage_billing_dead_letters, usage_billing_dedup RESTART IDENTITY
	`)
	require.NoError(b, err)
	repo := &queuedUsageBillingRepository{
		direct:         &usageBillingRepository{db: integrationDB},
		db:             integrationDB,
		consumerCount:  4,
		readBatchSize:  128,
		pollInterval:   50 * time.Millisecond,
		commandTimeout: 15 * time.Second,
		maxRetryDelay:  30 * time.Second,
		enqueueCh:      make(chan usageBillingEnqueueRequest, usageBillingEnqueueQueueSize),
		wakeCh:         make(chan struct{}, 4),
	}
	repo.start()
	defer repo.Stop()

	var sequence atomic.Uint64
	started := time.Now()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := sequence.Add(1)
			cmd := &service.UsageBillingCommand{
				RequestID: fmt.Sprintf("billing-benchmark-%d", n),
				APIKeyID:  1,
				UserID:    1,
			}
			if _, applyErr := repo.Apply(context.Background(), cmd); applyErr != nil {
				b.Error(applyErr)
				return
			}
		}
	})
	b.StopTimer()
	elapsed := time.Since(started)
	b.ReportMetric(float64(b.N)/elapsed.Minutes(), "rpm")
}
