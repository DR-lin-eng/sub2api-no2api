package egress

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
)

type Service struct {
	store       Store
	allocator   *Allocator
	cfg         *config.Config
	settings    RuntimeSettings
	probe       func(context.Context, platformegress.Route, platformegress.Policy, string, time.Duration) (*platformegress.ProbeResult, error)
	detect      func() (*platformegress.DetectedIPv6Network, error)
	probeSource func(context.Context, string, platformegress.Policy, string, time.Duration) (*platformegress.ProbeResult, error)

	workerMu     sync.Mutex
	workerCancel context.CancelFunc
	workerDone   chan struct{}
	healthMu     sync.RWMutex
	poolHealth   map[int64]poolHealth
	runtimeMu    sync.Mutex
	autoMu       sync.Mutex
	enabled      atomic.Bool
	stateLoaded  atomic.Bool
}

type poolHealth struct {
	healthy   bool
	checkedAt time.Time
	err       string
}

func NewService(store Store, cfg *config.Config) *Service {
	secret := ""
	if cfg != nil {
		secret = cfg.IPv6Egress.AllocationSecret
	}
	svc := &Service{
		store:       store,
		allocator:   NewAllocator(secret),
		cfg:         cfg,
		probe:       platformegress.Probe,
		detect:      platformegress.DetectIPv6Network,
		probeSource: platformegress.ProbeSource,
		poolHealth:  make(map[int64]poolHealth),
	}
	svc.enabled.Store(cfg != nil && cfg.IPv6Egress.Enabled)
	return svc
}

// SetRuntimeSettings attaches the persisted control-plane settings store. It
// is kept as a setter so small unit-test services can continue using the
// original NewService constructor without a database dependency.
func (s *Service) SetRuntimeSettings(settings RuntimeSettings) {
	if s == nil {
		return
	}
	s.runtimeMu.Lock()
	s.settings = settings
	s.stateLoaded.Store(false)
	s.runtimeMu.Unlock()
}

func (s *Service) IsEnabled() bool {
	if s == nil {
		return false
	}
	return s.enabled.Load()
}

func (s *Service) SecretConfigured() bool {
	if s == nil {
		return false
	}
	if s.allocator != nil && s.allocator.SecretConfigured() {
		return true
	}
	return s.cfg != nil && len(strings.TrimSpace(s.cfg.IPv6Egress.AllocationSecret)) >= 32
}

const defaultReconcileBatchSize = 1000

// Start checks the local network namespace and, on worker-enabled nodes,
// periodically fills bindings for inherited accounts. Missing bindings fail
// closed on request paths, so this worker never needs a hot-path lookup.
func (s *Service) Start() {
	if s == nil || s.cfg == nil {
		return
	}
	s.loadRuntimeState(context.Background())
	if !s.IsEnabled() {
		return
	}
	if err := s.ensureAllocationSecret(context.Background()); err != nil {
		slog.Warn("IPv6 egress allocation secret is unavailable", "error", err)
		s.enabled.Store(false)
		platformegress.SetRuntimeEnabled(false)
		return
	}
	s.startWorker()
}

func (s *Service) startWorker() {
	if s == nil || s.cfg == nil || !s.IsEnabled() {
		return
	}
	s.workerMu.Lock()
	defer s.workerMu.Unlock()
	if s.workerCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.workerCancel = cancel
	s.workerDone = done
	go s.runReconciler(ctx, done)
}

