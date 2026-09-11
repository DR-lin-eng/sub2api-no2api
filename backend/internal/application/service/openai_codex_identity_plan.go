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
	codexConversationThreadHeader   codexConversationSignalSource = "thread_header"
	codexConversationSessionHeader  codexConversationSignalSource = "session_header"
	codexConversationLegacyHeader   codexConversationSignalSource = "legacy_header"
	codexConversationPromptCache    codexConversationSignalSource = "prompt_cache_key"
	codexConversationContent        codexConversationSignalSource = "content"
	codexConversationClaudeMetadata codexConversationSignalSource = "claude_metadata"
	codexConversationRequestLocal   codexConversationSignalSource = "request_local"
)

type codexSimulationRootPlan struct {
	rootKey            string
	conversationSource codexConversationSignalSource
	canonicalBodyHash  [sha256.Size]byte
	requestSeed        string
	createdAtMS        int64
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
	epoch    string
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

// codexPersonaPreset borrows the plugin's measured Linux client personas while
// keeping version selection under the gateway's canonical Codex version sync.
// The preset list is deliberately platform-gated: a macOS or Windows process
// must not claim to be a Linux executable merely because C-level simulation is
// enabled.
type codexPersonaPreset struct {
	originator string
	os         string
	arch       string
	terminal   string
}

var codexLinuxAMD64PersonaPresets = []codexPersonaPreset{
	{originator: "codex_cli_rs", os: "Fedora 42", arch: "x86_64", terminal: "xterm-256color"},
	{originator: "codex_cli_rs", os: "Arch Linux rolling", arch: "x86_64", terminal: "alacritty"},
	{originator: "codex_cli_rs", os: "Fedora 42", arch: "x86_64", terminal: "kitty"},
	{originator: "codex_cli_rs", os: "Ubuntu 22.4.0", arch: "x86_64", terminal: "screen"},
	{originator: "codex_exec", os: "Debian 12.8", arch: "x86_64", terminal: "kitty"},
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
		settings.CLevelSimulationEnabled = cfg.CLevelSimulationEnabled
		settings.ExperimentalTransportEnabled = cfg.ExperimentalTransportEnabled
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
		epoch:    codexSimulationSettingsEpoch(settings),
		root: codexSimulationRootPlan{
			rootKey:            hex.EncodeToString(rootDigest[:]),
			conversationSource: source,
			canonicalBodyHash:  sha256.Sum256(canonicalBody),
			requestSeed:        requestSeed,
			createdAtMS:        time.Now().UnixMilli(),
		},
		turns: make(map[int]codexSimulationTurnPlan, 1),
	}
	c.Set(codexSimulationRequestStateContextKey, state)
}

