package repository

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/google/uuid"
)

func (b *cloudflareIngressBlocker) run() {
	defer b.wg.Done()
	defer b.running.Store(false)

	runtime := b.initial
	active := make(map[string]*cloudflareActiveRule)
	pending := make(map[string]*cloudflarePendingBlock)
	b.applyRuntimeHealth(runtime, len(active))
	if runtime.settings.Mode == service.CloudflareIngressModeWAFCustomRules && runtime.wafClient != nil && b.wafState != nil {
		if err := b.syncWAF(runtime, false); err != nil {
			b.recordFailure(err)
		}
		b.refreshWAFAnalytics(runtime, false)
	} else if runtime.client != nil {
		if err := b.reconcile(runtime, active); err != nil {
			b.recordFailure(err)
		} else {
			b.recordSuccess()
		}
		b.applyRuntimeHealth(runtime, len(active))
	}

	pollTicker := time.NewTicker(b.pollInterval)
	defer pollTicker.Stop()
	var settingsTicker *time.Ticker
	var settingsTick <-chan time.Time
	if b.settingRepo != nil && b.encryptor != nil {
		settingsTicker = time.NewTicker(b.settingsSyncInterval)
		settingsTick = settingsTicker.C
		defer settingsTicker.Stop()
	}
	nextReconcile := b.now().Add(time.Duration(runtime.settings.ReconcileIntervalSeconds) * time.Second)
	nextWAFSync := b.now().Add(time.Duration(runtime.settings.WAFSyncIntervalSeconds) * time.Second)
	nextAnalytics := b.now().Add(time.Duration(runtime.settings.AnalyticsIntervalSeconds) * time.Second)

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-b.queueWake:
			b.consumeQueue(runtime, pending)
			b.processPendingForMode(runtime, active, pending, cloudflareIngressPendingBatchSize)
			if b.hasQueuedRequests() {
				b.signalQueue()
			}
		case update := <-b.updates:
			err := b.applyRuntimeUpdate(&runtime, update.runtime, active, pending)
			if err == nil {
				nextReconcile = b.now().Add(time.Duration(runtime.settings.ReconcileIntervalSeconds) * time.Second)
				nextWAFSync = b.now().Add(time.Duration(runtime.settings.WAFSyncIntervalSeconds) * time.Second)
				nextAnalytics = b.now().Add(time.Duration(runtime.settings.AnalyticsIntervalSeconds) * time.Second)
			}
			update.result <- err
		case <-pollTicker.C:
			if runtime.settings.Mode == service.CloudflareIngressModeWAFCustomRules && runtime.wafClient != nil && b.wafState != nil {
				b.processPendingForMode(runtime, active, pending, cloudflareIngressPendingBatchSize)
				now := b.now()
				forceReconcile := !nextReconcile.After(now)
				if !nextWAFSync.After(now) || forceReconcile {
					if err := b.syncWAF(runtime, forceReconcile); err != nil {
						b.recordFailure(err)
					}
					nextWAFSync = now.Add(time.Duration(runtime.settings.WAFSyncIntervalSeconds) * time.Second)
					if forceReconcile {
						nextReconcile = now.Add(time.Duration(runtime.settings.ReconcileIntervalSeconds) * time.Second)
					}
				}
				if !nextAnalytics.After(now) {
					b.refreshWAFAnalytics(runtime, false)
					nextAnalytics = now.Add(time.Duration(runtime.settings.AnalyticsIntervalSeconds) * time.Second)
				}
			} else if runtime.client != nil {
				b.releaseExpired(runtime, active, 16)
				b.processPendingForMode(runtime, active, pending, cloudflareIngressPendingBatchSize)
				if !nextReconcile.After(b.now()) {
					if err := b.reconcile(runtime, active); err != nil {
						b.recordFailure(err)
					} else {
						b.recordSuccess()
					}
					nextReconcile = b.now().Add(time.Duration(runtime.settings.ReconcileIntervalSeconds) * time.Second)
				}
			}
			b.applyRuntimeHealth(runtime, len(active))
		case <-settingsTick:
			loaded, err := b.loadPersistedSettings()
			if err != nil {
				b.recordFailure(err)
				continue
			}
			if loaded.revision == runtime.revision {
				continue
			}
			if err := b.applyRuntimeUpdate(&runtime, loaded, active, pending); err != nil {
				b.recordFailure(fmt.Errorf("apply persisted Cloudflare ingress settings: %w", err))
				continue
			}
			nextReconcile = b.now().Add(time.Duration(runtime.settings.ReconcileIntervalSeconds) * time.Second)
			nextWAFSync = b.now().Add(time.Duration(runtime.settings.WAFSyncIntervalSeconds) * time.Second)
			nextAnalytics = b.now().Add(time.Duration(runtime.settings.AnalyticsIntervalSeconds) * time.Second)
		}
	}
}

