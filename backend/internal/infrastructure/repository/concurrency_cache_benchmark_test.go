package repository

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

// 基准测试用 TTL 配置
const benchSlotTTLMinutes = 15

var benchSlotTTL = time.Duration(benchSlotTTLMinutes) * time.Minute

var benchmarkSeparateLiveAcquireScript = redis.NewScript(`
	redis.replicate_commands()
	local key = KEYS[1]
	local liveKey = KEYS[2]
	local maxConcurrency = tonumber(ARGV[1])
	local ttl = tonumber(ARGV[2])
	local requestID = ARGV[3]
	local now = tonumber(redis.call('TIME')[1])
	redis.call('ZREMRANGEBYSCORE', key, '-inf', now - ttl)
	redis.call('ZREMRANGEBYSCORE', liveKey, '-inf', now - 60)
	if redis.call('ZSCORE', key, requestID) ~= false then
		redis.call('ZADD', key, now, requestID)
		redis.call('EXPIRE', key, ttl)
		return {1, now}
	end
	local count = redis.call('ZCARD', key) + redis.call('ZCARD', liveKey)
	if count < maxConcurrency then
		redis.call('ZADD', key, now, requestID)
		redis.call('EXPIRE', key, ttl)
		return {1, now}
	end
	return {0, now}
`)

var benchmarkSeparateLiveGetScript = redis.NewScript(`
	redis.replicate_commands()
	local key = KEYS[1]
	local liveKey = KEYS[2]
	local ttl = tonumber(ARGV[1])
	local now = tonumber(redis.call('TIME')[1])
	redis.call('ZREMRANGEBYSCORE', key, '-inf', now - ttl)
	redis.call('ZREMRANGEBYSCORE', liveKey, '-inf', now - 60)
	return redis.call('ZCARD', key) + redis.call('ZCARD', liveKey)
`)

// BenchmarkAccountConcurrency 用于对比 SCAN 与有序集合的计数性能。
func BenchmarkAccountConcurrency(b *testing.B) {
	rdb := newBenchmarkRedisClient(b)
	defer func() {
		_ = rdb.Close()
	}()

	cache := newBenchmarkConcurrencyCache(b, rdb)
	ctx := context.Background()

	for _, size := range []int{10, 100, 1000} {
		size := size
		b.Run(fmt.Sprintf("zset/slots=%d", size), func(b *testing.B) {
			accountID := time.Now().UnixNano()
			key := accountSlotKey(accountID)

			b.StopTimer()
			members := make([]redis.Z, 0, size)
			now := float64(time.Now().Unix())
			for i := 0; i < size; i++ {
				members = append(members, redis.Z{
					Score:  now,
					Member: fmt.Sprintf("req_%d", i),
				})
			}
			if err := rdb.ZAdd(ctx, key, members...).Err(); err != nil {
				b.Fatalf("初始化有序集合失败: %v", err)
			}
			if err := rdb.Expire(ctx, key, benchSlotTTL).Err(); err != nil {
				b.Fatalf("设置有序集合 TTL 失败: %v", err)
			}
			b.StartTimer()

			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := cache.GetAccountConcurrency(ctx, accountID); err != nil {
					b.Fatalf("获取并发数量失败: %v", err)
				}
			}

			b.StopTimer()
			if err := rdb.Del(ctx, key).Err(); err != nil {
				b.Fatalf("清理有序集合失败: %v", err)
			}
		})

		b.Run(fmt.Sprintf("scan/slots=%d", size), func(b *testing.B) {
			accountID := time.Now().UnixNano()
			pattern := fmt.Sprintf("%s%d:*", accountSlotKeyPrefix, accountID)
			keys := make([]string, 0, size)

			b.StopTimer()
			pipe := rdb.Pipeline()
			for i := 0; i < size; i++ {
				key := fmt.Sprintf("%s%d:req_%d", accountSlotKeyPrefix, accountID, i)
				keys = append(keys, key)
				pipe.Set(ctx, key, "1", benchSlotTTL)
			}
			if _, err := pipe.Exec(ctx); err != nil {
				b.Fatalf("初始化扫描键失败: %v", err)
			}
			b.StartTimer()

			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := scanSlotCount(ctx, rdb, pattern); err != nil {
					b.Fatalf("SCAN 计数失败: %v", err)
				}
			}

			b.StopTimer()
			if err := rdb.Del(ctx, keys...).Err(); err != nil {
				b.Fatalf("清理扫描键失败: %v", err)
			}
		})
	}
}

