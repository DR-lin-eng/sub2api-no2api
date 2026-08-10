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
	state    ClusterReleaseState
	rollout  *ClusterRollout
	targets  map[string]ClusterRolloutTarget
	instance map[string]ClusterInstance
}

func newClusterReleaseRepositoryStub(desiredVersion string) *clusterReleaseRepositoryStub {
	return &clusterReleaseRepositoryStub{
		state:    ClusterReleaseState{DesiredVersion: desiredVersion, UpdatedAt: time.Now().UTC()},
		targets:  make(map[string]ClusterRolloutTarget),
		instance: make(map[string]ClusterInstance),
	}
}

func (r *clusterReleaseRepositoryStub) EnsureState(_ context.Context, initialVersion string) error {
	if r.state.DesiredVersion == "" {
		r.state.DesiredVersion = initialVersion
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

func (r *clusterReleaseRepositoryStub) ClaimTarget(context.Context, string, string, string, time.Time) (bool, error) {
	return false, nil
}

func (r *clusterReleaseRepositoryStub) SetTargetStatus(context.Context, string, string, string, string, string, *time.Time) error {
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

type clusterReleaseGitHubClientStub struct {
	releases []*GitHubRelease
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
	cfg.Deployment.UpdateDriver = config.DeploymentUpdateDriverExternal
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
	require.True(t, releases.GetReadiness().Ready, "the source version remains ready while its target is pending")

	cluster.buildInfo.Version = "1.1.0"
	require.NoError(t, releases.refreshReadiness(context.Background()))
	readiness := releases.GetReadiness()
	require.False(t, readiness.Ready)
	require.Equal(t, "awaiting_target_verification", readiness.Reason)
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
