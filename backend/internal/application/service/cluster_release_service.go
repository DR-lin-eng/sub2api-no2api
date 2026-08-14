package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/sysutil"
	"github.com/google/uuid"
)

const (
	clusterRolloutInstallTimeout = 20 * time.Minute
	clusterRolloutRestartTimeout = 5 * time.Minute
)

type clusterReleaseUpdater interface {
	ResolveReleaseVersion(context.Context, string) (string, error)
	InstallVersion(context.Context, string) error
}

type ClusterReleaseService struct {
	repo    ClusterReleaseRepository
	cluster *ClusterService
	updater clusterReleaseUpdater
	cfg     *config.Config

	readiness atomic.Pointer[ClusterReadiness]
	draining  atomic.Bool
	inFlight  atomic.Int64

	ctx       context.Context
	cancel    context.CancelFunc
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup

	restartAsync func()
}

func NewClusterReleaseService(
	repo ClusterReleaseRepository,
	cluster *ClusterService,
	updater *UpdateService,
	cfg *config.Config,
) *ClusterReleaseService {
	ctx, cancel := context.WithCancel(context.Background())
	var releaseUpdater clusterReleaseUpdater
	if updater != nil {
		releaseUpdater = updater
	}
	svc := &ClusterReleaseService{
		repo:         repo,
		cluster:      cluster,
		updater:      releaseUpdater,
		cfg:          cfg,
		ctx:          ctx,
		cancel:       cancel,
		restartAsync: sysutil.RestartServiceAsync,
	}
	if cluster != nil {
		cluster.SetRequestLoadSource(svc)
	}
	svc.readiness.Store(&ClusterReadiness{
		Ready:          !svc.isMultiInstance(),
		Reason:         "initializing_release_state",
		NodeID:         svc.nodeID(),
		NodeName:       svc.nodeName(),
		CurrentVersion: svc.currentVersion(),
	})
	return svc
}

func ProvideClusterReleaseService(
	repo ClusterReleaseRepository,
	cluster *ClusterService,
	updater *UpdateService,
	cfg *config.Config,
) *ClusterReleaseService {
	svc := NewClusterReleaseService(repo, cluster, updater, cfg)
	svc.Start()
	return svc
}

func (s *ClusterReleaseService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		if !s.isMultiInstance() {
			s.storeReadiness(ClusterReadiness{Ready: true})
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.repo.EnsureState(ctx, s.currentVersion()); err != nil {
			logger.LegacyPrintf("service.cluster_release", "[ClusterRelease] initialize state failed: %v", err)
		} else if err := s.refreshReadiness(ctx); err != nil {
			logger.LegacyPrintf("service.cluster_release", "[ClusterRelease] initialize readiness failed: %v", err)
		}
		cancel()

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runLoop()
		}()
	})
}

func (s *ClusterReleaseService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		s.cancel()
		s.wg.Wait()
	})
}

func (s *ClusterReleaseService) isMultiInstance() bool {
	return s != nil && s.cfg != nil && s.cfg.Deployment.IsMultiInstance()
}

func (s *ClusterReleaseService) nodeName() string {
	if s == nil || s.cluster == nil {
		return ""
	}
	return s.cluster.NodeName()
}

func (s *ClusterReleaseService) nodeID() string {
	if s == nil || s.cluster == nil {
		return ""
	}
	return s.cluster.NodeID()
}

func (s *ClusterReleaseService) runnerID() string {
	if s == nil || s.cluster == nil {
		return ""
	}
	return s.cluster.RunnerID()
}

func (s *ClusterReleaseService) currentVersion() string {
	if s == nil || s.cluster == nil {
		return ""
	}
	return normalizeClusterReleaseVersion(s.cluster.Version())
}

