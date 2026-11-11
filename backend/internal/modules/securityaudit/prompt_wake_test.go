package securityaudit

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuditEnqueueNotifiesOnlyAfterDurablePublication(t *testing.T) {
	for _, fail := range []bool{false, true} {
		trace := []string{}
		repo := &fakeJobRepository{trace: &trace}
		if fail {
			repo.publishErr = errors.New("publish failed")
		}
		enqueuer := NewEnqueuer(&fakeConfigStore{cfg: asyncConfig(), active: true}, repo, &fakePayloadStore{})
		calls := 0
		enqueuer.notify = func() {
			calls++
			require.Equal(t, "publish_queued", trace[len(trace)-1])
		}
		err := enqueuer.Enqueue(context.Background(), asyncRequest())
		if fail {
			require.Error(t, err)
			require.Zero(t, calls)
		} else {
			require.NoError(t, err)
			require.Equal(t, 1, calls)
		}
	}
}

func TestAuditNotifyDistributesWithoutWakeAllQueryAmplification(t *testing.T) {
	cfg := asyncConfig()
	cfg.WorkerCount = 4
	runner := NewRunner(&fakeConfigStore{cfg: cfg, active: true}, nil, nil, nil, nil)
	for _, wake := range runner.wake {
		<-wake
	}
	runner.Notify()
	ready := 0
	for _, wake := range runner.wake {
		ready += len(wake)
	}
	require.Equal(t, 1, ready, "one local enqueue must not wake every worker")
	for range 100 {
		runner.Notify()
	}
	ready = 0
	for _, wake := range runner.wake {
		ready += len(wake)
	}
	require.Equal(t, cfg.WorkerCount, ready, "bursts are coalesced and wake only configured workers")
}
