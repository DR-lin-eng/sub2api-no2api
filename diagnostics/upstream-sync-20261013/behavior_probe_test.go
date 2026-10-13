//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestUpstream20261013ObservedBehavior(t *testing.T) {
	account := &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{"account_mode": AccountModeCoding, "base_url": "https://api.kimi.ai/coding/v1"}}
	fmt.Printf("KIMI_INTERNATIONAL_PROVIDER=%q\n", account.GetCodingPlanProvider())
	gravity := &Account{Platform: PlatformAntigravity, Extra: map[string]any{modelRateLimitsKey: map[string]any{antigravityGeminiModelRateLimitKey: map[string]any{"rate_limit_reset_at": time.Now().Add(time.Hour).Format(time.RFC3339)}}}}
	fmt.Printf("ANTIGRAVITY_FAMILY_LIMIT image=%t text=%t\n", gravity.isModelRateLimitedWithContext(context.Background(), "gemini-3.1-flash-image-preview"), gravity.isModelRateLimitedWithContext(context.Background(), "gemini-3-flash"))
	input := []byte(`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGVsbG8="}}]}]}]}`)
	body, err := convertClaudeMessagesToGeminiGenerateContent(input)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err = json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	parts := decoded["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	images := 0
	for _, part := range parts {
		if _, ok := part.(map[string]any)["inlineData"]; ok {
			images++
		}
	}
	fmt.Printf("GEMINI_TOOL_RESULT images=%d parts=%d\n", images, len(parts))
	repo := &modelNotFoundAccountRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	ruleAccount := &Account{ID: 123, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"temp_unschedulable_enabled": true, "temp_unschedulable_rules": []any{map[string]any{"error_code": 502, "keywords": []any{"temporary failure"}, "duration_minutes": 1, "scope": "account"}}}}
	svc.HandleTempUnschedulable(context.Background(), ruleAccount, 502, []byte("temporary failure"), "model-a")
	fmt.Printf("EXPLICIT_ACCOUNT_SCOPE account_writes=%d model_writes=%d\n", repo.tempCalls, len(repo.modelRateLimitCalls))
}
