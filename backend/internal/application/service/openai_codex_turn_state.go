package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const openAICodexTurnStateHeader = "x-codex-turn-state"

const openAICompatTurnStateKeyContextKey = "openai_compat_turn_state_key"
const openAICompatTurnStateBodyContextKey = "openai_compat_turn_state_body"

const openAIHTTPSharedTurnStateTimeout = 100 * time.Millisecond

var openAIHTTPSharedTurnStateWriteSlots = make(chan struct{}, 128)

type openAICodexTurnStateOrigin struct {
	accountID int64
	expiresAt time.Time
}

// openAICodexTurnStateSeed identifies a downstream API-key/session pair. The
// original session header is used so a failover attempt cannot accidentally
// match the isolated outbound value.
func openAICodexTurnStateSeed(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	sessionID := extractClientSessionID(c.Request.Header)
	if sessionID == "" {
		return ""
	}
	return strconv.FormatInt(getAPIKeyIDFromContext(c), 10) + "\x00" + sessionID
}

func extractOpenAICodexTurnState(headers http.Header) string {
	if headers == nil {
		return ""
	}
	return strings.TrimSpace(headers.Get(openAICodexTurnStateHeader))
}

// relayOpenAICodexTurnState writes the upstream state before the response is
// committed and records its minting account. Missing upstream state explicitly
// clears a value left by an earlier failover attempt.
func (s *OpenAIGatewayService) relayOpenAICodexTurnState(c *gin.Context, account *Account, upstream http.Header) {
	if c == nil || c.Writer == nil {
		return
	}
	key := http.CanonicalHeaderKey(openAICodexTurnStateHeader)
	state := extractOpenAICodexTurnState(upstream)
	if state == "" {
		c.Writer.Header().Del(key)
		return
	}
	c.Writer.Header().Set(key, state)
	s.noteOpenAICodexTurnStateProvenance(c, account)
}

// stageOpenAICodexTurnState keeps state in the first-output header set. It does
// not record provenance until the set is committed to the client.
func stageOpenAICodexTurnState(dst *http.Header, upstream http.Header) {
	if dst == nil {
		return
	}
	key := http.CanonicalHeaderKey(openAICodexTurnStateHeader)
	state := extractOpenAICodexTurnState(upstream)
	if state == "" {
		if *dst != nil {
			(*dst).Del(key)
		}
		return
	}
	if *dst == nil {
		*dst = make(http.Header)
	}
	(*dst).Set(key, state)
}

func (s *OpenAIGatewayService) noteStagedOpenAICodexTurnStateCommitted(c *gin.Context, account *Account, staged http.Header) {
	if extractOpenAICodexTurnState(staged) == "" {
		return
	}
	s.noteOpenAICodexTurnStateProvenance(c, account)
}

func (s *OpenAIGatewayService) bindStagedOpenAICodexTurnState(c *gin.Context, account *Account, staged http.Header) {
	if s == nil || c == nil || account == nil || !account.IsOpenAIOAuth() || staged == nil {
		return
	}
	state := extractOpenAICodexTurnState(staged)
	if state == "" {
		return
	}
	if key, ok := c.Get(openAICompatTurnStateKeyContextKey); ok {
		if sessionKey, ok := key.(string); ok && strings.TrimSpace(sessionKey) != "" {
			s.bindOpenAICompatSessionTurnState(context.Background(), c, account, sessionKey, state)
		}
	}
	if body, ok := c.Get(openAICompatTurnStateBodyContextKey); ok {
		if rawBody, ok := body.([]byte); ok {
			s.bindOpenAIHTTPSharedTurnState(context.Background(), c, account, rawBody, state)
		}
	}
}

func stageOpenAICompatTurnStateKey(c *gin.Context, account *Account, body []byte) {
	if c == nil || account == nil || !account.IsOpenAIOAuth() {
		return
	}
	c.Set(openAICompatTurnStateKeyContextKey, openAIPassthroughTurnStateKey(c, account, body))
	c.Set(openAICompatTurnStateBodyContextKey, body)
}

func openAIHTTPTurnStateSessionHash(s *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) string {
	if s == nil || account == nil || account.ID <= 0 {
		return ""
	}
	hash := s.GenerateSessionHash(c, body)
	if strings.TrimSpace(hash) == "" {
		return ""
	}
	return "http:" + strconv.FormatInt(account.ID, 10) + ":" + hash
}

