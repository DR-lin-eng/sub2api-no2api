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
	identity  string
	listeners map[chan OpenAIUsagePushEvent]struct{}
	cancel    context.CancelFunc
	last      *OpenAIUsagePushEvent
	failedAt  time.Time
}

// SubscribeUsage shares one upstream connection per account in this process.
// The last observer releases it. A failed connection cools down for a minute;
// callers keep their existing HTTP/passive refresh behavior during that time.
func (s *OpenAIQuotaService) SubscribeUsage(ctx context.Context, accountID int64) (<-chan OpenAIUsagePushEvent, func(), error) {
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
		return nil, nil, fmt.Errorf("usage push is cooling down")
	}
	if stream == nil {
		if len(s.usagePushStreams) >= 256 {
			return nil, nil, fmt.Errorf("usage push capacity reached")
		}
		streamCtx, cancel := context.WithCancel(context.Background())
		stream = &openAIUsagePushStream{identity: identity, listeners: make(map[chan OpenAIUsagePushEvent]struct{}), cancel: cancel}
		s.usagePushStreams[accountID] = stream
		go s.runSharedUsagePush(streamCtx, accountID, stream)
	}
	if len(stream.listeners) >= 16 {
		return nil, nil, fmt.Errorf("too many usage observers")
	}
	channel := make(chan OpenAIUsagePushEvent, 1)
	stream.listeners[channel] = struct{}{}
	if stream.last != nil {
		channel <- *stream.last
	}
	release := func() {
		s.usagePushMu.Lock()
		defer s.usagePushMu.Unlock()
		if _, exists := stream.listeners[channel]; !exists {
			return
		}
		delete(stream.listeners, channel)
		if len(stream.listeners) == 0 {
			stream.cancel()
			if stream.failedAt.IsZero() && s.usagePushStreams[accountID] == stream {
				delete(s.usagePushStreams, accountID)
			}
		}
	}
	return channel, release, nil
}

func (s *OpenAIQuotaService) runSharedUsagePush(ctx context.Context, accountID int64, stream *openAIUsagePushStream) {
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