func (s *ClusterReleaseService) pollInterval() time.Duration {
	seconds := 5
	if s != nil && s.cfg != nil && s.cfg.Deployment.RolloutPollSeconds > 0 {
		seconds = s.cfg.Deployment.RolloutPollSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *ClusterReleaseService) drainGrace() time.Duration {
	seconds := 10
	if s != nil && s.cfg != nil && s.cfg.Deployment.RolloutDrainGraceSeconds >= 0 {
		seconds = s.cfg.Deployment.RolloutDrainGraceSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *ClusterReleaseService) drainTimeout() time.Duration {
	seconds := 900
	if s != nil && s.cfg != nil && s.cfg.Deployment.RolloutDrainTimeoutSeconds > 0 {
		seconds = s.cfg.Deployment.RolloutDrainTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *ClusterReleaseService) requiredHeartbeats() int {
	required := 2
	if s != nil && s.cfg != nil && s.cfg.Deployment.RolloutVerifyHeartbeats > 0 {
		required = s.cfg.Deployment.RolloutVerifyHeartbeats
	}
	return required
}

func normalizeClusterReleaseVersion(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}

func (s *ClusterReleaseService) GetOverview(ctx context.Context) (*ClusterReleaseOverview, error) {
	if s == nil || s.repo == nil || s.cluster == nil {
		return nil, errors.New("cluster release service is unavailable")
	}
	state, err := s.repo.GetState(ctx)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &ClusterReleaseState{}
	}
	var active *ClusterRollout
	if state.ActiveRolloutID != "" {
		active, err = s.repo.GetRollout(ctx, state.ActiveRolloutID)
		if err != nil {
			return nil, err
		}
	}
	recent, err := s.repo.ListRecentRollouts(ctx, 10)
	if err != nil {
		return nil, err
	}
	clusterStatus, err := s.cluster.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, instance := range clusterStatus.Instances {
		if instance.Status != ClusterInstanceStatusOnline {
			continue
		}
		counts[normalizeClusterReleaseVersion(instance.Version)]++
	}
	versions := make([]ClusterVersionCount, 0, len(counts))
	for version, nodes := range counts {
		versions = append(versions, ClusterVersionCount{Version: version, Nodes: nodes})
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].Version < versions[j].Version })
	consistent := len(versions) <= 1
	if consistent && state.DesiredVersion != "" && len(versions) == 1 {
		consistent = versions[0].Version == normalizeClusterReleaseVersion(state.DesiredVersion)
	}
	return &ClusterReleaseOverview{
		State:          *state,
		ActiveRollout:  active,
		RecentRollouts: recent,
		VersionCounts:  versions,
		Consistent:     consistent,
	}, nil
}