func (s *OpenAIGatewayService) bindOpenAIHTTPSharedTurnState(ctx context.Context, c *gin.Context, account *Account, body []byte, state string) {
	if s == nil || account == nil || !account.IsOpenAIOAuth() || strings.TrimSpace(state) == "" {
		return
	}
	store := s.getOpenAIWSStateStore()
	if store == nil || openAIWSStateStoreIsProcessLocal(store) {
		return
	}
	sessionHash := openAIHTTPTurnStateSessionHash(s, c, account, body)
	if sessionHash == "" {
		return
	}
	groupID := getOpenAIGroupIDFromContext(c)
	if ctx == nil {
		ctx = context.Background()
	}
	storeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIHTTPSharedTurnStateTimeout)
	select {
	case openAIHTTPSharedTurnStateWriteSlots <- struct{}{}:
	default:
		// The trailer/local binding remains authoritative; shed a shared-cache
		// write rather than creating an unbounded goroutine queue under load.
		cancel()
		return
	}
	go func() {
		defer func() { <-openAIHTTPSharedTurnStateWriteSlots }()
		defer cancel()
		logOpenAIWSSessionTurnStateWarn("set_http", groupID, sessionHash,
			store.BindSessionTurnState(storeCtx, groupID, sessionHash, state, s.openAIWSSessionStickyTTL()))
	}()
}

func (s *OpenAIGatewayService) getOpenAIHTTPSharedTurnState(ctx context.Context, c *gin.Context, account *Account, body []byte) string {
	if s == nil || account == nil || !account.IsOpenAIOAuth() {
		return ""
	}
	store := s.getOpenAIWSStateStore()
	if store == nil || openAIWSStateStoreIsProcessLocal(store) {
		return ""
	}
	sessionHash := openAIHTTPTurnStateSessionHash(s, c, account, body)
	if sessionHash == "" {
		return ""
	}
	if ctx == nil {
		ctx = context.Background()
	}
	groupID := getOpenAIGroupIDFromContext(c)
	readCtx, cancel := context.WithTimeout(ctx, openAIHTTPSharedTurnStateTimeout)
	defer cancel()
	state, ok, err := store.GetSessionTurnState(readCtx, groupID, sessionHash)
	logOpenAIWSSessionTurnStateWarn("get_http", groupID, sessionHash, err)
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(state)
}

func openAIWSStateStoreIsProcessLocal(store OpenAIWSStateStore) bool {
	local, ok := store.(*defaultOpenAIWSStateStore)
	return ok && local != nil && local.cache == nil
}

func (s *OpenAIGatewayService) noteOpenAICodexTurnStateProvenance(c *gin.Context, account *Account) {
	if s == nil || account == nil || account.ID <= 0 {
		return
	}
	seed := openAICodexTurnStateSeed(c)
	if seed == "" {
		return
	}
	ttl := s.openAIWSSessionStickyTTL()
	if ttl <= 0 {
		ttl = time.Hour
	}
	s.openaiCodexTurnStateOrigins.Store(seed, openAICodexTurnStateOrigin{
		accountID: account.ID,
		expiresAt: time.Now().Add(ttl),
	})
	s.sweepOpenAICodexTurnStateOrigins()
}

// guardOpenAICodexTurnStateEcho strips only a state known to have been minted
// by another account. Unknown values remain untouched for compatibility with
// clients that started a session outside this gateway.
func (s *OpenAIGatewayService) guardOpenAICodexTurnStateEcho(c *gin.Context, account *Account, headers http.Header) {
	if s == nil || account == nil || headers == nil || extractOpenAICodexTurnState(headers) == "" {
		return
	}
	seed := openAICodexTurnStateSeed(c)
	if seed == "" {
		return
	}
	value, ok := s.openaiCodexTurnStateOrigins.Load(seed)
	if !ok {
		return
	}
	origin, ok := value.(openAICodexTurnStateOrigin)
	if !ok {
		s.openaiCodexTurnStateOrigins.Delete(seed)
		return
	}
	if !origin.expiresAt.IsZero() && time.Now().After(origin.expiresAt) {
		s.openaiCodexTurnStateOrigins.Delete(seed)
		return
	}
	if origin.accountID != account.ID {
		headers.Del(openAICodexTurnStateHeader)
	}
}

func (s *OpenAIGatewayService) sweepOpenAICodexTurnStateOrigins() {
	if s == nil || s.openaiCodexTurnStateWrites.Add(1)%256 != 0 {
		return
	}
	now := time.Now()
	s.openaiCodexTurnStateOrigins.Range(func(key, value any) bool {
		origin, ok := value.(openAICodexTurnStateOrigin)
		if !ok || (!origin.expiresAt.IsZero() && now.After(origin.expiresAt)) {
			s.openaiCodexTurnStateOrigins.Delete(key)
		}
		return true
	})
}
