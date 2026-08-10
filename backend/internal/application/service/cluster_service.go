package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/google/uuid"
)

const (
	clusterRuntimeMaintenanceInterval = 10 * time.Minute
	clusterRuntimeRetention           = 7 * 24 * time.Hour
	clusterRuntimeTaskHistoryLimit    = 10_000

	ClusterInstanceStatusOnline  = "online"
	ClusterInstanceStatusStale   = "stale"
	ClusterInstanceStatusStopped = "stopped"

	ClusterTaskStatusRunning   = "running"
	ClusterTaskStatusSucceeded = "succeeded"
	ClusterTaskStatusFailed    = "failed"
	ClusterTaskStatusLost      = "lost"
)

var (
	ErrClusterTaskLeaseLost    = errors.New("cluster task lease lost")
	ErrClusterNodeNotFound     = infraerrors.NotFound("CLUSTER_NODE_NOT_FOUND", "cluster node not found")
	ErrClusterNodeNameConflict = infraerrors.Conflict("CLUSTER_NODE_NAME_CONFLICT", "cluster node name is already in use")
	ErrClusterNodeNameInvalid  = infraerrors.BadRequest("CLUSTER_NODE_NAME_INVALID", "cluster node name is invalid")
)

type ClusterInstance struct {
	NodeID         string     `json:"node_id"`
	RunnerID       string     `json:"runner_id"`
	NodeName       string     `json:"node_name"`
	DeploymentMode string     `json:"deployment_mode"`
	WorkerMode     string     `json:"worker_mode"`
	WorkerEnabled  bool       `json:"worker_enabled"`
	Version        string     `json:"version"`
	Hostname       string     `json:"hostname"`
	ProcessID      int        `json:"process_id"`
	DatabaseOK     bool       `json:"database_ok"`
	RedisOK        bool       `json:"redis_ok"`
	StartedAt      time.Time  `json:"started_at"`
	LastSeenAt     time.Time  `json:"last_seen_at"`
	StoppedAt      *time.Time `json:"stopped_at,omitempty"`
	Status         string     `json:"status"`
	Current        bool       `json:"current"`
}