func (s *Service) loadRuntimeState(ctx context.Context) {
	if s == nil || s.stateLoaded.Load() {
		return
	}
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if s.stateLoaded.Load() {
		return
	}
	enabled := s.cfg != nil && s.cfg.IPv6Egress.Enabled
	if s.settings != nil {
		if raw, err := s.settings.GetValue(ctx, RuntimeSettingKey); err == nil {
			if parsed, parseErr := strconv.ParseBool(strings.TrimSpace(raw)); parseErr == nil {
				enabled = parsed
			}
		} else if s.cfg != nil && s.cfg.IPv6Egress.Enabled {
			// Keep an explicitly enabled legacy YAML deployment running until
			// an administrator chooses the new database switch.
			enabled = true
		}
	}
	if enabled && s.settings != nil && (runtime.GOOS != "linux" || (s.cfg != nil && s.cfg.Deployment.IsMultiInstance())) {
		enabled = false
	}
	s.enabled.Store(enabled)
	s.stateLoaded.Store(true)
	platformegress.SetRuntimeEnabled(enabled)
}

// SetEnabled persists and applies the administrator switch immediately. It
// deliberately does not create a pool; AutoConfigure performs the network
// probe and pool creation as one explicit action.
func (s *Service) SetEnabled(ctx context.Context, enabled bool) error {
	if s == nil || s.cfg == nil {
		return fmt.Errorf("%w: configuration is unavailable", ErrRuntimeUnavailable)
	}
	s.runtimeMu.Lock()
	err := func() error {
		if enabled {
			if runtime.GOOS != "linux" {
				return fmt.Errorf("%w: Linux is required", ErrRuntimeUnavailable)
			}
			if s.cfg.Deployment.IsMultiInstance() {
				return fmt.Errorf("%w: standalone deployment is required", ErrRuntimeUnavailable)
			}
			if err := s.ensureAllocationSecretLocked(ctx); err != nil {
				return err
			}
		}
		if s.settings != nil {
			if err := s.settings.Set(ctx, RuntimeSettingKey, strconv.FormatBool(enabled)); err != nil {
				return fmt.Errorf("persist IPv6 egress switch: %w", err)
			}
		}
		s.enabled.Store(enabled)
		s.stateLoaded.Store(true)
		platformegress.SetRuntimeEnabled(enabled)
		return nil
	}()
	s.runtimeMu.Unlock()
	if err != nil {
		return err
	}
	if enabled {
		s.startWorker()
	} else {
		s.Stop()
	}
	return nil
}

func (s *Service) Stop() {
	if s == nil {
		return
	}
	s.workerMu.Lock()
	cancel := s.workerCancel
	done := s.workerDone
	s.workerCancel = nil
	s.workerDone = nil
	s.workerMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	if done != nil {
		<-done
	}
}

func (s *Service) runReconciler(ctx context.Context, done chan struct{}) {
	defer close(done)
	interval := 60 * time.Second
	if seconds := s.cfg.IPv6Egress.ReconcileIntervalSeconds; seconds > 0 {
		interval = time.Duration(seconds) * time.Second
	}
	reconcile := func() {
		if s.cfg.Deployment.WorkerEnabledResolved() {
			allocated, err := s.reconcileAllDefault(ctx, defaultReconcileBatchSize)
			if err != nil && !errors.Is(err, ErrPoolNotFound) && !errors.Is(err, context.Canceled) {
				slog.Warn("IPv6 egress binding reconciliation failed", "error", err)
			}
			if allocated > 0 {
				slog.Info("IPv6 egress bindings reconciled", "allocated", allocated)
			}
		}
		if err := s.preflightDefault(ctx); err != nil && !errors.Is(err, ErrPoolNotFound) && !errors.Is(err, ErrBindingNotFound) && !errors.Is(err, context.Canceled) {
			slog.Warn("IPv6 egress startup preflight failed", "error", err)
		}
	}
	reconcile()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

func (s *Service) reconcileAllDefault(ctx context.Context, limit int) (int, error) {
	total := 0
	for {
		completed, err := s.ReconcileDefault(ctx, limit)
		total += completed
		if err != nil || completed < limit {
			return total, err
		}
	}
}

func (s *Service) CreatePool(ctx context.Context, input CreatePoolInput) (*Pool, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return nil, fmt.Errorf("IPv6 egress pool name is required")
	}
	prefix, err := ValidatePoolCIDR(input.CIDR)
	if err != nil {
		return nil, err
	}
	input.CIDR = prefix.String()
	if input.IsDefault {
		if err := s.RuntimeReady(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: create the pool, bind and probe an account, then mark it as default", ErrPoolUnhealthy)
	}
	pool, err := s.store.CreatePool(ctx, input)
	return s.decoratePool(pool), err
}

func (s *Service) UpdatePool(ctx context.Context, id int64, input UpdatePoolInput) (*Pool, error) {
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("IPv6 egress pool name is required")
		}
		input.Name = &name
	}
	if input.Status != nil && *input.Status != PoolStatusActive && *input.Status != PoolStatusDisabled {
		return nil, fmt.Errorf("invalid IPv6 egress pool status %q", *input.Status)
	}
	if input.IsDefault != nil && *input.IsDefault {
		if err := s.RuntimeReady(); err != nil {
			return nil, err
		}
		if !s.poolIsHealthy(id) {
			return nil, ErrPoolUnhealthy
		}
	}
	pool, err := s.store.UpdatePool(ctx, id, input)
	if err == nil && input.Status != nil && *input.Status == PoolStatusDisabled {
		s.clearPoolHealth(id)
	}
	return s.decoratePool(pool), err
}