func (b *cloudflareIngressBlocker) applyRuntimeUpdate(
	current *cloudflareIngressRuntime,
	next cloudflareIngressRuntime,
	active map[string]*cloudflareActiveRule,
	pending map[string]*cloudflarePendingBlock,
) error {
	if current == nil || current.revision == next.revision {
		return nil
	}
	bindingChanged := cloudflareRuntimeBindingChanged(current.settings, next.settings)
	if bindingChanged && (b.activeRules.Load() > 0 || len(pending) > 0 || b.queuedBlocks.Load() > 0) {
		// A different replica may observe the credential replacement while it
		// still owns rules from the old zone. Honor the disable immediately, keep
		// the old client for cleanup, and retry the credential swap on the next
		// persisted-settings poll after the old work drains.
		if !next.settings.Enabled {
			current.settings.Enabled = false
			current.revision = cloudflareSettingsRevision(current.settings)
			for key := range pending {
				delete(pending, key)
				b.queuedBlocks.Add(-1)
			}
			b.signalQueue()
			b.applyRuntimeHealth(*current, len(active))
		}
		return errors.New("queued or active Cloudflare rules must drain before credentials can change")
	}
	if !next.settings.Enabled {
		for key := range pending {
			delete(pending, key)
			b.queuedBlocks.Add(-1)
		}
		b.signalQueue()
	}
	if bindingChanged {
		clear(active)
		b.activeRules.Store(0)
	}
	*current = next
	b.revision.Store(next.revision)
	b.applyRuntimeHealth(next, len(active))
	if next.settings.Mode == service.CloudflareIngressModeWAFCustomRules && next.wafClient != nil && b.wafState != nil {
		if err := b.syncWAF(next, false); err != nil {
			b.recordFailure(err)
		} else {
			b.recordSuccess()
		}
		b.refreshWAFAnalytics(next, false)
	} else if next.client != nil {
		if err := b.reconcile(next, active); err != nil {
			b.recordFailure(err)
		} else {
			b.recordSuccess()
		}
	}
	b.applyRuntimeHealth(next, len(active))
	return nil
}

func (b *cloudflareIngressBlocker) loadPersistedSettings() (cloudflareIngressRuntime, error) {
	ctx, cancel := context.WithTimeout(b.ctx, cloudflareSettingsSyncTimeout)
	defer cancel()
	settings, err := service.LoadPersistedCloudflareIngressSettings(ctx, b.settingRepo, b.encryptor)
	if err != nil {
		return cloudflareIngressRuntime{}, fmt.Errorf("refresh Cloudflare ingress settings: %w", err)
	}
	settings = normalizeCloudflareRuntimeSettings(settings)
	return b.buildRuntime(settings), nil
}

func (b *cloudflareIngressBlocker) consumeQueue(
	runtime cloudflareIngressRuntime,
	pending map[string]*cloudflarePendingBlock,
) {
	for _, request := range b.takeQueuedRequests(cloudflareIngressQueueBatchSize) {
		if !runtime.settings.Enabled || !cloudflareRuntimeConfigured(runtime, b.wafState) {
			b.queuedBlocks.Add(-1)
			continue
		}
		b.mergePending(pending, request)
	}
}

func (b *cloudflareIngressBlocker) mergePending(
	pending map[string]*cloudflarePendingBlock,
	request cloudflareBlockRequest,
) {
	if current := pending[request.key]; current != nil {
		if request.expiresAt.After(current.request.expiresAt) {
			current.request.expiresAt = request.expiresAt
		}
		b.queuedBlocks.Add(-1)
		return
	}
	pending[request.key] = &cloudflarePendingBlock{request: request}
}

func (b *cloudflareIngressBlocker) processPendingForMode(
	runtime cloudflareIngressRuntime,
	active map[string]*cloudflareActiveRule,
	pending map[string]*cloudflarePendingBlock,
	limit int,
) {
	if runtime.settings.Mode == service.CloudflareIngressModeWAFCustomRules {
		b.processWAFPending(runtime, pending, limit)
		return
	}
	b.processPending(runtime, active, pending, limit)
}

