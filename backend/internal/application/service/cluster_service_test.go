package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type clusterRepositoryStub struct {
	mu              sync.Mutex
	instances       map[string]ClusterInstance
	tasks           map[string]ClusterTaskRun
	renewals        int
	connectionStats ClusterConnectionStats
}

type clusterHealthCheckerStub bool

func (s clusterHealthCheckerStub) RedisHealthy(context.Context) bool {
	return bool(s)
}

func (s clusterHealthCheckerStub) ClusterConnectionStats() ClusterConnectionStats {
	return ClusterConnectionStats{Active: 2, Idle: 8, Max: 50}
}

type clusterLoadSamplerStub struct {
	load ClusterInstanceLoad
}

func (s clusterLoadSamplerStub) Sample(context.Context) ClusterInstanceLoad {
	return s.load
}

type clusterRequestLoadSourceStub int64

func (s clusterRequestLoadSourceStub) InFlightRequests() int64 {
	return int64(s)
}

func newClusterRepositoryStub() *clusterRepositoryStub {
	return &clusterRepositoryStub{instances: map[string]ClusterInstance{}, tasks: map[string]ClusterTaskRun{}}
}

func (r *clusterRepositoryStub) UpsertInstance(_ context.Context, instance ClusterInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.instances[instance.RunnerID] = instance
	return nil
}

func (r *clusterRepositoryStub) RenameNode(_ context.Context, nodeID, displayName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for runnerID, instance := range r.instances {
		if instance.NodeID == nodeID {
			instance.NodeName = displayName
			r.instances[runnerID] = instance
			return nil
		}
	}
	return ErrClusterNodeNotFound
}

func (r *clusterRepositoryStub) MarkInstanceStopped(_ context.Context, runnerID string, stoppedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	instance := r.instances[runnerID]
	instance.StoppedAt = &stoppedAt
	r.instances[runnerID] = instance
	return nil
}

func (r *clusterRepositoryStub) ListInstances(context.Context) ([]ClusterInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ClusterInstance, 0, len(r.instances))
	for _, instance := range r.instances {
		out = append(out, instance)
	}
	return out, nil
}

func (r *clusterRepositoryStub) ExpireStaleTasks(_ context.Context, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, task := range r.tasks {
		if task.Status == ClusterTaskStatusRunning && task.LeaseUntil.Before(now) {
			task.Status = ClusterTaskStatusLost
			r.tasks[key] = task
		}
	}
	return nil
}

func (r *clusterRepositoryStub) PruneRuntime(context.Context, time.Time, time.Time, int) error {
	return nil
}

func (r *clusterRepositoryStub) TryAcquireTask(_ context.Context, task ClusterTaskRun) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if active, ok := r.tasks[task.TaskKey]; ok && active.Status == ClusterTaskStatusRunning && active.LeaseUntil.After(time.Now()) {
		return false, nil
	}
	r.tasks[task.TaskKey] = task
	return true, nil
}

func (r *clusterRepositoryStub) RenewTaskLease(_ context.Context, runID, runnerID string, leaseUntil time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, task := range r.tasks {
		if task.RunID == runID && task.RunnerID == runnerID && task.Status == ClusterTaskStatusRunning {
			task.LeaseUntil = leaseUntil
			task.HeartbeatAt = time.Now()
			r.tasks[key] = task
			r.renewals++
			return nil
		}
	}
	return ErrClusterTaskLeaseLost
}

func (r *clusterRepositoryStub) FinishTask(_ context.Context, runID, runnerID, status string, result map[string]any, errorMessage string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, task := range r.tasks {
		if task.RunID == runID && task.RunnerID == runnerID && task.Status == ClusterTaskStatusRunning {
			task.Status = status
			task.Result = result
			task.ErrorMessage = errorMessage
			finishedAt := time.Now()
			task.FinishedAt = &finishedAt
			r.tasks[key] = task
			return nil
		}
	}
	return ErrClusterTaskLeaseLost
}

func (r *clusterRepositoryStub) ListTaskRuns(context.Context, int) ([]ClusterTaskRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ClusterTaskRun, 0, len(r.tasks))
	for _, task := range r.tasks {
		out = append(out, task)
	}
	return out, nil
}

func (r *clusterRepositoryStub) ClusterConnectionStats() ClusterConnectionStats {
	return r.connectionStats
}

func clusterTestConfig(nodeName, workerMode string) *config.Config {
	return &config.Config{Deployment: config.DeploymentConfig{
		Mode:                     config.DeploymentModeMultiInstance,
		NodeName:                 nodeName,
		WorkerEnabled:            workerMode,
		HeartbeatIntervalSeconds: 30,
		StaleAfterSeconds:        90,
		TaskLeaseSeconds:         1,
	}}
}

func TestClusterService_AutoWorkersContendForSingleTask(t *testing.T) {
	repo := newClusterRepositoryStub()
	nodeA := NewClusterService(repo, clusterTestConfig("api-a", config.WorkerModeAuto), nil, BuildInfo{Version: "test"})
	nodeB := NewClusterService(repo, clusterTestConfig("api-b", config.WorkerModeAuto), nil, BuildInfo{Version: "test"})
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		_, err := nodeA.RunTask(context.Background(), "backup:scheduled", nil, func(context.Context) (map[string]any, error) {
			close(entered)
			<-release
			return map[string]any{"backup_id": "b1"}, nil
		})
		done <- err
	}()
	<-entered

	ran, err := nodeB.RunTask(context.Background(), "backup:scheduled", nil, func(context.Context) (map[string]any, error) {
		return nil, errors.New("must not run")
	})
	require.NoError(t, err)
	require.False(t, ran)

	close(release)
	require.NoError(t, <-done)
	repo.mu.Lock()
	task := repo.tasks["backup:scheduled"]
	repo.mu.Unlock()
	require.Equal(t, ClusterTaskStatusSucceeded, task.Status)
	require.Equal(t, "b1", task.Result["backup_id"])
}