func (s *Service) RuntimeReady() error {
	if s == nil || s.cfg == nil || !s.IsEnabled() {
		return fmt.Errorf("%w: feature is disabled", ErrRuntimeUnavailable)
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("%w: Linux is required", ErrRuntimeUnavailable)
	}
	if s.cfg.Deployment.IsMultiInstance() {
		return fmt.Errorf("%w: standalone deployment is required", ErrRuntimeUnavailable)
	}
	if !s.SecretConfigured() {
		return fmt.Errorf("%w: allocation secret is not configured", ErrRuntimeUnavailable)
	}
	return nil
}

func (s *Service) ensureAllocationSecret(ctx context.Context) error {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	return s.ensureAllocationSecretLocked(ctx)
}

func (s *Service) ensureAllocationSecretLocked(ctx context.Context) error {
	if s == nil || s.allocator == nil {
		return ErrAllocationDisabled
	}
	if s.allocator.SecretConfigured() {
		return nil
	}
	if s.cfg != nil {
		if configured := strings.TrimSpace(s.cfg.IPv6Egress.AllocationSecret); len(configured) >= 32 {
			s.allocator.SetSecret(configured)
			return nil
		}
	}
	if s.settings == nil {
		return ErrAllocationDisabled
	}
	if raw, err := s.settings.GetValue(ctx, allocationSecretSettingKey); err == nil && len(strings.TrimSpace(raw)) >= 32 {
		s.allocator.SetSecret(strings.TrimSpace(raw))
		return nil
	}
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Errorf("generate IPv6 allocation secret: %w", err)
	}
	secret := hex.EncodeToString(bytes[:])
	if err := s.settings.Set(ctx, allocationSecretSettingKey, secret); err != nil {
		return fmt.Errorf("persist IPv6 allocation secret: %w", err)
	}
	s.allocator.SetSecret(secret)
	return nil
}

func (s *Service) DeletePool(ctx context.Context, id int64) error {
	if err := s.store.DeletePool(ctx, id); err != nil {
		return err
	}
	s.clearPoolHealth(id)
	return nil
}

func (s *Service) ListPools(ctx context.Context) ([]Pool, error) {
	pools, err := s.store.ListPools(ctx)
	if err != nil {
		return nil, err
	}
	for i := range pools {
		s.decoratePool(&pools[i])
	}
	return pools, nil
}

func (s *Service) ListBindings(ctx context.Context, offset, limit int, search string) (*BindingPage, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.store.ListBindings(ctx, offset, limit, strings.TrimSpace(search))
}

