package service

import (
	"context"
	"sync"

	"github.com/cespare/xxhash/v2"
)

const openAIContentSessionTrackerShardCount = 64

type openAIContentSessionTrackerKey struct {
	groupID     int64
	sessionHash string
}

type openAIContentSessionTrackerShard struct {
	mu     sync.Mutex
	active map[openAIContentSessionTrackerKey]uint32
}

type openAIContentSessionTracker struct {
	shards [openAIContentSessionTrackerShardCount]openAIContentSessionTrackerShard
}

func (t *openAIContentSessionTracker) shard(groupID int64, sessionHash string) *openAIContentSessionTrackerShard {
	hash := xxhash.Sum64String(sessionHash) ^ uint64(groupID)
	return &t.shards[hash&(openAIContentSessionTrackerShardCount-1)]
}

func (t *openAIContentSessionTracker) begin(groupID int64, sessionHash string) bool {
	if t == nil || sessionHash == "" {
		return false
	}
	key := openAIContentSessionTrackerKey{groupID: groupID, sessionHash: sessionHash}
	shard := t.shard(groupID, sessionHash)
	shard.mu.Lock()
	if shard.active == nil {
		shard.active = make(map[openAIContentSessionTrackerKey]uint32)
	}
	active := shard.active[key]
	shard.active[key] = active + 1
	shard.mu.Unlock()
	return active > 0
}

func (t *openAIContentSessionTracker) release(groupID int64, sessionHash string) {
	if t == nil || sessionHash == "" {
		return
	}
	key := openAIContentSessionTrackerKey{groupID: groupID, sessionHash: sessionHash}
	shard := t.shard(groupID, sessionHash)
	shard.mu.Lock()
	active := shard.active[key]
	if active <= 1 {
		delete(shard.active, key)
	} else {
		shard.active[key] = active - 1
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
) (tracked bool, concurrent bool) {
	if sessionHash == "" || !s.isOpenAIContentSessionBurstBalanceEnabled(ctx) {
		return false, false
	}
	tracker := s.openAIContentSessionTracker()
	if tracker == nil {
		return false, false
	}
	return true, tracker.begin(derefGroupID(groupID), sessionHash)
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
