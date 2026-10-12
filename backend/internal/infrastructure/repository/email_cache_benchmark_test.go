//go:build unit

package repository

import (
	"context"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

type emailCommandCounter struct{ commands atomic.Int64 }

func (h *emailCommandCounter) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) { return next(ctx, network, addr) }
}

func (h *emailCommandCounter) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.commands.Add(1)
		return next(ctx, cmd)
	}
}

func (h *emailCommandCounter) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

// Runs unchanged on the frozen baseline and candidate against real Redis.
// Setup is excluded, and the script is warmed before counting round trips.
func BenchmarkEmailCodeVerification(b *testing.B) {
	addr := os.Getenv("SUB2API_SYNC_REDIS_ADDR")
	if addr == "" {
		b.Skip("set SUB2API_SYNC_REDIS_ADDR to an isolated Redis instance")
	}
	for _, code := range []string{"123456", "654321"} {
		b.Run(map[string]string{"123456": "success", "654321": "failure"}[code], func(b *testing.B) {
			ctx := context.Background()
			rdb := redis.NewClient(&redis.Options{Addr: addr})
			defer func() { _ = rdb.Close() }()
			counter := &emailCommandCounter{}
			rdb.AddHook(counter)
			cache := NewEmailCache(rdb)
			svc := service.NewEmailService(nil, cache)
			const email = "sync-benchmark@example.com"
			_ = svc.VerifyCode(ctx, email, "")
			var commands int64
			b.ResetTimer()
			for range b.N {
				b.StopTimer()
				data := &service.VerificationCodeData{Code: "123456", ExpiresAt: time.Now().Add(time.Minute)}
				if err := cache.SetVerificationCode(ctx, email, data, time.Minute); err != nil {
					b.Fatal(err)
				}
				before := counter.commands.Load()
				b.StartTimer()
				err := svc.VerifyCode(ctx, email, code)
				b.StopTimer()
				if (code == "123456") != (err == nil) {
					b.Fatalf("unexpected verification result: %v", err)
				}
				commands += counter.commands.Load() - before
				b.StartTimer()
			}
			b.ReportMetric(float64(commands)/float64(b.N), "redis-commands/op")
		})
	}
}
