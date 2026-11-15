package service

import (
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/antigravity"
	"github.com/stretchr/testify/require"
)

func TestAntigravityBareGeminiVariantsRespectExplicitMappings(t *testing.T) {
	svc := &AntigravityGatewayService{}
	account := &Account{Platform: PlatformAntigravity, Credentials: map[string]any{}}
	for _, level := range []string{"low", "medium", "high"} {
		got := svc.getMappedModelForThinkingLevel(account, "gemini-3.8-flash", level)
		require.Equal(t, "gemini-3.8-flash-"+level, got)
	}
	account.Credentials["model_mapping"] = map[string]any{"gemini-3.8-flash": "custom-model"}
	require.Equal(t, "custom-model", svc.getMappedModelForThinkingLevel(account, "gemini-3.8-flash", "low"))
	account.Credentials["model_mapping"] = map[string]any{"gemini-3.8-flash": "gemini-3.8-flash"}
	require.Equal(t, "gemini-3.8-flash", svc.getMappedModelForThinkingLevel(account, "gemini-3.8-flash", "low"))
}

func TestAntigravityThinkingDepthIsConsistentAcrossProtocols(t *testing.T) {
	for _, tc := range []struct {
		native string
		claude *antigravity.ThinkingConfig
		want   string
	}{
		{`{"generationConfig":{"thinkingConfig":{"thinkingBudget":0}}}`, &antigravity.ThinkingConfig{Type: "disabled"}, "low"},
		{`{"generationConfig":{"thinkingConfig":{"thinkingBudget":1024}}}`, &antigravity.ThinkingConfig{BudgetTokens: 1024}, "low"},
		{`{"generationConfig":{"thinkingConfig":{"thinkingBudget":8192}}}`, &antigravity.ThinkingConfig{BudgetTokens: 8192}, "medium"},
		{`{"generationConfig":{"thinkingConfig":{"thinkingBudget":-1}}}`, nil, "high"},
	} {
		require.Equal(t, tc.want, geminiThinkingLevelFromBody([]byte(tc.native)))
		require.Equal(t, tc.want, geminiThinkingLevelFromClaudeThinking(tc.claude))
	}
}
