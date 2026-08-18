package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	// CodexProjectIDHeader is an ingress-only namespace signal. It is consumed
	// by the root plan and must never be copied to an upstream request.
	CodexProjectIDHeader = "X-Sub2API-Codex-Project-ID"

	codexSimulationSettingsContextKey     = "codex_simulation_settings_snapshot"
	codexSimulationRequestStateContextKey = "codex_simulation_request_state"
	codexSimulationAttemptContextKey      = "codex_simulation_attempt"
)

type codexConversationSignalSource string

const (
	codexConversationThreadHeader  codexConversationSignalSource = "thread_header"
	codexConversationSessionHeader codexConversationSignalSource = "session_header"
	codexConversationLegacyHeader  codexConversationSignalSource = "legacy_header"
	codexConversationPromptCache   codexConversationSignalSource = "prompt_cache_key"
	codexConversationContent       codexConversationSignalSource = "content"
	codexConversationRequestLocal  codexConversationSignalSource = "request_local"
)

type codexSimulationRootPlan struct {
	rootKey            string
	conversationSource codexConversationSignalSource
	canonicalBodyHash  [sha256.Size]byte
	requestSeed        string
}

type codexSimulationTurnPlan struct {
	seed        string
	startedAtMS int64
}

// codexSimulationRequestState owns one immutable root plan and caches the
// turn-scoped seeds separately. Same-turn retries therefore reuse a turn ID,
// while a long-lived downstream WebSocket receives a new turn plan per frame.
type codexSimulationRequestState struct {
	settings CodexSimulationSettings
	root     codexSimulationRootPlan

	mu    sync.Mutex
	turns map[int]codexSimulationTurnPlan
}