func (b *cloudflareIngressBlocker) processWAFPending(
	runtime cloudflareIngressRuntime,
	pending map[string]*cloudflarePendingBlock,
	limit int,
) {
	if !runtime.settings.Enabled || runtime.wafClient == nil || b.wafState == nil {
		return
	}
	now := b.now()
	keys := make([]string, 0, limit)
	requests := make([]cloudflareBlockRequest, 0, limit)
	for key, item := range pending {
		if len(keys) >= limit || item.retryAt.After(now) {
			continue
		}
		if !item.request.expiresAt.After(now) {
			delete(pending, key)
			b.queuedBlocks.Add(-1)
			continue
		}
		keys = append(keys, key)
		requests = append(requests, item.request)
	}
	if len(requests) == 0 {
		return
	}
	if err := b.wafState.UpsertBlocks(b.ctx, requests); err != nil {
		for _, key := range keys {
			item := pending[key]
			if item == nil {
				continue
			}
			item.attempt++
			item.retryAt = now.Add(cloudflareRetryDelay(item.attempt))
		}
		b.recordFailure(err)
		return
	}
	for _, key := range keys {
		delete(pending, key)
		b.queuedBlocks.Add(-1)
		b.applied.Add(1)
	}
	b.recordSuccess()
}

func (b *cloudflareIngressBlocker) processPending(
	runtime cloudflareIngressRuntime,
	active map[string]*cloudflareActiveRule,
	pending map[string]*cloudflarePendingBlock,
	limit int,
) {
	if !runtime.settings.Enabled || runtime.client == nil {
		return
	}
	now := b.now()
	processed := 0
	for key, item := range pending {
		if processed >= limit || item.retryAt.After(now) {
			continue
		}
		processed++
		if !item.request.expiresAt.After(now) {
			delete(pending, key)
			b.queuedBlocks.Add(-1)
			continue
		}
		if err := b.applyBlock(runtime, active, item.request); err != nil {
			item.attempt++
			item.retryAt = now.Add(cloudflareRetryDelay(item.attempt))
			b.recordFailure(err)
			continue
		}
		delete(pending, key)
		b.queuedBlocks.Add(-1)
		b.applied.Add(1)
		b.recordSuccess()
	}
}

func (b *cloudflareIngressBlocker) applyBlock(
	runtime cloudflareIngressRuntime,
	active map[string]*cloudflareActiveRule,
	request cloudflareBlockRequest,
) error {
	if current := active[request.key]; current != nil {
		if !request.expiresAt.After(current.expiresAt) {
			return nil
		}
		if err := runtime.client.updateRule(b.ctx, current.rule, request.expiresAt); err != nil {
			return fmt.Errorf("extend Cloudflare ingress block: %w", err)
		}
		current.expiresAt = request.expiresAt
		current.rule.Notes = cloudflareRuleNote(request.expiresAt)
		current.attempt = 0
		current.retryAt = time.Time{}
		return nil
	}

	managed, err := runtime.client.listManagedRules(b.ctx, request.value)
	if err != nil {
		return fmt.Errorf("find Cloudflare ingress block: %w", err)
	}
	for _, rule := range managed {
		key, ok := cloudflareAccessRuleKey(rule.Configuration)
		if !ok || key != request.key {
			continue
		}
		expiresAt, ok := cloudflareRuleExpiry(rule.Notes)
		if !ok {
			continue
		}
		if !expiresAt.After(b.now()) {
			if err := runtime.client.deleteRule(b.ctx, rule.ID); err != nil {
				return fmt.Errorf("delete expired Cloudflare ingress block: %w", err)
			}
			b.released.Add(1)
			continue
		}
		state := &cloudflareActiveRule{rule: rule, expiresAt: expiresAt}
		active[request.key] = state
		b.activeRules.Store(int64(len(active)))
		if request.expiresAt.After(expiresAt) {
			if err := runtime.client.updateRule(b.ctx, rule, request.expiresAt); err != nil {
				return fmt.Errorf("extend discovered Cloudflare ingress block: %w", err)
			}
			state.expiresAt = request.expiresAt
			state.rule.Notes = cloudflareRuleNote(request.expiresAt)
		}
		return nil
	}

	if len(active) >= runtime.settings.MaxActiveRules {
		return fmt.Errorf("cloudflare ingress block limit reached (%d)", runtime.settings.MaxActiveRules)
	}
	rule, err := runtime.client.createRule(b.ctx, request.target, request.value, request.expiresAt)
	if err != nil {
		return fmt.Errorf("create Cloudflare ingress block: %w", err)
	}
	if rule.Configuration.Target == "" {
		rule.Configuration = cloudflareAccessRuleConfiguration{Target: request.target, Value: request.value}
	}
	if rule.Notes == "" {
		rule.Notes = cloudflareRuleNote(request.expiresAt)
	}
	active[request.key] = &cloudflareActiveRule{rule: rule, expiresAt: request.expiresAt}
	b.activeRules.Store(int64(len(active)))
	return nil
}

