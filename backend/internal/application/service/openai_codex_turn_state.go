package service

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const openAICodexTurnStateHeader = "x-codex-turn-state"

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