func (s *codexSimulationRequestState) turn(turn int) codexSimulationTurnPlan {
	if turn <= 0 {
		turn = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.turns[turn]; ok {
		return existing
	}
	created := codexSimulationTurnPlan{
		seed:        uuid.Must(uuid.NewV7()).String(),
		startedAtMS: time.Now().UnixMilli(),
	}
	s.turns[turn] = created
	return created
}

type codexSimulationPrincipal struct {
	key    string
	source string
}

type codexSimulationProfile struct {
	id         string
	userAgent  string
	originator string
	version    string
}

type codexSimulationAttempt struct {
	request      *codexSimulationRequestState
	principal    codexSimulationPrincipal
	turn         int
	fingerprint  *codexFingerprintIDs
	continuation *codexContinuationAttempt
}

func (s *OpenAIGatewayService) codexSimulationIdentitySecret() string {
	return strings.TrimSpace(s.codexSimulationSettingsSnapshot(context.Background(), nil).IdentitySecret)
}

func (s *OpenAIGatewayService) codexFullSimulationEnabledForAccount(c *gin.Context, account *Account) bool {
	settings := s.codexSimulationSettingsSnapshot(context.Background(), c)
	return settings.FullSimulationEnabled && settings.configured() && account != nil && account.IsOpenAIOAuth() &&
		account.GetCodexFingerprintMode() == codexFingerprintFull
}

func (s *OpenAIGatewayService) codexSimulationSettingsSnapshot(ctx context.Context, c *gin.Context) CodexSimulationSettings {
	if request, ok := codexSimulationRequestStateFromGin(c); ok {
		return request.settings
	}
	if c != nil {
		if value, exists := c.Get(codexSimulationSettingsContextKey); exists {
			if settings, ok := value.(CodexSimulationSettings); ok {
				return settings
			}
		}
	}

	settings := CodexSimulationSettings{
		ContinuationMode: string(codexContinuationOff),
		StateTTLSeconds:  codexSimulationDefaultStateTTLSeconds,
	}
	if s != nil && s.settingService != nil {
		settings = s.settingService.CodexSimulationSettingsSnapshot(ctx)
	} else if s != nil && s.cfg != nil {
		cfg := s.cfg.Gateway.CodexSimulation
		settings.FullSimulationEnabled = cfg.FullSimulationEnabled
		settings.IdentitySecret = strings.TrimSpace(cfg.IdentitySecret)
		settings.ContinuationMode = normalizeCodexContinuationMode(cfg.ContinuationMode)
		if cfg.StateTTLSeconds > 0 {
			settings.StateTTLSeconds = cfg.StateTTLSeconds
		}
	}
	if c != nil {
		c.Set(codexSimulationSettingsContextKey, settings)
	}
	return settings
}

// CodexSimulationRequestEnabled is the hard request-path gate. When it is
// false, callers must stay on the pre-simulation OAuth path without creating
// an attempt or mutating request identity/continuation state.
func (s *OpenAIGatewayService) CodexSimulationRequestEnabled(c *gin.Context) bool {
	request, ok := codexSimulationRequestStateFromGin(c)
	return ok && request.settings.configured()
}

func (s *OpenAIGatewayService) codexSimulationRuntimeEnabled(ctx context.Context) bool {
	return s.codexSimulationSettingsSnapshot(ctx, nil).configured()
}

// PrepareCodexSimulationRequest creates the immutable root before account
// selection. The canonical body is hashed but never retained or mutated here.
func (s *OpenAIGatewayService) PrepareCodexSimulationRequest(
	c *gin.Context,
	apiKeyID int64,
	groupID *int64,
	canonicalBody []byte,
) {
	if c == nil {
		return
	}
	if existing, ok := codexSimulationRequestStateFromGin(c); ok && existing != nil {
		return
	}
	settings := s.codexSimulationSettingsSnapshot(c.Request.Context(), c)
	if !settings.configured() {
		return
	}

	signal, source := resolveCodexConversationSignal(c, canonicalBody)
	requestSeed := uuid.Must(uuid.NewV7()).String()
	if signal == "" {
		signal = requestSeed
		source = codexConversationRequestLocal
	}
	groupValue := int64(0)
	if groupID != nil {
		groupValue = *groupID
	}
	apiNamespace := "api_key:" + strconv.FormatInt(apiKeyID, 10) + ":group:" + strconv.FormatInt(groupValue, 10)
	projectSignal := strings.TrimSpace(c.GetHeader(CodexProjectIDHeader))
	rootDigest := codexSimulationHMAC(
		settings.IdentitySecret,
		"root:v1",
		apiNamespace,
		projectSignal,
		string(source),
		signal,
	)
	state := &codexSimulationRequestState{
		settings: settings,
		root: codexSimulationRootPlan{
			rootKey:            hex.EncodeToString(rootDigest[:]),
			conversationSource: source,
			canonicalBodyHash:  sha256.Sum256(canonicalBody),
			requestSeed:        requestSeed,
		},
		turns: make(map[int]codexSimulationTurnPlan, 1),
	}
	c.Set(codexSimulationRequestStateContextKey, state)
}

func resolveCodexConversationSignal(c *gin.Context, canonicalBody []byte) (string, codexConversationSignalSource) {
	if c != nil {
		if value := strings.TrimSpace(c.GetHeader("thread-id")); value != "" {
			return value, codexConversationThreadHeader
		}
		if value := strings.TrimSpace(c.GetHeader("session-id")); value != "" {
			return value, codexConversationSessionHeader
		}
		for _, header := range []string{
			"session_id",
			"conversation_id",
			openCodeSessionAffinityHeader,
			openCodeSessionIDHeader,
			openCodeNativeSessionHeader,
			codeBuddyConversationHeader,
		} {
			if value := strings.TrimSpace(c.GetHeader(header)); value != "" {
				return value, codexConversationLegacyHeader
			}
		}
	}
	if value := strings.TrimSpace(gjson.GetBytes(canonicalBody, "prompt_cache_key").String()); value != "" {
		return value, codexConversationPromptCache
	}
	if value := deriveOpenAIContentSessionSeed(canonicalBody); value != "" {
		return value, codexConversationContent
	}
	return "", codexConversationRequestLocal
}

func codexSimulationRequestStateFromGin(c *gin.Context) (*codexSimulationRequestState, bool) {
	if c == nil {
		return nil, false
	}
	value, exists := c.Get(codexSimulationRequestStateContextKey)
	if !exists {
		return nil, false
	}
	state, ok := value.(*codexSimulationRequestState)
	return state, ok && state != nil
}

func codexSimulationAttemptFromGin(c *gin.Context) (*codexSimulationAttempt, bool) {
	if c == nil {
		return nil, false
	}
	value, exists := c.Get(codexSimulationAttemptContextKey)
	if !exists {
		return nil, false
	}
	attempt, ok := value.(*codexSimulationAttempt)
	return attempt, ok && attempt != nil
}

func (s *OpenAIGatewayService) ensureCodexSimulationRequest(c *gin.Context, body []byte) *codexSimulationRequestState {
	if state, ok := codexSimulationRequestStateFromGin(c); ok {
		return state
	}
	apiKeyID := getAPIKeyIDFromContext(c)
	groupID := getOpenAIGroupIDFromContext(c)
	s.PrepareCodexSimulationRequest(c, apiKeyID, &groupID, body)
	state, _ := codexSimulationRequestStateFromGin(c)
	return state
}

// PrepareCodexSimulationAttempt derives the current principal attempt from the
// request root. It is called immediately before Forward in the handler loop.
func (s *OpenAIGatewayService) PrepareCodexSimulationAttempt(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) ([]byte, error) {
	return s.prepareCodexSimulationAttemptForTurn(ctx, c, account, body, 1)
}

func (s *OpenAIGatewayService) prepareCodexSimulationAttemptForTurn(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	turn int,
) ([]byte, error) {
	if c == nil || account == nil || !account.IsOpenAIOAuth() {
		return body, nil
	}
	requestState := s.ensureCodexSimulationRequest(c, body)
	if requestState == nil {
		return body, nil
	}
	if turn > 1 && !s.codexSimulationRuntimeEnabled(ctx) {
		return body, newCodexContinuationTerminalError(
			"Codex simulation was disabled; reconnect to continue on the original OAuth path",
			nil,
		)
	}
	principal := s.resolveCodexSimulationPrincipalWithSecret(account, requestState.settings.IdentitySecret)
	if principal.key == "" {
		return body, nil
	}

	continuation, attemptBody, err := s.prepareCodexContinuationAttempt(ctx, c, requestState, principal, body)
	if err != nil {
		return body, err
	}
	var ids *codexFingerprintIDs
	if requestState.settings.FullSimulationEnabled && requestState.settings.configured() &&
		account.GetCodexFingerprintMode() == codexFingerprintFull {
		ids = s.resolveCodexFullSimulationIDs(ctx, requestState, principal, turn)
	}
	attempt := &codexSimulationAttempt{
		request:      requestState,
		principal:    principal,
		turn:         turn,
		fingerprint:  ids,
		continuation: continuation,
	}
	c.Set(codexSimulationAttemptContextKey, attempt)
	stageCodexFingerprintIDs(c, ids)
	return attemptBody, nil
}

func (s *OpenAIGatewayService) resolveCodexSimulationPrincipal(account *Account) codexSimulationPrincipal {
	return s.resolveCodexSimulationPrincipalWithSecret(account, s.codexSimulationIdentitySecret())
}

func (s *OpenAIGatewayService) resolveCodexSimulationPrincipalWithSecret(account *Account, secret string) codexSimulationPrincipal {
	principal := s.codexSimulationPrincipalForAccountWithSecret(account, secret)
	switch principal.source {
	case "chatgpt_account_id":
		s.codexPrincipalUpstreamTotal.Add(1)
	case "local_account_id":
		s.codexPrincipalLocalTotal.Add(1)
	}
	return principal
}

func (s *OpenAIGatewayService) codexSimulationPrincipalForAccountWithSecret(account *Account, secret string) codexSimulationPrincipal {
	if account == nil {
		return codexSimulationPrincipal{}
	}
	raw := ""
	source := ""
	if upstreamID := strings.TrimSpace(account.GetCredential("chatgpt_account_id")); upstreamID != "" {
		raw = "chatgpt:" + upstreamID
		source = "chatgpt_account_id"
	} else {
		// Never collapse missing upstream principals into one shared empty value.
		raw = "local:" + accountIDString(account.ID)
		source = "local_account_id"
	}
	digest := codexSimulationHMAC(secret, "principal:v1", raw)
	return codexSimulationPrincipal{key: hex.EncodeToString(digest[:]), source: source}
}

type OpenAICodexPrincipalSourceStats struct {
	ChatGPTAccountID uint64
	LocalAccountID   uint64
}

func (s *OpenAIGatewayService) CodexPrincipalSourceStats() OpenAICodexPrincipalSourceStats {
	if s == nil {
		return OpenAICodexPrincipalSourceStats{}
	}
	return OpenAICodexPrincipalSourceStats{
		ChatGPTAccountID: s.codexPrincipalUpstreamTotal.Load(),
		LocalAccountID:   s.codexPrincipalLocalTotal.Load(),
	}
}

func (s *OpenAIGatewayService) resolveCodexFullSimulationIDs(
	ctx context.Context,
	request *codexSimulationRequestState,
	principal codexSimulationPrincipal,
	turn int,
) *codexFingerprintIDs {
	if request == nil || principal.key == "" {
		return nil
	}
	secret := request.settings.IdentitySecret
	turnPlan := request.turn(turn)
	generationKey := codexSimulationGenerationStateKey(request.root.rootKey, principal.key)
	generation := uint64(0)
	if store := s.getCodexSimulationStateStore(); store != nil {
		if stored, err := store.generationWithTTL(ctx, generationKey, request.settings.stateTTL()); err == nil {
			generation = stored
		}
	}
	sessionID := codexSimulationUUID(secret, "session:v2", request.root.rootKey, principal.key)
	profile := resolveCodexSimulationProfile(secret, principal.key)
	return &codexFingerprintIDs{
		mode:            codexFingerprintFull,
		fullSimulation:  true,
		rootKey:         request.root.rootKey,
		principalKey:    principal.key,
		installationID:  codexSimulationUUID(secret, "installation:v2", principal.key),
		sessionID:       sessionID,
		threadID:        sessionID,
		turnID:          codexSimulationUUID(secret, "turn:v2", request.root.rootKey, principal.key, turnPlan.seed),
		windowID:        codexSimulationUUID(secret, "window:v2", request.root.rootKey, principal.key, strconv.FormatUint(generation, 10)),
		promptCacheKey:  sessionID,
		generation:      generation,
		turnStartedAtMS: turnPlan.startedAtMS,
		profile:         profile,
	}
}

func resolveCodexSimulationProfile(secret, principalKey string) codexSimulationProfile {
	version := codexClientVersionFromUA(codexCanonicalUserAgent())
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	digest := codexSimulationHMAC(secret, "profile:v1", runtime.GOOS, arch, principalKey)
	terminals := []string{"xterm-256color", "screen-256color"}
	terminal := terminals[int(digest[0])%len(terminals)]
	osValue := "Ubuntu 22.4.0"
	switch runtime.GOOS {
	case "darwin":
		osValue = "Mac OS 14.0.0"
	case "windows":
		osValue = "Windows 11"
		terminal = "WindowsTerminal"
	}
	originator := openai.CodexCLIOriginator
	return codexSimulationProfile{
		id:         hex.EncodeToString(digest[:8]),
		userAgent:  fmt.Sprintf("%s/%s (%s; %s) %s", originator, version, osValue, arch, terminal),
		originator: originator,
		version:    version,
	}
}

func codexSimulationHMAC(secret, domain string, parts ...string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, []byte(secret))
	writeCodexHMACPart(mac.Write, domain)
	for _, part := range parts {
		writeCodexHMACPart(mac.Write, part)
	}
	var digest [sha256.Size]byte
	copy(digest[:], mac.Sum(nil))
	return digest
}

func writeCodexHMACPart(write func([]byte) (int, error), value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = write(length[:])
	_, _ = write([]byte(value))
}

func codexSimulationUUID(secret, domain string, parts ...string) string {
	digest := codexSimulationHMAC(secret, domain, parts...)
	var id uuid.UUID
	copy(id[:], digest[:len(id)])
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return id.String()
}

func codexSimulationGenerationStateKey(rootKey, principalKey string) string {
	if rootKey == "" || principalKey == "" {
		return ""
	}
	return codexSimulationStatePrefix + "generation:" + rootKey + ":" + principalKey
}
