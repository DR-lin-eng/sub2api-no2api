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
	cpuUsage := 37.5
	memoryUsed := int64(256 * 1024 * 1024)
	memoryLimit := int64(1024 * 1024 * 1024)
	memoryPercent := 25.0

	require.NoError(t, repo.UpsertInstance(ctx, service.ClusterInstance{
		NodeID:         "node-a-" + suffix,
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
		Load: &service.ClusterInstanceLoad{
			CPUUsagePercent:        &cpuUsage,
			MemoryUsedBytes:        &memoryUsed,
			MemoryLimitBytes:       &memoryLimit,
			MemoryUsagePercent:     &memoryPercent,
			InFlightRequests:       6,
			GoroutineCount:         48,
			DBConnectionsActive:    3,
			DBConnectionsIdle:      5,
			DBConnectionsMax:       20,
			RedisConnectionsActive: 2,
			RedisConnectionsIdle:   8,
			RedisConnectionsMax:    50,
			CollectedAt:            now,
		},
	}))
	instances, err := repo.ListInstances(ctx)
	require.NoError(t, err)
	require.Contains(t, instanceRunnerIDs(instances), runnerID)
	for _, instance := range instances {
		if instance.RunnerID != runnerID {
			continue
		}
		require.NotNil(t, instance.Load)
		require.Equal(t, 37.5, *instance.Load.CPUUsagePercent)
		require.Equal(t, memoryUsed, *instance.Load.MemoryUsedBytes)
		require.Equal(t, int64(6), instance.Load.InFlightRequests)
		require.Equal(t, 3, instance.Load.DBConnectionsActive)
		require.Equal(t, 50, instance.Load.RedisConnectionsMax)
	}
	require.NoError(t, repo.RenameNode(ctx, "node-a-"+suffix, "primary-"+suffix))
	instances, err = repo.ListInstances(ctx)
	require.NoError(t, err)
	renamedFound := false
	for _, instance := range instances {
		if instance.NodeID == "node-a-"+suffix {
			renamedFound = true
			require.Equal(t, "primary-"+suffix, instance.NodeName)
		}
	}
	require.True(t, renamedFound)

	newRunnerID := "runner-a-new-" + suffix
	require.NoError(t, repo.UpsertInstance(ctx, service.ClusterInstance{
		NodeID:         "node-a-" + suffix,
		RunnerID:       newRunnerID,
		NodeName:       "configured-name-ignored-" + suffix,
		DeploymentMode: "multi_instance",
		WorkerMode:     "auto",
		WorkerEnabled:  true,
		Version:        "test-new",
		Hostname:       "test-host",
		ProcessID:      101,
		DatabaseOK:     true,
		RedisOK:        true,
		StartedAt:      now.Add(time.Second),
		LastSeenAt:     now.Add(time.Second),
	}))
	instances, err = repo.ListInstances(ctx)
	require.NoError(t, err)
	matching := make([]service.ClusterInstance, 0, 1)
	for _, instance := range instances {
		if instance.NodeID == "node-a-"+suffix {
			matching = append(matching, instance)
		}
	}
	require.Len(t, matching, 1)
	require.Equal(t, newRunnerID, matching[0].RunnerID)
	require.Equal(t, "primary-"+suffix, matching[0].NodeName)

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
	staleNodeID := "stale-node-" + suffix
	staleRunnerID := "stale-runner-" + suffix
	require.NoError(t, repo.UpsertInstance(ctx, service.ClusterInstance{
		NodeID:         staleNodeID,
		RunnerID:       staleRunnerID,
		NodeName:       staleNodeID,
		DeploymentMode: "multi_instance",
		WorkerMode:     "auto",
		WorkerEnabled:  true,
		Version:        "test-old",
		Hostname:       "stale-host",
		ProcessID:      102,
		DatabaseOK:     true,
		RedisOK:        true,
		StartedAt:      old,
		LastSeenAt:     old,
	}))
	require.NoError(t, repo.MarkInstanceStopped(ctx, runnerID, old))
	require.NoError(t, repo.PruneRuntime(ctx, now.Add(-7*24*time.Hour), time.Now().Add(time.Minute), 100))
	instances, err = repo.ListInstances(ctx)
	require.NoError(t, err)
	require.NotContains(t, instanceRunnerIDs(instances), runnerID)
	require.NotContains(t, instanceRunnerIDs(instances), staleRunnerID)
	tasks, err = repo.ListTaskRuns(ctx, 200)
	require.NoError(t, err)
	for _, task := range tasks {
		require.NotEqual(t, taskA.RunID, task.RunID)
	}
}

