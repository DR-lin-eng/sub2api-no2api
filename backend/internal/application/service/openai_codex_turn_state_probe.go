package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
	"golang.org/x/net/http/httpguts"
)

const (
	openAICodexAutoProbeStaleAfter       = 45 * time.Minute
	openAICodexAutoProbeRetryInterval    = 5 * time.Minute
	openAICodexAutoProbeSweepInterval    = time.Minute
	openAICodexAutoProbeAttemptTimeout   = 15 * time.Second
	openAICodexAutoProbeMaxProxyAttempts = 4
	openAICodexAutoProbeMaxConcurrent    = 1
	openAICodexAutoProbeMaxTargets       = 4096
	openAICodexAutoProbeMaxEvents        = 64
	openAICodexAutoProbeLeaderLockTTL    = 2 * time.Minute
	openAICodexAutoProbeLockTimeout      = 2 * time.Second
)

type openAICodexAutoProbeTarget struct {
	Key           string
	AccountID     int64
	Model         string
	LastStateHash [32]byte
	MissingSince  time.Time
	LastHealthyAt time.Time
	NextProbeAt   time.Time
	InFlight      bool
}

func (s *OpenAIGatewayService) StartCodexTurnStateAutoProbe(parent context.Context) {
	if s == nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	s.codexAutoProbeOnce.Do(func() {
		ctx, cancel := context.WithCancel(parent)
		s.codexAutoProbeCancel = cancel
		s.codexAutoProbeWG.Add(1)
		go func() {
			defer s.codexAutoProbeWG.Done()
			ticker := time.NewTicker(openAICodexAutoProbeSweepInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case now := <-ticker.C:
					s.runOpenAICodexAutoProbeSweep(ctx, now)
				}
			}
		}()
	})
}