func (s *ClusterReleaseService) CreateRollout(ctx context.Context, input CreateClusterRolloutInput) (*ClusterRollout, error) {
	if !s.isMultiInstance() {
		return nil, ErrClusterRolloutRequiresMultiInstance
	}
	if s.repo == nil || s.cluster == nil || s.updater == nil {
		return nil, errors.New("cluster release service is unavailable")
	}
	targetVersion, err := s.updater.ResolveReleaseVersion(ctx, input.TargetVersion)
	if err != nil {
		return nil, err
	}
	targetVersion = normalizeClusterReleaseVersion(targetVersion)

	clusterStatus, err := s.cluster.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	online, err := snapshotOnlineClusterNodes(clusterStatus.Instances)
	if err != nil {
		return nil, err
	}
	if len(online) == 0 {
		return nil, ErrClusterRolloutNoNodes
	}

	nodeIDs := make([]string, 0, len(online))
	for nodeID := range online {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Slice(nodeIDs, func(i, j int) bool {
		left := online[nodeIDs[i]]
		right := online[nodeIDs[j]]
		if left.NodeName == right.NodeName {
			return nodeIDs[i] < nodeIDs[j]
		}
		return left.NodeName < right.NodeName
	})

	now := time.Now().UTC()
	targets := make([]ClusterRolloutTarget, 0, len(nodeIDs))
	allAtTarget := true
	sourceVersion := ""
	sourceUniform := true
	for ordinal, nodeID := range nodeIDs {
		instance := online[nodeID]
		current := normalizeClusterReleaseVersion(instance.Version)
		if ordinal == 0 {
			sourceVersion = current
		} else if current != sourceVersion {
			sourceUniform = false
		}
		status := ClusterRolloutTargetPending
		var completedAt *time.Time
		if current == targetVersion {
			status = ClusterRolloutTargetSucceeded
			value := now
			completedAt = &value
		} else {
			allAtTarget = false
		}
		targets = append(targets, ClusterRolloutTarget{
			NodeID:         nodeID,
			NodeName:       instance.NodeName,
			Ordinal:        ordinal,
			SourceVersion:  current,
			TargetVersion:  targetVersion,
			Status:         status,
			SourceRunnerID: instance.RunnerID,
			CompletedAt:    completedAt,
		})
	}
	if allAtTarget {
		return nil, ErrClusterRolloutAlreadyAtTarget
	}
	if !sourceUniform {
		sourceVersion = ""
	}

	rollout := ClusterRollout{
		ID:             uuid.NewString(),
		SourceVersion:  sourceVersion,
		TargetVersion:  targetVersion,
		Status:         ClusterRolloutStatusRunning,
		Strategy:       ClusterRolloutStrategyRolling,
		MaxUnavailable: 1,
		CreatedBy:      input.CreatedBy,
		StartedAt:      now,
	}
	for i := range targets {
		targets[i].RolloutID = rollout.ID
	}
	if err := s.repo.CreateRollout(ctx, rollout, targets); err != nil {
		return nil, err
	}
	if err := s.refreshReadiness(ctx); err != nil {
		logger.LegacyPrintf("service.cluster_release", "[ClusterRelease] refresh after create failed rollout=%s err=%v", rollout.ID, err)
	}
	created, err := s.repo.GetRollout(ctx, rollout.ID)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *ClusterReleaseService) GetRollout(ctx context.Context, rolloutID string) (*ClusterRollout, error) {
	rollout, err := s.repo.GetRollout(ctx, strings.TrimSpace(rolloutID))
	if err != nil {
		return nil, err
	}
	if rollout == nil {
		return nil, ErrClusterRolloutNotFound
	}
	return rollout, nil
}

func snapshotOnlineClusterNodes(instances []ClusterInstance) (map[string]ClusterInstance, error) {
	result := make(map[string]ClusterInstance)
	for _, instance := range instances {
		if instance.Status != ClusterInstanceStatusOnline {
			continue
		}
		nodeID := strings.TrimSpace(instance.NodeID)
		if nodeID == "" {
			continue
		}
		previous, exists := result[nodeID]
		if !exists {
			result[nodeID] = instance
			continue
		}
		newer := instance
		older := previous
		if previous.StartedAt.After(instance.StartedAt) {
			newer = previous
			older = instance
		}
		if older.LastSeenAt.After(newer.StartedAt) {
			return nil, ErrClusterRolloutDuplicateNode.WithMetadata(map[string]string{"node_id": nodeID, "node_name": instance.NodeName})
		}
		result[nodeID] = newer
	}
	return result, nil
}

func (s *ClusterReleaseService) PauseRollout(ctx context.Context, rolloutID string) error {
	if err := s.repo.PauseRollout(ctx, rolloutID, "paused by administrator"); err != nil {
		return err
	}
	return s.refreshReadiness(ctx)
}

func (s *ClusterReleaseService) ResumeRollout(ctx context.Context, rolloutID string) error {
	if err := s.repo.ResumeRollout(ctx, rolloutID); err != nil {
		return err
	}
	return s.refreshReadiness(ctx)
}

func (s *ClusterReleaseService) CancelRollout(ctx context.Context, rolloutID string) error {
	if err := s.repo.CancelRollout(ctx, rolloutID); err != nil {
		return err
	}
	return s.refreshReadiness(ctx)
}

func (s *ClusterReleaseService) RetryTarget(ctx context.Context, rolloutID, nodeID string) error {
	if err := s.repo.RetryTarget(ctx, rolloutID, nodeID); err != nil {
		return err
	}
	return s.refreshReadiness(ctx)
}

func (s *ClusterReleaseService) GetReadiness() ClusterReadiness {
	if s == nil {
		return ClusterReadiness{Ready: false, Reason: "release_service_unavailable"}
	}
	value := ClusterReadiness{
		Ready:          !s.isMultiInstance(),
		NodeID:         s.nodeID(),
		NodeName:       s.nodeName(),
		CurrentVersion: s.currentVersion(),
	}
	if current := s.readiness.Load(); current != nil {
		value = *current
	}
	value.Draining = s.draining.Load()
	value.InFlight = s.inFlight.Load()
	return value
}

func (s *ClusterReleaseService) TryBeginRequest() bool {
	if s == nil || !s.isMultiInstance() {
		return true
	}
	if !s.GetReadiness().Ready || s.draining.Load() {
		return false
	}
	s.inFlight.Add(1)
	if !s.GetReadiness().Ready || s.draining.Load() {
		s.inFlight.Add(-1)
		return false
	}
	return true
}

func (s *ClusterReleaseService) EndRequest() {
	if s == nil || !s.isMultiInstance() {
		return
	}
	s.inFlight.Add(-1)
}

func (s *ClusterReleaseService) InFlightRequests() int64 {
	if s == nil {
		return 0
	}
	value := s.inFlight.Load()
	if value < 0 {
		return 0
	}
	return value
}

func (s *ClusterReleaseService) runLoop() {
	ticker := time.NewTicker(s.pollInterval())
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
			if err := s.processOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.LegacyPrintf("service.cluster_release", "[ClusterRelease] poll failed node=%s err=%v", s.nodeName(), err)
			}
			cancel()
		}
	}
}