func (b *cloudflareIngressBlocker) releaseExpired(
	runtime cloudflareIngressRuntime,
	active map[string]*cloudflareActiveRule,
	limit int,
) {
	now := b.now()
	processed := 0
	for key, state := range active {
		if processed >= limit || state.expiresAt.After(now) || state.retryAt.After(now) {
			continue
		}
		processed++
		remote, err := runtime.client.getRule(b.ctx, state.rule.ID)
		if errors.Is(err, errCloudflareRuleNotFound) {
			delete(active, key)
			b.activeRules.Store(int64(len(active)))
			b.released.Add(1)
			b.recordSuccess()
			continue
		}
		if err != nil {
			state.attempt++
			state.retryAt = now.Add(cloudflareRetryDelay(state.attempt))
			b.recordFailure(fmt.Errorf("refresh Cloudflare ingress block before release: %w", err))
			continue
		}
		remoteExpiry, managed := cloudflareRuleExpiry(remote.Notes)
		if !managed {
			delete(active, key)
			b.activeRules.Store(int64(len(active)))
			b.recordSuccess()
			continue
		}
		if remoteExpiry.After(now) {
			state.rule = remote
			state.expiresAt = remoteExpiry
			state.attempt = 0
			state.retryAt = time.Time{}
			b.recordSuccess()
			continue
		}
		if err := runtime.client.deleteRule(b.ctx, state.rule.ID); err != nil {
			state.attempt++
			state.retryAt = now.Add(cloudflareRetryDelay(state.attempt))
			b.recordFailure(fmt.Errorf("release Cloudflare ingress block: %w", err))
			continue
		}
		delete(active, key)
		b.activeRules.Store(int64(len(active)))
		b.released.Add(1)
		b.recordSuccess()
	}
}

func (b *cloudflareIngressBlocker) reconcile(
	runtime cloudflareIngressRuntime,
	active map[string]*cloudflareActiveRule,
) error {
	rules, err := runtime.client.listManagedRules(b.ctx, "")
	if err != nil {
		return fmt.Errorf("reconcile Cloudflare ingress blocks: %w", err)
	}
	now := b.now()
	next := make(map[string]*cloudflareActiveRule, len(rules))
	var reconcileErrors []error
	for _, rule := range rules {
		key, ok := cloudflareAccessRuleKey(rule.Configuration)
		if !ok {
			continue
		}
		expiresAt, ok := cloudflareRuleExpiry(rule.Notes)
		if !ok {
			continue
		}
		if !expiresAt.After(now) {
			if err := runtime.client.deleteRule(b.ctx, rule.ID); err != nil {
				reconcileErrors = append(reconcileErrors, fmt.Errorf("delete expired rule %s: %w", rule.ID, err))
			} else {
				b.released.Add(1)
			}
			continue
		}
		if current := next[key]; current != nil {
			keep, remove := current, rule
			if expiresAt.After(current.expiresAt) {
				keep = &cloudflareActiveRule{rule: rule, expiresAt: expiresAt}
				remove = current.rule
			}
			if err := runtime.client.deleteRule(b.ctx, remove.ID); err != nil {
				reconcileErrors = append(reconcileErrors, fmt.Errorf("delete duplicate rule %s: %w", remove.ID, err))
			}
			next[key] = keep
			continue
		}
		next[key] = &cloudflareActiveRule{rule: rule, expiresAt: expiresAt}
	}
	clear(active)
	for key, state := range next {
		active[key] = state
	}
	b.activeRules.Store(int64(len(active)))
	return errors.Join(reconcileErrors...)
}

