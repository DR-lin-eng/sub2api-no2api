package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
)

const (
	DefaultRequestPriorityPendingLimitPerInstance = 256
	DefaultRequestPriorityPendingMiBPerInstance   = 256
	requestPriorityAdmissionReconcileInterval     = 30 * time.Second
	requestPriorityAdmissionReloadTimeout         = 5 * time.Second
)

type RequestPriorityAdmissionSettingsSubscription interface {
	Messages() <-chan struct{}
	Close() error
}

type RequestPriorityAdmissionSettingsNotifier interface {
	Subscribe(ctx context.Context) RequestPriorityAdmissionSettingsSubscription
	Publish(ctx context.Context) error
}

type requestPriorityAdmissionSyncState struct {
	notifier RequestPriorityAdmissionSettingsNotifier
	cancel   context.CancelFunc
	done     chan struct{}
}

// RequestPriorityAdmissionSettings is an immutable snapshot read by the
// request-admission hot path. Values are stored in MiB at the settings boundary
// so the admin API remains human-readable.
type RequestPriorityAdmissionSettings struct {
	Enabled                 bool
	PendingLimitPerInstance int
	PendingMiBPerInstance   int
}

func defaultRequestPriorityAdmissionSettings() *RequestPriorityAdmissionSettings {
	return &RequestPriorityAdmissionSettings{
		PendingLimitPerInstance: DefaultRequestPriorityPendingLimitPerInstance,
		PendingMiBPerInstance:   DefaultRequestPriorityPendingMiBPerInstance,
	}
}

func normalizeRequestPriorityAdmissionSettings(settings RequestPriorityAdmissionSettings) RequestPriorityAdmissionSettings {
	if settings.PendingLimitPerInstance <= 0 {
		settings.PendingLimitPerInstance = DefaultRequestPriorityPendingLimitPerInstance
	}
	if settings.PendingMiBPerInstance <= 0 {
		settings.PendingMiBPerInstance = DefaultRequestPriorityPendingMiBPerInstance
	}
	return settings
}

// PendingBytesPerInstance returns the configured request-body budget in bytes.
func (s RequestPriorityAdmissionSettings) PendingBytesPerInstance() int64 {
	s = normalizeRequestPriorityAdmissionSettings(s)
	return int64(s.PendingMiBPerInstance) * 1024 * 1024
}

// RequestPriorityAdmissionSettingsSnapshot returns a value copy without locks,
// allocations, or repository access.
func (s *SettingService) RequestPriorityAdmissionSettingsSnapshot() RequestPriorityAdmissionSettings {
	if s != nil {
		if snapshot := s.requestPriorityAdmissionSettings.Load(); snapshot != nil {
			return *snapshot
		}
	}
	return *defaultRequestPriorityAdmissionSettings()
}

// LoadRequestPriorityAdmissionSettings refreshes the process-local snapshot.
// It is intended for startup and cross-instance settings notifications, never
// for the request path.
func (s *SettingService) LoadRequestPriorityAdmissionSettings(ctx context.Context) error {
	if s == nil || s.settingRepo == nil {
		return nil
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyRequestPriorityAdmissionEnabled,
		SettingKeyRequestPriorityPendingLimitPerInstance,
		SettingKeyRequestPriorityPendingMiBPerInstance,
	})
	if err != nil {
		return err
	}
	settings := parseRequestPriorityAdmissionSettings(values)
	s.storeRequestPriorityAdmissionSettings(settings)
	return nil
}

// SetRequestPriorityAdmissionSettingsSink connects the settings snapshot to a
// subsystem-owned runtime cache. The current value is published immediately.
func (s *SettingService) SetRequestPriorityAdmissionSettingsSink(sink func(RequestPriorityAdmissionSettings)) {
	if s == nil {
		return
	}
	s.requestPriorityAdmissionSinkMu.Lock()
	s.requestPriorityAdmissionSettingsSink = sink
	s.requestPriorityAdmissionSinkMu.Unlock()
	if sink != nil {
		sink(s.RequestPriorityAdmissionSettingsSnapshot())
	}
}

func (s *SettingService) storeRequestPriorityAdmissionSettings(settings RequestPriorityAdmissionSettings) {
	if s == nil {
		return
	}
	settings = normalizeRequestPriorityAdmissionSettings(settings)
	snapshot := settings
	s.requestPriorityAdmissionSettings.Store(&snapshot)
	s.requestPriorityAdmissionSinkMu.RLock()
	sink := s.requestPriorityAdmissionSettingsSink
	s.requestPriorityAdmissionSinkMu.RUnlock()
	if sink != nil {
		sink(settings)
	}
}