func (s *ClusterReleaseService) processOnce(ctx context.Context) error {
	if _, err := s.repo.FailExpiredTargets(ctx, time.Now().UTC()); err != nil {
		return fmt.Errorf("expire rollout targets: %w", err)
	}
	if err := s.refreshReadiness(ctx); err != nil {
		return err
	}
	rollout, err := s.repo.GetActiveRollout(ctx)
	if err != nil || rollout == nil {
		return err
	}
	target, err := s.repo.GetTargetForNode(ctx, rollout.ID, s.nodeID())
	if err != nil || target == nil {
		return err
	}
	currentVersion := s.currentVersion()
	if currentVersion == normalizeClusterReleaseVersion(target.TargetVersion) {
		return s.observeUpdatedNode(ctx, rollout, target)
	}
	if rollout.Status != ClusterRolloutStatusRunning || target.Status != ClusterRolloutTargetPending {
		return nil
	}
	leaseUntil := time.Now().UTC().Add(s.drainTimeout() + time.Minute)
	claimed, err := s.repo.ClaimTarget(ctx, rollout.ID, target.NodeID, s.runnerID(), leaseUntil)
	if err != nil || !claimed {
		return err
	}
	return s.executeBinaryTarget(rollout.ID, target.NodeID, target.NodeName, target.TargetVersion)
}

func (s *ClusterReleaseService) observeUpdatedNode(ctx context.Context, rollout *ClusterRollout, target *ClusterRolloutTarget) error {
	if target.Status == ClusterRolloutTargetSucceeded {
		return nil
	}
	instance, err := s.repo.GetRunnerInstance(ctx, s.runnerID())
	if err != nil || instance == nil {
		return err
	}
	if !instance.DatabaseOK || !instance.RedisOK || instance.StoppedAt != nil {
		return nil
	}
	leaseUntil := time.Now().UTC().Add(clusterRolloutRestartTimeout)
	_, completed, err := s.repo.ObserveTargetHeartbeat(
		ctx,
		rollout.ID,
		target.NodeID,
		s.runnerID(),
		s.currentVersion(),
		instance.LastSeenAt,
		s.requiredHeartbeats(),
		leaseUntil,
	)
	if err != nil {
		if infraerrors.Code(err) == infraerrors.Code(ErrClusterRolloutInvalidState) {
			return nil
		}
		return err
	}
	if completed {
		logger.LegacyPrintf("service.cluster_release", "[ClusterRelease] rollout completed rollout=%s target=%s", rollout.ID, target.TargetVersion)
	}
	return s.refreshReadiness(ctx)
}

