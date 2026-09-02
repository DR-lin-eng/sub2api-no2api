package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	"github.com/stretchr/testify/require"
)

type supportChatRetentionPolicyStub struct {
	days     int
	sequence []int
	err      error
	calls    int
}

func (p *supportChatRetentionPolicyStub) GetSupportChatRetentionDays(context.Context) (int, error) {
	p.calls++
	if len(p.sequence) > 0 {
		days := p.sequence[0]
		p.sequence = p.sequence[1:]
		return days, p.err
	}
	return p.days, p.err
}

type supportChatRetentionCleanerStub struct {
	results []chat.RetentionCleanupResult
	cutoffs []time.Time
	limits  []int
}

func (c *supportChatRetentionCleanerStub) CleanupExpiredMessages(
	_ context.Context,
	cutoff time.Time,
	limit int,
) (chat.RetentionCleanupResult, error) {
	c.cutoffs = append(c.cutoffs, cutoff)
	c.limits = append(c.limits, limit)
	if len(c.results) == 0 {
		return chat.RetentionCleanupResult{}, nil
	}
	result := c.results[0]
	c.results = c.results[1:]
	return result, nil
}

type supportChatRetentionBlockingCleaner struct {
	started  chan struct{}
	canceled chan struct{}
}

func (c *supportChatRetentionBlockingCleaner) CleanupExpiredMessages(
	ctx context.Context,
	_ time.Time,
	_ int,
) (chat.RetentionCleanupResult, error) {
	close(c.started)
	<-ctx.Done()
	close(c.canceled)
	return chat.RetentionCleanupResult{}, ctx.Err()
}

func TestSupportChatRetentionRunOnceUsesCurrentPolicyAndDrainsBatches(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	cleaner := &supportChatRetentionCleanerStub{results: []chat.RetentionCleanupResult{
		{MessagesDeleted: 2, AssetsDeleted: 2},
		{MessagesDeleted: 1, AssetsDeleted: 0},
	}}
	svc := &SupportChatRetentionService{
		cleaner:    cleaner,
		policy:     &supportChatRetentionPolicyStub{days: 30},
		instance:   "test-instance",
		batchSize:  2,
		maxBatches: 5,
	}

	result, err := svc.RunOnce(context.Background(), now)
	require.NoError(t, err)
	require.Equal(t, chat.RetentionCleanupResult{MessagesDeleted: 3, AssetsDeleted: 2}, result)
	require.Len(t, cleaner.cutoffs, 2)
	require.Equal(t, now.Add(-30*24*time.Hour), cleaner.cutoffs[0])
	require.Equal(t, []int{2, 2}, cleaner.limits)
}

func TestSupportChatRetentionRunOnceStopsWhenAdminDisablesCleanupBetweenBatches(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	cleaner := &supportChatRetentionCleanerStub{results: []chat.RetentionCleanupResult{
		{MessagesDeleted: 2},
		{MessagesDeleted: 2},
	}}
	policy := &supportChatRetentionPolicyStub{sequence: []int{30, 0}}
	svc := &SupportChatRetentionService{
		cleaner:    cleaner,
		policy:     policy,
		instance:   "test-instance",
		batchSize:  2,
		maxBatches: 5,
	}

	result, err := svc.RunOnce(context.Background(), now)
	require.NoError(t, err)
	require.Equal(t, chat.RetentionCleanupResult{MessagesDeleted: 2}, result)
	require.Equal(t, 2, policy.calls)
	require.Len(t, cleaner.cutoffs, 1)
}

func TestSupportChatRetentionRunOnceSkipsPermanentRetentionAndPolicyErrors(t *testing.T) {
	cleaner := &supportChatRetentionCleanerStub{}
	svc := &SupportChatRetentionService{
		cleaner: cleaner,
		policy:  &supportChatRetentionPolicyStub{},
	}

	result, err := svc.RunOnce(context.Background(), time.Now())
	require.NoError(t, err)
	require.Equal(t, chat.RetentionCleanupResult{}, result)
	require.Empty(t, cleaner.cutoffs)

	svc.policy = &supportChatRetentionPolicyStub{err: errors.New("database unavailable")}
	_, err = svc.RunOnce(context.Background(), time.Now())
	require.ErrorContains(t, err, "load support chat retention policy")
	require.Empty(t, cleaner.cutoffs)
}

func TestSupportChatRetentionStopCancelsActiveCleanup(t *testing.T) {
	runCtx, runCancel := context.WithCancel(context.Background())
	cleaner := &supportChatRetentionBlockingCleaner{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
	}
	svc := &SupportChatRetentionService{
		cleaner:    cleaner,
		policy:     &supportChatRetentionPolicyStub{days: 30},
		instance:   "test-instance",
		interval:   time.Hour,
		runTimeout: time.Minute,
		batchSize:  1,
		maxBatches: 1,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
		runCtx:     runCtx,
		runCancel:  runCancel,
	}

	svc.Start()
	select {
	case <-cleaner.started:
	case <-time.After(time.Second):
		t.Fatal("retention cleanup did not start")
	}
	svc.Stop()
	select {
	case <-cleaner.canceled:
	case <-time.After(time.Second):
		t.Fatal("retention cleanup context was not canceled")
	}
}
