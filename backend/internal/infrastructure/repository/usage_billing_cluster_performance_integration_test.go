//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestDurableUsageBillingQueueClusterPerformance is intentionally opt-in. It
// provides a repeatable before/after workload for changes to cluster-wide
// billing consumption without making the ordinary integration suite expensive.
func TestDurableUsageBillingQueueClusterPerformance(t *testing.T) {
	if os.Getenv("SUB2API_RUN_CLUSTER_BILLING_STRESS") != "1" {
		t.Skip("set SUB2API_RUN_CLUSTER_BILLING_STRESS=1 to run the clustered billing stress test")
	}

	const (
		jobCount         = 50_000
		userCount        = 3_000
		primaryHotJobs   = 2_000
		secondaryHotJobs = 200
		initialBalance   = 100_000.0
		costPerJob       = 0.001
		workersPerNode   = 4
	)

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	client := testEntClient(t)
	prefix := "billing-cluster-" + uuid.NewString()
	sharedAccount := mustCreateAccount(t, client, &service.Account{
		Name: prefix + "-shared-account",
		Type: service.AccountTypeAPIKey,
	})
	targets := make([]billingClusterPerformanceTarget, userCount)
	for i := range targets {
		user := mustCreateUser(t, client, &service.User{
			Email:        fmt.Sprintf("%s-%d@example.com", prefix, i),
			PasswordHash: "hash",
			Balance:      initialBalance,
		})
		targets[i] = billingClusterPerformanceTarget{
			user: user,
			apiKey: mustCreateApiKey(t, client, &service.APIKey{
				UserID: user.ID,
				Key:    fmt.Sprintf("sk-%s-%d", prefix, i),
				Name:   fmt.Sprintf("%s-%d", prefix, i),
			}),
		}
	}

	for _, nodeCount := range []int{1, 2, 4} {
		nodeCount := nodeCount
		t.Run(fmt.Sprintf("nodes_%d", nodeCount), func(t *testing.T) {
			resetDurableBillingQueueTables(t)
			_, err := integrationDB.ExecContext(ctx, "UPDATE users SET balance = $1 WHERE email LIKE $2", initialBalance, prefix+"-%")
			require.NoError(t, err)
			_, err = integrationDB.ExecContext(ctx, `
				UPDATE accounts
				SET extra = jsonb_set(COALESCE(extra, '{}'::jsonb), '{quota_used}', '0'::jsonb, true)
				WHERE id = $1
			`, sharedAccount.ID)
			require.NoError(t, err)

			runPrefix := fmt.Sprintf("%s-nodes-%d-%s", prefix, nodeCount, uuid.NewString())
			commands, jobsByTarget := buildBillingClusterPerformanceCommands(
				targets,
				sharedAccount.ID,
				runPrefix,
				jobCount,
				primaryHotJobs,
				secondaryHotJobs,
				costPerJob,
			)

			nodes := make([]*billingClusterPerformanceNode, 0, nodeCount)
			for i := 0; i < nodeCount; i++ {
				node := newBillingClusterPerformanceNode(t, ctx, i, workersPerNode)
				nodes = append(nodes, node)
			}
			consumerCtx, stopConsumers := context.WithCancel(ctx)
			var consumerWG sync.WaitGroup
			for _, node := range nodes {
				for workerID := 0; workerID < workersPerNode; workerID++ {
					consumerWG.Add(1)
					go runBillingClusterPerformanceConsumer(consumerCtx, &consumerWG, node)
				}
			}

			producer := newDurableBillingQueueIntegrationRepo()
			producer.consumerCount = 1
			producer.wakeCh = make(chan struct{}, 1)
			producerCtx, stopProducer := context.WithCancel(ctx)
			producer.cancel = stopProducer
			producer.wg.Add(1)
			go producer.runEnqueueBatcher(producerCtx)

			metricsCtx, stopMetrics := context.WithCancel(ctx)
			metricsDone := make(chan billingClusterPerformanceMetrics, 1)
			go sampleBillingClusterPerformanceMetrics(metricsCtx, runPrefix, metricsDone)

			latencies := make([]int64, len(commands))
			work := make(chan int, 1024)
			var enqueueWG sync.WaitGroup
			var enqueueFailures atomic.Int64
			firstErr := make(chan error, 1)
			const producerWorkers = 64
			enqueueWG.Add(producerWorkers)
			startedAt := time.Now()
			for i := 0; i < producerWorkers; i++ {
				go func() {
					defer enqueueWG.Done()
					for index := range work {
						started := time.Now()
						_, applyErr := producer.Apply(ctx, &commands[index])
						latencies[index] = time.Since(started).Nanoseconds()
						if applyErr != nil {
							enqueueFailures.Add(1)
							select {
							case firstErr <- applyErr:
							default:
							}
						}
					}
				}()
			}
			for i := range commands {
				work <- i
			}
			close(work)
			enqueueWG.Wait()
			enqueueElapsed := time.Since(startedAt)
			if enqueueFailures.Load() != 0 {
				t.Fatalf("%d of %d billing submissions failed: %v", enqueueFailures.Load(), jobCount, <-firstErr)
			}
			stopProducer()
			producer.wg.Wait()

			drainStartedAt := time.Now()
			waitForBillingClusterPerformanceDrain(t, ctx, runPrefix, jobCount)
			drainElapsed := time.Since(drainStartedAt)
			totalElapsed := time.Since(startedAt)
			stopConsumers()
			consumerWG.Wait()
			stopMetrics()
			metrics := <-metricsDone

			var totalProcessed uint64
			shares := make([]float64, len(nodes))
			var openConnections int
			var waitCount int64
			var waitDuration time.Duration
			for _, node := range nodes {
				totalProcessed += node.processed.Load()
			}
			for i, node := range nodes {
				if totalProcessed > 0 {
					shares[i] = float64(node.processed.Load()) * 100 / float64(totalProcessed)
				}
				stats := node.db.Stats()
				openConnections += stats.OpenConnections
				waitCount += stats.WaitCount
				waitDuration += stats.WaitDuration
				require.NoError(t, node.db.Close())
			}

			var balanceTotal, accountQuota float64
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COALESCE(SUM(balance), 0) FROM users WHERE email LIKE $1", prefix+"-%").Scan(&balanceTotal))
			require.NoError(t, integrationDB.QueryRowContext(ctx, `
				SELECT COALESCE((extra->>'quota_used')::numeric, 0)
				FROM accounts
				WHERE id = $1
			`, sharedAccount.ID).Scan(&accountQuota))
			require.InDelta(t, initialBalance*userCount-jobCount*costPerJob, balanceTotal, 1e-5)
			require.InDelta(t, jobCount*costPerJob, accountQuota, 1e-5)
			for index, target := range targets {
				if jobsByTarget[index] == 0 {
					continue
				}
				var balance float64
				require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", target.user.ID).Scan(&balance))
				require.InDelta(t, initialBalance-float64(jobsByTarget[index])*costPerJob, balance, 1e-7)
			}
			pendingKeys, err := producer.scanUsageBillingRedisKeys(ctx, usageBillingPendingPrefix+"*")
			require.NoError(t, err)
			overlayKeys, err := producer.scanUsageBillingRedisKeys(ctx, usageBillingOverlayPrefix+"*")
			require.NoError(t, err)
			require.Empty(t, pendingKeys)
			require.Empty(t, overlayKeys)

			t.Logf(
				"nodes=%d jobs=%d enqueue_elapsed=%s enqueue_rate=%.0f/s enqueue_p95=%s drain_after_enqueue=%s total_elapsed=%s max_oldest_age=%s max_lock_waiters=%d open_connections=%d pool_wait_count=%d pool_wait=%s node_share=%v",
				nodeCount,
				jobCount,
				enqueueElapsed,
				float64(jobCount)/enqueueElapsed.Seconds(),
				billingClusterPerformanceP95(latencies),
				drainElapsed,
				totalElapsed,
				metrics.maxOldestAge,
				metrics.maxLockWaiters,
				openConnections,
				waitCount,
				waitDuration,
				shares,
			)
		})
	}
}