func BenchmarkPriorityAdmissionAccountFastPath(b *testing.B) {
	rdb := newBenchmarkRedisClient(b)
	defer func() { _ = rdb.Close() }()
	cache := newBenchmarkConcurrencyCache(b, rdb)
	ctx := context.Background()
	request := service.PriorityAccountAdmissionRequest{
		AccountID:      time.Now().UnixNano(),
		MaxConcurrency: 64,
		MaxWaiting:     100,
		Tier:           service.RequestSchedulingTierNormal,
		RequestID:      "benchmark-stable-request",
		WaitTimeout:    30 * time.Second,
	}
	status, err := cache.AcquirePriorityAccountSlot(ctx, request)
	if err != nil || status != service.PriorityAccountAdmissionAcquired {
		b.Fatalf("warm priority admission script: status=%d err=%v", status, err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		status, err = cache.AcquirePriorityAccountSlot(ctx, request)
		if err != nil || status != service.PriorityAccountAdmissionAcquired {
			b.Fatalf("priority admission: status=%d err=%v", status, err)
		}
	}
}

func BenchmarkOrdinaryConcurrencyHotPath(b *testing.B) {
	const clients = 50
	rdb := newBenchmarkRedisClient(b)
	defer func() { _ = rdb.Close() }()
	cache := newBenchmarkConcurrencyCache(b, rdb)
	ctx := context.Background()

	b.Run("AcquireAccount/UnifiedZSET/clients=50", func(b *testing.B) {
		accountID := time.Now().UnixNano()
		defer cleanupBenchmarkAccountKeys(ctx, rdb, accountID)
		b.ReportAllocs()
		runFixedClientBenchmark(b, clients, func(worker int) error {
			acquired, err := cache.AcquireAccountSlot(ctx, accountID, clients, fmt.Sprintf("bench-acquire-%d", worker))
			if err != nil {
				return err
			}
			if !acquired {
				return fmt.Errorf("worker %d was rejected", worker)
			}
			return nil
		})
	})

	b.Run("AcquireAccount/SeparateLiveZSET/clients=50", func(b *testing.B) {
		accountID := time.Now().UnixNano()
		defer cleanupBenchmarkAccountKeys(ctx, rdb, accountID)
		b.ReportAllocs()
		runFixedClientBenchmark(b, clients, func(worker int) error {
			acquired, err := benchmarkAcquireAccountSeparateLive(
				ctx, cache, accountID, clients, fmt.Sprintf("bench-acquire-%d", worker),
			)
			if err != nil {
				return err
			}
			if !acquired {
				return fmt.Errorf("worker %d was rejected", worker)
			}
			return nil
		})
	})

	b.Run("GetAccount/UnifiedZSET/clients=50", func(b *testing.B) {
		accountID := time.Now().UnixNano()
		acquired, err := cache.AcquireAccountSlot(ctx, accountID, 1, "bench-get")
		if err != nil || !acquired {
			b.Fatalf("prepare account slot: acquired=%v err=%v", acquired, err)
		}
		defer cleanupBenchmarkAccountKeys(ctx, rdb, accountID)
		b.ReportAllocs()
		runFixedClientBenchmark(b, clients, func(_ int) error {
			_, err := cache.GetAccountConcurrency(ctx, accountID)
			return err
		})
	})

	b.Run("GetAccount/SeparateLiveZSET/clients=50", func(b *testing.B) {
		accountID := time.Now().UnixNano()
		acquired, err := cache.AcquireAccountSlot(ctx, accountID, 1, "bench-get")
		if err != nil || !acquired {
			b.Fatalf("prepare account slot: acquired=%v err=%v", acquired, err)
		}
		defer cleanupBenchmarkAccountKeys(ctx, rdb, accountID)
		b.ReportAllocs()
		runFixedClientBenchmark(b, clients, func(_ int) error {
			_, err := benchmarkSeparateLiveGetScript.Run(
				ctx,
				rdb,
				[]string{accountSlotKey(accountID), benchmarkLiveAccountSlotKey(accountID)},
				cache.slotTTLSeconds,
			).Int()
			return err
		})
	})

	b.Run("GetAccountBatch/UnifiedZSET/entities=100/clients=50", func(b *testing.B) {
		baseID := time.Now().UnixNano()
		accountIDs := make([]int64, 100)
		keys := make([]string, 100)
		for i := range accountIDs {
			accountIDs[i] = baseID + int64(i)
			keys[i] = accountSlotKey(accountIDs[i])
		}
		defer func() { _ = rdb.Del(ctx, keys...).Err() }()
		b.ReportAllocs()
		runFixedClientBenchmark(b, clients, func(_ int) error {
			_, err := cache.GetAccountConcurrencyBatch(ctx, accountIDs)
			return err
		})
	})

	b.Run("GetAccountBatch/SeparateLiveZSET/entities=100/clients=50", func(b *testing.B) {
		baseID := time.Now().UnixNano()
		accountIDs := make([]int64, 100)
		for i := range accountIDs {
			accountIDs[i] = baseID + int64(i)
		}
		b.ReportAllocs()
		runFixedClientBenchmark(b, clients, func(_ int) error {
			return benchmarkGetAccountBatchSeparateLive(ctx, rdb, cache.slotTTLSeconds, accountIDs)
		})
	})

	b.Run("GetAPIKeyBatch/UnifiedZSET/entities=100/clients=50", func(b *testing.B) {
		baseID := time.Now().UnixNano()
		apiKeyIDs := make([]int64, 100)
		keys := make([]string, 100)
		for i := range apiKeyIDs {
			apiKeyIDs[i] = baseID + int64(i)
			keys[i] = apiKeySlotKey(apiKeyIDs[i])
		}
		defer func() { _ = rdb.Del(ctx, keys...).Err() }()
		b.ReportAllocs()
		runFixedClientBenchmark(b, clients, func(_ int) error {
			_, err := cache.GetAPIKeyConcurrencyBatch(ctx, apiKeyIDs)
			return err
		})
	})

	b.Run("GetAPIKeyBatch/SeparateLiveZSET/entities=100/clients=50", func(b *testing.B) {
		baseID := time.Now().UnixNano()
		apiKeyIDs := make([]int64, 100)
		for i := range apiKeyIDs {
			apiKeyIDs[i] = baseID + int64(i)
		}
		b.ReportAllocs()
		runFixedClientBenchmark(b, clients, func(_ int) error {
			return benchmarkGetAPIKeyBatchSeparateLive(ctx, rdb, cache.slotTTLSeconds, apiKeyIDs)
		})
	})
}

func BenchmarkAccountsLoadBatchLargePool(b *testing.B) {
	rdb := newBenchmarkRedisClient(b)
	defer func() { _ = rdb.Close() }()
	cache := newBenchmarkConcurrencyCache(b, rdb)
	ctx := context.Background()

	for _, size := range []int{3000, 5000} {
		accounts := make([]service.AccountWithConcurrency, size)
		baseID := time.Now().UnixNano()
		for i := range accounts {
			accounts[i] = service.AccountWithConcurrency{ID: baseID + int64(i), MaxConcurrency: 4}
		}
		b.Run(fmt.Sprintf("accounts=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				result, err := cache.GetAccountsLoadBatch(ctx, accounts)
				if err != nil || len(result) != size {
					b.Fatalf("load batch: count=%d err=%v", len(result), err)
				}
			}
		})
	}
}

