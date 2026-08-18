package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

const (
	ClusterRolloutStatusRunning = "running"
	ClusterRolloutStatusPaused  = "paused"
	// AwaitingConfirmation means every target has verified the candidate
	// version, but the administrator has not locked it as the cluster version.
	ClusterRolloutStatusAwaitingConfirmation = "awaiting_confirmation"
	ClusterRolloutStatusCompleted            = "completed"
	ClusterRolloutStatusCancelled            = "cancelled"

	ClusterRolloutTargetPending    = "pending"
	ClusterRolloutTargetDraining   = "draining"
	ClusterRolloutTargetInstalling = "installing"
	ClusterRolloutTargetRestarting = "restarting"
	ClusterRolloutTargetVerifying  = "verifying"
	ClusterRolloutTargetSucceeded  = "succeeded"
	ClusterRolloutTargetFailed     = "failed"
	ClusterRolloutTargetCancelled  = "cancelled"

	ClusterRolloutStrategyRolling = "rolling"
)

var (
	ErrClusterRolloutNotFound              = infraerrors.NotFound("CLUSTER_ROLLOUT_NOT_FOUND", "cluster rollout not found")
	ErrClusterRolloutActive                = infraerrors.Conflict("CLUSTER_ROLLOUT_ACTIVE", "another cluster rollout is active")
	ErrClusterRolloutNoNodes               = infraerrors.Conflict("CLUSTER_ROLLOUT_NO_ONLINE_NODES", "no online nodes are available for rollout")
	ErrClusterRolloutDuplicateNode         = infraerrors.Conflict("CLUSTER_ROLLOUT_DUPLICATE_NODE", "multiple online runners use the same node_id")
	ErrClusterRolloutAlreadyAtTarget       = infraerrors.Conflict("CLUSTER_ALREADY_AT_TARGET", "all online nodes already run the target version")
	ErrClusterRolloutInvalidState          = infraerrors.Conflict("CLUSTER_ROLLOUT_INVALID_STATE", "cluster rollout is not in a valid state for this operation")
	ErrClusterRolloutTargetNotFound        = infraerrors.NotFound("CLUSTER_ROLLOUT_TARGET_NOT_FOUND", "cluster rollout target not found")
	ErrClusterRolloutActiveTarget          = infraerrors.Conflict("CLUSTER_ROLLOUT_TARGET_ACTIVE", "a rollout target is still active")
	ErrClusterRolloutNotReadyToConfirm     = infraerrors.Conflict("CLUSTER_ROLLOUT_NOT_READY_TO_CONFIRM", "all rollout targets must be verified before confirmation")
	ErrClusterRolloutRequiresMultiInstance = infraerrors.Conflict("MULTI_INSTANCE_ROLLOUT_REQUIRED", "use the cluster rollout workflow in multi-instance mode")
)

type ClusterReleaseState struct {
	DesiredVersion string `json:"desired_version"`
	// LockedVersion is the version enforced by the readiness gate. It is empty
	// while a rollout is in progress or the cluster has been deliberately left
	// unlocked after a cancellation.
	LockedVersion   string    `json:"locked_version,omitempty"`
	ActiveRolloutID string    `json:"active_rollout_id,omitempty"`
	Generation      int64     `json:"generation"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ClusterRollout struct {
	ID             string                 `json:"id"`
	SourceVersion  string                 `json:"source_version,omitempty"`
	TargetVersion  string                 `json:"target_version"`
	Status         string                 `json:"status"`
	Strategy       string                 `json:"strategy"`
	MaxUnavailable int                    `json:"max_unavailable"`
	CreatedBy      int64                  `json:"created_by"`
	ErrorMessage   string                 `json:"error_message,omitempty"`
	StartedAt      time.Time              `json:"started_at"`
	CompletedAt    *time.Time             `json:"completed_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	Targets        []ClusterRolloutTarget `json:"targets"`
}

