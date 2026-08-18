package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type clusterReleaseRepositoryStub struct {
	state             ClusterReleaseState
	rollout           *ClusterRollout
	targets           map[string]ClusterRolloutTarget
	instance          map[string]ClusterInstance
	claimEnabled      bool
	claimedNodeIDs    []string
	statusTransitions []string
}

func newClusterReleaseRepositoryStub(desiredVersion string) *clusterReleaseRepositoryStub {
	return &clusterReleaseRepositoryStub{
		state:    ClusterReleaseState{DesiredVersion: desiredVersion, LockedVersion: desiredVersion, UpdatedAt: time.Now().UTC()},
		targets:  make(map[string]ClusterRolloutTarget),
		instance: make(map[string]ClusterInstance),
	}
}

func (r *clusterReleaseRepositoryStub) EnsureState(_ context.Context, initialVersion string) error {
	if r.state.Generation == 0 && r.state.ActiveRolloutID == "" && r.state.DesiredVersion == "" && r.state.LockedVersion == "" {
		r.state.DesiredVersion = initialVersion
		r.state.LockedVersion = initialVersion
	}
	return nil
}

func (r *clusterReleaseRepositoryStub) GetState(context.Context) (*ClusterReleaseState, error) {
	state := r.state
	return &state, nil
}

func (r *clusterReleaseRepositoryStub) CreateRollout(_ context.Context, rollout ClusterRollout, targets []ClusterRolloutTarget) error {
	if r.state.ActiveRolloutID != "" {
		return ErrClusterRolloutActive
	}
	r.state.ActiveRolloutID = rollout.ID
	r.state.DesiredVersion = rollout.TargetVersion
	r.state.LockedVersion = ""
	r.state.Generation++
	copyRollout := rollout
	copyRollout.Targets = append([]ClusterRolloutTarget(nil), targets...)
	r.rollout = &copyRollout
	for _, target := range targets {
		r.targets[target.NodeID] = target
	}
	return nil
}

func (r *clusterReleaseRepositoryStub) GetRollout(_ context.Context, rolloutID string) (*ClusterRollout, error) {
	if r.rollout == nil || r.rollout.ID != rolloutID {
		return nil, nil
	}
	rollout := *r.rollout
	rollout.Targets = append([]ClusterRolloutTarget(nil), r.rollout.Targets...)
	return &rollout, nil
}

func (r *clusterReleaseRepositoryStub) GetActiveRollout(ctx context.Context) (*ClusterRollout, error) {
	return r.GetRollout(ctx, r.state.ActiveRolloutID)
}

func (r *clusterReleaseRepositoryStub) ListRecentRollouts(context.Context, int) ([]ClusterRollout, error) {
	if r.rollout == nil {
		return nil, nil
	}
	return []ClusterRollout{*r.rollout}, nil
}

func (r *clusterReleaseRepositoryStub) GetTargetForNode(_ context.Context, rolloutID, nodeID string) (*ClusterRolloutTarget, error) {
	if r.rollout == nil || r.rollout.ID != rolloutID {
		return nil, nil
	}
	target, ok := r.targets[nodeID]
	if !ok {
		return nil, nil
	}
	return &target, nil
}

func (r *clusterReleaseRepositoryStub) GetRunnerInstance(_ context.Context, runnerID string) (*ClusterInstance, error) {
	instance, ok := r.instance[runnerID]
	if !ok {
		return nil, nil
	}
	return &instance, nil
}

func (r *clusterReleaseRepositoryStub) ClaimTarget(_ context.Context, rolloutID, nodeID, runnerID string, leaseUntil time.Time) (bool, error) {
	if !r.claimEnabled {
		return false, nil
	}
	target, ok := r.targets[nodeID]
	if !ok || target.RolloutID != rolloutID || target.Status != ClusterRolloutTargetPending {
		return false, nil
	}
	target.Status = ClusterRolloutTargetInstalling
	target.LeaseOwner = runnerID
	target.LeaseUntil = &leaseUntil
	target.Attempt++
	r.targets[nodeID] = target
	r.claimedNodeIDs = append(r.claimedNodeIDs, nodeID)
	return true, nil
}

func (r *clusterReleaseRepositoryStub) SetTargetStatus(_ context.Context, rolloutID, nodeID, runnerID, status, errorMessage string, leaseUntil *time.Time) error {
	target, ok := r.targets[nodeID]
	if !ok || target.RolloutID != rolloutID {
		return ErrClusterRolloutTargetNotFound
	}
	target.Status = status
	target.LeaseOwner = runnerID
	target.LeaseUntil = leaseUntil
	target.ErrorMessage = errorMessage
	r.targets[nodeID] = target
	r.statusTransitions = append(r.statusTransitions, status)
	return nil
}

func (r *clusterReleaseRepositoryStub) ObserveTargetHeartbeat(context.Context, string, string, string, string, time.Time, int, time.Time) (*ClusterRolloutTarget, bool, error) {
	return nil, false, nil
}