func TestClusterService_ExplicitDisabledWorkerDoesNotRun(t *testing.T) {
	repo := newClusterRepositoryStub()
	node := NewClusterService(repo, clusterTestConfig("api-only", config.WorkerModeDisabled), nil, BuildInfo{})
	called := false
	ran, err := node.RunTask(context.Background(), "scheduled_test:scan", nil, func(context.Context) (map[string]any, error) {
		called = true
		return nil, nil
	})
	require.NoError(t, err)
	require.False(t, ran)
	require.False(t, called)
}

func TestClusterService_StatusReportsCurrentNodeAndWorker(t *testing.T) {
	repo := newClusterRepositoryStub()
	repo.connectionStats = ClusterConnectionStats{Active: 4, Idle: 6, Max: 20}
	node := NewClusterService(repo, clusterTestConfig("api-a", config.WorkerModeAuto), clusterHealthCheckerStub(true), BuildInfo{Version: "1.2.3"})
	cpu := 61.5
	memoryUsed := int64(512 * 1024 * 1024)
	memoryLimit := int64(1024 * 1024 * 1024)
	memoryPercent := 50.0
	node.loadSampler = clusterLoadSamplerStub{load: ClusterInstanceLoad{
		CPUUsagePercent:    &cpu,
		MemoryUsedBytes:    &memoryUsed,
		MemoryLimitBytes:   &memoryLimit,
		MemoryUsagePercent: &memoryPercent,
		GoroutineCount:     42,
		CollectedAt:        time.Now().UTC(),
	}}
	node.SetRequestLoadSource(clusterRequestLoadSourceStub(7))
	repo.tasks["backup:scheduled"] = ClusterTaskRun{
		RunID:       "run-1",
		TaskKey:     "backup:scheduled",
		Status:      ClusterTaskStatusRunning,
		RunnerID:    node.RunnerID(),
		StartedAt:   time.Now().UTC(),
		HeartbeatAt: time.Now().UTC(),
		LeaseUntil:  time.Now().UTC().Add(time.Minute),
	}
	status, err := node.GetStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, "api-a", status.Deployment.NodeName)
	require.True(t, status.Deployment.FrontendEnabled)
	require.True(t, status.Deployment.WorkerEnabled)
	require.Equal(t, config.DeploymentUpdateDriverBinary, status.Deployment.UpdateDriver)
	require.Equal(t, 1, status.Summary.OnlineNodes)
	require.Equal(t, 1, status.Summary.WorkerNodes)
	require.Len(t, status.Instances, 1)
	require.True(t, status.Instances[0].Current)
	require.True(t, status.Instances[0].RedisOK)
	require.NotNil(t, status.Instances[0].Load)
	require.Equal(t, 61.5, *status.Instances[0].Load.CPUUsagePercent)
	require.Equal(t, int64(7), status.Instances[0].Load.InFlightRequests)
	require.Equal(t, 1, status.Instances[0].Load.ActiveTasks)
	require.Equal(t, 42, status.Instances[0].Load.GoroutineCount)
	require.Equal(t, 4, status.Instances[0].Load.DBConnectionsActive)
	require.Equal(t, 20, status.Instances[0].Load.DBConnectionsMax)
	require.Equal(t, 2, status.Instances[0].Load.RedisConnectionsActive)
	require.Equal(t, 50, status.Instances[0].Load.RedisConnectionsMax)
}

func TestClusterService_StatusHidesMetricsOlderThanHeartbeat(t *testing.T) {
	repo := newClusterRepositoryStub()
	node := NewClusterService(repo, clusterTestConfig("api-a", config.WorkerModeAuto), nil, BuildInfo{Version: "1.2.3"})
	now := time.Now().UTC()
	cpu := 88.0
	node.loadSampler = clusterLoadSamplerStub{load: ClusterInstanceLoad{CollectedAt: now}}
	repo.instances["remote-runner"] = ClusterInstance{
		NodeID:         "remote-node",
		RunnerID:       "remote-runner",
		NodeName:       "remote",
		DeploymentMode: config.DeploymentModeMultiInstance,
		WorkerMode:     config.WorkerModeAuto,
		WorkerEnabled:  true,
		DatabaseOK:     true,
		RedisOK:        true,
		StartedAt:      now.Add(-time.Hour),
		LastSeenAt:     now,
		Load: &ClusterInstanceLoad{
			CPUUsagePercent: &cpu,
			CollectedAt:     now.Add(-61 * time.Second),
		},
	}

	status, err := node.GetStatus(context.Background())
	require.NoError(t, err)
	for i := range status.Instances {
		if status.Instances[i].RunnerID == "remote-runner" {
			require.Nil(t, status.Instances[i].Load)
			return
		}
	}
	t.Fatal("remote instance not found")
}
