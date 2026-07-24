//go:build integration

package repository

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

const redisBurstStressRequests = 50_000

type redisBurstResult struct {
	elapsed    time.Duration
	dials      int64
	totalConns uint32
	idleConns  uint32
}

func runRedisBurst50000(t *testing.T, client *redis.Client) time.Duration {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	start := make(chan struct{})
	var ready sync.WaitGroup
	var done sync.WaitGroup
	var failures atomic.Int64
	ready.Add(redisBurstStressRequests)
	done.Add(redisBurstStressRequests)
	for range redisBurstStressRequests {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			if err := client.Ping(ctx).Err(); err != nil {
				failures.Add(1)
			}
		}()
	}
	ready.Wait()
	startedAt := time.Now()
	close(start)
	done.Wait()
	require.Zero(t, failures.Load())
	return time.Since(startedAt)
}

func TestRedisMaxIdleConnsConcurrent50000BurstWaves(t *testing.T) {
	if os.Getenv("SUB2API_RUN_50K_REDIS_TEST") != "1" {
		t.Skip("set SUB2API_RUN_50K_REDIS_TEST=1 to run the 50k Redis burst stress test")
	}

	results := make(map[string][2]redisBurstResult, 2)
	for _, tc := range []struct {
		name         string
		maxIdleConns int
	}{
		{name: "unlimited_idle", maxIdleConns: 0},
		{name: "max_idle_256", maxIdleConns: 256},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var dialCount atomic.Int64
			client := redis.NewClient(&redis.Options{
				Addr:         integrationRedis.Options().Addr,
				PoolSize:     1024,
				MinIdleConns: 128,
				MaxIdleConns: tc.maxIdleConns,
				DialTimeout:  2 * time.Second,
				ReadTimeout:  2 * time.Second,
				WriteTimeout: 2 * time.Second,
				PoolTimeout:  10 * time.Second,
				OnConnect: func(context.Context, *redis.Conn) error {
					dialCount.Add(1)
					return nil
				},
			})
			defer func() { require.NoError(t, client.Close()) }()
			require.NoError(t, client.Ping(context.Background()).Err())

			wave1StartedDials := dialCount.Load()
			wave1Elapsed := runRedisBurst50000(t, client)
			wave1Stats := client.PoolStats()
			wave1 := redisBurstResult{
				elapsed:    wave1Elapsed,
				dials:      dialCount.Load() - wave1StartedDials,
				totalConns: wave1Stats.TotalConns,
				idleConns:  wave1Stats.IdleConns,
			}

			time.Sleep(500 * time.Millisecond)
			wave2StartedDials := dialCount.Load()
			wave2Elapsed := runRedisBurst50000(t, client)
			wave2Stats := client.PoolStats()
			wave2 := redisBurstResult{
				elapsed:    wave2Elapsed,
				dials:      dialCount.Load() - wave2StartedDials,
				totalConns: wave2Stats.TotalConns,
				idleConns:  wave2Stats.IdleConns,
			}

			t.Logf(
				"requests_per_wave=%d max_idle=%d wave1=%s wave1_dials=%d wave1_total=%d wave1_idle=%d wave2=%s wave2_dials=%d wave2_total=%d wave2_idle=%d",
				redisBurstStressRequests,
				tc.maxIdleConns,
				wave1.elapsed,
				wave1.dials,
				wave1.totalConns,
				wave1.idleConns,
				wave2.elapsed,
				wave2.dials,
				wave2.totalConns,
				wave2.idleConns,
			)
			if tc.maxIdleConns > 0 {
				require.LessOrEqual(t, wave2.idleConns, uint32(tc.maxIdleConns))
			}
			results[tc.name] = [2]redisBurstResult{wave1, wave2}
		})
	}

	unlimited := results["unlimited_idle"]
	capped := results["max_idle_256"]
	require.Greater(t, capped[0].dials, int64(256), "stress wave did not exceed the configured idle cap")
	require.Less(t, unlimited[1].dials, capped[1].dials, "retaining hot connections must reduce second-wave redials")
}
