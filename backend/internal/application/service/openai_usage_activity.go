package service

import (
	"net/http"
	"strings"
	"time"
)

func isOpenAIUsageModelCall(req *http.Request, account *Account) bool {
	if req == nil || req.URL == nil || req.Method != http.MethodPost || account == nil || !account.IsOpenAIOAuth() {
		return false
	}
	if isOpenAIOAuthGatewayModelRequest(req, account) {
		return true
	}
	// Account-specific relays can replace the backend-api/codex prefix. Keep
	// model sends identifiable without treating quota/reset/profile POSTs as use.
	path := strings.TrimSuffix(req.URL.Path, "/")
	for _, suffix := range []string{"/responses", "/responses/compact", "/responses/input_tokens", "/chat/completions", "/images/generations", "/images/edits", "/embeddings"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

// NotifyAccountUsed is a memory-only signal from the actual model send path.
// Quota reads, model catalogs, page visits and usage-push heartbeats never call
// it, so an idle account cannot keep its own subscription alive indefinitely.
func (s *OpenAIUsageSyncService) NotifyAccountUsed(account *Account) {
	if s == nil || account == nil || account.ID <= 0 || !account.IsOpenAIOAuth() || account.Status != StatusActive {
		return
	}
	now := time.Now()
	s.activityMu.Lock()
	previous := s.activity[account.ID]
	if len(s.activity) >= 32768 {
		for id, used := range s.activity {
			if now.Sub(used) >= s.opts.idleWindow {
				delete(s.activity, id)
			}
		}
	}
	if len(s.activity) < 32768 || !previous.IsZero() {
		s.activity[account.ID] = now
	}
	s.activityMu.Unlock()
	if previous.IsZero() || now.Sub(previous) >= s.opts.idleWindow {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

func (s *OpenAIUsageSyncService) lastActivity(id int64, persisted time.Time) time.Time {
	s.activityMu.Lock()
	defer s.activityMu.Unlock()
	used := s.activity[id]
	if !used.IsZero() && time.Since(used) >= s.opts.idleWindow {
		delete(s.activity, id)
	}
	if used.After(persisted) {
		return used
	}
	return persisted
}

func (s *OpenAIGatewayService) notifyOpenAIUsageActivity(account *Account) {
	// Native/passthrough WS turns can reuse an existing handshake. Activity
	// must also reach the independent attribution scheduler after a runtime
	// toggle or a persisted next-check time expires.
	if s != nil {
		s.scheduleOpenAILenovoAttribution(account)
	}
	if s == nil || s.usageSync == nil || account == nil || !account.IsOpenAIOAuth() || account.Status != StatusActive {
		return
	}
	s.usageSync.NotifyAccountUsed(account)
	// Reuse the existing bounded-time batched last-used writer. This makes
	// calls handled by another node visible to the elected background owner.
	if s.deferredService != nil {
		s.deferredService.ScheduleLastUsedUpdate(account.ID)
	}
}
