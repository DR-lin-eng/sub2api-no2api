package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
	"golang.org/x/net/http/httpguts"
)

const (
	openAICodexDefaultTurnStateCharacters = 292
	openAICodexAutoTurnStateTTL           = time.Hour
	openAICodexAutoTurnStateMissTTL       = 5 * time.Second
	openAICodexAutoTurnStateMaxEntries    = 4096
	openAICodexAutoTurnStateStoreTimeout  = 100 * time.Millisecond

	openAICodexTurnStateModelContextKey = "openai_codex_turn_state_model"
)

var openAICodexAutoTurnStateWriteSlots = make(chan struct{}, 128)

type openAICodexAutoTurnStateBinding struct {
	State      string    `json:"state"`
	CapturedAt time.Time `json:"captured_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func openAICodexTurnStateCharacterCount(state string) int {
	return utf8.RuneCountInString(strings.TrimSpace(state))
}

func (s *OpenAIGatewayService) codexAutoTurnStateTargetLength(ctx context.Context) int {
	if s != nil && s.settingService != nil {
		if length := s.settingService.CodexSimulationSettingsSnapshot(ctx).TurnStateTargetLength; length > 0 {
			return length
		}
	}
	return openAICodexDefaultTurnStateCharacters
}

func captureOpenAICodexTurnStateMetadata(headers http.Header, payload []byte) string {
	if headers == nil || len(payload) == 0 || strings.TrimSpace(gjson.GetBytes(payload, "type").String()) != "codex.response.metadata" {
		return ""
	}
	state := strings.TrimSpace(gjson.GetBytes(payload, "headers.x-codex-turn-state").String())
	if state != "" && len(state) <= codexTurnStateMaxValueBytes && httpguts.ValidHeaderFieldValue(state) {
		headers.Set(openAICodexTurnStateHeader, state)
	} else {
		return ""
	}
	return state
}

func captureOpenAICodexTurnStateFromSSE(headers http.Header, body string) string {
	state := ""
	forEachOpenAISSEDataPayload(body, func(payload []byte) {
		if captured := captureOpenAICodexTurnStateMetadata(headers, payload); captured != "" {
			state = captured
		}
	})
	return state
}

func openAICodexAutoTurnStateKey(account *Account, model string) string {
	if account == nil || !account.IsOpenAIOAuth() {
		return ""
	}
	model = strings.ToLower(strings.TrimSpace(model))
	if account.ID <= 0 || model == "" {
		return ""
	}
	return "auto-replay:v2:" + strconv.FormatInt(account.ID, 10) + "\x00" + model
}

func stageOpenAICodexTurnStateModel(c *gin.Context, model string) {
	if c == nil {
		return
	}
	if model = strings.TrimSpace(model); model != "" {
		c.Set(openAICodexTurnStateModelContextKey, model)
	}
}

func openAICodexTurnStateModel(c *gin.Context) string {
	if c == nil {
		return ""
	}
	value, ok := c.Get(openAICodexTurnStateModelContextKey)
	if !ok {
		return ""
	}
	model, _ := value.(string)
	return strings.TrimSpace(model)
}

func (s *OpenAIGatewayService) codexAutoTurnStateModelIsWatched(ctx context.Context, model string) bool {
	if s == nil || s.settingService == nil {
		return false
	}
	settings := s.settingService.CodexSimulationSettingsSnapshot(ctx)
	return settings.TurnStateAutoReplayEnabled && codexTurnStateModelIsWatched(settings, model)
}

func (s *OpenAIGatewayService) observeOpenAICodexTurnState(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	model string,
	state string,
) {
	if s == nil || account == nil || !account.IsOpenAIOAuth() || !s.codexAutoTurnStateModelIsWatched(ctx, model) {
		return
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return
	}
	state = strings.TrimSpace(state)
	characters := openAICodexTurnStateCharacterCount(state)
	lengthMatch := characters == s.codexAutoTurnStateTargetLength(ctx) && len(state) <= codexTurnStateMaxValueBytes && httpguts.ValidHeaderFieldValue(state)
	previousState := s.loadOpenAICodexAutoTurnState(ctx, account, model)
	previousCapturedAt := s.openAICodexAutoTurnStateCapturedAt(account, model)
	isNew := lengthMatch && state != previousState && !s.openAICodexAutoProbeStateSeen(account, model, state)
	logger.L().Info("codex turn state observed",
		zap.Int64("account_id", account.ID),
		zap.String("upstream_model", model),
		zap.Int("state_characters", characters),
		zap.Bool("healthy_length", lengthMatch),
		zap.Bool("new_state", isNew),
		zap.Bool("proxy_enabled", account.ProxyID != nil && account.Proxy != nil),
	)
	if !isNew {
		if previousCapturedAt.IsZero() {
			previousCapturedAt = time.Now()
		}
		s.noteOpenAICodexAutoProbeObservation(account, model, false, previousCapturedAt)
		return
	}
	s.storeOpenAICodexAutoTurnState(ctx, account, model, state)
	s.noteOpenAICodexAutoProbeNewState(account, model, state, time.Now())
}

func (s *OpenAIGatewayService) openAICodexAutoTurnStateCapturedAt(account *Account, model string) time.Time {
	key := openAICodexAutoTurnStateKey(account, model)
	if s == nil || key == "" {
		return time.Time{}
	}
	s.codexAutoTurnStateMu.RLock()
	binding := s.codexAutoTurnStates[key]
	s.codexAutoTurnStateMu.RUnlock()
	return binding.CapturedAt
}

func (s *OpenAIGatewayService) storeOpenAICodexAutoTurnState(ctx context.Context, account *Account, model, state string) {
	key := openAICodexAutoTurnStateKey(account, model)
	if key == "" || openAICodexTurnStateCharacterCount(state) != s.codexAutoTurnStateTargetLength(ctx) ||
		len(state) > codexTurnStateMaxValueBytes || !httpguts.ValidHeaderFieldValue(state) {
		return
	}
	now := time.Now()
	binding := openAICodexAutoTurnStateBinding{
		State:      strings.TrimSpace(state),
		CapturedAt: now,
		ExpiresAt:  now.Add(openAICodexAutoTurnStateTTL),
	}
	s.codexAutoTurnStateMu.Lock()
	if s.codexAutoTurnStates == nil {
		s.codexAutoTurnStates = make(map[string]openAICodexAutoTurnStateBinding, 64)
	}
	ensureBindingCapacity(s.codexAutoTurnStates, key, openAICodexAutoTurnStateMaxEntries)
	s.codexAutoTurnStates[key] = binding
	s.codexAutoTurnStateMu.Unlock()

	store := s.getOpenAIWSStateStore()
	if store == nil {
		return
	}
	encoded, err := json.Marshal(binding)
	if err != nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	storeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAICodexAutoTurnStateStoreTimeout)
	select {
	case openAICodexAutoTurnStateWriteSlots <- struct{}{}:
	default:
		cancel()
		return
	}
	go func() {
		defer func() { <-openAICodexAutoTurnStateWriteSlots }()
		defer cancel()
		if err := store.BindSessionTurnState(storeCtx, 0, key, string(encoded), openAICodexAutoTurnStateTTL); err != nil {
			logger.L().Warn("codex automatic turn state cache write failed", zap.Error(err))
		}
	}()
}

func (s *OpenAIGatewayService) loadOpenAICodexAutoTurnState(ctx context.Context, account *Account, model string) string {
	key := openAICodexAutoTurnStateKey(account, model)
	if s == nil || key == "" {
		return ""
	}
	now := time.Now()
	s.codexAutoTurnStateMu.RLock()
	binding, ok := s.codexAutoTurnStates[key]
	s.codexAutoTurnStateMu.RUnlock()
	if ok && now.Before(binding.ExpiresAt) {
		if binding.State == "" {
			return ""
		}
		if openAICodexTurnStateCharacterCount(binding.State) == s.codexAutoTurnStateTargetLength(ctx) &&
			len(binding.State) <= codexTurnStateMaxValueBytes && httpguts.ValidHeaderFieldValue(binding.State) {
			return binding.State
		}
	}
	if ok {
		s.codexAutoTurnStateMu.Lock()
		delete(s.codexAutoTurnStates, key)
		s.codexAutoTurnStateMu.Unlock()
	}

	store := s.getOpenAIWSStateStore()
	if store == nil {
		return ""
	}
	if ctx == nil {
		ctx = context.Background()
	}
	readCtx, cancel := context.WithTimeout(ctx, openAICodexAutoTurnStateStoreTimeout)
	defer cancel()
	encoded, found, err := store.GetSessionTurnState(readCtx, 0, key)
	if err != nil || !found {
		s.cacheOpenAICodexAutoTurnStateMiss(key, now)
		return ""
	}
	if err := json.Unmarshal([]byte(encoded), &binding); err != nil || !now.Before(binding.ExpiresAt) ||
		openAICodexTurnStateCharacterCount(binding.State) != s.codexAutoTurnStateTargetLength(ctx) ||
		len(binding.State) > codexTurnStateMaxValueBytes || !httpguts.ValidHeaderFieldValue(binding.State) {
		return ""
	}
	s.codexAutoTurnStateMu.Lock()
	if s.codexAutoTurnStates == nil {
		s.codexAutoTurnStates = make(map[string]openAICodexAutoTurnStateBinding, 64)
	}
	ensureBindingCapacity(s.codexAutoTurnStates, key, openAICodexAutoTurnStateMaxEntries)
	s.codexAutoTurnStates[key] = binding
	s.codexAutoTurnStateMu.Unlock()
	return binding.State
}

func (s *OpenAIGatewayService) cacheOpenAICodexAutoTurnStateMiss(key string, now time.Time) {
	if s == nil || key == "" {
		return
	}
	s.codexAutoTurnStateMu.Lock()
	defer s.codexAutoTurnStateMu.Unlock()
	if s.codexAutoTurnStates == nil {
		s.codexAutoTurnStates = make(map[string]openAICodexAutoTurnStateBinding, 64)
	}
	ensureBindingCapacity(s.codexAutoTurnStates, key, openAICodexAutoTurnStateMaxEntries)
	s.codexAutoTurnStates[key] = openAICodexAutoTurnStateBinding{ExpiresAt: now.Add(openAICodexAutoTurnStateMissTTL)}
}

func (s *OpenAIGatewayService) resolveCodexTurnStateReplay(c *gin.Context, account *Account, model, current string) string {
	if s == nil || s.settingService == nil || account == nil || !account.IsOpenAIOAuth() {
		return strings.TrimSpace(current)
	}
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	settings := s.settingService.CodexSimulationSettingsSnapshot(ctx)
	if settings.TurnStateAutoReplayEnabled && codexTurnStateModelIsWatched(settings, model) {
		return s.loadOpenAICodexAutoTurnState(ctx, account, model)
	}
	if settings.TurnStateReplayEnabled {
		if state := randomCodexTurnStateForAccount(settings, account.ID); state != "" {
			return state
		}
	}
	return strings.TrimSpace(current)
}
