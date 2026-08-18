//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestClusterReleaseRepository_RollsNodesInOrderAndConvergesDesiredVersion(t *testing.T) {
	ctx := context.Background()
	releaseRepo := NewClusterReleaseRepository(integrationDB)
	clusterRepo := NewClusterRepository(integrationDB)
	suffix := uuid.NewString()
	rolloutID := uuid.NewString()
	nodeA := "release-a-" + suffix
	nodeB := "release-b-" + suffix
	now := time.Now().UTC().Truncate(time.Microsecond)

	_, err := integrationDB.ExecContext(ctx, `UPDATE cluster_release_state SET desired_version = '1.0.0', locked_version = '1.0.0', active_rollout_id = NULL WHERE singleton = TRUE`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `UPDATE cluster_release_state SET desired_version = '1.0.0', locked_version = '1.0.0', active_rollout_id = NULL WHERE singleton = TRUE`)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM cluster_release_rollouts WHERE id = $1`, rolloutID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM cluster_instances WHERE node_id IN ($1,$2)`, nodeA, nodeB)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM cluster_nodes WHERE node_id IN ($1,$2)`, nodeA, nodeB)
	})

	upsertReleaseTestInstance := func(nodeID, runnerID, version string, heartbeat time.Time) {
		require.NoError(t, clusterRepo.UpsertInstance(ctx, service.ClusterInstance{
			NodeID:         nodeID,
			RunnerID:       runnerID,
			NodeName:       nodeID,
			DeploymentMode: "multi_instance",
			WorkerMode:     "auto",
			WorkerEnabled:  true,
			Version:        version,
			Hostname:       nodeID,
			ProcessID:      1,
			DatabaseOK:     true,
			RedisOK:        true,
			StartedAt:      heartbeat,
			LastSeenAt:     heartbeat,
		}))
	}

	upsertReleaseTestInstance(nodeA, nodeA+"-old", "1.0.0", now)
	upsertReleaseTestInstance(nodeB, nodeB+"-old", "1.0.0", now)
	runner, err := releaseRepo.GetRunnerInstance(ctx, nodeA+"-old")
	require.NoError(t, err)
	require.Equal(t, nodeA, runner.NodeID)
	require.Equal(t, nodeA, runner.NodeName)
	require.Equal(t, now, runner.LastSeenAt)
	require.NoError(t, releaseRepo.EnsureState(ctx, "1.0.0"))
	require.NoError(t, releaseRepo.CreateRollout(ctx, service.ClusterRollout{
		ID:             rolloutID,
		SourceVersion:  "1.0.0",
		TargetVersion:  "1.1.0",
		Status:         service.ClusterRolloutStatusRunning,
		Strategy:       service.ClusterRolloutStrategyRolling,
		MaxUnavailable: 1,
		StartedAt:      now,
	}, []service.ClusterRolloutTarget{
		{
			RolloutID:      rolloutID,
			NodeID:         nodeA,
			NodeName:       nodeA,
			Ordinal:        0,
			SourceVersion:  "1.0.0",
			TargetVersion:  "1.1.0",
			Status:         service.ClusterRolloutTargetPending,
			SourceRunnerID: nodeA + "-old",
		},
		{
			RolloutID:      rolloutID,
			NodeID:         nodeB,
			NodeName:       nodeB,
			Ordinal:        1,
			SourceVersion:  "1.0.0",
			TargetVersion:  "1.1.0",
			Status:         service.ClusterRolloutTargetPending,
			SourceRunnerID: nodeB + "-old",
		},
	}))

	claimed, err := releaseRepo.ClaimTarget(ctx, rolloutID, nodeB, nodeB+"-old", now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, claimed, "the second node must wait for the first node")
	require.NoError(t, releaseRepo.PauseRollout(ctx, rolloutID, "integration test pause"))
	claimed, err = releaseRepo.ClaimTarget(ctx, rolloutID, nodeA, nodeA+"-old", now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, claimed, "a paused rollout must not issue a target lease")
	require.NoError(t, releaseRepo.ResumeRollout(ctx, rolloutID))
	claimed, err = releaseRepo.ClaimTarget(ctx, rolloutID, nodeA, nodeA+"-old", now.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, claimed)
	restartLease := now.Add(5 * time.Minute)
	require.NoError(t, releaseRepo.SetTargetStatus(ctx, rolloutID, nodeA, nodeA+"-old", service.ClusterRolloutTargetRestarting, "", &restartLease))

	upsertReleaseTestInstance(nodeA, nodeA+"-new", "1.1.0", now.Add(time.Second))
	target, completed, err := releaseRepo.ObserveTargetHeartbeat(ctx, rolloutID, nodeA, nodeA+"-new", "1.1.0", now.Add(time.Second), 2, restartLease)
	require.NoError(t, err)
	require.False(t, completed)
	require.Equal(t, service.ClusterRolloutTargetVerifying, target.Status)
	require.Equal(t, 1, target.VerificationCount)

	upsertReleaseTestInstance(nodeA, nodeA+"-new", "1.1.0", now.Add(2*time.Second))
	target, completed, err = releaseRepo.ObserveTargetHeartbeat(ctx, rolloutID, nodeA, nodeA+"-new", "1.1.0", now.Add(2*time.Second), 2, restartLease)
	require.NoError(t, err)
	require.False(t, completed)
	require.Equal(t, service.ClusterRolloutTargetSucceeded, target.Status)

	claimed, err = releaseRepo.ClaimTarget(ctx, rolloutID, nodeB, nodeB+"-old", now.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, releaseRepo.SetTargetStatus(ctx, rolloutID, nodeB, nodeB+"-old", service.ClusterRolloutTargetRestarting, "", &restartLease))
	upsertReleaseTestInstance(nodeB, nodeB+"-new", "1.1.0", now.Add(3*time.Second))
	_, completed, err = releaseRepo.ObserveTargetHeartbeat(ctx, rolloutID, nodeB, nodeB+"-new", "1.1.0", now.Add(3*time.Second), 1, restartLease)
	require.NoError(t, err)
	require.True(t, completed)

	state, err := releaseRepo.GetState(ctx)
	require.NoError(t, err)
	require.Equal(t, "1.1.0", state.DesiredVersion)
	require.Empty(t, state.LockedVersion)
	require.Equal(t, rolloutID, state.ActiveRolloutID)
	rollout, err := releaseRepo.GetRollout(ctx, rolloutID)
	require.NoError(t, err)
	require.Equal(t, service.ClusterRolloutStatusAwaitingConfirmation, rollout.Status)

	require.NoError(t, releaseRepo.ConfirmRollout(ctx, rolloutID))
	state, err = releaseRepo.GetState(ctx)
	require.NoError(t, err)
	require.Equal(t, "1.1.0", state.LockedVersion)
	require.Empty(t, state.ActiveRolloutID)
	rollout, err = releaseRepo.GetRollout(ctx, rolloutID)
	require.NoError(t, err)
	require.Equal(t, service.ClusterRolloutStatusCompleted, rollout.Status)
}
