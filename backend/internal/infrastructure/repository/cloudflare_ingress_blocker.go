package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	cloudflareIngressPollInterval     = time.Second
	cloudflareRetryMaxDelay           = time.Minute
	cloudflareSettingsSyncInterval    = 10 * time.Second
	cloudflareSettingsSyncTimeout     = 3 * time.Second
	cloudflareIngressQueueBatchSize   = 256
	cloudflareIngressPendingBatchSize = 8
	cloudflareWAFSyncLeaderLockKey    = "cloudflare-ingress:waf-sync"
	cloudflareWAFAnalyticsLockKey     = "cloudflare-ingress:waf-analytics"
	cloudflareWAFLeaderLockTTL        = 2 * time.Minute
)

type cloudflareBlockRequest struct {
	key       string
	target    string
	value     string
	expiresAt time.Time
}

type cloudflarePendingBlock struct {
	request cloudflareBlockRequest
	attempt int
	retryAt time.Time
}

type cloudflareActiveRule struct {
	rule      cloudflareAccessRule
	expiresAt time.Time
	attempt   int
	retryAt   time.Time
}

type cloudflareIngressRuntime struct {
	settings  service.CloudflareIngressSettings
	client    *cloudflareIngressClient
	wafClient *cloudflareWAFClient
	revision  string
}

type cloudflareSettingsUpdate struct {
	runtime cloudflareIngressRuntime
	result  chan error
}

type cloudflareIngressBlocker struct {
	queueMu   sync.Mutex
	queue     []cloudflareBlockRequest
	queueHead int
	queueWake chan struct{}
	updates   chan cloudflareSettingsUpdate

	settingRepo    service.SettingRepository
	encryptor      service.SecretEncryptor
	clientBuild    func(service.CloudflareIngressSettings) *cloudflareIngressClient
	wafClientBuild func(service.CloudflareIngressSettings) *cloudflareWAFClient
	wafState       *cloudflareWAFStateStore
	leaderLock     service.LeaderLockCache
	instanceID     string
	initial        cloudflareIngressRuntime

	pollInterval         time.Duration
	settingsSyncInterval time.Duration
	now                  func() time.Time

	ctx    context.Context
	cancel context.CancelFunc
	start  sync.Once
	stop   sync.Once
	wg     sync.WaitGroup

	started       atomic.Bool
	enabled       atomic.Bool
	configured    atomic.Bool
	running       atomic.Bool
	queueCapacity atomic.Int64
	activeRules   atomic.Int64
	queuedBlocks  atomic.Int64
	enqueued      atomic.Uint64
	applied       atomic.Uint64
	released      atomic.Uint64
	failures      atomic.Uint64
	dropped       atomic.Uint64
	lastError     atomic.Value
	lastSuccessAt atomic.Int64
	revision      atomic.Value
	mode          atomic.Value

	wafHealthMu sync.RWMutex
	wafHealth   service.InvalidAuthWAFHealth
}

// ProvideCloudflareIngressBlocker starts an idle runtime backed exclusively by
// persisted admin settings. No YAML or environment credential fallback exists.
func ProvideCloudflareIngressBlocker(
	settingRepo service.SettingRepository,
	encryptor service.SecretEncryptor,
	rdb *redis.Client,
	leaderLock service.LeaderLockCache,
) service.InvalidAuthEdgeBlocker {
	settings := service.DefaultCloudflareIngressSettings()
	ctx, cancel := context.WithTimeout(context.Background(), cloudflareSettingsSyncTimeout)
	loaded, err := service.LoadPersistedCloudflareIngressSettings(ctx, settingRepo, encryptor)
	cancel()
	if err != nil {
		slog.Warn("load Cloudflare ingress settings at startup failed; edge blocking stays disabled", "error", err)
	} else {
		settings = loaded
	}
	blocker := newCloudflareIngressBlocker(settings, nil)
	blocker.settingRepo = settingRepo
	blocker.encryptor = encryptor
	blocker.wafState = newCloudflareWAFStateStore(rdb)
	blocker.leaderLock = leaderLock
	blocker.Start()
	return blocker
}

func newCloudflareIngressBlocker(
	settings service.CloudflareIngressSettings,
	client *cloudflareIngressClient,
) *cloudflareIngressBlocker {
	settings = normalizeCloudflareRuntimeSettings(settings)
	ctx, cancel := context.WithCancel(context.Background())
	baseURL := cloudflareAPIBaseURL
	if client != nil && strings.TrimSpace(client.baseURL) != "" {
		baseURL = client.baseURL
	}
	clientBuild := func(next service.CloudflareIngressSettings) *cloudflareIngressClient {
		if strings.TrimSpace(next.ZoneID) == "" || strings.TrimSpace(next.APIToken) == "" {
			return nil
		}
		return newCloudflareIngressClient(
			baseURL,
			next.ZoneID,
			next.APIToken,
			time.Duration(next.RequestTimeoutSeconds)*time.Second,
		)
	}
	wafClientBuild := func(next service.CloudflareIngressSettings) *cloudflareWAFClient {
		return newCloudflareWAFClient(baseURL, next)
	}
	if client == nil && settings.Mode == service.CloudflareIngressModeZoneAccessRules {
		client = clientBuild(settings)
	}
	revision := cloudflareSettingsRevision(settings)
	initial := cloudflareIngressRuntime{settings: settings, revision: revision}
	if settings.Mode == service.CloudflareIngressModeWAFCustomRules {
		initial.wafClient = wafClientBuild(settings)
	} else {
		initial.client = client
	}
	blocker := &cloudflareIngressBlocker{
		queueWake:            make(chan struct{}, 1),
		updates:              make(chan cloudflareSettingsUpdate),
		clientBuild:          clientBuild,
		wafClientBuild:       wafClientBuild,
		instanceID:           uuid.NewString(),
		initial:              initial,
		pollInterval:         cloudflareIngressPollInterval,
		settingsSyncInterval: cloudflareSettingsSyncInterval,
		now:                  time.Now,
		ctx:                  ctx,
		cancel:               cancel,
	}
	blocker.lastError.Store("")
	blocker.revision.Store(revision)
	blocker.mode.Store(settings.Mode)
	blocker.enabled.Store(settings.Enabled)
	blocker.configured.Store(cloudflareRuntimeConfigured(initial, blocker.wafState))
	blocker.queueCapacity.Store(int64(settings.QueueCapacity))
	blocker.wafHealth = service.InvalidAuthWAFHealth{
		Hostname:  settings.WAFHostname,
		Hostnames: append([]string(nil), settings.WAFHostnames...),
		RuleCount: len(settings.WAFRuleIDs),
	}
	return blocker
}