// codexSimulationSettingsEpoch binds a long-lived request/WS to the exact
// identity policy snapshot that created it. Rotating the secret, changing the
// continuation mode, or changing the state TTL therefore requires a reconnect
// instead of allowing a connection to straddle two virtual clients.
func codexSimulationSettingsEpoch(settings CodexSimulationSettings) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf(
		"enabled=%t\x00c_level=%t\x00experimental_transport=%t\x00mode=%s\x00ttl=%d\x00secret=%s",
		settings.FullSimulationEnabled,
		settings.CLevelSimulationEnabled,
		settings.ExperimentalTransportEnabled,
		settings.continuationMode(),
		settings.StateTTLSeconds,
		settings.IdentitySecret,
	)))
	return hex.EncodeToString(digest[:])
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
			claudeCodeSessionHeader,
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
	if value := deriveClaudeCodeMetadataSessionSeed(canonicalBody); value != "" {
		return value, codexConversationClaudeMetadata
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
	if turn > 1 {
		currentSettings := s.codexSimulationSettingsSnapshot(ctx, nil)
		if !currentSettings.configured() || codexSimulationSettingsEpoch(currentSettings) != requestState.epoch {
			return body, newCodexContinuationTerminalError(
				"Codex simulation settings changed; reconnect to continue with the current virtual client",
				nil,
			)
		}
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
		ids = s.resolveCodexFullSimulationIDs(ctx, requestState, principal, account, turn)
		if ids != nil {
			ids.requestKind = codexRequestKindForContext(c, body)
			ids.directInstallationHeader = ids.requestKind == codexRequestKindCompaction
		}
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

func codexRequestKindForContext(c *gin.Context, body []byte) string {
	if isOpenAIResponsesCompactPath(c) || isOpenAINativeCompactionV2(c) {
		return codexRequestKindCompaction
	}
	if strings.EqualFold(strings.TrimSpace(gjson.GetBytes(body, "generate").String()), "false") {
		return codexRequestKindPrewarm
	}
	return codexRequestKindTurn
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
	raw := account.CodexVirtualClientKey()
	source := "local_account_id"
	if strings.HasPrefix(raw, "chatgpt:") {
		source = "chatgpt_account_id"
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
	account *Account,
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
	sessionID := codexSimulationUUIDv7(secret, "session:v2", request.root.createdAtMS, request.root.rootKey, principal.key)
	profile := resolveCodexSimulationProfile(secret, principal.key)
	windowNumber := int(generation) + 1
	windowID := sessionID + ":" + strconv.Itoa(windowNumber)
	contextWindowID := s.ensureCodexContextWindowID(ctx, account)
	return &codexFingerprintIDs{
		mode:            codexFingerprintFull,
		fullSimulation:  true,
		rootKey:         request.root.rootKey,
		principalKey:    principal.key,
		installationID:  codexSimulationUUID(secret, "installation:v2", principal.key),
		sessionID:       sessionID,
		threadID:        sessionID,
		turnID:          codexSimulationUUIDv7(secret, "turn:v2", turnPlan.startedAtMS, request.root.rootKey, principal.key, turnPlan.seed),
		windowID:        windowID,
		windowNumber:    windowNumber,
		contextWindowID: contextWindowID,
		promptCacheKey:  sessionID,
		generation:      generation,
		turnStartedAtMS: turnPlan.startedAtMS,
		profile:         profile,
		identitySecret:  secret,
	}
}

func resolveCodexSimulationProfile(secret, principalKey string) codexSimulationProfile {
	version := CodexCanonicalClientVersion()
	if version == "" {
		version = "0.153.4"
	}
	digest := codexSimulationHMAC(secret, "profile:v2", principalKey)
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		preset := codexLinuxAMD64PersonaPresets[int(digest[0])%len(codexLinuxAMD64PersonaPresets)]
		return codexSimulationProfile{
			id:         hex.EncodeToString(digest[:8]),
			userAgent:  fmt.Sprintf("%s/%s (%s; %s) %s", preset.originator, version, preset.os, preset.arch, preset.terminal),
			originator: preset.originator,
			version:    version,
		}
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	digest = codexSimulationHMAC(secret, "profile:v1", runtime.GOOS, arch, principalKey)
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

// codexSimulationUUIDv7 preserves the Codex wire convention where the first
// 48 bits carry the Unix-millisecond creation time. The remaining bits come
// from the identity-secret HMAC, so the value is deterministic for one
// simulation plan while retaining UUIDv7's timestamp shape.
func codexSimulationUUIDv7(secret, domain string, timestampMS int64, parts ...string) string {
	digest := codexSimulationHMAC(secret, domain, parts...)
	var id uuid.UUID
	copy(id[:], digest[:len(id)])
	if timestampMS <= 0 {
		timestampMS = time.Now().UnixMilli()
	}
	ts := uint64(timestampMS) & ((uint64(1) << 48) - 1)
	id[0] = byte(ts >> 40)
	id[1] = byte(ts >> 32)
	id[2] = byte(ts >> 24)
	id[3] = byte(ts >> 16)
	id[4] = byte(ts >> 8)
	id[5] = byte(ts)
	id[6] = (id[6] & 0x0f) | 0x70
	id[8] = (id[8] & 0x3f) | 0x80
	return id.String()
}

// ensureCodexContextWindowID creates the account-scoped random context UUID
// exactly once in this process and persists it for accounts loaded from the
// database. A fresh request then reuses the account value instead of exposing
// a caller-provided window identity.
func (s *OpenAIGatewayService) ensureCodexContextWindowID(ctx context.Context, account *Account) string {
	if s == nil || account == nil || !account.IsOpenAIOAuth() {
		return ""
	}
	s.codexContextWindowMu.Lock()
	defer s.codexContextWindowMu.Unlock()
	if existing := account.GetCodexContextWindowID(); existing != "" {
		if account.ID > 0 {
			s.codexContextWindowIDs.Store(account.ID, existing)
		}
		return existing
	}
	if account.ID > 0 {
		if cached, ok := s.codexContextWindowIDs.Load(account.ID); ok {
			if existing, ok := cached.(string); ok && existing != "" {
				if account.Extra == nil {
					account.Extra = make(map[string]any)
				}
				account.Extra[CodexContextWindowIDExtraKey] = existing
				return existing
			}
		}
	}
	id := account.EnsureCodexContextWindowID()
	if account.ID > 0 {
		s.codexContextWindowIDs.Store(account.ID, id)
	}
	if id == "" || s.accountRepo == nil || s.settingService == nil || account.ID <= 0 {
		return id
	}
	// AccountRepository.UpdateExtra is an atomic JSONB merge. Keep the request
	// usable if a legacy test double or a transient database error rejects this
	// best-effort backfill; the in-memory account remains fixed for this request.
	func() {
		defer func() { _ = recover() }()
		if ctx == nil {
			ctx = context.Background()
		}
		persistCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = s.accountRepo.UpdateExtra(persistCtx, account.ID, map[string]any{
			CodexContextWindowIDExtraKey: id,
		})
	}()
	return id
}

func codexSimulationGenerationStateKey(rootKey, principalKey string) string {
	if rootKey == "" || principalKey == "" {
		return ""
	}
	return codexSimulationStatePrefix + "generation:" + rootKey + ":" + principalKey
}
