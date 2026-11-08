package service

import (
	"context"
)

func (s *OpenAIUsageSyncService) runAccount(ctx context.Context, id int64, permits chan struct{}) {
	delay := s.opts.retryMin
	for ctx.Err() == nil {
		select {
		case permits <- struct{}{}:
		case <-ctx.Done():
			return
		}
		held := true
		releasePermit := func() {
			if held {
				<-permits
				held = false
			}
		}
		account, err := s.quota.accountRepo.GetByID(ctx, id)
		if err != nil {
			releasePermit()
			if !waitUsageSync(ctx, delay) {
				return
			}
			delay = min(delay*2, s.opts.retryMax)
			continue
		}
		if account == nil || account.Status != StatusActive || !account.IsOpenAIOAuth() {
			releasePermit()
			return
		}
		events, release, err := s.quota.subscribeUsage(ctx, id, true)
		if err == nil {
			s.quota.usagePushMu.Lock()
			stream := s.quota.usagePushStreams[id]
			s.quota.usagePushMu.Unlock()
			observing := true
			for observing {
				select {
				case <-ctx.Done():
					observing = false
				case event := <-events:
					if event.Transport == "websocket" {
						releasePermit()
						delay = s.opts.retryMin
					}
					if event.Type == "fallback" {
						observing = false
					}
				}
			}
			release()
			if stream != nil && stream.done != nil {
				stream.cancel()
				<-stream.done
			}
		}
		releasePermit()
		if !waitUsageSync(ctx, delay) {
			return
		}
		delay = min(delay*2, s.opts.retryMax)
	}
}