// StartRequestPriorityAdmissionSettingsSync subscribes to cross-instance
// invalidations and periodically reconciles with the database in case a
// Pub/Sub message is missed. Neither path is used by request processing.
func (s *SettingService) StartRequestPriorityAdmissionSettingsSync(ctx context.Context, notifier RequestPriorityAdmissionSettingsNotifier) {
	s.startRequestPriorityAdmissionSettingsSync(ctx, notifier, requestPriorityAdmissionReconcileInterval)
}

func (s *SettingService) startRequestPriorityAdmissionSettingsSync(ctx context.Context, notifier RequestPriorityAdmissionSettingsNotifier, reconcileInterval time.Duration) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if reconcileInterval <= 0 {
		reconcileInterval = requestPriorityAdmissionReconcileInterval
	}
	s.StopRequestPriorityAdmissionSettingsSync()

	syncCtx, cancel := context.WithCancel(ctx)
	state := &requestPriorityAdmissionSyncState{
		notifier: notifier,
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	s.requestPriorityAdmissionSyncMu.Lock()
	s.requestPriorityAdmissionSync = state
	s.requestPriorityAdmissionSyncMu.Unlock()

	go s.runRequestPriorityAdmissionSettingsSync(syncCtx, state, reconcileInterval)
}

func (s *SettingService) runRequestPriorityAdmissionSettingsSync(ctx context.Context, state *requestPriorityAdmissionSyncState, reconcileInterval time.Duration) {
	defer close(state.done)
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()

	var messages <-chan struct{}
	if state.notifier != nil {
		subscription := state.notifier.Subscribe(ctx)
		if subscription != nil {
			messages = subscription.Messages()
			defer func() { _ = subscription.Close() }()
		}
	}

	reload := func() {
		reloadCtx, cancel := context.WithTimeout(ctx, requestPriorityAdmissionReloadTimeout)
		defer cancel()
		if err := s.LoadRequestPriorityAdmissionSettings(reloadCtx); err != nil && ctx.Err() == nil {
			logger.LegacyPrintf("service.setting", "Warning: refresh request priority admission settings failed: %v", err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-messages:
			if !ok {
				messages = nil
				continue
			}
			reload()
		case <-ticker.C:
			reload()
		}
	}
}

// StopRequestPriorityAdmissionSettingsSync ends the background subscriber and
// database reconciliation loop. It is safe to call repeatedly.
func (s *SettingService) StopRequestPriorityAdmissionSettingsSync() {
	if s == nil {
		return
	}
	s.requestPriorityAdmissionSyncMu.Lock()
	state := s.requestPriorityAdmissionSync
	s.requestPriorityAdmissionSync = nil
	s.requestPriorityAdmissionSyncMu.Unlock()
	if state == nil {
		return
	}
	state.cancel()
	select {
	case <-state.done:
	case <-time.After(requestPriorityAdmissionReloadTimeout):
		logger.LegacyPrintf("service.setting", "Warning: timed out stopping request priority admission settings sync")
	}
}

func (s *SettingService) publishRequestPriorityAdmissionSettingsUpdate(ctx context.Context) {
	if s == nil {
		return
	}
	s.requestPriorityAdmissionSyncMu.Lock()
	state := s.requestPriorityAdmissionSync
	var notifier RequestPriorityAdmissionSettingsNotifier
	if state != nil {
		notifier = state.notifier
	}
	s.requestPriorityAdmissionSyncMu.Unlock()
	if notifier == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := notifier.Publish(ctx); err != nil {
		logger.LegacyPrintf("service.setting", "Warning: publish request priority admission settings update failed: %v", err)
	}
}

func parseRequestPriorityAdmissionSettings(values map[string]string) RequestPriorityAdmissionSettings {
	settings := RequestPriorityAdmissionSettings{
		Enabled:                 strings.EqualFold(strings.TrimSpace(values[SettingKeyRequestPriorityAdmissionEnabled]), "true"),
		PendingLimitPerInstance: parsePositiveRequestPrioritySetting(values[SettingKeyRequestPriorityPendingLimitPerInstance], DefaultRequestPriorityPendingLimitPerInstance),
		PendingMiBPerInstance:   parsePositiveRequestPrioritySetting(values[SettingKeyRequestPriorityPendingMiBPerInstance], DefaultRequestPriorityPendingMiBPerInstance),
	}
	return normalizeRequestPriorityAdmissionSettings(settings)
}

func parsePositiveRequestPrioritySetting(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