func (r *clusterReleaseRepositoryStub) FailExpiredTargets(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func (r *clusterReleaseRepositoryStub) PauseRollout(context.Context, string, string) error {
	return nil
}

func (r *clusterReleaseRepositoryStub) ResumeRollout(context.Context, string) error {
	return nil
}

func (r *clusterReleaseRepositoryStub) CancelRollout(context.Context, string) error {
	return nil
}

func (r *clusterReleaseRepositoryStub) RetryTarget(context.Context, string, string) error {
	return nil
}

func (r *clusterReleaseRepositoryStub) ConfirmRollout(_ context.Context, rolloutID string) error {
	if r.rollout == nil || r.rollout.ID != rolloutID {
		return ErrClusterRolloutNotFound
	}
	for _, target := range r.targets {
		if target.Status != ClusterRolloutTargetSucceeded {
			return ErrClusterRolloutNotReadyToConfirm
		}
	}
	r.rollout.Status = ClusterRolloutStatusCompleted
	r.state.LockedVersion = r.rollout.TargetVersion
	r.state.DesiredVersion = r.rollout.TargetVersion
	r.state.ActiveRolloutID = ""
	r.state.Generation++
	return nil
}

type clusterReleaseGitHubClientStub struct {
	releases []*GitHubRelease
}

type clusterReleaseUpdaterStub struct {
	installedVersions []string
}

func (s *clusterReleaseUpdaterStub) ResolveReleaseVersion(_ context.Context, version string) (string, error) {
	return normalizeClusterReleaseVersion(version), nil
}

func (s *clusterReleaseUpdaterStub) InstallVersion(_ context.Context, version string) error {
	s.installedVersions = append(s.installedVersions, normalizeClusterReleaseVersion(version))
	return nil
}

func (s *clusterReleaseGitHubClientStub) FetchLatestRelease(context.Context, string) (*GitHubRelease, error) {
	if len(s.releases) == 0 {
		return nil, errors.New("release not found")
	}
	return s.releases[0], nil
}

func (s *clusterReleaseGitHubClientStub) FetchRecentReleases(context.Context, string, int) ([]*GitHubRelease, error) {
	return s.releases, nil
}

func (s *clusterReleaseGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	return errors.New("download is not expected")
}

func (s *clusterReleaseGitHubClientStub) FetchReleaseFile(context.Context, string, int64) ([]byte, error) {
	return nil, errors.New("download is not expected")
}

func TestClusterReleaseService_CreateRolloutTargetsEveryLogicalNode(t *testing.T) {
	clusterRepo := newClusterRepositoryStub()
	now := time.Now().UTC()
	clusterRepo.instances["runner-b"] = ClusterInstance{
		NodeID:         "node-b",
		RunnerID:       "runner-b",
		NodeName:       "worker-b",
		DeploymentMode: config.DeploymentModeMultiInstance,
		WorkerMode:     config.WorkerModeAuto,
		WorkerEnabled:  true,
		Version:        "1.0.0",
		Hostname:       "host-b",
		DatabaseOK:     true,
		RedisOK:        true,
		StartedAt:      now.Add(-time.Minute),
		LastSeenAt:     now,
	}

	cfg := clusterTestConfig("api-a", config.WorkerModeAuto)
	cfg.Deployment.NodeID = "node-a"
	cluster := NewClusterService(clusterRepo, cfg, clusterHealthCheckerStub(true), BuildInfo{Version: "1.0.0"})
	releaseRepo := newClusterReleaseRepositoryStub("1.0.0")
	updater := NewUpdateService(nil, &clusterReleaseGitHubClientStub{releases: []*GitHubRelease{
		{TagName: "v1.1.0", Name: "v1.1.0"},
	}}, "1.0.0", "release")
	releases := NewClusterReleaseService(releaseRepo, cluster, updater, cfg)

	rollout, err := releases.CreateRollout(context.Background(), CreateClusterRolloutInput{
		TargetVersion: "1.1.0",
		CreatedBy:     42,
	})
	require.NoError(t, err)
	require.Equal(t, "1.1.0", rollout.TargetVersion)
	require.Equal(t, int64(42), rollout.CreatedBy)
	require.Len(t, rollout.Targets, 2)
	require.Equal(t, []string{"node-a", "node-b"}, []string{
		rollout.Targets[0].NodeID,
		rollout.Targets[1].NodeID,
	})
	require.Equal(t, []string{"api-a", "worker-b"}, []string{
		rollout.Targets[0].NodeName,
		rollout.Targets[1].NodeName,
	})
	require.Equal(t, rollout.ID, releaseRepo.state.ActiveRolloutID)
	require.Equal(t, "1.1.0", releaseRepo.state.DesiredVersion, "candidate version is announced before the first restart")
	require.Empty(t, releaseRepo.state.LockedVersion, "readiness remains unlocked until manual confirmation")
	require.True(t, releases.GetReadiness().Ready, "the source version remains ready while its target is pending")

	cluster.buildInfo.Version = "1.1.0"
	require.NoError(t, releases.refreshReadiness(context.Background()))
	readiness := releases.GetReadiness()
	require.True(t, readiness.Ready, "the target version may serve while the rollout is being verified")
}

func TestClusterReleaseService_FixedBinaryDriverInstallsAndRestartsClaimedTarget(t *testing.T) {
	cfg := clusterTestConfig("api-a", config.WorkerModeAuto)
	cfg.Deployment.NodeID = "node-a"
	cfg.Deployment.RolloutDrainGraceSeconds = 10
	cfg.Deployment.RolloutDrainTimeoutSeconds = 1
	cluster := NewClusterService(newClusterRepositoryStub(), cfg, clusterHealthCheckerStub(true), BuildInfo{Version: "1.0.0"})

	releaseRepo := newClusterReleaseRepositoryStub("1.0.0")
	releaseRepo.state.ActiveRolloutID = "rollout-1"
	releaseRepo.rollout = &ClusterRollout{
		ID:            "rollout-1",
		TargetVersion: "1.1.0",
		Status:        ClusterRolloutStatusRunning,
	}
	releaseRepo.targets["node-a"] = ClusterRolloutTarget{
		RolloutID:     "rollout-1",
		NodeID:        "node-a",
		NodeName:      "api-a",
		SourceVersion: "1.0.0",
		TargetVersion: "1.1.0",
		Status:        ClusterRolloutTargetPending,
	}
	releaseRepo.claimEnabled = true

	releases := NewClusterReleaseService(releaseRepo, cluster, nil, cfg)
	updater := &clusterReleaseUpdaterStub{}
	releases.updater = updater
	releases.inFlight.Store(1)
	restarted := make(chan struct{}, 1)
	releases.restartAsync = func() { restarted <- struct{}{} }

	startedAt := time.Now()
	require.NoError(t, releases.processOnce(context.Background()))
	require.Less(t, time.Since(startedAt), 500*time.Millisecond, "the binary download path must not wait for the legacy drain settings")
	require.Equal(t, []string{"node-a"}, releaseRepo.claimedNodeIDs)
	require.Equal(t, []string{ClusterRolloutTargetInstalling, ClusterRolloutTargetRestarting}, releaseRepo.statusTransitions)
	require.Equal(t, []string{"1.1.0"}, updater.installedVersions)
	require.Equal(t, ClusterRolloutTargetRestarting, releaseRepo.targets["node-a"].Status)
	require.False(t, releases.GetReadiness().Draining, "rollouts no longer wait in a local drain phase")
	require.True(t, releases.GetReadiness().Ready, "the source binary remains eligible while the target restarts")

	select {
	case <-restarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for binary rollout restart")
	}
}

func TestClusterReleaseService_ConfirmRolloutLocksVerifiedVersion(t *testing.T) {
	cfg := clusterTestConfig("api-a", config.WorkerModeAuto)
	cfg.Deployment.NodeID = "node-a"
	cluster := NewClusterService(newClusterRepositoryStub(), cfg, clusterHealthCheckerStub(true), BuildInfo{Version: "1.1.0"})
	releaseRepo := newClusterReleaseRepositoryStub("1.0.0")
	releaseRepo.state.ActiveRolloutID = "rollout-1"
	releaseRepo.state.DesiredVersion = "1.1.0"
	releaseRepo.state.LockedVersion = ""
	releaseRepo.rollout = &ClusterRollout{ID: "rollout-1", TargetVersion: "1.1.0", Status: ClusterRolloutStatusAwaitingConfirmation}
	releaseRepo.targets["node-a"] = ClusterRolloutTarget{
		RolloutID: "rollout-1", NodeID: "node-a", SourceVersion: "1.0.0", TargetVersion: "1.1.0", Status: ClusterRolloutTargetSucceeded,
	}
	releases := NewClusterReleaseService(releaseRepo, cluster, nil, cfg)

	require.NoError(t, releases.ConfirmRollout(context.Background(), "rollout-1"))
	require.Empty(t, releaseRepo.state.ActiveRolloutID)
	require.Equal(t, "1.1.0", releaseRepo.state.LockedVersion)
	require.True(t, releases.GetReadiness().Ready)
}

func TestSnapshotOnlineClusterNodesRejectsOverlappingRunnersForOneIdentity(t *testing.T) {
	now := time.Now().UTC()
	_, err := snapshotOnlineClusterNodes([]ClusterInstance{
		{
			NodeID:     "shared-node",
			NodeName:   "api-a",
			RunnerID:   "runner-old",
			Status:     ClusterInstanceStatusOnline,
			StartedAt:  now.Add(-time.Minute),
			LastSeenAt: now.Add(time.Second),
		},
		{
			NodeID:     "shared-node",
			NodeName:   "api-a",
			RunnerID:   "runner-new",
			Status:     ClusterInstanceStatusOnline,
			StartedAt:  now,
			LastSeenAt: now.Add(2 * time.Second),
		},
	})
	require.ErrorIs(t, err, ErrClusterRolloutDuplicateNode)
}