func benchmarkAcquireAccountSeparateLive(
	ctx context.Context,
	cache *concurrencyCache,
	accountID int64,
	maxConcurrency int,
	requestID string,
) (bool, error) {
	result, now, err := runScriptInt64Pair(ctx, cache.rdb, benchmarkSeparateLiveAcquireScript, []string{
		accountSlotKey(accountID),
		benchmarkLiveAccountSlotKey(accountID),
	}, maxConcurrency, cache.slotTTLSeconds, requestID)
	if err != nil {
		return false, err
	}
	if result == 1 {
		cache.touchActiveIndexAt(ctx, accountActiveIndexKey, accountID, now+int64(cache.slotTTLSeconds))
	}
	return result == 1, nil
}

func benchmarkGetAccountBatchSeparateLive(
	ctx context.Context,
	rdb *redis.Client,
	slotTTLSeconds int,
	accountIDs []int64,
) error {
	now, err := rdb.Time(ctx).Result()
	if err != nil {
		return err
	}
	pipe := rdb.Pipeline()
	for _, accountID := range accountIDs {
		pipe.ZRemRangeByScore(ctx, accountSlotKey(accountID), "-inf", fmt.Sprint(now.Unix()-int64(slotTTLSeconds)))
		pipe.ZRemRangeByScore(ctx, benchmarkLiveAccountSlotKey(accountID), "-inf", fmt.Sprint(now.Unix()-60))
		pipe.ZCard(ctx, accountSlotKey(accountID))
		pipe.ZCard(ctx, benchmarkLiveAccountSlotKey(accountID))
	}
	_, err = pipe.Exec(ctx)
	return err
}

