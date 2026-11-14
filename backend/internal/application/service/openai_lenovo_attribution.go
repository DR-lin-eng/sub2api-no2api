package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/lenovoattribution"
	"github.com/google/uuid"
)

const openAILenovoAttributionTimeout = 15 * time.Second
const openAILenovoAttributionWorkers = 8
const openAILenovoAttributionCacheLimit = 4096

// OpenAILenovoAttributionRepository provides account/credential/state CAS. The
// reservation is durable before any network request, so replicas and restarts
// share the installation UUID, response result, cooldown and in-flight lease.
type OpenAILenovoAttributionRepository interface {
	CompareAndSwapOpenAILenovoAttribution(context.Context, *Account, any, *lenovoattribution.State) (bool, error)
}

type openAILenovoAttributionLocal struct {
	binding string
	until   time.Time
	active  bool
}

func openAILenovoAttributionEligible(account *Account) bool {
	return account != nil && account.ID > 0 && account.IsOpenAIOAuth() &&
		!account.IsShadow() && !account.IsOpenAIPersonalAccessToken() &&
		!account.IsOpenAIAgentIdentity() && !account.IsOpenAISIWC() &&
		account.Status == StatusActive && strings.TrimSpace(account.GetChatGPTAccountID()) != "" &&
		strings.TrimSpace(account.GetChatGPTUserID()) != ""
}

func openAILenovoAttributionBinding(account *Account) (string, string) {
	digest := strings.TrimSpace(account.GetExtraString(lenovoattribution.DigestExtraKey))
	if _, err := hex.DecodeString(digest); err != nil || len(digest) != sha256.Size*2 {
		digest = ""
	}
	digest = strings.ToLower(digest)
	// A local account is an independent installation, even when two imports
	// share a workspace or a virtual Codex client namespace.
	raw, _ := json.Marshal([]any{account.ID, account.GetChatGPTAccountID(), account.GetChatGPTUserID(), digest})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), digest
}

func (s *OpenAIGatewayService) scheduleOpenAILenovoAttribution(account *Account) {
	if s == nil || !openAILenovoAttributionEligible(account) || s.httpUpstream == nil || s.accountRepo == nil ||
		s.settingService == nil || !s.settingService.OpenAILenovoAttributionEnabled(context.Background()) {
		return
	}
	if _, ok := s.accountRepo.(OpenAILenovoAttributionRepository); !ok {
		return
	}
	binding, _ := openAILenovoAttributionBinding(account)
	now := time.Now()
	s.openAILenovoAttributionMu.Lock()
	if old, ok := s.openAILenovoAttribution[account.ID]; ok && (old.active || old.binding == binding && now.Before(old.until)) {
		s.openAILenovoAttributionMu.Unlock()
		return
	}
	if s.openAILenovoAttributionActive >= openAILenovoAttributionWorkers {
		s.openAILenovoAttributionMu.Unlock()
		return
	}
	if s.openAILenovoAttribution == nil {
		s.openAILenovoAttribution = make(map[int64]openAILenovoAttributionLocal)
	}
	if len(s.openAILenovoAttribution) >= openAILenovoAttributionCacheLimit {
		for id, old := range s.openAILenovoAttribution {
			if !old.active && !now.Before(old.until) {
				delete(s.openAILenovoAttribution, id)
			}
		}
		if len(s.openAILenovoAttribution) >= openAILenovoAttributionCacheLimit {
			// Durable state is authoritative. Evict a cached idle account rather
			// than preventing a larger account pool from ever being processed.
			for id, old := range s.openAILenovoAttribution {
				if !old.active {
					delete(s.openAILenovoAttribution, id)
					break
				}
			}
		}
	}
	s.openAILenovoAttribution[account.ID] = openAILenovoAttributionLocal{binding: binding, active: true}
	s.openAILenovoAttributionActive++
	s.openAILenovoAttributionMu.Unlock()
	id := account.ID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		result, err := s.fetchOpenAILenovoAttribution(ctx, id, binding)
		until := time.Now().Add(time.Minute)
		if result != nil && !result.Terminal {
			until = result.NextRetryAt
			if result.LeaseUntil.After(until) {
				until = result.LeaseUntil
			}
		}
		s.openAILenovoAttributionMu.Lock()
		s.openAILenovoAttributionActive--
		s.openAILenovoAttribution[id] = openAILenovoAttributionLocal{binding: binding, until: until}
		s.openAILenovoAttributionMu.Unlock()
		if err != nil {
			slog.Warn("openai_lenovo_attribution_incomplete", "account_id", id, "reason", "request_or_persistence_failed")
		}
	}()
}