func TestClusterRepository_DuplicateConfiguredNamesRemainSeparateNodes(t *testing.T) {
	repo := NewClusterRepository(integrationDB)
	ctx := context.Background()
	suffix := uuid.NewString()
	nodeA := "duplicate-name-a-" + suffix
	nodeB := "duplicate-name-b-" + suffix
	runnerA := nodeA + "-runner"
	runnerB := nodeB + "-runner"
	configuredName := "shared-name-" + suffix
	now := time.Now().UTC().Truncate(time.Microsecond)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM cluster_instances WHERE node_id IN ($1,$2)`, nodeA, nodeB)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM cluster_nodes WHERE node_id IN ($1,$2)`, nodeA, nodeB)
	})

	upsert := func(nodeID, runnerID string) {
		require.NoError(t, repo.UpsertInstance(ctx, service.ClusterInstance{
			NodeID:         nodeID,
			RunnerID:       runnerID,
			NodeName:       configuredName,
			DeploymentMode: "multi_instance",
			WorkerMode:     "auto",
			WorkerEnabled:  true,
			Version:        "test",
			Hostname:       runnerID,
			ProcessID:      1,
			DatabaseOK:     true,
			RedisOK:        true,
			StartedAt:      now,
			LastSeenAt:     now,
		}))
	}

	upsert(nodeA, runnerA)
	upsert(nodeB, runnerB)
	instances, err := repo.ListInstances(ctx)
	require.NoError(t, err)
	displayNames := make(map[string]string)
	for _, instance := range instances {
		if instance.NodeID == nodeA || instance.NodeID == nodeB {
			displayNames[instance.NodeID] = instance.NodeName
		}
	}
	require.Len(t, displayNames, 2)
	require.NotEqual(t, displayNames[nodeA], displayNames[nodeB])
	require.Contains(t, []string{displayNames[nodeA], displayNames[nodeB]}, configuredName)
}

func TestClusterRepository_AdoptsLegacyRunnerHistoryIntoDurableNode(t *testing.T) {
	repo := NewClusterRepository(integrationDB)
	ctx := context.Background()
	suffix := uuid.NewString()
	legacyNodeID := "legacy-" + suffix
	durableNodeID := "durable-" + suffix
	oldRunnerID := "legacy-runner-" + suffix
	newRunnerID := "durable-runner-" + suffix
	configuredName := "upgrade-node-" + suffix
	now := time.Now().UTC().Truncate(time.Microsecond)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM cluster_instances WHERE runner_id IN ($1,$2)`, oldRunnerID, newRunnerID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM cluster_nodes WHERE node_id IN ($1,$2)`, legacyNodeID, durableNodeID)
	})

	upsert := func(nodeID, runnerID, version string, startedAt time.Time) {
		require.NoError(t, repo.UpsertInstance(ctx, service.ClusterInstance{
			NodeID:         nodeID,
			RunnerID:       runnerID,
			NodeName:       configuredName,
			DeploymentMode: "multi_instance",
			WorkerMode:     "auto",
			WorkerEnabled:  true,
			Version:        version,
			Hostname:       configuredName,
			ProcessID:      1,
			DatabaseOK:     true,
			RedisOK:        true,
			StartedAt:      startedAt,
			LastSeenAt:     startedAt,
		}))
	}

	upsert(legacyNodeID, oldRunnerID, "1.0.0", now)
	upsert(durableNodeID, newRunnerID, "1.1.0", now.Add(time.Second))

	var adoptedNodeID string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT node_id FROM cluster_instances WHERE runner_id = $1`, oldRunnerID).Scan(&adoptedNodeID))
	require.Equal(t, durableNodeID, adoptedNodeID)

	instances, err := repo.ListInstances(ctx)
	require.NoError(t, err)
	matching := make([]service.ClusterInstance, 0, 1)
	for _, instance := range instances {
		if instance.NodeID == durableNodeID {
			matching = append(matching, instance)
		}
	}
	require.Len(t, matching, 1)
	require.Equal(t, newRunnerID, matching[0].RunnerID)
	require.Equal(t, "1.1.0", matching[0].Version)
}

func instanceRunnerIDs(instances []service.ClusterInstance) []string {
	ids := make([]string, 0, len(instances))
	for _, instance := range instances {
		ids = append(ids, instance.RunnerID)
	}
	return ids
}