func (s *OpenAIGatewayService) noteOpenAICodexAutoProbeObservation(account *Account, model string, healthy bool, observedAt time.Time) {
	key := openAICodexAutoTurnStateKey(account, model)
	if s == nil || key == "" {
		return
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	s.codexAutoProbeMu.Lock()
	defer s.codexAutoProbeMu.Unlock()
	if s.codexAutoProbeTargets == nil {
		s.codexAutoProbeTargets = make(map[string]openAICodexAutoProbeTarget, 64)
	}
	target := s.codexAutoProbeTargets[key]
	target.Key = key
	target.AccountID = account.ID
	target.Model = strings.TrimSpace(model)
	if healthy {
		target.LastHealthyAt = observedAt
		target.MissingSince = time.Time{}
		target.NextProbeAt = observedAt.Add(openAICodexAutoProbeStaleAfter)
	} else {
		if target.MissingSince.IsZero() {
			target.MissingSince = observedAt
		}
		target.NextProbeAt = target.MissingSince.Add(openAICodexAutoProbeStaleAfter)
		if !target.LastHealthyAt.IsZero() {
			healthyDeadline := target.LastHealthyAt.Add(openAICodexAutoProbeStaleAfter)
			if healthyDeadline.Before(target.NextProbeAt) {
				target.NextProbeAt = healthyDeadline
			}
		}
	}
	ensureBindingCapacity(s.codexAutoProbeTargets, key, openAICodexAutoProbeMaxTargets)
	s.codexAutoProbeTargets[key] = target
}

func (s *OpenAIGatewayService) openAICodexAutoProbeStateSeen(account *Account, model, state string) bool {
	key := openAICodexAutoTurnStateKey(account, model)
	if s == nil || key == "" || state == "" {
		return false
	}
	s.codexAutoProbeMu.Lock()
	target, ok := s.codexAutoProbeTargets[key]
	s.codexAutoProbeMu.Unlock()
	return ok && !target.LastHealthyAt.IsZero() && target.LastStateHash == sha256.Sum256([]byte(state))
}

func (s *OpenAIGatewayService) noteOpenAICodexAutoProbeNewState(account *Account, model, state string, observedAt time.Time) {
	s.noteOpenAICodexAutoProbeObservation(account, model, true, observedAt)
	key := openAICodexAutoTurnStateKey(account, model)
	if s == nil || key == "" {
		return
	}
	s.codexAutoProbeMu.Lock()
	target := s.codexAutoProbeTargets[key]
	target.LastStateHash = sha256.Sum256([]byte(state))
	s.codexAutoProbeTargets[key] = target
	s.codexAutoProbeMu.Unlock()
}

func (s *OpenAIGatewayService) runOpenAICodexAutoProbeSweep(ctx context.Context, now time.Time) {
	if s == nil || s.settingService == nil {
		return
	}
	settings := s.settingService.CodexSimulationSettingsSnapshot(ctx)
	if !settings.TurnStateAutoReplayEnabled || len(settings.TurnStateWatchModels) == 0 {
		s.codexAutoProbeMu.Lock()
		clear(s.codexAutoProbeTargets)
		s.codexAutoProbeMu.Unlock()
		return
	}
	if now.IsZero() {
		now = time.Now()
	}

	due := make([]openAICodexAutoProbeTarget, 0, openAICodexAutoProbeMaxConcurrent)
	s.codexAutoProbeMu.Lock()
	for key, target := range s.codexAutoProbeTargets {
		if !codexTurnStateModelIsWatched(settings, target.Model) {
			delete(s.codexAutoProbeTargets, key)
			continue
		}
		if target.InFlight || target.NextProbeAt.IsZero() || now.Before(target.NextProbeAt) {
			continue
		}
		target.InFlight = true
		s.codexAutoProbeTargets[key] = target
		due = append(due, target)
		if len(due) >= openAICodexAutoProbeMaxConcurrent {
			break
		}
	}
	s.codexAutoProbeMu.Unlock()

	for _, target := range due {
		if ctx.Err() != nil || s.codexAutoProbeStopped.Load() {
			return
		}
		err := s.probeOpenAICodexTurnState(ctx, target)
		s.finishOpenAICodexAutoProbe(target.Key, err == nil, time.Now())
		if err != nil && ctx.Err() == nil {
			logger.L().Warn("codex automatic turn state probe failed",
				zap.Int64("account_id", target.AccountID),
				zap.String("upstream_model", target.Model),
				zap.Error(err),
			)
		}
	}
}

func (s *OpenAIGatewayService) finishOpenAICodexAutoProbe(key string, healthy bool, finishedAt time.Time) {
	s.codexAutoProbeMu.Lock()
	defer s.codexAutoProbeMu.Unlock()
	target, ok := s.codexAutoProbeTargets[key]
	if !ok {
		return
	}
	target.InFlight = false
	if healthy {
		target.LastHealthyAt = finishedAt
		target.MissingSince = time.Time{}
		target.NextProbeAt = finishedAt.Add(openAICodexAutoProbeStaleAfter)
	} else {
		target.NextProbeAt = finishedAt.Add(openAICodexAutoProbeRetryInterval)
	}
	s.codexAutoProbeTargets[key] = target
}

func (s *OpenAIGatewayService) probeOpenAICodexTurnState(ctx context.Context, target openAICodexAutoProbeTarget) error {
	if s == nil || s.accountRepo == nil || s.proxyRepo == nil {
		return errors.New("turn state probe dependencies are unavailable")
	}
	if s.codexAutoProbeLock != nil {
		lockSum := sha256.Sum256([]byte(target.Key))
		lockKey := "codex:turn-state-probe:" + hex.EncodeToString(lockSum[:])
		owner := uuid.NewString()
		lockCtx, cancel := context.WithTimeout(ctx, openAICodexAutoProbeLockTimeout)
		acquired, lockErr := s.codexAutoProbeLock.TryAcquireLeaderLock(lockCtx, lockKey, owner, openAICodexAutoProbeLeaderLockTTL)
		cancel()
		if lockErr != nil {
			return fmt.Errorf("acquire turn state probe lock: %w", lockErr)
		}
		if !acquired {
			return errors.New("turn state probe is already running on another instance")
		}
		defer func() {
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), openAICodexAutoProbeLockTimeout)
			defer releaseCancel()
			_ = s.codexAutoProbeLock.ReleaseLeaderLock(releaseCtx, lockKey, owner)
		}()
	}
	account, err := s.accountRepo.GetByID(ctx, target.AccountID)
	if err != nil {
		return fmt.Errorf("load account: %w", err)
	}
	if account == nil || !account.IsOpenAIOAuth() || account.IsShadow() || !account.IsSchedulable() {
		return errors.New("account is not an active direct OpenAI OAuth account")
	}
	if !account.IsModelSupportedForRequest(target.Model, target.Model) {
		return errors.New("account does not support the watched model")
	}
	previousState := s.loadOpenAICodexAutoTurnState(ctx, account, target.Model)
	if previousState != "" {
		capturedAt := s.openAICodexAutoTurnStateCapturedAt(account, target.Model)
		if capturedAt.After(target.LastHealthyAt) {
			return nil
		}
	}
	proxies, err := s.proxyRepo.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("list active proxies: %w", err)
	}
	now := time.Now()
	eligible := proxies[:0]
	for i := range proxies {
		proxy := &proxies[i]
		if proxy.IsExpired(now) || proxy.HealthStatus == ProxyHealthUnhealthy {
			continue
		}
		eligible = append(eligible, *proxy)
	}
	if len(eligible) == 0 {
		return errors.New("no active proxy is available")
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].ID < eligible[j].ID })
	attempts := min(len(eligible), openAICodexAutoProbeMaxProxyAttempts)
	start := int(s.codexAutoProbeProxyCursor.Add(1)-1) % len(eligible)
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		proxy := eligible[(start+attempt)%len(eligible)]
		if ctx.Err() != nil {
			return ctx.Err()
		}
		state, probeErr := s.probeOpenAICodexTurnStateViaProxy(ctx, account, target.Model, &proxy)
		if probeErr == nil && (state == previousState || s.openAICodexAutoProbeStateSeen(account, target.Model, state)) {
			probeErr = errors.New("proxy returned the previous turn state")
		}
		if probeErr == nil {
			s.observeCodexTurnStateMetadata(ctx, nil, account, target.Model, state, "proxy_probe")
			s.storeOpenAICodexAutoTurnState(ctx, account, target.Model, state)
			s.noteOpenAICodexAutoProbeNewState(account, target.Model, state, time.Now())
			logger.L().Info("codex automatic turn state probe succeeded",
				zap.Int64("account_id", account.ID),
				zap.String("upstream_model", target.Model),
				zap.Int64("proxy_id", proxy.ID),
				zap.Int("state_characters", openAICodexTurnStateCharacterCount(state)),
			)
			return nil
		}
		lastErr = probeErr
		logger.L().Info("codex automatic turn state proxy attempt failed",
			zap.Int64("account_id", account.ID),
			zap.String("upstream_model", target.Model),
			zap.Int64("proxy_id", proxy.ID),
			zap.Error(probeErr),
		)
	}
	return fmt.Errorf("all %d proxy attempts failed: %w", attempts, lastErr)
}

