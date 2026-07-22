package service

import (
	"context"
	"sync/atomic"
	"testing"
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
	platformConcurrency, _, accountConcurrency,
		platformAvailability, _, accountAvailability, _, err := svc.GetConcurrencySnapshot(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("GetConcurrencySnapshot() error = %v", err)
	}
	if got := repo.calls.Load(); got != 1 {
		t.Fatalf("account loads = %d, want 1", got)
	}
	if platformConcurrency["openai"].MaxCapacity != 4 || accountConcurrency[7].MaxCapacity != 4 {
		t.Fatalf("unexpected concurrency snapshot: platform=%+v account=%+v", platformConcurrency, accountConcurrency)
	}
	if platformAvailability["openai"].AvailableCount != 1 || !accountAvailability[7].IsAvailable {
		t.Fatalf("unexpected availability snapshot: platform=%+v account=%+v", platformAvailability, accountAvailability)
	}
}
