package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/logger"
	"go.uber.org/zap"
)

const (
	defaultOpenAIProxyStreamFailureThreshold  = 2
	defaultOpenAIProxyStreamFailureWindow     = time.Minute
	defaultOpenAIProxyStreamQuarantineTTL     = 10 * time.Minute
	defaultOpenAIProxyStreamCircuitMaxEntries = 4096
	defaultOpenAIProxyStreamFailureCollapse   = 3 * time.Second
	openAIProxyStreamFailOpenLogInterval      = 5 * time.Second
)

type openAIProxyStreamCircuitSettings struct {
	disabled         bool
	failureThreshold int
	failureWindow    time.Duration
	quarantineTTL    time.Duration
	collapseInterval time.Duration
	maxEntries       int
}

type openAIProxyStreamCircuitEntry struct {
	failureCount         int
	windowStart          time.Time
	blockedUntil         time.Time
	lastTouched          time.Time
	lastFailureStartedAt time.Time
	lastSuccessStartedAt time.Time
	lastFailureAt        time.Time
}

type openAIProxyStreamBlockedSnapshot struct {
	blockedUntilByProxy map[int64]time.Time
}

// openAIProxyStreamCircuit is an in-process, proxy-ID keyed circuit. It is
// intentionally bounded and ephemeral: a restart clears observations, while a
// tripped entry expires automatically after its TTL.
type openAIProxyStreamCircuit struct {
	mu              sync.Mutex
	settings        openAIProxyStreamCircuitSettings
	entries         map[int64]openAIProxyStreamCircuitEntry
	blockedSnapshot atomic.Pointer[openAIProxyStreamBlockedSnapshot]
}

func resolveOpenAIProxyStreamCircuitSettings(s *OpenAIGatewayService) openAIProxyStreamCircuitSettings {
	settings := openAIProxyStreamCircuitSettings{
		failureThreshold: defaultOpenAIProxyStreamFailureThreshold,
		failureWindow:    defaultOpenAIProxyStreamFailureWindow,
		quarantineTTL:    defaultOpenAIProxyStreamQuarantineTTL,
		collapseInterval: defaultOpenAIProxyStreamFailureCollapse,
		maxEntries:       defaultOpenAIProxyStreamCircuitMaxEntries,
	}
	if s == nil || s.cfg == nil {
		return settings
	}
	cfg := s.cfg.Gateway.OpenAIProxyStreamCircuit
	settings.disabled = cfg.Disabled
	if cfg.FailureThreshold > 0 {
		settings.failureThreshold = cfg.FailureThreshold
	}
	if cfg.WindowSeconds > 0 {
		settings.failureWindow = time.Duration(cfg.WindowSeconds) * time.Second
	}
	if cfg.TTLSeconds > 0 {
		settings.quarantineTTL = time.Duration(cfg.TTLSeconds) * time.Second
	}
	return settings
}

func (s *OpenAIGatewayService) openAIProxyStreamCircuitEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIProxyStreamCircuit.Enabled && !s.cfg.Gateway.OpenAIProxyStreamCircuit.Disabled
}

func newOpenAIProxyStreamCircuit(settings openAIProxyStreamCircuitSettings) *openAIProxyStreamCircuit {
	if settings.failureThreshold <= 0 {
		settings.failureThreshold = defaultOpenAIProxyStreamFailureThreshold
	}
	if settings.failureWindow <= 0 {
		settings.failureWindow = defaultOpenAIProxyStreamFailureWindow
	}
	if settings.quarantineTTL <= 0 {
		settings.quarantineTTL = defaultOpenAIProxyStreamQuarantineTTL
	}
	if settings.maxEntries <= 0 {
		settings.maxEntries = defaultOpenAIProxyStreamCircuitMaxEntries
	}
	if settings.collapseInterval < 0 {
		settings.collapseInterval = 0
	}
	return &openAIProxyStreamCircuit{
		settings: settings,
		entries:  make(map[int64]openAIProxyStreamCircuitEntry),
	}
}

func (s *OpenAIGatewayService) getOpenAIProxyStreamCircuit() *openAIProxyStreamCircuit {
	if !s.openAIProxyStreamCircuitEnabled() {
		return nil
	}
	s.openaiProxyStreamCircuitOnce.Do(func() {
		if s.openaiProxyStreamCircuit == nil {
			s.openaiProxyStreamCircuit = newOpenAIProxyStreamCircuit(resolveOpenAIProxyStreamCircuitSettings(s))
		}
	})
	return s.openaiProxyStreamCircuit
}