type billingClusterPerformanceTarget struct {
	user   *service.User
	apiKey *service.APIKey
}

type billingClusterPerformanceNode struct {
	db        *sql.DB
	repo      *queuedUsageBillingRepository
	processed atomic.Uint64
}

type billingClusterPerformanceMetrics struct {
	maxOldestAge   time.Duration
	maxLockWaiters int
}

func buildBillingClusterPerformanceCommands(
	targets []billingClusterPerformanceTarget,
	sharedAccountID int64,
	prefix string,
	jobCount, primaryHotJobs, secondaryHotJobs int,
	cost float64,
) ([]service.UsageBillingCommand, []int) {
	commands := make([]service.UsageBillingCommand, jobCount)
	jobsByTarget := make([]int, len(targets))
	for i := range commands {
		targetIndex := 0
		switch {
		case i < primaryHotJobs:
			targetIndex = 0
		case i < primaryHotJobs+secondaryHotJobs:
			targetIndex = 1
		default:
			targetIndex = 2 + (i-primaryHotJobs-secondaryHotJobs)%(len(targets)-2)
		}
		jobsByTarget[targetIndex]++
		target := targets[targetIndex]
		commands[i] = service.UsageBillingCommand{
			RequestID:        fmt.Sprintf("%s-%d", prefix, i),
			APIKeyID:         target.apiKey.ID,
			UserID:           target.user.ID,
			AccountID:        sharedAccountID,
			AccountType:      service.AccountTypeAPIKey,
			BalanceCost:      cost,
			AccountQuotaCost: cost,
		}
	}
	return commands, jobsByTarget
}

