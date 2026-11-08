package service

import (
	"context"
	"encoding/json"
	"time"
)

func (s *OpenAIQuotaService) observeManagedUsage(ctx context.Context, accountID int64) (<-chan OpenAIUsagePushEvent, func(), error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil || account.Status != StatusActive || !account.IsOpenAIOAuth() {
		return nil, nil, usageInvalid("usage push is limited to active OpenAI OAuth accounts")
	}
	s.usagePushMu.Lock()
	stream := s.usagePushStreams[accountID]
	if stream != nil && stream.failedAt.IsZero() && stream.background {
		if len(stream.listeners) >= 17 {
			s.usagePushMu.Unlock()
			return nil, nil, usageInvalid("too many usage observers")
		}
		channel := make(chan OpenAIUsagePushEvent, 1)
		stream.listeners[channel] = struct{}{}
		if stream.last != nil {
			channel <- *stream.last
		}
		s.usagePushMu.Unlock()
		return channel, s.usageObserverRelease(accountID, stream, channel), nil
	}
	s.usagePushMu.Unlock()
	// The elected background worker may live on another node. This reads only
	// our own database; it cannot trigger upstream quota or model requests.
	watchCtx, cancel := context.WithCancel(ctx)
	channel := make(chan OpenAIUsagePushEvent, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		last := ""
		for {
			if account == nil || account.Status != StatusActive || !account.IsOpenAIOAuth() {
				select {
				case <-channel:
				default:
				}
				channel <- OpenAIUsagePushEvent{Type: "fallback", Transport: "polling"}
				return
			}
			raw, _ := json.Marshal(account.Extra[openAIQuotaRateLimitSnapshotKey])
			if string(raw) != last {
				var quota OpenAIQuotaUsage
				if json.Unmarshal(raw, &quota) == nil && quota.FetchedAt > 0 {
					last = string(raw)
					select {
					case <-channel:
					default:
					}
					channel <- OpenAIUsagePushEvent{Type: "quota", Transport: "polling", Quota: &quota}
				}
			}
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
			}
			callCtx, stop := context.WithTimeout(watchCtx, 5*time.Second)
			account, err = s.accountRepo.GetByID(callCtx, accountID)
			stop()
			if err != nil {
				account = nil
			}
		}
	}()
	return channel, func() { cancel(); <-done }, nil
}
