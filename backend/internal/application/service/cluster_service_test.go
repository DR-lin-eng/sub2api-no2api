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
	mu        sync.Mutex
	instances map[string]ClusterInstance
	tasks     map[string]ClusterTaskRun
	renewals  int
}

type clusterHealthCheckerStub bool

func (s clusterHealthCheckerStub) RedisHealthy(context.Context) bool {
	return bool(s)
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
	node := NewClusterService(repo, clusterTestConfig("api-a", config.WorkerModeAuto), clusterHealthCheckerStub(true), BuildInfo{Version: "1.2.3"})
	status, err := node.GetStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, "api-a", status.Deployment.NodeName)
	require.True(t, status.Deployment.FrontendEnabled)
	require.True(t, status.Deployment.WorkerEnabled)
	require.Equal(t, 1, status.Summary.OnlineNodes)
	require.Equal(t, 1, status.Summary.WorkerNodes)
	require.Len(t, status.Instances, 1)
	require.True(t, status.Instances[0].Current)
	require.True(t, status.Instances[0].RedisOK)
}