func (s *Service) ProbeAccount(ctx context.Context, accountID int64) (*platformegress.ProbeResult, error) {
	if s == nil || s.cfg == nil || !s.IsEnabled() {
		return nil, platformegress.ErrIPv6Disabled
	}
	binding, err := s.store.GetBinding(ctx, accountID)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(s.cfg.IPv6Egress.ProbeTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	probe := s.probe
	if probe == nil {
		probe = platformegress.Probe
	}
	result, probeErr := probe(ctx, binding.Route(false), platformegress.Policy{
		IPv6Enabled: true,
		FreeBind:    s.cfg.IPv6Egress.FreeBind,
	}, s.cfg.IPv6Egress.ProbeURL, timeout)
	s.recordPoolHealth(binding.PoolID, probeErr)
	return result, probeErr
}

// AutoConfigure detects the public IPv6 network visible from the application
// namespace, verifies the current source address through the configured IPv6
// probe, and creates or reuses a healthy default /64 pool. It is intentionally
// idempotent so pressing the button again does not create duplicate pools.
func (s *Service) AutoConfigure(ctx context.Context) (*AutoConfigureResult, error) {
	if s == nil || s.cfg == nil {
		return nil, fmt.Errorf("%w: configuration is unavailable", ErrAutoConfigure)
	}
	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	wasEnabled := s.IsEnabled()
	if !wasEnabled {
		if err := s.SetEnabled(ctx, true); err != nil {
			return nil, fmt.Errorf("%w: enable runtime: %v", ErrAutoConfigure, err)
		}
		// Configure the pool before the periodic worker starts probing an old
		// or missing default. The worker is started again after the pool is
		// healthy and selected below.
		s.Stop()
	} else if s.settings != nil {
		s.runtimeMu.Lock()
		err := s.settings.Set(ctx, RuntimeSettingKey, "true")
		s.runtimeMu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("%w: persist runtime switch: %v", ErrAutoConfigure, err)
		}
	}
	rollback := func() {
		if !wasEnabled {
			if err := s.SetEnabled(context.Background(), false); err != nil {
				slog.Warn("failed to roll back IPv6 egress auto-configuration", "error", err)
			}
		}
	}
	detect := s.detect
	if detect == nil {
		detect = platformegress.DetectIPv6Network
	}
	detected, err := detect()
	if err != nil {
		rollback()
		return nil, fmt.Errorf("%w: %v", ErrAutoConfigure, err)
	}
	if err := s.ensureAllocationSecret(ctx); err != nil {
		rollback()
		return nil, fmt.Errorf("%w: %v", ErrAutoConfigure, err)
	}
	timeout := time.Duration(s.cfg.IPv6Egress.ProbeTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	target := strings.TrimSpace(s.cfg.IPv6Egress.ProbeURL)
	if target == "" {
		target = "https://api64.ipify.org"
	}
	parsedTarget, err := url.Parse(target)
	if err != nil || !strings.EqualFold(parsedTarget.Scheme, "https") || strings.TrimSpace(parsedTarget.Hostname()) == "" {
		rollback()
		return nil, fmt.Errorf("%w: probe URL must be an absolute HTTPS URL", ErrAutoConfigure)
	}
	probeSource := s.probeSource
	if probeSource == nil {
		probeSource = platformegress.ProbeSource
	}
	probe, err := probeSource(ctx, detected.Address.String(), platformegress.Policy{
		IPv6Enabled: true,
		FreeBind:    s.cfg.IPv6Egress.FreeBind,
	}, target, timeout)
	if err != nil {
		rollback()
		return nil, fmt.Errorf("%w: IPv6 source probe: %v", ErrAutoConfigure, err)
	}

	prefix := detected.Prefix.String()
	pools, err := s.store.ListPools(ctx)
	if err != nil {
		rollback()
		return nil, fmt.Errorf("%w: list pools: %v", ErrAutoConfigure, err)
	}
	var pool *Pool
	created := false
	for i := range pools {
		if strings.TrimSpace(pools[i].CIDR) == prefix {
			pool = &pools[i]
			break
		}
	}
	if pool == nil {
		pool, err = s.store.CreatePool(ctx, CreatePoolInput{
			Name:      "auto-" + strings.TrimSpace(detected.Interface),
			CIDR:      prefix,
			NodeID:    nil,
			IsDefault: false,
		})
		if err != nil {
			rollback()
			return nil, fmt.Errorf("%w: create detected pool: %v", ErrAutoConfigure, err)
		}
		created = true
	}
	if pool.Status == PoolStatusDisabled {
		active := PoolStatusActive
		pool, err = s.store.UpdatePool(ctx, pool.ID, UpdatePoolInput{Status: &active})
		if err != nil {
			rollback()
			return nil, fmt.Errorf("%w: activate detected pool: %v", ErrAutoConfigure, err)
		}
	}
	s.recordPoolHealth(pool.ID, nil)
	if !pool.IsDefault {
		makeDefault := true
		pool, err = s.UpdatePool(ctx, pool.ID, UpdatePoolInput{IsDefault: &makeDefault})
		if err != nil {
			rollback()
			return nil, fmt.Errorf("%w: select detected pool: %v", ErrAutoConfigure, err)
		}
	}
	if !wasEnabled {
		s.startWorker()
	}
	return &AutoConfigureResult{
		Enabled:  s.IsEnabled(),
		Created:  created,
		Detected: *detected,
		Pool:     s.decoratePool(pool),
		Probe:    probe,
	}, nil
}

func (s *Service) preflightDefault(ctx context.Context) error {
	if err := s.RuntimeReady(); err != nil {
		return err
	}
	pool, err := s.store.GetDefaultPool(ctx)
	if err != nil {
		return err
	}
	binding, err := s.store.GetAnyBindingForPool(ctx, pool.ID)
	if err != nil {
		return err
	}
	_, err = s.ProbeAccount(ctx, binding.AccountID)
	return err
}

func (s *Service) recordPoolHealth(poolID int64, probeErr error) {
	if s == nil || poolID <= 0 {
		return
	}
	health := poolHealth{healthy: probeErr == nil, checkedAt: time.Now().UTC()}
	if probeErr != nil {
		health.err = probeErr.Error()
	}
	s.healthMu.Lock()
	s.poolHealth[poolID] = health
	s.healthMu.Unlock()
}

func (s *Service) clearPoolHealth(poolID int64) {
	if s == nil || poolID <= 0 {
		return
	}
	s.healthMu.Lock()
	delete(s.poolHealth, poolID)
	s.healthMu.Unlock()
}

func (s *Service) poolIsHealthy(poolID int64) bool {
	if s == nil || poolID <= 0 {
		return false
	}
	s.healthMu.RLock()
	health, ok := s.poolHealth[poolID]
	s.healthMu.RUnlock()
	return ok && health.healthy
}

func (s *Service) decoratePool(pool *Pool) *Pool {
	if s == nil || pool == nil {
		return pool
	}
	s.healthMu.RLock()
	health, ok := s.poolHealth[pool.ID]
	s.healthMu.RUnlock()
	if !ok {
		return pool
	}
	healthValue := health.healthy
	checkedAt := health.checkedAt
	pool.RouteHealthy = &healthValue
	pool.LastProbeAt = &checkedAt
	pool.ProbeError = health.err
	return pool
}

func (s *Service) SetAccountRoute(ctx context.Context, accountID int64, input SetAccountRouteInput) (*Binding, error) {
	if accountID <= 0 {
		return nil, fmt.Errorf("invalid account id")
	}
	switch input.Mode {
	case platformegress.ModeDirect, platformegress.ModeExternalProxy:
		if err := s.store.SetAccountMode(ctx, accountID, input.Mode); err != nil {
			return nil, err
		}
		if err := s.store.DeleteBinding(ctx, accountID); err != nil && !errors.Is(err, ErrBindingNotFound) {
			return nil, err
		}
		return nil, nil
	case platformegress.ModeInherit:
		pool, err := s.store.GetDefaultPool(ctx)
		if err != nil {
			if !errors.Is(err, ErrPoolNotFound) {
				return nil, err
			}
			if modeErr := s.store.SetAccountMode(ctx, accountID, input.Mode); modeErr != nil {
				return nil, modeErr
			}
			if deleteErr := s.store.DeleteBinding(ctx, accountID); deleteErr != nil && !errors.Is(deleteErr, ErrBindingNotFound) {
				return nil, deleteErr
			}
			return nil, nil
		}
		binding, err := s.ensureBinding(ctx, accountID, *pool, false)
		if err != nil {
			return nil, err
		}
		if err := s.store.SetAccountMode(ctx, accountID, input.Mode); err != nil {
			return nil, err
		}
		return binding, nil
	case platformegress.ModeIPv6Pool:
		if err := s.RuntimeReady(); err != nil {
			return nil, err
		}
		var pool *Pool
		var err error
		if input.PoolID != nil && *input.PoolID > 0 {
			pool, err = s.store.GetPool(ctx, *input.PoolID)
		} else {
			pool, err = s.store.GetDefaultPool(ctx)
		}
		if err != nil {
			return nil, err
		}
		binding, err := s.ensureBinding(ctx, accountID, *pool, false)
		if err != nil {
			return nil, err
		}
		if err := s.store.SetAccountMode(ctx, accountID, input.Mode); err != nil {
			return nil, err
		}
		return binding, nil
	default:
		return nil, fmt.Errorf("invalid account egress mode %q", input.Mode)
	}
}

func (s *Service) RotateBinding(ctx context.Context, accountID int64) (*Binding, error) {
	current, err := s.store.GetBinding(ctx, accountID)
	if err != nil {
		return nil, err
	}
	pool, err := s.store.GetPool(ctx, current.PoolID)
	if err != nil {
		return nil, err
	}
	return s.ensureBinding(ctx, accountID, *pool, true)
}

func (s *Service) ReconcileDefault(ctx context.Context, limit int) (int, error) {
	pool, err := s.store.GetDefaultPool(ctx)
	if err != nil {
		return 0, err
	}
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	ids, err := s.store.ListInheritedAccountIDsWithoutBinding(ctx, limit)
	if err != nil {
		return 0, err
	}
	completed := 0
	for _, accountID := range ids {
		if _, err := s.ensureBinding(ctx, accountID, *pool, false); err != nil {
			return completed, fmt.Errorf("allocate account %d: %w", accountID, err)
		}
		completed++
	}
	return completed, nil
}

func (s *Service) ensureBinding(ctx context.Context, accountID int64, pool Pool, rotate bool) (*Binding, error) {
	if pool.Status != PoolStatusActive {
		return nil, ErrPoolDisabled
	}
	current, err := s.store.GetBinding(ctx, accountID)
	if err != nil && !errors.Is(err, ErrBindingNotFound) {
		return nil, err
	}
	if current != nil && current.PoolID == pool.ID && !rotate {
		return current, nil
	}

	version := int64(1)
	var expectedVersion *int64
	if current != nil {
		version = current.Version + 1
		expected := current.Version
		expectedVersion = &expected
	}
	for attempt := 0; attempt < maxAllocationAttempts; attempt++ {
		address, allocErr := s.allocator.Address(pool, accountID, version, attempt)
		if allocErr != nil {
			return nil, allocErr
		}
		binding, saveErr := s.store.UpsertBinding(ctx, Binding{
			AccountID:  accountID,
			PoolID:     pool.ID,
			PoolName:   pool.Name,
			PoolStatus: pool.Status,
			SourceIPv6: address,
			Status:     BindingStatusActive,
			Version:    version,
		}, expectedVersion)
		if saveErr == nil {
			return binding, nil
		}
		if errors.Is(saveErr, ErrAddressConflict) {
			continue
		}
		return nil, saveErr
	}
	return nil, fmt.Errorf("IPv6 egress pool allocation exhausted after %d collision attempts", maxAllocationAttempts)
}