func (b *cloudflareIngressBlocker) Start() {
	if b == nil {
		return
	}
	b.start.Do(func() {
		b.started.Store(true)
		b.wg.Add(1)
		go b.run()
	})
}

func (b *cloudflareIngressBlocker) Stop() {
	if b == nil {
		return
	}
	b.stop.Do(func() {
		b.cancel()
		b.wg.Wait()
	})
}

func (b *cloudflareIngressBlocker) EnqueueBlock(clientKey string, expiresAt time.Time) bool {
	if b == nil || !b.enabled.Load() || !b.configured.Load() || !expiresAt.After(b.now()) {
		return false
	}
	request, ok := cloudflareBlockTarget(clientKey, expiresAt)
	if !ok {
		return false
	}
	if !b.reserveQueueSlot() {
		b.dropped.Add(1)
		return false
	}
	b.queueMu.Lock()
	b.queue = append(b.queue, request)
	b.queueMu.Unlock()
	b.enqueued.Add(1)
	b.signalQueue()
	return true
}

func (b *cloudflareIngressBlocker) Health() service.InvalidAuthEdgeHealth {
	if b == nil {
		return service.InvalidAuthEdgeHealth{}
	}
	health := service.InvalidAuthEdgeHealth{
		Enabled:       b.enabled.Load(),
		Running:       b.running.Load(),
		QueueDepth:    int(max(b.queuedBlocks.Load(), 0)),
		QueueCapacity: int(b.queueCapacity.Load()),
		ActiveRules:   int(b.activeRules.Load()),
		Enqueued:      b.enqueued.Load(),
		Applied:       b.applied.Load(),
		Released:      b.released.Load(),
		Failures:      b.failures.Load(),
		Dropped:       b.dropped.Load(),
	}
	if value := b.mode.Load(); value != nil {
		health.Mode, _ = value.(string)
	}
	if value := b.lastError.Load(); value != nil {
		health.LastError, _ = value.(string)
	}
	if nanos := b.lastSuccessAt.Load(); nanos > 0 {
		lastSuccess := time.Unix(0, nanos).UTC()
		health.LastSuccessAt = &lastSuccess
	}
	if health.Mode == service.CloudflareIngressModeWAFCustomRules {
		b.wafHealthMu.RLock()
		wafHealth := b.wafHealth
		b.wafHealthMu.RUnlock()
		health.WAF = &wafHealth
	}
	return health
}

func (b *cloudflareIngressBlocker) ValidateCloudflareIngressSettings(
	ctx context.Context,
	settings service.CloudflareIngressSettings,
) error {
	if b == nil {
		return errors.New("cloudflare ingress runtime is unavailable")
	}
	settings = normalizeCloudflareRuntimeSettings(settings)
	if settings.Mode == service.CloudflareIngressModeWAFCustomRules {
		client := b.wafClientBuild(settings)
		if client == nil || b.wafState == nil {
			return errors.New("cloudflare WAF runtime is unavailable")
		}
		if err := client.validateRules(ctx, settings.WAFRuleIDs); err != nil {
			return fmt.Errorf("validate Cloudflare custom WAF rules: %w", err)
		}
		return nil
	}
	client := b.clientBuild(settings)
	if client == nil {
		return errors.New("cloudflare zone ID and API token are required")
	}
	if err := client.validateAccess(ctx); err != nil {
		return fmt.Errorf("validate Cloudflare Zone IP Access Rules permission: %w", err)
	}
	return nil
}

func (b *cloudflareIngressBlocker) ApplyCloudflareIngressSettings(
	ctx context.Context,
	settings service.CloudflareIngressSettings,
) error {
	if b == nil || !b.started.Load() {
		return errors.New("cloudflare ingress runtime is not running")
	}
	settings = normalizeCloudflareRuntimeSettings(settings)
	update := cloudflareSettingsUpdate{
		runtime: b.buildRuntime(settings),
		result:  make(chan error, 1),
	}
	select {
	case b.updates <- update:
	case <-ctx.Done():
		return ctx.Err()
	case <-b.ctx.Done():
		return errors.New("cloudflare ingress runtime is stopped")
	}
	select {
	case err := <-update.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-b.ctx.Done():
		return errors.New("cloudflare ingress runtime is stopped")
	}
}
