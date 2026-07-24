package service

import (
	"context"
	"sync"

	"github.com/cespare/xxhash/v2"
)

const (
	openAIContentSessionTrackerShardCount         = 64
	openAIContentSessionBurstBalanceMaxSelections = 64
	openAIContentSessionBurstCandidateLimit       = 16
	openAIContentSessionLoadBalanceMaxParallel    = 64
	openAIContentSessionBurstCandidateAttempts    = 2
)

type openAIContentSessionTrackerKey struct {
	groupID     int64
	sessionHash string
}

type openAIContentSessionTrackerShard struct {
	mu     sync.Mutex
	active map[openAIContentSessionTrackerKey]*openAIContentSessionTrackerState
}

type openAIContentSessionTrackerState struct {
	requests       uint32
	selects        uint32
	candidates     [openAIContentSessionBurstCandidateLimit]int64
	candidateCount uint8
	nextCandidate  uint32
}

type openAIContentSessionTracker struct {
	shards          [openAIContentSessionTrackerShardCount]openAIContentSessionTrackerShard
	loadBalanceOnce sync.Once
	loadBalanceSem  chan struct{}
}

func (t *openAIContentSessionTracker) shard(groupID int64, sessionHash string) *openAIContentSessionTrackerShard {
	hash := xxhash.Sum64String(sessionHash) ^ uint64(groupID)
	return &t.shards[hash&(openAIContentSessionTrackerShardCount-1)]
}

func (t *openAIContentSessionTracker) begin(groupID int64, sessionHash string) (concurrent bool, overflow bool) {
	if t == nil || sessionHash == "" {
		return false, false
	}
	key := openAIContentSessionTrackerKey{groupID: groupID, sessionHash: sessionHash}
	shard := t.shard(groupID, sessionHash)
	shard.mu.Lock()
	if shard.active == nil {
		shard.active = make(map[openAIContentSessionTrackerKey]*openAIContentSessionTrackerState)
	}
	state := shard.active[key]
	if state == nil {
		state = &openAIContentSessionTrackerState{}
		shard.active[key] = state
	}
	concurrent = state.requests > 0 && state.selects < openAIContentSessionBurstBalanceMaxSelections
	overflow = state.requests > 0 && !concurrent
	state.requests++
	if concurrent {
		state.selects++
	}
	shard.mu.Unlock()
	return concurrent, overflow
}

// recordCandidate keeps the bounded account set selected during an active
// content-session burst. Overflow requests use this set instead of scanning
// the full account pool again.
func (t *openAIContentSessionTracker) recordCandidate(groupID int64, sessionHash string, accountID int64) {
	if t == nil || sessionHash == "" || accountID <= 0 {
		return
	}
	key := openAIContentSessionTrackerKey{groupID: groupID, sessionHash: sessionHash}
	shard := t.shard(groupID, sessionHash)
	shard.mu.Lock()
	state := shard.active[key]
	if state != nil {
		for i := 0; i < int(state.candidateCount); i++ {
			if state.candidates[i] == accountID {
				shard.mu.Unlock()
				return
			}
		}
		if state.candidateCount < openAIContentSessionBurstCandidateLimit {
			state.candidates[state.candidateCount] = accountID
			state.candidateCount++
		}
	}
	shard.mu.Unlock()
}

func (t *openAIContentSessionTracker) nextCandidates(groupID int64, sessionHash string) ([openAIContentSessionBurstCandidateAttempts]int64, int) {
	var candidates [openAIContentSessionBurstCandidateAttempts]int64
	if t == nil || sessionHash == "" {
		return candidates, 0
	}
	key := openAIContentSessionTrackerKey{groupID: groupID, sessionHash: sessionHash}
	shard := t.shard(groupID, sessionHash)
	shard.mu.Lock()
	state := shard.active[key]
	if state == nil || state.candidateCount == 0 {
		shard.mu.Unlock()
		return candidates, 0
	}
	count := min(int(state.candidateCount), len(candidates))
	start := int(state.nextCandidate % uint32(state.candidateCount))
	for i := 0; i < count; i++ {
		candidates[i] = state.candidates[(start+i)%int(state.candidateCount)]
	}
	// Advance one position per request so the two-account attempt window slides
	// across the ring instead of repeatedly pairing the same accounts.
	state.nextCandidate++
	shard.mu.Unlock()
	return candidates, count
}