type ClusterTaskRun struct {
	ID           int64          `json:"id"`
	RunID        string         `json:"run_id"`
	TaskKey      string         `json:"task_key"`
	Status       string         `json:"status"`
	NodeName     string         `json:"node_name"`
	RunnerID     string         `json:"runner_id"`
	Metadata     map[string]any `json:"metadata"`
	Result       map[string]any `json:"result"`
	ErrorMessage string         `json:"error_message"`
	StartedAt    time.Time      `json:"started_at"`
	HeartbeatAt  time.Time      `json:"heartbeat_at"`
	LeaseUntil   time.Time      `json:"lease_until"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
}

type ClusterRepository interface {
	UpsertInstance(ctx context.Context, instance ClusterInstance) error
	RenameNode(ctx context.Context, nodeID, displayName string) error
	MarkInstanceStopped(ctx context.Context, runnerID string, stoppedAt time.Time) error
	ListInstances(ctx context.Context) ([]ClusterInstance, error)
	ExpireStaleTasks(ctx context.Context, now time.Time) error
	PruneRuntime(ctx context.Context, stoppedBefore, taskBefore time.Time, maxTaskHistory int) error
	TryAcquireTask(ctx context.Context, task ClusterTaskRun) (bool, error)
	RenewTaskLease(ctx context.Context, runID, runnerID string, leaseUntil time.Time) error
	FinishTask(ctx context.Context, runID, runnerID, status string, result map[string]any, errorMessage string) error
	ListTaskRuns(ctx context.Context, limit int) ([]ClusterTaskRun, error)
}

type ClusterHealthChecker interface {
	RedisHealthy(ctx context.Context) bool
}

type ClusterDeploymentStatus struct {
	Mode                       string `json:"mode"`
	NodeID                     string `json:"node_id"`
	NodeName                   string `json:"node_name"`
	RunnerID                   string `json:"runner_id"`
	WorkerMode                 string `json:"worker_mode"`
	WorkerEnabled              bool   `json:"worker_enabled"`
	FrontendEnabled            bool   `json:"frontend_enabled"`
	HeartbeatIntervalSeconds   int    `json:"heartbeat_interval_seconds"`
	StaleAfterSeconds          int    `json:"stale_after_seconds"`
	TaskLeaseSeconds           int    `json:"task_lease_seconds"`
	UpdateDriver               string `json:"update_driver"`
	RolloutPollSeconds         int    `json:"rollout_poll_seconds"`
	RolloutDrainGraceSeconds   int    `json:"rollout_drain_grace_seconds"`
	RolloutDrainTimeoutSeconds int    `json:"rollout_drain_timeout_seconds"`
	RolloutVerifyHeartbeats    int    `json:"rollout_verify_heartbeats"`
}

type ClusterSummary struct {
	OnlineNodes    int `json:"online_nodes"`
	StaleNodes     int `json:"stale_nodes"`
	StoppedNodes   int `json:"stopped_nodes"`
	WorkerNodes    int `json:"worker_nodes"`
	ActiveTasks    int `json:"active_tasks"`
	UnhealthyNodes int `json:"unhealthy_nodes"`
}

type ClusterStatus struct {
	Deployment ClusterDeploymentStatus `json:"deployment"`
	Summary    ClusterSummary          `json:"summary"`
	Instances  []ClusterInstance       `json:"instances"`
	Tasks      []ClusterTaskRun        `json:"tasks"`
	Release    *ClusterReleaseOverview `json:"release,omitempty"`
	ObservedAt time.Time               `json:"observed_at"`
}

type ClusterTaskCoordinator interface {
	RunTask(ctx context.Context, taskKey string, metadata map[string]any, fn func(context.Context) (map[string]any, error)) (bool, error)
	WorkerEnabled() bool
}

type ClusterService struct {
	repo      ClusterRepository
	cfg       *config.Config
	health    ClusterHealthChecker
	buildInfo BuildInfo

	nodeID    string
	nodeName  string
	runnerID  string
	startedAt time.Time

	ctx             context.Context
	cancel          context.CancelFunc
	startOnce       sync.Once
	stopOnce        sync.Once
	wg              sync.WaitGroup
	maintenanceMu   sync.Mutex
	lastMaintenance time.Time
}

func NewClusterService(repo ClusterRepository, cfg *config.Config, health ClusterHealthChecker, buildInfo BuildInfo) *ClusterService {
	nodeID := "sub2api-node"
	if cfg != nil && strings.TrimSpace(cfg.Deployment.NodeID) != "" {
		nodeID = strings.TrimSpace(cfg.Deployment.NodeID)
	} else if cfg != nil && strings.TrimSpace(cfg.Deployment.NodeName) != "" {
		nodeID = strings.TrimSpace(cfg.Deployment.NodeName)
	}
	return newClusterService(repo, cfg, health, buildInfo, nodeID)
}

func newClusterService(repo ClusterRepository, cfg *config.Config, health ClusterHealthChecker, buildInfo BuildInfo, nodeID string) *ClusterService {
	ctx, cancel := context.WithCancel(context.Background())
	nodeName := "sub2api-node"
	if cfg != nil && strings.TrimSpace(cfg.Deployment.NodeName) != "" {
		nodeName = strings.TrimSpace(cfg.Deployment.NodeName)
	}
	return &ClusterService{
		repo:      repo,
		cfg:       cfg,
		health:    health,
		buildInfo: buildInfo,
		nodeID:    nodeID,
		nodeName:  nodeName,
		runnerID:  fmt.Sprintf("%s-%s", nodeID, uuid.NewString()),
		startedAt: time.Now().UTC(),
		ctx:       ctx,
		cancel:    cancel,
	}
}

func ProvideClusterService(repo ClusterRepository, cfg *config.Config, health ClusterHealthChecker, buildInfo BuildInfo) (*ClusterService, error) {
	if cfg == nil || !cfg.Deployment.IsMultiInstance() {
		svc := NewClusterService(repo, cfg, health, buildInfo)
		svc.Start()
		return svc, nil
	}
	nodeID, err := resolveClusterNodeID(cfg)
	if err != nil {
		return nil, err
	}
	svc := newClusterService(repo, cfg, health, buildInfo, nodeID)
	svc.Start()
	return svc, nil
}

func (s *ClusterService) Start() {
	if s == nil || s.repo == nil {
		return
	}
	s.startOnce.Do(func() {
		s.reportWithLog()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			interval := s.heartbeatInterval()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-s.ctx.Done():
					return
				case <-ticker.C:
					s.reportWithLog()
				}
			}
		}()
	})
}

func (s *ClusterService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		s.cancel()
		s.wg.Wait()
		if s.repo != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = s.repo.MarkInstanceStopped(ctx, s.runnerID, time.Now().UTC())
		}
	})
}

func (s *ClusterService) WorkerEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Deployment.WorkerEnabledResolved()
}

func (s *ClusterService) NodeName() string {
	if s == nil {
		return ""
	}
	return s.nodeName
}

func (s *ClusterService) NodeID() string {
	if s == nil {
		return ""
	}
	return s.nodeID
}

func (s *ClusterService) RunnerID() string {
	if s == nil {
		return ""
	}
	return s.runnerID
}

func (s *ClusterService) Version() string {
	if s == nil {
		return ""
	}
	return s.buildInfo.Version
}

func (s *ClusterService) RenameNode(ctx context.Context, nodeID, displayName string) error {
	if s == nil || s.repo == nil {
		return errors.New("cluster service is unavailable")
	}
	nodeID = strings.TrimSpace(nodeID)
	displayName = strings.TrimSpace(displayName)
	if nodeID == "" || displayName == "" || len(displayName) > 128 {
		return ErrClusterNodeNameInvalid
	}
	for _, char := range displayName {
		if char < 0x20 || char == 0x7f {
			return ErrClusterNodeNameInvalid
		}
	}
	return s.repo.RenameNode(ctx, nodeID, displayName)
}

func (s *ClusterService) heartbeatInterval() time.Duration {
	seconds := 30
	if s != nil && s.cfg != nil && s.cfg.Deployment.HeartbeatIntervalSeconds > 0 {
		seconds = s.cfg.Deployment.HeartbeatIntervalSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *ClusterService) staleAfter() time.Duration {
	seconds := 90
	if s != nil && s.cfg != nil && s.cfg.Deployment.StaleAfterSeconds > 0 {
		seconds = s.cfg.Deployment.StaleAfterSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *ClusterService) taskLeaseTTL() time.Duration {
	seconds := 60
	if s != nil && s.cfg != nil && s.cfg.Deployment.TaskLeaseSeconds > 0 {
		seconds = s.cfg.Deployment.TaskLeaseSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *ClusterService) currentInstance(ctx context.Context) ClusterInstance {
	hostname, _ := os.Hostname()
	redisOK := false
	if s.health != nil {
		pingCtx, cancel := context.WithTimeout(ctx, time.Second)
		redisOK = s.health.RedisHealthy(pingCtx)
		cancel()
	}
	mode := config.DeploymentModeStandalone
	workerMode := config.WorkerModeAuto
	workerEnabled := true
	if s.cfg != nil {
		mode = s.cfg.Deployment.Mode
		workerMode = s.cfg.Deployment.WorkerMode()
		workerEnabled = s.cfg.Deployment.WorkerEnabledResolved()
	}
	return ClusterInstance{
		NodeID:         s.nodeID,
		RunnerID:       s.runnerID,
		NodeName:       s.nodeName,
		DeploymentMode: mode,
		WorkerMode:     workerMode,
		WorkerEnabled:  workerEnabled,
		Version:        s.buildInfo.Version,
		Hostname:       hostname,
		ProcessID:      os.Getpid(),
		DatabaseOK:     true,
		RedisOK:        redisOK,
		StartedAt:      s.startedAt,
		LastSeenAt:     time.Now().UTC(),
		Status:         ClusterInstanceStatusOnline,
		Current:        true,
	}
}

func (s *ClusterService) reportWithLog() {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	if err := s.repo.UpsertInstance(ctx, s.currentInstance(ctx)); err != nil {
		logger.LegacyPrintf("service.cluster", "[Cluster] heartbeat failed node=%s: %v", s.nodeName, err)
		return
	}
	now := time.Now().UTC()
	if !s.beginMaintenance(now) {
		return
	}
	if err := s.repo.ExpireStaleTasks(ctx, now); err != nil {
		logger.LegacyPrintf("service.cluster", "[Cluster] expire stale tasks failed: %v", err)
	}
	if err := s.repo.PruneRuntime(
		ctx,
		now.Add(-clusterRuntimeRetention),
		now.Add(-clusterRuntimeRetention),
		clusterRuntimeTaskHistoryLimit,
	); err != nil {
		logger.LegacyPrintf("service.cluster", "[Cluster] prune runtime history failed: %v", err)
	}
}

func (s *ClusterService) beginMaintenance(now time.Time) bool {
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	if !s.lastMaintenance.IsZero() && now.Sub(s.lastMaintenance) < clusterRuntimeMaintenanceInterval {
		return false
	}
	s.lastMaintenance = now
	return true
}

func (s *ClusterService) GetStatus(ctx context.Context) (*ClusterStatus, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("cluster service is unavailable")
	}
	if err := s.repo.UpsertInstance(ctx, s.currentInstance(ctx)); err != nil {
		return nil, fmt.Errorf("refresh current cluster instance: %w", err)
	}
	now := time.Now().UTC()
	_ = s.repo.ExpireStaleTasks(ctx, now)
	instances, err := s.repo.ListInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cluster instances: %w", err)
	}
	tasks, err := s.repo.ListTaskRuns(ctx, 100)
	if err != nil {
		return nil, fmt.Errorf("list cluster tasks: %w", err)
	}

	deployment := config.DeploymentConfig{
		Mode:                     config.DeploymentModeStandalone,
		WorkerEnabled:            config.WorkerModeAuto,
		HeartbeatIntervalSeconds: 30,
		StaleAfterSeconds:        90,
		TaskLeaseSeconds:         60,
	}
	if s.cfg != nil {
		deployment = s.cfg.Deployment
	}
	status := &ClusterStatus{
		Deployment: ClusterDeploymentStatus{
			Mode:                       deployment.Mode,
			NodeID:                     s.nodeID,
			NodeName:                   s.nodeName,
			RunnerID:                   s.runnerID,
			WorkerMode:                 deployment.WorkerMode(),
			WorkerEnabled:              deployment.WorkerEnabledResolved(),
			FrontendEnabled:            true,
			HeartbeatIntervalSeconds:   deployment.HeartbeatIntervalSeconds,
			StaleAfterSeconds:          deployment.StaleAfterSeconds,
			TaskLeaseSeconds:           deployment.TaskLeaseSeconds,
			UpdateDriver:               deployment.UpdateDriverMode(),
			RolloutPollSeconds:         deployment.RolloutPollSeconds,
			RolloutDrainGraceSeconds:   deployment.RolloutDrainGraceSeconds,
			RolloutDrainTimeoutSeconds: deployment.RolloutDrainTimeoutSeconds,
			RolloutVerifyHeartbeats:    deployment.RolloutVerifyHeartbeats,
		},
		Instances:  instances,
		Tasks:      tasks,
		ObservedAt: now,
	}
	staleBefore := now.Add(-s.staleAfter())
	for i := range status.Instances {
		instance := &status.Instances[i]
		instance.Current = instance.RunnerID == s.runnerID
		if instance.Current {
			status.Deployment.NodeName = instance.NodeName
		}
		switch {
		case instance.StoppedAt != nil:
			instance.Status = ClusterInstanceStatusStopped
			status.Summary.StoppedNodes++
		case instance.LastSeenAt.Before(staleBefore):
			instance.Status = ClusterInstanceStatusStale
			status.Summary.StaleNodes++
		default:
			instance.Status = ClusterInstanceStatusOnline
			status.Summary.OnlineNodes++
			if instance.WorkerEnabled {
				status.Summary.WorkerNodes++
			}
			if !instance.DatabaseOK || !instance.RedisOK {
				status.Summary.UnhealthyNodes++
			}
		}
	}
	for i := range status.Tasks {
		if status.Tasks[i].Status == ClusterTaskStatusRunning {
			status.Summary.ActiveTasks++
		}
	}
	return status, nil
}

func (s *ClusterService) RunTask(ctx context.Context, taskKey string, metadata map[string]any, fn func(context.Context) (map[string]any, error)) (bool, error) {
	if s == nil || s.repo == nil || fn == nil {
		return false, errors.New("cluster task coordinator is unavailable")
	}
	if !s.WorkerEnabled() {
		return false, nil
	}
	taskKey = strings.TrimSpace(taskKey)
	if taskKey == "" {
		return false, errors.New("cluster task key is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC()
	task := ClusterTaskRun{
		RunID:       uuid.NewString(),
		TaskKey:     taskKey,
		Status:      ClusterTaskStatusRunning,
		NodeName:    s.nodeName,
		RunnerID:    s.runnerID,
		Metadata:    metadata,
		Result:      map[string]any{},
		StartedAt:   now,
		HeartbeatAt: now,
		LeaseUntil:  now.Add(s.taskLeaseTTL()),
	}
	acquired, err := s.repo.TryAcquireTask(ctx, task)
	if err != nil || !acquired {
		return acquired, err
	}

	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	heartbeatErr := make(chan error, 1)
	ttl := s.taskLeaseTTL()
	interval := ttl / 3
	if interval < time.Second {
		interval = time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-taskCtx.Done():
				return
			case <-ticker.C:
				renewCtx, renewCancel := context.WithTimeout(context.Background(), 3*time.Second)
				err := s.repo.RenewTaskLease(renewCtx, task.RunID, s.runnerID, time.Now().UTC().Add(ttl))
				renewCancel()
				if err != nil {
					select {
					case heartbeatErr <- err:
					default:
					}
					cancel()
					return
				}
			}
		}
	}()

	result, runErr := fn(taskCtx)
	close(done)
	select {
	case leaseErr := <-heartbeatErr:
		if runErr == nil {
			runErr = fmt.Errorf("%w: %v", ErrClusterTaskLeaseLost, leaseErr)
		}
	default:
	}
	status := ClusterTaskStatusSucceeded
	errorMessage := ""
	if runErr != nil {
		status = ClusterTaskStatusFailed
		errorMessage = runErr.Error()
	}
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
	finishErr := s.repo.FinishTask(finishCtx, task.RunID, s.runnerID, status, result, errorMessage)
	finishCancel()
	if runErr != nil {
		return true, runErr
	}
	if finishErr != nil {
		return true, finishErr
	}
	return true, nil
}