func (s *ClusterReleaseService) executeBinaryTarget(rolloutID, nodeID, nodeName, targetVersion string) error {
	s.draining.Store(true)
	s.storeReadiness(ClusterReadiness{
		Ready:          false,
		Reason:         "rollout_draining",
		RolloutID:      rolloutID,
		TargetStatus:   ClusterRolloutTargetDraining,
		DesiredVersion: normalizeClusterReleaseVersion(targetVersion),
	})
	fail := func(reason string, err error) error {
		message := reason
		if err != nil {
			message = reason + ": " + err.Error()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.repo.SetTargetStatus(ctx, rolloutID, nodeID, s.runnerID(), ClusterRolloutTargetFailed, message, nil)
		s.draining.Store(false)
		_ = s.refreshReadiness(ctx)
		return errors.New(message)
	}

	if grace := s.drainGrace(); grace > 0 {
		timer := time.NewTimer(grace)
		select {
		case <-s.ctx.Done():
			timer.Stop()
			return fail("rollout stopped while draining", s.ctx.Err())
		case <-timer.C:
		}
	}
	deadline := time.Now().Add(s.drainTimeout())
	for s.inFlight.Load() > 0 {
		if time.Now().After(deadline) {
			return fail("timed out waiting for in-flight requests", nil)
		}
		select {
		case <-s.ctx.Done():
			return fail("rollout stopped while draining", s.ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}

	installLease := time.Now().UTC().Add(clusterRolloutInstallTimeout + time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := s.repo.SetTargetStatus(ctx, rolloutID, nodeID, s.runnerID(), ClusterRolloutTargetInstalling, "", &installLease)
	cancel()
	if err != nil {
		return fail("failed to mark target installing", err)
	}

	installCtx, installCancel := context.WithTimeout(s.ctx, clusterRolloutInstallTimeout)
	err = s.updater.InstallVersion(installCtx, targetVersion)
	installCancel()
	if err != nil {
		return fail("failed to install target version", err)
	}

	restartLease := time.Now().UTC().Add(clusterRolloutRestartTimeout)
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	err = s.repo.SetTargetStatus(ctx, rolloutID, nodeID, s.runnerID(), ClusterRolloutTargetRestarting, "", &restartLease)
	cancel()
	if err != nil {
		return fail("failed to mark target restarting", err)
	}

	logger.LegacyPrintf("service.cluster_release", "[ClusterRelease] installed target; restarting node=%s rollout=%s target=%s", nodeName, rolloutID, targetVersion)
	go func() {
		time.Sleep(500 * time.Millisecond)
		s.restartAsync()
	}()
	return nil
}

func (s *ClusterReleaseService) refreshReadiness(ctx context.Context) error {
	readiness := ClusterReadiness{
		Ready:          !s.isMultiInstance(),
		NodeID:         s.nodeID(),
		NodeName:       s.nodeName(),
		CurrentVersion: s.currentVersion(),
		Draining:       s.draining.Load(),
	}
	if !s.isMultiInstance() {
		s.storeReadiness(readiness)
		return nil
	}
	if s.draining.Load() {
		readiness.Reason = "rollout_draining"
		s.storeReadiness(readiness)
		return nil
	}
	state, err := s.repo.GetState(ctx)
	if err != nil {
		readiness.Reason = "release_state_unavailable"
		s.storeReadiness(readiness)
		return err
	}
	if state == nil {
		readiness.Reason = "release_state_missing"
		s.storeReadiness(readiness)
		return nil
	}
	readiness.DesiredVersion = normalizeClusterReleaseVersion(state.DesiredVersion)
	if state.ActiveRolloutID == "" {
		readiness.Ready = readiness.DesiredVersion == "" || readiness.CurrentVersion == readiness.DesiredVersion
		if !readiness.Ready {
			readiness.Reason = "version_mismatch"
		}
		s.storeReadiness(readiness)
		return nil
	}

	readiness.RolloutID = state.ActiveRolloutID
	target, err := s.repo.GetTargetForNode(ctx, state.ActiveRolloutID, s.nodeID())
	if err != nil {
		readiness.Reason = "rollout_target_unavailable"
		s.storeReadiness(readiness)
		return err
	}
	if target == nil {
		readiness.Reason = "node_not_enrolled_in_rollout"
		s.storeReadiness(readiness)
		return nil
	}
	readiness.TargetStatus = target.Status
	sourceVersion := normalizeClusterReleaseVersion(target.SourceVersion)
	targetVersion := normalizeClusterReleaseVersion(target.TargetVersion)
	switch target.Status {
	case ClusterRolloutTargetPending:
		readiness.Ready = readiness.CurrentVersion == sourceVersion
		if !readiness.Ready {
			readiness.Reason = "awaiting_target_verification"
		}
	case ClusterRolloutTargetSucceeded:
		readiness.Ready = readiness.CurrentVersion == targetVersion
		if !readiness.Ready {
			readiness.Reason = "verified_version_mismatch"
		}
	case ClusterRolloutTargetFailed:
		readiness.Ready = readiness.CurrentVersion == sourceVersion
		if !readiness.Ready {
			readiness.Reason = "failed_target_version_mismatch"
		}
	default:
		readiness.Reason = "rollout_" + target.Status
	}
	s.storeReadiness(readiness)
	return nil
}

func (s *ClusterReleaseService) storeReadiness(value ClusterReadiness) {
	value.NodeName = s.nodeName()
	value.NodeID = s.nodeID()
	value.CurrentVersion = s.currentVersion()
	value.Draining = s.draining.Load()
	value.InFlight = s.inFlight.Load()
	s.readiness.Store(&value)
}
