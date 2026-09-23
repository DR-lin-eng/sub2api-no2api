package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	proxyHealthCycleInterval = time.Minute
	proxyHealthBatchLimit    = 100
	proxyHealthConcurrency   = 8
	proxyHealthProbeTimeout  = 12 * time.Second
	proxyHealthLeaderLockKey = "proxy:health:leader"
	proxyHealthLeaderLockTTL = 5 * time.Minute
)

type ProxyHealthRepository interface {
	ListDueProxyHealthChecks(ctx context.Context, cutoff, now time.Time, limit int) ([]Proxy, error)
	RecordProxyHealthCheck(ctx context.Context, proxyID int64, checkedAt time.Time, success bool, message string, failureThreshold int) (deactivated bool, err error)
	RebalanceActiveProxyAccounts(ctx context.Context, now time.Time) (int64, error)
}

type proxyHealthProbeResult struct {
	proxyID   int64
	checkedAt time.Time
	success   bool
	message   string
}

// ProxyHealthService periodically probes active proxies and removes a proxy
// from the automatic pool after the configured consecutive-failure threshold.
type ProxyHealthService struct {
	repo           ProxyHealthRepository
	settingService *SettingService
	prober         ProxyExitInfoProber
	lockCache      LeaderLockCache
	db             *sql.DB
	instanceID     string
	now            func() time.Time

	parentCtx    context.Context
	parentCancel context.CancelFunc
	cycleMu      sync.Mutex
	mu           sync.Mutex
	started      bool
	stopped      bool
	wg           sync.WaitGroup
}

func NewProxyHealthService(repo ProxyHealthRepository, settingService *SettingService, prober ProxyExitInfoProber) *ProxyHealthService {
	ctx, cancel := context.WithCancel(context.Background())
	return &ProxyHealthService{
		repo:           repo,
		settingService: settingService,
		prober:         prober,
		instanceID:     uuid.NewString(),
		now:            time.Now,
		parentCtx:      ctx,
		parentCancel:   cancel,
	}
}

func (s *ProxyHealthService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

func (s *ProxyHealthService) Start() {
	if s == nil || s.repo == nil || s.settingService == nil || s.prober == nil {
		return
	}
	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go s.runLoop()
}

func (s *ProxyHealthService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.parentCancel()
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *ProxyHealthService) runLoop() {
	defer s.wg.Done()
	if err := s.RunDue(s.parentCtx); err != nil {
		log.Printf("[ProxyHealth] initial health check failed: %v", err)
	}
	ticker := time.NewTicker(proxyHealthCycleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.parentCtx.Done():
			return
		case <-ticker.C:
			if err := s.RunDue(s.parentCtx); err != nil {
				log.Printf("[ProxyHealth] scheduled health check failed: %v", err)
			}
		}
	}
}

func (s *ProxyHealthService) RunDue(ctx context.Context) error {
	if s == nil || s.repo == nil || s.settingService == nil || s.prober == nil {
		return nil
	}
	s.cycleMu.Lock()
	defer s.cycleMu.Unlock()

	settings, err := s.settingService.GetProxyAutoAssignmentSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled || !settings.HealthCheckEnabled {
		return nil
	}
	release, acquired := tryAcquireSingletonLeaderLock(
		ctx,
		s.lockCache,
		s.db,
		proxyHealthLeaderLockKey,
		s.instanceID,
		proxyHealthLeaderLockTTL,
	)
	if !acquired {
		return nil
	}
	defer release()

	now := s.now().UTC()
	cutoff := now.Add(-time.Duration(settings.HealthCheckIntervalMinutes) * time.Minute)
	proxies, err := s.repo.ListDueProxyHealthChecks(ctx, cutoff, now, proxyHealthBatchLimit)
	if err != nil {
		return fmt.Errorf("list due proxy health checks: %w", err)
	}
	results := make([]proxyHealthProbeResult, len(proxies))
	semaphore := make(chan struct{}, proxyHealthConcurrency)
	var wg sync.WaitGroup
	for i := range proxies {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				results[i] = proxyHealthProbeResult{proxyID: proxies[i].ID, checkedAt: s.now().UTC(), message: "probe_canceled"}
				return
			}
			probeCtx, cancel := context.WithTimeout(ctx, proxyHealthProbeTimeout)
			defer cancel()
			_, _, probeErr := s.prober.ProbeProxy(probeCtx, proxies[i].URL())
			result := proxyHealthProbeResult{
				proxyID:   proxies[i].ID,
				checkedAt: s.now().UTC(),
				success:   probeErr == nil,
			}
			if probeErr != nil {
				result.message = "probe_failed"
			}
			results[i] = result
		}()
	}
	wg.Wait()

	deactivated := 0
	var applyErrors []error
	for i := range results {
		becameInactive, recordErr := s.repo.RecordProxyHealthCheck(
			ctx,
			results[i].proxyID,
			results[i].checkedAt,
			results[i].success,
			results[i].message,
			settings.FailureThreshold,
		)
		if recordErr != nil {
			applyErrors = append(applyErrors, fmt.Errorf("record proxy %d health: %w", results[i].proxyID, recordErr))
			continue
		}
		if becameInactive {
			deactivated++
		}
	}
	if deactivated > 0 {
		changed, rebalanceErr := s.repo.RebalanceActiveProxyAccounts(ctx, s.now().UTC())
		if rebalanceErr != nil {
			applyErrors = append(applyErrors, fmt.Errorf("rebalance after %d unhealthy proxies: %w", deactivated, rebalanceErr))
		} else {
			log.Printf("[ProxyHealth] deactivated %d unhealthy proxies and reassigned %d accounts", deactivated, changed)
		}
	}
	return errors.Join(applyErrors...)
}