func newBillingClusterPerformanceNode(t *testing.T, ctx context.Context, index, workerCount int) *billingClusterPerformanceNode {
	t.Helper()
	dsn, err := url.Parse(integrationPostgresDSN)
	require.NoError(t, err)
	query := dsn.Query()
	query.Set("application_name", fmt.Sprintf("billing-cluster-node-%d", index))
	dsn.RawQuery = query.Encode()
	db, err := openSQLWithRetry(ctx, dsn.String(), 30*time.Second)
	require.NoError(t, err)
	db.SetMaxOpenConns(workerCount + 2)
	db.SetMaxIdleConns(workerCount + 2)
	repo := &queuedUsageBillingRepository{
		direct:         &usageBillingRepository{db: db},
		db:             db,
		rdb:            integrationRedis,
		consumerCount:  workerCount,
		readBatchSize:  128,
		pollInterval:   10 * time.Millisecond,
		commandTimeout: 15 * time.Second,
		maxRetryDelay:  time.Second,
		enqueueCh:      make(chan usageBillingEnqueueRequest, usageBillingEnqueueQueueSize),
		wakeCh:         make(chan struct{}, workerCount),
	}
	return &billingClusterPerformanceNode{db: db, repo: repo}
}

func runBillingClusterPerformanceConsumer(ctx context.Context, wg *sync.WaitGroup, node *billingClusterPerformanceNode) {
	defer wg.Done()
	for ctx.Err() == nil {
		processed, err := node.repo.processUsageBillingCycle(ctx, 0, false)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if processed == 0 {
			time.Sleep(2 * time.Millisecond)
			continue
		}
		node.processed.Add(uint64(processed))
	}
}

func waitForBillingClusterPerformanceDrain(t *testing.T, ctx context.Context, prefix string, expected int) {
	t.Helper()
	for {
		var jobs, dedup int
		err := integrationDB.QueryRowContext(ctx, `
			SELECT
				(SELECT COUNT(*) FROM usage_billing_jobs WHERE request_id LIKE $1),
				(SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id LIKE $1)
		`, prefix+"-%").Scan(&jobs, &dedup)
		require.NoError(t, err)
		if jobs == 0 && dedup == expected {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("cluster billing queue did not drain: jobs=%d dedup=%d: %v", jobs, dedup, ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func sampleBillingClusterPerformanceMetrics(ctx context.Context, prefix string, result chan<- billingClusterPerformanceMetrics) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	metrics := billingClusterPerformanceMetrics{}
	defer func() { result <- metrics }()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var oldestSeconds float64
			var lockWaiters int
			err := integrationDB.QueryRowContext(ctx, `
				SELECT COALESCE(EXTRACT(EPOCH FROM NOW() - MIN(created_at)), 0)
				FROM usage_billing_jobs
				WHERE request_id LIKE $1
			`, prefix+"-%").Scan(&oldestSeconds)
			if err == nil && time.Duration(oldestSeconds*float64(time.Second)) > metrics.maxOldestAge {
				metrics.maxOldestAge = time.Duration(oldestSeconds * float64(time.Second))
			}
			err = integrationDB.QueryRowContext(ctx, `
				SELECT COUNT(*)
				FROM pg_stat_activity
				WHERE application_name LIKE 'billing-cluster-node-%'
					AND wait_event_type = 'Lock'
			`).Scan(&lockWaiters)
			if err == nil && lockWaiters > metrics.maxLockWaiters {
				metrics.maxLockWaiters = lockWaiters
			}
		}
	}
}

func billingClusterPerformanceP95(samples []int64) time.Duration {
	values := append([]int64(nil), samples...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	if len(values) == 0 {
		return 0
	}
	index := (len(values)*95 + 99) / 100
	if index > 0 {
		index--
	}
	return time.Duration(values[index])
}
