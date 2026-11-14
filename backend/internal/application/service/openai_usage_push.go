package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// Only sanitized quota data crosses the admin socket. Signed upstream socket
// URLs and authentication headers stay inside the service.
type OpenAIUsagePushEvent struct {
	Type      string            `json:"type"`
	Transport string            `json:"transport"`
	Quota     *OpenAIQuotaUsage `json:"quota,omitempty"`
}

type openAIUsagePushStream struct {
	identity   string
	listeners  map[chan OpenAIUsagePushEvent]struct{}
	cancel     context.CancelFunc
	last       *OpenAIUsagePushEvent
	failedAt   time.Time
	done       chan struct{}
	background bool
}

// SubscribeUsage observes the background-owned stream. Production browsers
// never start or stop upstream WS connections. Non-owner nodes read persisted
// snapshots so viewing an account cannot create a second upstream subscription.
func (s *OpenAIQuotaService) SubscribeUsage(ctx context.Context, accountID int64) (<-chan OpenAIUsagePushEvent, func(), error) {
	if s != nil && s.usageSyncManaged.Load() {
		return s.observeManagedUsage(ctx, accountID)
	}
	return s.subscribeUsage(ctx, accountID, false)
}

func (s *OpenAIQuotaService) subscribeUsage(ctx context.Context, accountID int64, background bool) (<-chan OpenAIUsagePushEvent, func(), error) {
	if s == nil || s.accountRepo == nil {
		return nil, nil, usageInvalid("usage account is unavailable")
	}
	requested, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || requested == nil {
		return nil, nil, usageInvalid("usage account is unavailable")
	}
	if requested.Status != StatusActive {
		return nil, nil, usageInvalid("usage push is limited to active accounts")
	}
	account, err := s.quotaCredentialAccount(ctx, accountID)
	if err != nil {
		return nil, nil, err
	}
	if !account.IsOpenAIOAuth() {
		return nil, nil, usageInvalid("usage push requires an OpenAI OAuth account")
	}
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	encoded, err := json.Marshal(struct {
		Credentials map[string]any
		Route       any
		ParentID    int64
	}{account.Credentials, accountEgressRoute(account, proxyURL), account.ID})
	if err != nil {
		return nil, nil, fmt.Errorf("usage account identity unavailable")
	}
	identity := fmt.Sprintf("%x", sha256.Sum256(encoded))
	s.usagePushMu.Lock()
	defer s.usagePushMu.Unlock()
	if s.usagePushStreams == nil {
		s.usagePushStreams = make(map[int64]*openAIUsagePushStream)
	}
	for id, entry := range s.usagePushStreams {
		if len(entry.listeners) == 0 && !entry.failedAt.IsZero() && time.Since(entry.failedAt) >= time.Minute {
			delete(s.usagePushStreams, id)
		}
	}
	stream := s.usagePushStreams[accountID]
	if stream != nil && stream.identity != identity {
		stream.cancel()
		for listener := range stream.listeners {
			select {
			case <-listener:
			default:
			}
			listener <- OpenAIUsagePushEvent{Type: "fallback", Transport: "polling"}
		}
		delete(s.usagePushStreams, accountID)
		stream = nil
	}
	if stream != nil && !stream.failedAt.IsZero() {
		if !background {
			return nil, nil, fmt.Errorf("usage push is cooling down")
		}
		// Background workers own retry scheduling. A failed stream must be
		// replaced before the next attempt; otherwise the worker would attach
		// to a completed channel and never perform a new handshake.
		stream.cancel()
		delete(s.usagePushStreams, accountID)
		stream = nil
	}
	if stream == nil {
		if !background && len(s.usagePushStreams) >= 256 {
			return nil, nil, fmt.Errorf("usage push capacity reached")
		}
		streamCtx, cancel := context.WithCancel(ctx)
		stream = &openAIUsagePushStream{identity: identity, listeners: make(map[chan OpenAIUsagePushEvent]struct{}), cancel: cancel, done: make(chan struct{}), background: background}
		s.usagePushStreams[accountID] = stream
		go s.runSharedUsagePush(streamCtx, accountID, stream)
	}
	if len(stream.listeners) >= 17 {
		return nil, nil, fmt.Errorf("too many usage observers")
	}
	channel := make(chan OpenAIUsagePushEvent, 1)
	stream.listeners[channel] = struct{}{}
	if stream.last != nil {
		channel <- *stream.last
	}
	release := s.usageObserverRelease(accountID, stream, channel)
	return channel, release, nil
}

func (s *OpenAIQuotaService) usageObserverRelease(accountID int64, stream *openAIUsagePushStream, channel chan OpenAIUsagePushEvent) func() {
	return func() {
		s.usagePushMu.Lock()
		if _, exists := stream.listeners[channel]; !exists {
			s.usagePushMu.Unlock()
			return
		}
		delete(stream.listeners, channel)
		if len(stream.listeners) == 0 {
			if stream.background {
				s.usagePushMu.Unlock()
				return
			}
			stream.cancel()
			if stream.failedAt.IsZero() && s.usagePushStreams[accountID] == stream {
				delete(s.usagePushStreams, accountID)
			}
			done := stream.done
			s.usagePushMu.Unlock()
			if done != nil {
				<-done
			}
			return
		}
		s.usagePushMu.Unlock()
	}
}

func (s *OpenAIQuotaService) runSharedUsagePush(ctx context.Context, accountID int64, stream *openAIUsagePushStream) {
	defer func() {
		if ctx.Err() != nil {
			s.usagePushMu.Lock()
			if s.usagePushStreams[accountID] == stream {
				delete(s.usagePushStreams, accountID)
			}
			for listener := range stream.listeners {
				select {
				case <-listener:
				default:
				}
				listener <- OpenAIUsagePushEvent{Type: "fallback", Transport: "polling"}
			}
			s.usagePushMu.Unlock()
		}
		close(stream.done)
	}()
	publish := func(event OpenAIUsagePushEvent) {
		s.usagePushMu.Lock()
		defer s.usagePushMu.Unlock()
		if ctx.Err() != nil {
			return
		}
		stream.last = &event
		for listener := range stream.listeners {
			// This channel is a quota projection, never a billing queue. Slow
			// browsers need the newest snapshot, not every intermediate value.
			select {
			case <-listener:
			default:
			}
			listener <- event
		}
	}
	err := s.runUsagePush(ctx, accountID, publish)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		s.usagePushMu.Lock()
		stream.failedAt = time.Now()
		s.usagePushMu.Unlock()
		publish(OpenAIUsagePushEvent{Type: "fallback", Transport: "polling"})
	}
}