func (b *cloudflareIngressBlocker) syncWAF(runtime cloudflareIngressRuntime, force bool) error {
	if runtime.wafClient == nil || b.wafState == nil {
		return errors.New("cloudflare WAF runtime is unavailable")
	}
	release, acquired, err := b.acquireWAFLeaderLock(cloudflareWAFSyncLeaderLockKey)
	if err != nil {
		return fmt.Errorf("acquire cloudflare WAF sync lock: %w", err)
	}
	if !acquired {
		b.loadCachedWAFHealth(runtime)
		return nil
	}
	defer release()

	now := b.now().UTC()
	entries, removed, total, err := b.wafState.Snapshot(b.ctx, now, runtime.settings.MaxActiveRules)
	if err != nil {
		return err
	}
	if removed > 0 {
		b.released.Add(uint64(removed))
	}
	expressions, included, expressionOverflow := cloudflareWAFExpressions(
		runtime.settings.WAFHostnames,
		entries,
		len(runtime.settings.WAFRuleIDs),
	)
	overflow := max(int(total)-included, expressionOverflow)
	b.activeRules.Store(total)
	b.updateWAFHealth(runtime, func(health *service.InvalidAuthWAFHealth) {
		health.OverflowEntries = overflow
	})

	binding := cloudflareWAFBindingRevision(runtime.settings)
	revision := cloudflareWAFExpressionRevision(binding, expressions)
	state, err := b.wafState.LoadSyncState(b.ctx)
	if err != nil {
		return err
	}
	if force && state.Binding == binding && !state.SyncedAt.IsZero() &&
		now.Sub(state.SyncedAt) < time.Duration(runtime.settings.WAFSyncIntervalSeconds)*time.Second {
		force = false
	}
	fullReconcileDue := state.SyncedAt.IsZero() || now.Sub(state.SyncedAt) >= time.Duration(runtime.settings.ReconcileIntervalSeconds)*time.Second
	if !force && !fullReconcileDue && state.Binding == binding && state.Revision == revision {
		b.updateWAFHealth(runtime, func(health *service.InvalidAuthWAFHealth) {
			health.SyncedEntries = state.SyncedEntries
			if !state.SyncedAt.IsZero() {
				syncedAt := state.SyncedAt
				health.LastSyncedAt = &syncedAt
			}
		})
		return nil
	}

	if _, err := runtime.wafClient.syncExpressions(b.ctx, runtime.settings.WAFRuleIDs, expressions); err != nil {
		return err
	}
	state = cloudflareWAFSyncState{
		Revision:      revision,
		Binding:       binding,
		SyncedEntries: included,
		SyncedAt:      now,
	}
	if err := b.wafState.SaveSyncState(b.ctx, state); err != nil {
		return err
	}
	b.updateWAFHealth(runtime, func(health *service.InvalidAuthWAFHealth) {
		health.SyncedEntries = included
		health.LastSyncedAt = &now
	})
	b.recordSuccess()
	if overflow > 0 {
		return fmt.Errorf("cloudflare WAF shard capacity exceeded by %d entries", overflow)
	}
	return nil
}

func (b *cloudflareIngressBlocker) refreshWAFAnalytics(runtime cloudflareIngressRuntime, force bool) {
	if runtime.wafClient == nil || b.wafState == nil {
		return
	}
	binding := cloudflareWAFBindingRevision(runtime.settings)
	now := b.now().UTC()
	interval := time.Duration(runtime.settings.AnalyticsIntervalSeconds) * time.Second
	snapshot, err := b.wafState.LoadAnalytics(b.ctx)
	if err == nil {
		b.applyWAFAnalyticsSnapshot(runtime, snapshot)
		if !force && snapshot.Binding == binding && !snapshot.CheckedAt.IsZero() && now.Sub(snapshot.CheckedAt) < interval {
			return
		}
	}
	if !runtime.settings.Enabled {
		return
	}

	release, acquired, lockErr := b.acquireWAFLeaderLock(cloudflareWAFAnalyticsLockKey)
	if lockErr != nil {
		b.recordFailure(fmt.Errorf("acquire cloudflare WAF analytics lock: %w", lockErr))
		return
	}
	if !acquired {
		return
	}
	defer release()

	snapshot, err = b.wafState.LoadAnalytics(b.ctx)
	if err == nil && !force && snapshot.Binding == binding && !snapshot.CheckedAt.IsZero() && now.Sub(snapshot.CheckedAt) < interval {
		b.applyWAFAnalyticsSnapshot(runtime, snapshot)
		return
	}
	if snapshot.Binding != binding {
		snapshot = cloudflareWAFAnalyticsSnapshot{
			Binding:   binding,
			Hostname:  runtime.settings.WAFHostname,
			Hostnames: append([]string(nil), runtime.settings.WAFHostnames...),
		}
	}
	analytics, queryErr := runtime.wafClient.queryAnalytics(
		b.ctx,
		runtime.settings.WAFHostnames,
		runtime.settings.WAFRuleIDs,
		now,
	)
	snapshot.Binding = binding
	snapshot.Hostname = runtime.settings.WAFHostname
	snapshot.Hostnames = append([]string(nil), runtime.settings.WAFHostnames...)
	snapshot.CheckedAt = now
	if queryErr != nil {
		snapshot.Error = queryErr.Error()
	} else {
		snapshot.HostnameRequests24h = analytics.HostnameRequests
		snapshot.BlockedRequests24h = analytics.BlockedRequests
		snapshot.HostnameStats = make([]cloudflareWAFHostnameAnalyticsSnapshot, 0, len(analytics.Hostnames))
		for _, item := range analytics.Hostnames {
			snapshot.HostnameStats = append(snapshot.HostnameStats, cloudflareWAFHostnameAnalyticsSnapshot{
				Hostname:           item.Hostname,
				Requests24h:        item.Requests,
				BlockedRequests24h: item.BlockedRequests,
			})
		}
		snapshot.WindowStart = analytics.WindowStart
		snapshot.UpdatedAt = now
		snapshot.Error = ""
	}
	if saveErr := b.wafState.SaveAnalytics(b.ctx, snapshot, 48*time.Hour); saveErr != nil {
		b.recordFailure(saveErr)
		return
	}
	b.applyWAFAnalyticsSnapshot(runtime, snapshot)
	if queryErr != nil {
		b.recordFailure(queryErr)
	}
}

