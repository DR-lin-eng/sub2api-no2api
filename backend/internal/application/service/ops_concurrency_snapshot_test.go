package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type opsAccountsSnapshotRepoProbe struct {
	AccountRepository
	calls    atomic.Int64
	accounts []Account
}

func (p *opsAccountsSnapshotRepoProbe) ListOpsAccountsForStats(context.Context, string, *int64) ([]Account, error) {
	p.calls.Add(1)
	return append([]Account(nil), p.accounts...), nil
}

func TestGetConcurrencySnapshotLoadsAccountsOnce(t *testing.T) {
	repo := &opsAccountsSnapshotRepoProbe{accounts: []Account{{
		ID:          7,
		Name:        "shared-account",
		Platform:    "openai",
		Concurrency: 4,
		Status:      StatusActive,
		Schedulable: true,
	}}}
	svc := &OpsService{accountRepo: repo}
	svc.sessionIDRateMetrics = NewOpenAISessionIDRateMetrics()
	svc.sessionIDRateMetrics.Record(7, "session-a", time.Now())
	platformConcurrency, _, accountConcurrency,
		platformAvailability, _, accountAvailability, sessionGrowth, _, err := svc.GetConcurrencySnapshot(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("GetConcurrencySnapshot() error = %v", err)
	}
	if got := repo.calls.Load(); got != 1 {
		t.Fatalf("account loads = %d, want 1", got)
	}
	if platformConcurrency["openai"].MaxCapacity != 4 || accountConcurrency[7].MaxCapacity != 4 {
		t.Fatalf("unexpected concurrency snapshot: platform=%+v account=%+v", platformConcurrency, accountConcurrency)
	}
	if sessionGrowth.TotalPerMinute != 1 || accountConcurrency[7].SessionIDGrowthPerMinute != 1 {
		t.Fatalf("unexpected session growth: summary=%+v account=%+v", sessionGrowth, accountConcurrency[7])
	}
	if platformAvailability["openai"].AvailableCount != 1 || !accountAvailability[7].IsAvailable {
		t.Fatalf("unexpected availability snapshot: platform=%+v account=%+v", platformAvailability, accountAvailability)
	}
}
