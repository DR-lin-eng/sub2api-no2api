package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"time"
)

// A committed charge must still attempt quota persistence if the request/worker
// context expired. Bound that independent attempt, and keep failures observable;
// this path has no durable retry queue.
func persistPostUsagePlatformQuota(ctx context.Context, repo UserPlatformQuotaRepository, userID int64, platform string, cost float64) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	dbCtx, cancel := context.WithTimeout(base, 5*time.Second)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			userPlatformQuotaDBIncrErrorTotal.Add(1)
			logger.LegacyPrintf("service.gateway", "ALERT: panic persisting user platform quota user=%d platform=%s: %v", userID, platform, r)
		}
	}()
	if err := repo.IncrementUsageWithReset(dbCtx, userID, platform, cost, time.Now().UTC()); err != nil {
		userPlatformQuotaDBIncrErrorTotal.Add(1)
		logger.LegacyPrintf("service.gateway", "ALERT: incr user platform quota DB failed user=%d platform=%s cost=%f: %v", userID, platform, cost, err)
	}
}
