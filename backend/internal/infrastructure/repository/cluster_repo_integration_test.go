//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestClusterRepository_NodeHeartbeatAndTaskLease(t *testing.T) {
	repo := NewClusterRepository(integrationDB)
	ctx := context.Background()
	suffix := uuid.NewString()
	runnerID := "runner-a-" + suffix
	now := time.Now().UTC().Truncate(time.Microsecond)

	require.NoError(t, repo.UpsertInstance(ctx, service.ClusterInstance{
		RunnerID:       runnerID,
		NodeName:       "api-a-" + suffix,
		DeploymentMode: "multi_instance",
		WorkerMode:     "auto",
		WorkerEnabled:  true,
		Version:        "test",
		Hostname:       "test-host",
		ProcessID:      100,
		DatabaseOK:     true,
		RedisOK:        true,
		StartedAt:      now,
		LastSeenAt:     now,
	}))
	instances, err := repo.ListInstances(ctx)
	require.NoError(t, err)
	require.Contains(t, instanceRunnerIDs(instances), runnerID)

	taskKey := fmt.Sprintf("test:cluster:%s", suffix)
	taskA := service.ClusterTaskRun{
		RunID:       uuid.NewString(),
		TaskKey:     taskKey,
		NodeName:    "api-a",
		RunnerID:    runnerID,
		Metadata:    map[string]any{"source": "integration"},
		StartedAt:   now,
		HeartbeatAt: now,
		LeaseUntil:  now.Add(time.Minute),
	}
	acquired, err := repo.TryAcquireTask(ctx, taskA)
	require.NoError(t, err)
	require.True(t, acquired)

	taskB := taskA
	taskB.RunID = uuid.NewString()
	taskB.RunnerID = "runner-b-" + suffix
	acquired, err = repo.TryAcquireTask(ctx, taskB)
	require.NoError(t, err)
	require.False(t, acquired)

	require.NoError(t, repo.RenewTaskLease(ctx, taskA.RunID, runnerID, time.Now().Add(2*time.Minute)))
	require.NoError(t, repo.FinishTask(ctx, taskA.RunID, runnerID, service.ClusterTaskStatusSucceeded, map[string]any{"count": 1}, ""))
	require.ErrorIs(t, repo.RenewTaskLease(ctx, taskA.RunID, runnerID, time.Now().Add(3*time.Minute)), service.ErrClusterTaskLeaseLost)

	tasks, err := repo.ListTaskRuns(ctx, 200)
	require.NoError(t, err)
	found := false
	for _, task := range tasks {
		if task.RunID == taskA.RunID {
			found = true
			require.Equal(t, service.ClusterTaskStatusSucceeded, task.Status)
			require.EqualValues(t, 1, task.Result["count"])
		}
	}
	require.True(t, found)

	old := now.Add(-8 * 24 * time.Hour)
	require.NoError(t, repo.MarkInstanceStopped(ctx, runnerID, old))
	require.NoError(t, repo.PruneRuntime(ctx, now.Add(-7*24*time.Hour), time.Now().Add(time.Minute), 100))
	instances, err = repo.ListInstances(ctx)
	require.NoError(t, err)
	require.NotContains(t, instanceRunnerIDs(instances), runnerID)
	tasks, err = repo.ListTaskRuns(ctx, 200)
	require.NoError(t, err)
	for _, task := range tasks {
		require.NotEqual(t, taskA.RunID, task.RunID)
	}
}

func instanceRunnerIDs(instances []service.ClusterInstance) []string {
	ids := make([]string, 0, len(instances))
	for _, instance := range instances {
		ids = append(ids, instance.RunnerID)
	}
	return ids
}
