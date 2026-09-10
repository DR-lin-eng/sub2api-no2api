package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const distillationSessionWindow uint64 = 10000

const (
	distillationSessionStateKey = "gateway_distillation_session_state"
)

type distillationCounter interface {
	IncrementDistillation(ctx context.Context, groupID, accountID int64) (int64, error)
}

type distillationSessionState struct {
	groupID   int64
	accountID int64
	sessionID string
	bucket    uint64
}

var localDistillationCounters sync.Map // map[distillationCounterKey]*atomic.Uint64

type distillationCounterKey struct {
	groupID   int64
	accountID int64
}

func distillationGroupFromGin(c *gin.Context) *Group {
	if c == nil {
		return nil
	}
	if c.Request != nil {
		if group, ok := c.Request.Context().Value(ctxkey.Group).(*Group); ok && group != nil {
			return group
		}
	}
	if value, exists := c.Get("api_key"); exists {
		if apiKey, ok := value.(*APIKey); ok && apiKey != nil && apiKey.Group != nil {
			return apiKey.Group
		}
	}
	return nil
}

// IsDistillationGroupRequest is the single runtime gate for the lightweight
// distillation path. It deliberately requires an Anthropic OAuth/SetupToken
// account because the rewritten identity is metadata.user_id.
func (s *GatewayService) IsDistillationGroupRequest(c *gin.Context, account *Account) bool {
	group := distillationGroupFromGin(c)
	return group != nil && group.IsDistillationGroup && account != nil && account.IsAnthropicOAuthOrSetupToken()
}

func (s *GatewayService) nextDistillationCounter(ctx context.Context, groupID, accountID int64) uint64 {
	if s != nil && s.rpmCache != nil {
		if counter, ok := s.rpmCache.(distillationCounter); ok {
			if value, err := counter.IncrementDistillation(ctx, groupID, accountID); err == nil && value > 0 {
				return uint64(value)
			}
		}
	}
	key := distillationCounterKey{groupID: groupID, accountID: accountID}
	value, _ := localDistillationCounters.LoadOrStore(key, &atomic.Uint64{})
	counter, ok := value.(*atomic.Uint64)
	if !ok || counter == nil {
		return 0
	}
	return counter.Add(1)
}

// DistillationSessionID returns one deterministic synthetic session ID for the
// current request/account. A request-local map prevents retries of the same
// account from consuming more than one request-window slot.
func (s *GatewayService) DistillationSessionID(ctx context.Context, c *gin.Context, account *Account) (string, bool) {
	if !s.IsDistillationGroupRequest(c, account) {
		return "", false
	}
	group := distillationGroupFromGin(c)
	if group == nil || account == nil {
		return "", false
	}
	if c != nil {
		if value, exists := c.Get(distillationSessionStateKey); exists {
			if states, ok := value.(map[int64]distillationSessionState); ok {
				if state, found := states[account.ID]; found && state.groupID == group.ID {
					return state.sessionID, true
				}
			}
		}
	}

	count := s.nextDistillationCounter(ctx, group.ID, account.ID)
	bucket := (count - 1) / distillationSessionWindow
	sessionID := generateUUIDFromSeed(fmt.Sprintf("sub2api:distill:v1:%d:%d:%d", group.ID, account.ID, bucket))
	if c != nil {
		states := map[int64]distillationSessionState{}
		if value, exists := c.Get(distillationSessionStateKey); exists {
			if existing, ok := value.(map[int64]distillationSessionState); ok {
				states = existing
			}
		}
		states[account.ID] = distillationSessionState{groupID: group.ID, accountID: account.ID, sessionID: sessionID, bucket: bucket}
		c.Set(distillationSessionStateKey, states)
	}
	return sessionID, true
}

var distillationCacheFields = map[string]struct{}{
	"cache_control":          {},
	"prompt_cache_key":       {},
	"prompt_cache_retention": {},
}

// stripDistillationCacheFields removes cache identity and breakpoints without
// re-marshalling the request, preserving ordering and large thinking payloads.
func stripDistillationCacheFields(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	paths := make([]string, 0, 8)
	var walk func(gjson.Result, string)
	walk = func(value gjson.Result, path string) {
		if value.Type != gjson.JSON {
			return
		}
		value.ForEach(func(key, child gjson.Result) bool {
			name := key.String()
			childPath := name
			if path != "" {
				if key.Type == gjson.Number {
					childPath = path + "." + strconv.FormatInt(key.Int(), 10)
				} else {
					childPath = path + "." + name
				}
			}
			if _, remove := distillationCacheFields[name]; remove {
				paths = append(paths, childPath)
				return true
			}
			walk(child, childPath)
			return true
		})
	}
	walk(gjson.ParseBytes(body), "")
	for i := len(paths) - 1; i >= 0; i-- {
		if next, err := sjson.DeleteBytes(body, paths[i]); err == nil {
			body = next
		}
	}
	return body
}
