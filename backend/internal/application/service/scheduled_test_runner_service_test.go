//go:build unit

package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type scheduledRunnerPlanRepoStub struct {
	listDueCalls atomic.Int64
}

func (s *scheduledRunnerPlanRepoStub) Create(context.Context, *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	return nil, nil
}
func (s *scheduledRunnerPlanRepoStub) GetByID(context.Context, int64) (*ScheduledTestPlan, error) {
	return nil, nil
}
func (s *scheduledRunnerPlanRepoStub) ListByAccountID(context.Context, int64) ([]*ScheduledTestPlan, error) {
	return nil, nil
}
func (s *scheduledRunnerPlanRepoStub) ListDue(context.Context, time.Time) ([]*ScheduledTestPlan, error) {
	s.listDueCalls.Add(1)
	return nil, nil
}
func (s *scheduledRunnerPlanRepoStub) Update(context.Context, *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	return nil, nil
}
func (s *scheduledRunnerPlanRepoStub) Delete(context.Context, int64) error { return nil }
func (s *scheduledRunnerPlanRepoStub) UpdateAfterRun(context.Context, int64, time.Time, time.Time) error {
	return nil
}

func TestScheduledTestRunnerSkipsScanWhenPeerHoldsLock(t *testing.T) {
	cache := &fakeLeaderLockCache{}
	_, _ = cache.TryAcquireLeaderLock(context.Background(), scheduledTestLeaderLockKey, "peer", time.Minute)
	repo := &scheduledRunnerPlanRepoStub{}
	svc := NewScheduledTestRunnerService(repo, nil, nil, nil, nil)
	svc.SetLeaderLock(cache, nil)

	svc.runScheduledNow()

	if got := repo.listDueCalls.Load(); got != 0 {
		t.Fatalf("peer-held distributed lock should skip due-plan scan, got %d calls", got)
	}
}

func TestScheduledTestRunnerReleasesLockAfterScan(t *testing.T) {
	cache := &fakeLeaderLockCache{}
	repo := &scheduledRunnerPlanRepoStub{}
	svc := NewScheduledTestRunnerService(repo, nil, nil, nil, nil)
	svc.SetLeaderLock(cache, nil)

	svc.runScheduledNow()
	svc.runScheduledNow()

	if got := repo.listDueCalls.Load(); got != 2 {
		t.Fatalf("runner should re-acquire and scan each cycle, got %d calls", got)
	}
}