func benchmarkGetAPIKeyBatchSeparateLive(
	ctx context.Context,
	rdb *redis.Client,
	slotTTLSeconds int,
	apiKeyIDs []int64,
) error {
	now, err := rdb.Time(ctx).Result()
	if err != nil {
		return err
	}
	pipe := rdb.Pipeline()
	for _, apiKeyID := range apiKeyIDs {
		pipe.ZRemRangeByScore(ctx, apiKeySlotKey(apiKeyID), "-inf", fmt.Sprint(now.Unix()-int64(slotTTLSeconds)))
		pipe.ZRemRangeByScore(ctx, benchmarkLiveAPIKeySlotKey(apiKeyID), "-inf", fmt.Sprint(now.Unix()-60))
		pipe.ZCard(ctx, apiKeySlotKey(apiKeyID))
		pipe.ZCard(ctx, benchmarkLiveAPIKeySlotKey(apiKeyID))
	}
	_, err = pipe.Exec(ctx)
	return err
}

func benchmarkLiveAccountSlotKey(accountID int64) string {
	return fmt.Sprintf("concurrency:live:account:%d", accountID)
}

func benchmarkLiveAPIKeySlotKey(apiKeyID int64) string {
	return fmt.Sprintf("concurrency:live:api_key:%d", apiKeyID)
}

func cleanupBenchmarkAccountKeys(ctx context.Context, rdb *redis.Client, accountID int64) {
	_ = rdb.Del(ctx, accountSlotKey(accountID), benchmarkLiveAccountSlotKey(accountID)).Err()
	_ = rdb.ZRem(ctx, accountActiveIndexKey, fmt.Sprint(accountID)).Err()
}

func runFixedClientBenchmark(b *testing.B, clients int, operation func(worker int) error) {
	b.Helper()
	start := make(chan struct{})
	errCh := make(chan error, clients)
	var workers sync.WaitGroup
	workers.Add(clients)
	for worker := 0; worker < clients; worker++ {
		worker := worker
		go func() {
			defer workers.Done()
			<-start
			for iteration := worker; iteration < b.N; iteration += clients {
				if err := operation(worker); err != nil {
					errCh <- err
					return
				}
			}
		}()
	}
	b.ResetTimer()
	close(start)
	workers.Wait()
	b.StopTimer()
	close(errCh)
	for err := range errCh {
		b.Error(err)
	}
}

func scanSlotCount(ctx context.Context, rdb *redis.Client, pattern string) (int, error) {
	var cursor uint64
	count := 0
	for {
		keys, nextCursor, err := rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return 0, err
		}
		count += len(keys)
		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}
	return count, nil
}

func newBenchmarkRedisClient(b *testing.B) *redis.Client {
	b.Helper()

	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		b.Skip("未设置 TEST_REDIS_URL，跳过 Redis 基准测试")
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		b.Fatalf("解析 TEST_REDIS_URL 失败: %v", err)
	}

	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		b.Fatalf("Redis 连接失败: %v", err)
	}

	return client
}

func newBenchmarkConcurrencyCache(b *testing.B, redisClient *redis.Client) *concurrencyCache {
	b.Helper()
	cache, ok := NewConcurrencyCache(redisClient, benchSlotTTLMinutes, int(benchSlotTTL.Seconds())).(*concurrencyCache)
	if !ok {
		b.Fatal("unexpected concurrency cache implementation")
	}
	return cache
}