func (c *openAIProxyStreamCircuit) recordFailure(proxyID int64, requestStartedAt, observedAt time.Time) (bool, time.Time) {
	if c == nil || c.settings.disabled || proxyID <= 0 {
		return false, time.Time{}
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	if requestStartedAt.IsZero() {
		requestStartedAt = observedAt
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[proxyID]
	if exists && requestStartedAt.Before(entry.lastSuccessStartedAt) {
		return false, entry.blockedUntil
	}
	if exists && requestStartedAt.Before(entry.lastFailureStartedAt) {
		return false, entry.blockedUntil
	}
	if exists && observedAt.Before(entry.blockedUntil) {
		entry.lastFailureStartedAt = requestStartedAt
		entry.lastTouched = observedAt
		c.entries[proxyID] = entry
		return false, entry.blockedUntil
	}
	if !exists {
		if c.ensureCapacityLocked(observedAt) {
			c.publishBlockedSnapshotLocked(observedAt)
		}
	}
	if entry.windowStart.IsZero() || observedAt.Before(entry.windowStart) || observedAt.Sub(entry.windowStart) > c.settings.failureWindow {
		entry.failureCount = 0
		entry.windowStart = observedAt
		entry.blockedUntil = time.Time{}
	}
	if c.settings.collapseInterval > 0 && !entry.lastFailureAt.IsZero() &&
		!observedAt.Before(entry.lastFailureAt) && observedAt.Sub(entry.lastFailureAt) < c.settings.collapseInterval {
		entry.lastTouched = observedAt
		c.entries[proxyID] = entry
		return false, entry.blockedUntil
	}
	entry.failureCount++
	entry.lastFailureStartedAt = requestStartedAt
	entry.lastFailureAt = observedAt
	entry.lastTouched = observedAt
	tripped := entry.failureCount >= c.settings.failureThreshold
	if tripped {
		entry.blockedUntil = observedAt.Add(c.settings.quarantineTTL)
	}
	c.entries[proxyID] = entry
	if tripped {
		c.publishBlockedSnapshotLocked(observedAt)
	}
	return tripped, entry.blockedUntil
}

func (c *openAIProxyStreamCircuit) recordSuccess(proxyID int64, requestStartedAt, observedAt time.Time) bool {
	if c == nil || proxyID <= 0 {
		return false
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	if requestStartedAt.IsZero() {
		requestStartedAt = observedAt
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, exists := c.entries[proxyID]
	if exists && requestStartedAt.Before(entry.lastFailureStartedAt) {
		return false
	}
	if exists && requestStartedAt.Before(entry.lastSuccessStartedAt) {
		return false
	}
	if !exists {
		if c.ensureCapacityLocked(observedAt) {
			c.publishBlockedSnapshotLocked(observedAt)
		}
	}
	wasBlocked := !entry.blockedUntil.IsZero() && observedAt.Before(entry.blockedUntil)
	entry.failureCount = 0
	entry.windowStart = time.Time{}
	entry.blockedUntil = time.Time{}
	entry.lastSuccessStartedAt = requestStartedAt
	entry.lastTouched = observedAt
	c.entries[proxyID] = entry
	if wasBlocked {
		c.publishBlockedSnapshotLocked(observedAt)
	}
	return exists
}

func (c *openAIProxyStreamCircuit) isBlocked(proxyID int64, now time.Time) bool {
	if c == nil || c.settings.disabled || proxyID <= 0 {
		return false
	}
	snapshot := c.blockedSnapshot.Load()
	if snapshot == nil {
		return false
	}
	blockedUntil, ok := snapshot.blockedUntilByProxy[proxyID]
	return ok && now.Before(blockedUntil)
}

func (c *openAIProxyStreamCircuit) activeBlockCount(now time.Time) int {
	if c == nil || c.settings.disabled {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, entry := range c.entries {
		if !entry.blockedUntil.IsZero() && now.Before(entry.blockedUntil) {
			count++
		}
	}
	return count
}

func (c *openAIProxyStreamCircuit) publishBlockedSnapshotLocked(now time.Time) {
	blocked := make(map[int64]time.Time)
	for proxyID, entry := range c.entries {
		if !entry.blockedUntil.IsZero() && now.Before(entry.blockedUntil) {
			blocked[proxyID] = entry.blockedUntil
		}
	}
	if len(blocked) == 0 {
		c.blockedSnapshot.Store(nil)
		return
	}
	c.blockedSnapshot.Store(&openAIProxyStreamBlockedSnapshot{blockedUntilByProxy: blocked})
}

func (c *openAIProxyStreamCircuit) ensureCapacityLocked(now time.Time) bool {
	if len(c.entries) < c.settings.maxEntries {
		return false
	}
	changedBlockedSnapshot := false
	for proxyID, entry := range c.entries {
		staleObservation := entry.blockedUntil.IsZero() && now.Sub(entry.lastTouched) > c.settings.failureWindow
		expiredQuarantine := !entry.blockedUntil.IsZero() && !now.Before(entry.blockedUntil)
		if staleObservation || expiredQuarantine {
			changedBlockedSnapshot = changedBlockedSnapshot || !entry.blockedUntil.IsZero()
			delete(c.entries, proxyID)
		}
	}
	if len(c.entries) < c.settings.maxEntries {
		return changedBlockedSnapshot
	}
	var oldestProxyID int64
	var oldest time.Time
	for proxyID, entry := range c.entries {
		if oldestProxyID == 0 || entry.lastTouched.Before(oldest) {
			oldestProxyID = proxyID
			oldest = entry.lastTouched
		}
	}
	if oldestProxyID > 0 {
		changedBlockedSnapshot = changedBlockedSnapshot || !c.entries[oldestProxyID].blockedUntil.IsZero()
		delete(c.entries, oldestProxyID)
	}
	return changedBlockedSnapshot
}

func openAIProxyStreamCircuitProxyID(account *Account) (int64, bool) {
	if account == nil || account.Platform != PlatformOpenAI || account.ProxyID == nil || *account.ProxyID <= 0 {
		return 0, false
	}
	return *account.ProxyID, true
}

func (s *OpenAIGatewayService) recordOpenAIProxyStreamDisconnect(account *Account, streamErr error, upstreamRequestID string, requestStartedAt time.Time) {
	proxyID, ok := openAIProxyStreamCircuitProxyID(account)
	if !ok || !s.openAIProxyStreamCircuitEnabled() || streamErr == nil || errors.Is(streamErr, context.Canceled) || errors.Is(streamErr, context.DeadlineExceeded) {
		return
	}
	circuit := s.getOpenAIProxyStreamCircuit()
	tripped, until := circuit.recordFailure(proxyID, requestStartedAt, time.Now())
	if !tripped {
		return
	}
	logger.L().With(zap.String("component", "service.openai_gateway")).Warn(
		"openai.proxy_quarantined_stream_disconnect",
		zap.Int64("proxy_id", proxyID),
		zap.Int64("account_id", account.ID),
		zap.Time("until", until),
		zap.String("upstream_request_id", upstreamRequestID),
		zap.String("error", sanitizeUpstreamErrorMessage(streamErr.Error())),
	)
}

func (s *OpenAIGatewayService) clearOpenAIProxyStreamDisconnect(account *Account, requestStartedAt time.Time) {
	proxyID, ok := openAIProxyStreamCircuitProxyID(account)
	if !ok || !s.openAIProxyStreamCircuitEnabled() {
		return
	}
	if circuit := s.getOpenAIProxyStreamCircuit(); circuit != nil {
		circuit.recordSuccess(proxyID, requestStartedAt, time.Now())
	}
}

type openAIProxyStreamQuarantineBypassKey struct{}

func withOpenAIProxyStreamQuarantineBypass(ctx context.Context) context.Context {
	return context.WithValue(ctx, openAIProxyStreamQuarantineBypassKey{}, true)
}

func openAIProxyStreamQuarantineBypassed(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	bypassed, _ := ctx.Value(openAIProxyStreamQuarantineBypassKey{}).(bool)
	return bypassed
}

func (s *OpenAIGatewayService) isOpenAIProxyStreamQuarantined(ctx context.Context, account *Account) bool {
	proxyID, ok := openAIProxyStreamCircuitProxyID(account)
	if !ok {
		return false
	}
	if !s.openAIProxyStreamCircuitEnabled() {
		return false
	}
	if openAIProxyStreamQuarantineBypassed(ctx) {
		return false
	}
	circuit := s.getOpenAIProxyStreamCircuit()
	return circuit != nil && circuit.isBlocked(proxyID, time.Now())
}

func (s *OpenAIGatewayService) logOpenAIProxyStreamQuarantineFailOpen(requestedModel string, blockedProxies int) {
	now := time.Now().UnixNano()
	last := s.openaiProxyStreamFailOpenLogAt.Load()
	if now-last < int64(openAIProxyStreamFailOpenLogInterval) ||
		!s.openaiProxyStreamFailOpenLogAt.CompareAndSwap(last, now) {
		return
	}
	logger.L().With(zap.String("component", "service.openai_gateway")).Warn(
		"openai.proxy_stream_quarantine_fail_open",
		zap.Int("blocked_proxies", blockedProxies),
		zap.String("model", requestedModel),
	)
}
