package service

import (
	"context"
	"sort"
	"sync"
	"time"
)

const (
	openAIStreamDegradationMaxLevel   = 4
	openAIStreamDegradationMaxEntries = 10000
	openAIStreamDegradationTrimBatch  = 1000
	openAIStreamProbeLeasePadding     = 5 * time.Second
	openAIStreamCandidateTierProbe    = 0
	openAIStreamCandidateTierHealthy  = 1
	openAIStreamCandidateTierDegraded = 2
)

var openAIStreamDegradationProbeBackoff = [...]time.Duration{
	20 * time.Second,
	40 * time.Second,
	80 * time.Second,
	160 * time.Second,
}

// OpenAIStreamDegradationSnapshot is runtime-only scheduling state. It is
// exposed to the admin API but never persisted into the account record.
type OpenAIStreamDegradationSnapshot struct {
	Degraded            bool
	Level               int
	ConsecutiveTimeouts int
	DegradedSince       time.Time
	LastTimeoutAt       time.Time
	NextProbeAt         time.Time
}

type openAIStreamDegradationEntry struct {
	level               int
	consecutiveTimeouts int
	degradedSince       time.Time
	lastTimeoutAt       time.Time
	nextProbeAt         time.Time
	probeClaimedUntil   time.Time
	lastTouchedAt       time.Time
}

type openAIStreamDegradationState struct {
	mu      sync.RWMutex
	entries map[int64]openAIStreamDegradationEntry
}

func newOpenAIStreamDegradationState() *openAIStreamDegradationState {
	return &openAIStreamDegradationState{entries: make(map[int64]openAIStreamDegradationEntry)}
}

func (s *openAIStreamDegradationState) recordTimeout(accountID int64, now time.Time) OpenAIStreamDegradationSnapshot {
	if s == nil || accountID <= 0 {
		return OpenAIStreamDegradationSnapshot{}
	}
	if now.IsZero() {
		now = time.Now()
	}
	s.mu.Lock()
	entry := s.entries[accountID]
	if entry.level == 0 {
		entry.level = 1
		entry.degradedSince = now
	} else if entry.level < openAIStreamDegradationMaxLevel {
		entry.level++
	}
	entry.consecutiveTimeouts++
	entry.lastTimeoutAt = now
	entry.nextProbeAt = now.Add(openAIStreamDegradationProbeBackoff[entry.level-1])
	entry.probeClaimedUntil = time.Time{}
	entry.lastTouchedAt = now
	s.entries[accountID] = entry
	if len(s.entries) > openAIStreamDegradationMaxEntries {
		s.trimOldestLocked()
	}
	snapshot := openAIStreamDegradationSnapshot(entry)
	s.mu.Unlock()
	return snapshot
}

func (s *openAIStreamDegradationState) snapshot(accountID int64, now time.Time) (OpenAIStreamDegradationSnapshot, bool, bool) {
	if s == nil || accountID <= 0 {
		return OpenAIStreamDegradationSnapshot{}, false, false
	}
	if now.IsZero() {
		now = time.Now()
	}
	s.mu.RLock()
	entry, ok := s.entries[accountID]
	s.mu.RUnlock()
	if !ok || entry.level <= 0 {
		return OpenAIStreamDegradationSnapshot{}, false, false
	}
	probeDue := !now.Before(entry.nextProbeAt) && !now.Before(entry.probeClaimedUntil)
	return openAIStreamDegradationSnapshot(entry), true, probeDue
}