func (s *OpenAIGatewayService) probeOpenAICodexTurnStateViaProxy(
	ctx context.Context,
	account *Account,
	model string,
	proxy *Proxy,
) (string, error) {
	if account == nil || proxy == nil {
		return "", errors.New("account or proxy is nil")
	}
	attemptCtx, cancel := context.WithTimeout(ctx, openAICodexAutoProbeAttemptTimeout)
	defer cancel()
	token, _, err := s.GetAccessToken(attemptCtx, account)
	if err != nil {
		return "", fmt.Errorf("get access token: %w", err)
	}
	decision := OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}
	headers, _, err := s.buildOpenAIWSHeaders(
		attemptCtx,
		nil,
		account,
		token,
		decision,
		true,
		"",
		"",
		"turn-state-probe-"+uuid.NewString(),
		model,
		"",
	)
	if err != nil {
		return "", fmt.Errorf("build websocket headers: %w", err)
	}
	headers.Del(openAIWSTurnStateHeader)
	headers, err = s.refreshOpenAIAgentIdentityHeaders(attemptCtx, account, headers)
	if err != nil {
		return "", fmt.Errorf("refresh authentication headers: %w", err)
	}
	wsURL, err := s.buildOpenAIResponsesWSURL(account)
	if err != nil {
		return "", fmt.Errorf("build websocket url: %w", err)
	}
	routeAccount := *account
	proxyID := proxy.ID
	routeAccount.ProxyID = &proxyID
	routeAccount.Proxy = proxy
	conn, statusCode, responseHeaders, err := dialOpenAIWSRouteWithProfile(
		s.getOpenAIWSPassthroughDialer(),
		attemptCtx,
		wsURL,
		headers,
		platformegress.ExternalProxyRoute(proxy.URL()),
		s.resolveTLSProfile(&routeAccount),
	)
	if err != nil {
		return "", fmt.Errorf("dial websocket status=%d: %w", statusCode, err)
	}
	defer func() { _ = conn.Close() }()

	state := extractOpenAICodexTurnState(responseHeaders)
	payload := map[string]any{
		"type":         "response.create",
		"model":        strings.TrimSpace(model),
		"input":        []any{},
		"instructions": openai.DefaultInstructions,
		"store":        false,
		"stream":       true,
		"generate":     false,
	}
	if err := conn.WriteJSON(attemptCtx, payload); err != nil {
		return "", fmt.Errorf("write zero-output ping: %w", err)
	}
	for event := 0; event < openAICodexAutoProbeMaxEvents; event++ {
		message, readErr := conn.ReadMessage(attemptCtx)
		if readErr != nil {
			return "", fmt.Errorf("read zero-output ping: %w", readErr)
		}
		s.observeCodexEncryptedContentPayload(attemptCtx, nil, account, model, message, "proxy_probe")
		if metadataState := strings.TrimSpace(gjson.GetBytes(message, "headers.x-codex-turn-state").String()); metadataState != "" {
			state = metadataState
		}
		eventType, _, _ := parseOpenAIWSEventEnvelope(message)
		if eventType == "error" {
			return "", fmt.Errorf("zero-output ping error: %s", extractUpstreamErrorMessage(message))
		}
		if !isOpenAIWSTerminalEvent(eventType) {
			continue
		}
		terminal := normalizeOpenAIWSTerminalEvent(eventType)
		if terminal != "response.completed" && terminal != "response.done" {
			return "", fmt.Errorf("zero-output ping ended with %s", terminal)
		}
		characters := openAICodexTurnStateCharacterCount(state)
		targetLength := s.codexAutoTurnStateTargetLength(attemptCtx)
		if characters != targetLength || len(state) > codexTurnStateMaxValueBytes || !httpguts.ValidHeaderFieldValue(state) {
			return "", fmt.Errorf("turn state has %d characters (want %d) or is not a valid bounded header", characters, targetLength)
		}
		metadata := parseOpenAICodexTurnState(state, time.Now().UTC())
		if metadata.Valid && metadata.Expired {
			return "", errors.New("turn state is expired")
		}
		return state, nil
	}
	return "", fmt.Errorf("zero-output ping exceeded %d events", openAICodexAutoProbeMaxEvents)
}