func (s *OpenAIGatewayService) fetchOpenAILenovoAttribution(ctx context.Context, id int64, expectedBinding string) (*lenovoattribution.State, error) {
	if s.settingService == nil || !s.settingService.OpenAILenovoAttributionEnabled(ctx) {
		return nil, nil
	}
	repo, ok := s.accountRepo.(OpenAILenovoAttributionRepository)
	if !ok {
		return nil, errors.New("lenovo attribution persistence is unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !openAILenovoAttributionEligible(account) {
		return nil, nil
	}
	binding, digest := openAILenovoAttributionBinding(account)
	if binding != expectedBinding {
		return nil, newOpenAIOAuthPrincipalChangedError()
	}
	_, official, err := s.openAIOAuthCodexTargetURLWithContext(ctx, account)
	if err != nil || !official {
		return nil, err
	}
	token := account.GetOpenAIAccessToken()
	if token == "" {
		return nil, errors.New("OpenAI OAuth token is unavailable")
	}
	if err := validateOpenAIOAuthRequestPrincipal(account, token); err != nil {
		return nil, err
	}
	state, err := lenovoattribution.Read(account.Extra)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if state != nil && !state.Due(binding, now) {
		return state, nil
	}
	claim := lenovoattribution.State{Version: 1, Binding: binding, InstallationID: uuid.NewString()}
	if state != nil {
		claim.InstallationID = state.InstallationID
		if state.Binding == binding {
			claim = *state
		}
	}
	claim.LeaseID, claim.LeaseUntil = uuid.NewString(), now.Add(lenovoattribution.LeaseDuration).UTC()
	acquired, err := repo.CompareAndSwapOpenAILenovoAttribution(ctx, account, account.Extra[lenovoattribution.ExtraKey], &claim)
	if err != nil || !acquired {
		return nil, err
	}
	account.Extra = maps.Clone(account.Extra)
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	account.Extra[lenovoattribution.ExtraKey] = &claim
	body, _ := json.Marshal(lenovoattribution.Payload{InstallationID: claim.InstallationID, DeviceSerialDigest: digest})
	requestCtx, cancel := context.WithTimeout(ctx, openAILenovoAttributionTimeout)
	defer cancel()
	requestCtx = WithHTTPUpstreamRedirectsDisabled(requestCtx)
	requestCtx = WithHTTPUpstreamProfile(requestCtx, HTTPUpstreamProfileOpenAI)
	// Recheck the current principal and runtime opt-in after reservation and
	// immediately before transport. No token refresh or model retry is triggered.
	requestCtx = withOpenAIRequestCurrentCheck(requestCtx, func(checkCtx context.Context) error {
		if !s.settingService.OpenAILenovoAttributionEnabled(checkCtx) {
			return context.Canceled
		}
		current, err := s.accountRepo.GetByID(checkCtx, id)
		if err != nil {
			return err
		}
		if !openAILenovoAttributionEligible(current) {
			return newOpenAIOAuthPrincipalChangedError()
		}
		if err := validateOpenAIOAuthRefreshPrincipal(account, current); err != nil {
			return err
		}
		currentBinding, _ := openAILenovoAttributionBinding(current)
		if currentBinding != binding || resolveAccountProxyURL(account) != resolveAccountProxyURL(current) {
			return newOpenAIOAuthPrincipalChangedError()
		}
		return validateOpenAIOAuthRequestPrincipal(current, token)
	})
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, lenovoattribution.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	setOpenAIChatGPTAccountHeaders(req.Header, account)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// The gateway explicitly opts into the installation-only App request; it
	// does not assert Windows hardware provenance or manufacture a serial digest.
	applyOpenAIChatGPTDesktopSurfaceHeaders(req.Header)
	req = req.WithContext(WithHTTPUpstreamTLSProfile(req.Context(), s.resolveTLSProfile(account)))
	// Attribution has its own App wire contract, outside the model proof and
	// legacy integrity projection. Keep account egress/TLS while performing the
	// request-current check immediately before this dedicated transport call.
	var resp *http.Response
	sendErr := checkOpenAIRequestCurrent(req.Context())
	if sendErr == nil {
		resp, sendErr = doAccountHTTPUpstream(s.httpUpstream, req, resolveAccountProxyURL(account), account)
	}
	status, headers := 0, http.Header{}
	if resp != nil {
		status, headers = resp.StatusCode, resp.Header
		if resp.Body != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))
			_ = resp.Body.Close()
		}
	}
	if sendErr != nil {
		status, headers = 0, nil
	}
	completed := claim.Complete(status, headers, digest != "", time.Now())
	// Network cancellation may have exhausted the request budget. Reserve a
	// separate short persistence budget; compare both lease and credentials so a
	// late response cannot replace another worker's state or a new principal.
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer saveCancel()
	saved, saveErr := repo.CompareAndSwapOpenAILenovoAttribution(saveCtx, account, &claim, &completed)
	if saveErr != nil {
		return nil, saveErr
	}
	if !saved {
		return nil, newOpenAIOAuthPrincipalChangedError()
	}
	if sendErr != nil {
		return &completed, errors.New("lenovo attribution transport failed")
	}
	if status != http.StatusNoContent {
		return &completed, fmt.Errorf("lenovo attribution HTTP status %d", status)
	}
	return &completed, nil
}