func (s *openAIStreamDegradationState) claimProbe(accountID int64, now time.Time, lease time.Duration) bool {
	if s == nil || accountID <= 0 {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	if lease <= 0 {
		lease = openAIStreamDegradationProbeBackoff[0] + openAIStreamProbeLeasePadding
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[accountID]
	if !ok || entry.level <= 0 || now.Before(entry.nextProbeAt) || now.Before(entry.probeClaimedUntil) {
		return false
	}
	entry.probeClaimedUntil = now.Add(lease)
	entry.lastTouchedAt = now
	s.entries[accountID] = entry
	return true
}

func (s *openAIStreamDegradationState) releaseProbe(accountID int64) {
	if s == nil || accountID <= 0 {
		return
	}
	s.mu.Lock()
	entry, ok := s.entries[accountID]
	if ok {
		entry.probeClaimedUntil = time.Time{}
		entry.lastTouchedAt = time.Now()
		s.entries[accountID] = entry
	}
	s.mu.Unlock()
}

func (s *openAIStreamDegradationState) clear(accountID int64) {
	if s == nil || accountID <= 0 {
		return
	}
	s.mu.Lock()
	delete(s.entries, accountID)
	s.mu.Unlock()
}

func (s *openAIStreamDegradationState) clearAll() {
	if s == nil {
		return
	}
	s.mu.Lock()
	clear(s.entries)
	s.mu.Unlock()
}

func (s *openAIStreamDegradationState) trimOldestLocked() {
	target := openAIStreamDegradationMaxEntries - openAIStreamDegradationTrimBatch
	for len(s.entries) > target {
		var oldestID int64
		var oldest time.Time
		for accountID, entry := range s.entries {
			if oldestID == 0 || entry.lastTouchedAt.Before(oldest) {
				oldestID = accountID
				oldest = entry.lastTouchedAt
			}
		}
		if oldestID == 0 {
			return
		}
		delete(s.entries, oldestID)
	}
}

func openAIStreamDegradationSnapshot(entry openAIStreamDegradationEntry) OpenAIStreamDegradationSnapshot {
	return OpenAIStreamDegradationSnapshot{
		Degraded:            entry.level > 0,
		Level:               entry.level,
		ConsecutiveTimeouts: entry.consecutiveTimeouts,
		DegradedSince:       entry.degradedSince,
		LastTimeoutAt:       entry.lastTimeoutAt,
		NextProbeAt:         entry.nextProbeAt,
	}
}

func (s *OpenAIGatewayService) getOpenAIStreamDegradationState() *openAIStreamDegradationState {
	if s == nil {
		return nil
	}
	s.openaiStreamDegradationOnce.Do(func() {
		if s.openaiStreamDegradation == nil {
			s.openaiStreamDegradation = newOpenAIStreamDegradationState()
		}
	})
	return s.openaiStreamDegradation
}

func (s *OpenAIGatewayService) isOpenAIStreamDegradationEnabled() bool {
	if s == nil || s.settingService == nil || s.settingService.IsStreamResponseHeaderTimeoutDegradationEnabled() {
		if s != nil {
			s.openaiStreamDegradationOff.Store(false)
		}
		return true
	}
	if s.openaiStreamDegradationOff.CompareAndSwap(false, true) {
		if state := s.getOpenAIStreamDegradationState(); state != nil {
			state.clearAll()
		}
	}
	return false
}

func (s *OpenAIGatewayService) recordOpenAIStreamResponseHeaderTimeout(accountID int64, now time.Time) OpenAIStreamDegradationSnapshot {
	if !s.isOpenAIStreamDegradationEnabled() {
		return OpenAIStreamDegradationSnapshot{}
	}
	state := s.getOpenAIStreamDegradationState()
	if state == nil {
		return OpenAIStreamDegradationSnapshot{}
	}
	return state.recordTimeout(accountID, now)
}

// SnapshotOpenAIStreamDegradation exposes runtime health to the admin account
// response without widening AccountRuntimeBlocker or persisting transient state.
func (s *OpenAIGatewayService) SnapshotOpenAIStreamDegradation(accountID int64) (OpenAIStreamDegradationSnapshot, bool) {
	if !s.isOpenAIStreamDegradationEnabled() {
		return OpenAIStreamDegradationSnapshot{}, false
	}
	state := s.getOpenAIStreamDegradationState()
	if state == nil {
		return OpenAIStreamDegradationSnapshot{}, false
	}
	snapshot, degraded, _ := state.snapshot(accountID, time.Now())
	return snapshot, degraded
}

func (s *OpenAIGatewayService) openAIStreamCandidateTier(accountID int64, now time.Time) (int, bool) {
	if !s.isOpenAIStreamDegradationEnabled() {
		return openAIStreamCandidateTierHealthy, false
	}
	state := s.getOpenAIStreamDegradationState()
	if state == nil {
		return openAIStreamCandidateTierHealthy, false
	}
	_, degraded, probeDue := state.snapshot(accountID, now)
	if !degraded {
		return openAIStreamCandidateTierHealthy, false
	}
	if probeDue {
		return openAIStreamCandidateTierProbe, true
	}
	return openAIStreamCandidateTierDegraded, true
}

func (s *OpenAIGatewayService) claimOpenAIStreamRecoveryProbe(accountID int64, now time.Time) bool {
	if !s.isOpenAIStreamDegradationEnabled() {
		return false
	}
	state := s.getOpenAIStreamDegradationState()
	if state == nil {
		return false
	}
	seconds := DefaultStreamResponseHeaderTimeoutSeconds
	if s.settingService != nil {
		seconds = s.settingService.GetStreamResponseHeaderTimeoutSeconds()
	}
	lease := time.Duration(seconds)*time.Second + openAIStreamProbeLeasePadding
	return state.claimProbe(accountID, now, lease)
}

func (s *OpenAIGatewayService) releaseOpenAIStreamRecoveryProbe(accountID int64) {
	if state := s.getOpenAIStreamDegradationState(); state != nil {
		state.releaseProbe(accountID)
	}
}

func (s *OpenAIGatewayService) clearOpenAIStreamDegradation(accountID int64) {
	if state := s.getOpenAIStreamDegradationState(); state != nil {
		state.clear(accountID)
	}
}

func (s *OpenAIGatewayService) hasHealthyOpenAIStreamAccount(ctx context.Context, accounts []*Account) bool {
	if !isOpenAIStreamScheduling(ctx) {
		return false
	}
	now := time.Now()
	for _, account := range accounts {
		if account == nil {
			continue
		}
		_, degraded := s.openAIStreamCandidateTier(account.ID, now)
		if !degraded {
			return true
		}
	}
	return false
}

func (s *OpenAIGatewayService) sortOpenAIStreamAccounts(ctx context.Context, accounts []*Account) {
	if !isOpenAIStreamScheduling(ctx) || len(accounts) < 2 {
		return
	}
	now := time.Now()
	sort.SliceStable(accounts, func(i, j int) bool {
		left, _ := s.openAIStreamCandidateTier(accounts[i].ID, now)
		right, _ := s.openAIStreamCandidateTier(accounts[j].ID, now)
		return left < right
	})
}

func (s *OpenAIGatewayService) tryAcquireOpenAIStreamAwareAccountSlot(ctx context.Context, account *Account, hasHealthy bool) (*AcquireResult, error) {
	if account == nil {
		return nil, nil
	}
	result, err := s.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency)
	if err != nil || result == nil || !result.Acquired || !isOpenAIStreamScheduling(ctx) {
		return result, err
	}
	tier, degraded := s.openAIStreamCandidateTier(account.ID, time.Now())
	if degraded && hasHealthy {
		if tier != openAIStreamCandidateTierProbe || !s.claimOpenAIStreamRecoveryProbe(account.ID, time.Now()) {
			if result.ReleaseFunc != nil {
				result.ReleaseFunc()
			}
			return nil, nil
		}
	}
	return result, nil
}

func (s *OpenAIGatewayService) shouldSkipOpenAIStreamWait(ctx context.Context, accountID int64, hasHealthy bool) bool {
	if !hasHealthy || !isOpenAIStreamScheduling(ctx) {
		return false
	}
	_, degraded := s.openAIStreamCandidateTier(accountID, time.Now())
	return degraded
}