type ClusterRolloutTarget struct {
	RolloutID             string     `json:"rollout_id"`
	NodeID                string     `json:"node_id"`
	NodeName              string     `json:"node_name"`
	Ordinal               int        `json:"ordinal"`
	SourceVersion         string     `json:"source_version"`
	TargetVersion         string     `json:"target_version"`
	Status                string     `json:"status"`
	Attempt               int        `json:"attempt"`
	LeaseOwner            string     `json:"lease_owner,omitempty"`
	LeaseUntil            *time.Time `json:"lease_until,omitempty"`
	SourceRunnerID        string     `json:"source_runner_id,omitempty"`
	ObservedRunnerID      string     `json:"observed_runner_id,omitempty"`
	VerificationCount     int        `json:"verification_count"`
	LastVerifiedHeartbeat *time.Time `json:"last_verified_heartbeat,omitempty"`
	ErrorMessage          string     `json:"error_message,omitempty"`
	StartedAt             *time.Time `json:"started_at,omitempty"`
	CompletedAt           *time.Time `json:"completed_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type ClusterVersionCount struct {
	Version string `json:"version"`
	Nodes   int    `json:"nodes"`
}

type ClusterReleaseOverview struct {
	State          ClusterReleaseState   `json:"state"`
	ActiveRollout  *ClusterRollout       `json:"active_rollout,omitempty"`
	RecentRollouts []ClusterRollout      `json:"recent_rollouts"`
	VersionCounts  []ClusterVersionCount `json:"version_counts"`
	Consistent     bool                  `json:"consistent"`
}

type ClusterReadiness struct {
	Ready          bool   `json:"ready"`
	Reason         string `json:"reason,omitempty"`
	NodeID         string `json:"node_id"`
	NodeName       string `json:"node_name"`
	CurrentVersion string `json:"current_version"`
	DesiredVersion string `json:"desired_version,omitempty"`
	RolloutID      string `json:"rollout_id,omitempty"`
	TargetStatus   string `json:"target_status,omitempty"`
	Draining       bool   `json:"draining"`
	InFlight       int64  `json:"in_flight"`
}

type CreateClusterRolloutInput struct {
	TargetVersion string
	CreatedBy     int64
}

type ClusterReleaseRepository interface {
	EnsureState(ctx context.Context, initialVersion string) error
	GetState(ctx context.Context) (*ClusterReleaseState, error)
	CreateRollout(ctx context.Context, rollout ClusterRollout, targets []ClusterRolloutTarget) error
	GetRollout(ctx context.Context, rolloutID string) (*ClusterRollout, error)
	GetActiveRollout(ctx context.Context) (*ClusterRollout, error)
	ListRecentRollouts(ctx context.Context, limit int) ([]ClusterRollout, error)
	GetTargetForNode(ctx context.Context, rolloutID, nodeID string) (*ClusterRolloutTarget, error)
	GetRunnerInstance(ctx context.Context, runnerID string) (*ClusterInstance, error)
	ClaimTarget(ctx context.Context, rolloutID, nodeID, runnerID string, leaseUntil time.Time) (bool, error)
	SetTargetStatus(ctx context.Context, rolloutID, nodeID, runnerID, status, errorMessage string, leaseUntil *time.Time) error
	ObserveTargetHeartbeat(ctx context.Context, rolloutID, nodeID, runnerID, version string, heartbeatAt time.Time, required int, leaseUntil time.Time) (*ClusterRolloutTarget, bool, error)
	FailExpiredTargets(ctx context.Context, now time.Time) (int64, error)
	PauseRollout(ctx context.Context, rolloutID, reason string) error
	ResumeRollout(ctx context.Context, rolloutID string) error
	CancelRollout(ctx context.Context, rolloutID string) error
	RetryTarget(ctx context.Context, rolloutID, nodeID string) error
	ConfirmRollout(ctx context.Context, rolloutID string) error
}