func (b *cloudflareIngressBlocker) loadCachedWAFHealth(runtime cloudflareIngressRuntime) {
	if b.wafState == nil {
		return
	}
	state, err := b.wafState.LoadSyncState(b.ctx)
	if err == nil && state.Binding == cloudflareWAFBindingRevision(runtime.settings) {
		if active, countErr := b.wafState.CountActive(b.ctx, b.now()); countErr == nil {
			b.activeRules.Store(active)
		}
		b.updateWAFHealth(runtime, func(health *service.InvalidAuthWAFHealth) {
			health.SyncedEntries = state.SyncedEntries
			if !state.SyncedAt.IsZero() {
				syncedAt := state.SyncedAt
				health.LastSyncedAt = &syncedAt
			}
		})
	}
	snapshot, err := b.wafState.LoadAnalytics(b.ctx)
	if err == nil {
		b.applyWAFAnalyticsSnapshot(runtime, snapshot)
	}
}

func (b *cloudflareIngressBlocker) applyWAFAnalyticsSnapshot(
	runtime cloudflareIngressRuntime,
	snapshot cloudflareWAFAnalyticsSnapshot,
) {
	if snapshot.Binding != cloudflareWAFBindingRevision(runtime.settings) {
		return
	}
	b.updateWAFHealth(runtime, func(health *service.InvalidAuthWAFHealth) {
		health.HostnameRequests24h = snapshot.HostnameRequests24h
		health.BlockedRequests24h = snapshot.BlockedRequests24h
		health.HostnameStats = make([]service.InvalidAuthWAFHostnameHealth, 0, len(snapshot.HostnameStats))
		for _, item := range snapshot.HostnameStats {
			health.HostnameStats = append(health.HostnameStats, service.InvalidAuthWAFHostnameHealth{
				Hostname:           item.Hostname,
				Requests24h:        item.Requests24h,
				BlockedRequests24h: item.BlockedRequests24h,
			})
		}
		health.AnalyticsError = snapshot.Error
		if !snapshot.UpdatedAt.IsZero() {
			updatedAt := snapshot.UpdatedAt
			health.AnalyticsUpdatedAt = &updatedAt
		}
		if !snapshot.WindowStart.IsZero() {
			windowStart := snapshot.WindowStart
			health.AnalyticsWindowStart = &windowStart
		}
	})
}

func (b *cloudflareIngressBlocker) updateWAFHealth(
	runtime cloudflareIngressRuntime,
	update func(*service.InvalidAuthWAFHealth),
) {
	b.wafHealthMu.Lock()
	b.wafHealth.Hostname = runtime.settings.WAFHostname
	b.wafHealth.Hostnames = append([]string(nil), runtime.settings.WAFHostnames...)
	b.wafHealth.RuleCount = len(runtime.settings.WAFRuleIDs)
	if update != nil {
		update(&b.wafHealth)
	}
	b.wafHealthMu.Unlock()
}

func (b *cloudflareIngressBlocker) acquireWAFLeaderLock(key string) (func(), bool, error) {
	if b.leaderLock == nil {
		return func() {}, true, nil
	}
	owner := b.instanceID + ":" + uuid.NewString()
	acquired, err := b.leaderLock.TryAcquireLeaderLock(b.ctx, key, owner, cloudflareWAFLeaderLockTTL)
	if err != nil || !acquired {
		return func() {}, acquired, err
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), cloudflareSettingsSyncTimeout)
		defer cancel()
		_ = b.leaderLock.ReleaseLeaderLock(ctx, key, owner)
	}, true, nil
}

func (b *cloudflareIngressBlocker) applyRuntimeHealth(runtime cloudflareIngressRuntime, activeRules int) {
	b.enabled.Store(runtime.settings.Enabled)
	b.mode.Store(runtime.settings.Mode)
	b.configured.Store(cloudflareRuntimeConfigured(runtime, b.wafState))
	b.queueCapacity.Store(int64(runtime.settings.QueueCapacity))
	if runtime.settings.Mode == service.CloudflareIngressModeWAFCustomRules {
		activeRules = int(b.activeRules.Load())
	}
	b.running.Store(cloudflareRuntimeConfigured(runtime, b.wafState) && (runtime.settings.Enabled || activeRules > 0))
	b.updateWAFHealth(runtime, nil)
}