func (t *openAIContentSessionTracker) dropCandidate(groupID int64, sessionHash string, accountID int64) {
	if t == nil || sessionHash == "" || accountID <= 0 {
		return
	}
	key := openAIContentSessionTrackerKey{groupID: groupID, sessionHash: sessionHash}
	shard := t.shard(groupID, sessionHash)
	shard.mu.Lock()
	state := shard.active[key]
	if state != nil {
		for i := 0; i < int(state.candidateCount); i++ {
			if state.candidates[i] != accountID {
				continue
			}
			copy(state.candidates[i:], state.candidates[i+1:state.candidateCount])
			state.candidateCount--
			state.candidates[state.candidateCount] = 0
			if state.candidateCount == 0 {
				state.nextCandidate = 0
			} else {
				state.nextCandidate %= uint32(state.candidateCount)
			}
			break
		}
	}
	shard.mu.Unlock()
}

func (t *openAIContentSessionTracker) acquireLoadBalance(ctx context.Context) (func(), error) {
	if t == nil {
		return func() {}, nil
	}
	t.loadBalanceOnce.Do(func() {
		t.loadBalanceSem = make(chan struct{}, openAIContentSessionLoadBalanceMaxParallel)
	})
	select {
	case t.loadBalanceSem <- struct{}{}:
		return func() { <-t.loadBalanceSem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *openAIContentSessionTracker) release(groupID int64, sessionHash string) {
	if t == nil || sessionHash == "" {
		return
	}
	key := openAIContentSessionTrackerKey{groupID: groupID, sessionHash: sessionHash}
	shard := t.shard(groupID, sessionHash)
	shard.mu.Lock()
	state := shard.active[key]
	if state == nil || state.requests <= 1 {
		delete(shard.active, key)
	} else {
		state.requests--
	}
	shard.mu.Unlock()
}

func (s *OpenAIGatewayService) openAIContentSessionTracker() *openAIContentSessionTracker {
	if s == nil {
		return nil
	}
	s.openaiContentSessionOnce.Do(func() {
		s.openaiContentSessions = &openAIContentSessionTracker{}
	})
	return s.openaiContentSessions
}

func (s *OpenAIGatewayService) beginOpenAIContentSessionRequest(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
) (tracked bool, concurrent bool, overflow bool) {
	if sessionHash == "" || !s.isOpenAIContentSessionBurstBalanceEnabled(ctx) {
		return false, false, false
	}
	tracker := s.openAIContentSessionTracker()
	if tracker == nil {
		return false, false, false
	}
	concurrent, overflow = tracker.begin(derefGroupID(groupID), sessionHash)
	return true, concurrent, overflow
}

func (s *OpenAIGatewayService) acquireOpenAIContentSessionLoadBalance(ctx context.Context) (func(), error) {
	tracker := s.openAIContentSessionTracker()
	if tracker == nil {
		return func() {}, nil
	}
	return tracker.acquireLoadBalance(ctx)
}

func (s *OpenAIGatewayService) recordOpenAIContentSessionCandidate(ctx context.Context, groupID *int64, sessionHash string, accountID int64) {
	if s == nil || !openAIContentSessionRequestTracked(ctx) {
		return
	}
	if tracker := s.openaiContentSessions; tracker != nil {
		tracker.recordCandidate(derefGroupID(groupID), sessionHash, accountID)
	}
}

func (s *OpenAIGatewayService) nextOpenAIContentSessionBurstCandidates(ctx context.Context, groupID *int64, sessionHash string) ([openAIContentSessionBurstCandidateAttempts]int64, int) {
	var candidates [openAIContentSessionBurstCandidateAttempts]int64
	if s == nil || !openAIContentSessionRequestOverflow(ctx) {
		return candidates, 0
	}
	if tracker := s.openaiContentSessions; tracker != nil {
		return tracker.nextCandidates(derefGroupID(groupID), sessionHash)
	}
	return candidates, 0
}

func (s *OpenAIGatewayService) dropOpenAIContentSessionCandidate(ctx context.Context, groupID *int64, sessionHash string, accountID int64) {
	if s == nil || !openAIContentSessionRequestTracked(ctx) {
		return
	}
	if tracker := s.openaiContentSessions; tracker != nil {
		tracker.dropCandidate(derefGroupID(groupID), sessionHash, accountID)
	}
}

// ReleaseOpenAIContentSessionRequest releases request-lifetime tracking started
// by GenerateSessionHashForRequest. Explicit sessions and disabled tracking are no-ops.
func (s *OpenAIGatewayService) ReleaseOpenAIContentSessionRequest(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
) {
	if s == nil || !openAIContentSessionRequestTracked(ctx) {
		return
	}
	if tracker := s.openaiContentSessions; tracker != nil {
		tracker.release(derefGroupID(groupID), sessionHash)
	}
}
