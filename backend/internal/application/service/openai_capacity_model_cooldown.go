package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Only actual error fields may match a capacity cooldown rule. Output and
// metadata can quote keywords and must never change account scheduling.
func openAICapacityShedRuleBody(payload []byte) []byte {
	fields := gjson.GetManyBytes(payload, "response.error", "error")
	for _, field := range fields {
		if field.IsObject() {
			return append(append([]byte(`{"error":`), field.Raw...), '}')
		}
	}
	flat := make(map[string]json.RawMessage)
	for _, key := range []string{"code", "type", "message", "param", "headers"} {
		if value := gjson.GetBytes(payload, key); value.Exists() {
			flat[key] = json.RawMessage(value.Raw)
		}
	}
	body, err := json.Marshal(map[string]any{"error": flat})
	if err != nil {
		return nil
	}
	return body
}

func (s *OpenAIGatewayService) applyOpenAICapacityModelCooldown(ctx context.Context, account *Account, status int, payload []byte, model string) bool {
	if s == nil || s.rateLimitService == nil || account == nil || account.Platform != PlatformOpenAI ||
		!account.IsTempUnschedulableEnabled() || strings.TrimSpace(model) == "" {
		return false
	}
	body := openAICapacityShedRuleBody(payload)
	matches := matchTempUnschedulableRules(account, status, body)
	if len(matches) == 0 || matches[0].rule.Scope != TempUnschedScopeModel {
		return false
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	return s.rateLimitService.HandleTempUnschedulable(stateCtx, account, status, body, model)
}

func (s *OpenAIGatewayService) applyOpenAICapacityModelCooldownForRequest(c *gin.Context, account *Account, payload []byte, model string) bool {
	if !isOpenAIUpstreamCapacityShedEvent(payload) {
		return false
	}
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	return s.applyOpenAICapacityModelCooldown(ctx, account, http.StatusServiceUnavailable, payload, model)
}