func (b *cloudflareIngressBlocker) takeQueuedRequests(limit int) []cloudflareBlockRequest {
	b.queueMu.Lock()
	defer b.queueMu.Unlock()
	available := len(b.queue) - b.queueHead
	if available <= 0 {
		b.queue = nil
		b.queueHead = 0
		return nil
	}
	count := min(max(limit, 1), available)
	end := b.queueHead + count
	batch := append([]cloudflareBlockRequest(nil), b.queue[b.queueHead:end]...)
	clear(b.queue[b.queueHead:end])
	b.queueHead = end
	if b.queueHead == len(b.queue) {
		b.queue = nil
		b.queueHead = 0
	} else if b.queueHead >= 1024 && b.queueHead*2 >= len(b.queue) {
		remaining := append([]cloudflareBlockRequest(nil), b.queue[b.queueHead:]...)
		b.queue = remaining
		b.queueHead = 0
	}
	return batch
}

func (b *cloudflareIngressBlocker) hasQueuedRequests() bool {
	b.queueMu.Lock()
	defer b.queueMu.Unlock()
	return len(b.queue)-b.queueHead > 0
}

func (b *cloudflareIngressBlocker) signalQueue() {
	select {
	case b.queueWake <- struct{}{}:
	default:
	}
}

func (b *cloudflareIngressBlocker) recordFailure(err error) {
	if err == nil {
		return
	}
	b.failures.Add(1)
	message := err.Error()
	if len(message) > 512 {
		message = message[:512]
	}
	b.lastError.Store(message)
}

func (b *cloudflareIngressBlocker) reserveQueueSlot() bool {
	capacity := b.queueCapacity.Load()
	for {
		current := b.queuedBlocks.Load()
		if current >= capacity {
			return false
		}
		if b.queuedBlocks.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

func (b *cloudflareIngressBlocker) recordSuccess() {
	b.lastError.Store("")
	b.lastSuccessAt.Store(b.now().UnixNano())
}

func normalizeCloudflareRuntimeSettings(settings service.CloudflareIngressSettings) service.CloudflareIngressSettings {
	defaults := service.DefaultCloudflareIngressSettings()
	settings.Mode = strings.ToLower(strings.TrimSpace(settings.Mode))
	if settings.Mode == "" {
		settings.Mode = service.CloudflareIngressModeZoneAccessRules
	}
	settings.ZoneID = strings.ToLower(strings.TrimSpace(settings.ZoneID))
	settings.APIToken = strings.TrimSpace(settings.APIToken)
	hostnames := append([]string(nil), settings.WAFHostnames...)
	if len(hostnames) == 0 && settings.WAFHostname != "" {
		hostnames = []string{settings.WAFHostname}
	}
	seenHostnames := make(map[string]struct{}, len(hostnames))
	normalizedHostnames := make([]string, 0, len(hostnames))
	for _, hostname := range hostnames {
		hostname = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(hostname)), ".")
		if hostname == "" {
			continue
		}
		if _, ok := seenHostnames[hostname]; ok {
			continue
		}
		seenHostnames[hostname] = struct{}{}
		normalizedHostnames = append(normalizedHostnames, hostname)
	}
	sort.Strings(normalizedHostnames)
	settings.WAFHostnames = normalizedHostnames
	settings.WAFHostname = ""
	if len(settings.WAFHostnames) > 0 {
		settings.WAFHostname = settings.WAFHostnames[0]
	}
	for index := range settings.WAFRuleIDs {
		settings.WAFRuleIDs[index] = strings.ToLower(strings.TrimSpace(settings.WAFRuleIDs[index]))
	}
	if settings.WAFSyncIntervalSeconds < service.CloudflareIngressMinWAFSyncIntervalSeconds || settings.WAFSyncIntervalSeconds > service.CloudflareIngressMaxWAFSyncIntervalSeconds {
		settings.WAFSyncIntervalSeconds = defaults.WAFSyncIntervalSeconds
	}
	if settings.AnalyticsIntervalSeconds < service.CloudflareIngressMinAnalyticsIntervalSeconds || settings.AnalyticsIntervalSeconds > service.CloudflareIngressMaxAnalyticsIntervalSeconds {
		settings.AnalyticsIntervalSeconds = defaults.AnalyticsIntervalSeconds
	}
	if settings.RequestTimeoutSeconds < service.CloudflareIngressMinRequestTimeoutSeconds || settings.RequestTimeoutSeconds > service.CloudflareIngressMaxRequestTimeoutSeconds {
		settings.RequestTimeoutSeconds = defaults.RequestTimeoutSeconds
	}
	if settings.QueueCapacity < service.CloudflareIngressMinQueueCapacity || settings.QueueCapacity > service.CloudflareIngressMaxQueueCapacity {
		settings.QueueCapacity = defaults.QueueCapacity
	}
	if settings.MaxActiveRules < service.CloudflareIngressMinMaxActiveRules || settings.MaxActiveRules > service.CloudflareIngressMaxMaxActiveRules {
		settings.MaxActiveRules = defaults.MaxActiveRules
	}
	if settings.ReconcileIntervalSeconds < service.CloudflareIngressMinReconcileIntervalSeconds || settings.ReconcileIntervalSeconds > service.CloudflareIngressMaxReconcileIntervalSeconds {
		settings.ReconcileIntervalSeconds = defaults.ReconcileIntervalSeconds
	}
	return settings
}

func cloudflareSettingsRevision(settings service.CloudflareIngressSettings) string {
	payload := fmt.Sprintf(
		"%t\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d\x00%d\x00%d\x00%d\x00%d\x00%d",
		settings.Enabled,
		settings.Mode,
		settings.ZoneID,
		settings.APIToken,
		strings.Join(settings.WAFHostnames, ","),
		strings.Join(settings.WAFRuleIDs, ","),
		settings.WAFSyncIntervalSeconds,
		settings.AnalyticsIntervalSeconds,
		settings.RequestTimeoutSeconds,
		settings.QueueCapacity,
		settings.MaxActiveRules,
		settings.ReconcileIntervalSeconds,
	)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
}

func (b *cloudflareIngressBlocker) buildRuntime(settings service.CloudflareIngressSettings) cloudflareIngressRuntime {
	settings = normalizeCloudflareRuntimeSettings(settings)
	runtime := cloudflareIngressRuntime{
		settings: settings,
		revision: cloudflareSettingsRevision(settings),
	}
	if settings.Mode == service.CloudflareIngressModeWAFCustomRules {
		runtime.wafClient = b.wafClientBuild(settings)
	} else {
		runtime.client = b.clientBuild(settings)
	}
	return runtime
}

func cloudflareRuntimeConfigured(runtime cloudflareIngressRuntime, state *cloudflareWAFStateStore) bool {
	if runtime.settings.Mode == service.CloudflareIngressModeWAFCustomRules {
		return runtime.wafClient != nil && state != nil && len(runtime.settings.WAFHostnames) > 0 && len(runtime.settings.WAFRuleIDs) > 0
	}
	return runtime.client != nil
}

func cloudflareRuntimeBindingChanged(current, next service.CloudflareIngressSettings) bool {
	return current.Mode != next.Mode ||
		current.ZoneID != next.ZoneID ||
		current.APIToken != next.APIToken ||
		strings.Join(current.WAFHostnames, "\x00") != strings.Join(next.WAFHostnames, "\x00") ||
		strings.Join(current.WAFRuleIDs, "\x00") != strings.Join(next.WAFRuleIDs, "\x00")
}

func cloudflareWAFBindingRevision(settings service.CloudflareIngressSettings) string {
	payload := strings.Join([]string{
		settings.Mode,
		settings.ZoneID,
		strings.Join(settings.WAFHostnames, ","),
		strings.Join(settings.WAFRuleIDs, ","),
	}, "\x00")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
}

func cloudflareWAFExpressionRevision(binding string, expressions []string) string {
	payload := binding + "\x00" + strings.Join(expressions, "\x00")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
}

func cloudflareBlockTarget(clientKey string, expiresAt time.Time) (cloudflareBlockRequest, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(clientKey))
	if err != nil {
		return cloudflareBlockRequest{}, false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return cloudflareBlockRequest{}, false
	}
	target := "ip"
	value := addr.String()
	if addr.Is6() {
		target = "ip_range"
		value = netip.PrefixFrom(addr, 64).Masked().String()
	}
	return cloudflareBlockRequest{
		key: target + "\x00" + value, target: target, value: value, expiresAt: expiresAt,
	}, true
}

func cloudflareAccessRuleKey(configuration cloudflareAccessRuleConfiguration) (string, bool) {
	target := strings.ToLower(strings.TrimSpace(configuration.Target))
	value := strings.TrimSpace(configuration.Value)
	if target == "ip" {
		addr, err := netip.ParseAddr(value)
		if err != nil || !addr.Unmap().Is4() {
			return "", false
		}
		return target + "\x00" + addr.Unmap().String(), true
	}
	if target == "ip_range" {
		prefix, err := netip.ParsePrefix(value)
		if err != nil || !prefix.Addr().Is6() || prefix.Bits() != 64 {
			return "", false
		}
		return target + "\x00" + prefix.Masked().String(), true
	}
	return "", false
}

func cloudflareRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := min(attempt-1, 6)
	delay := time.Second * time.Duration(1<<shift)
	return min(delay, cloudflareRetryMaxDelay)
}
