package service

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// This observational fixture uses the same input on baseline, candidate and rollback.
func TestSyncTransaction20261012(t *testing.T) {
	logs := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(logs, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	svc.cfg.Gateway.CodexImageGenerationBridgeEnabled = true
	for _, platform := range []string{PlatformOpenAI, PlatformKimi, PlatformDeepseek, PlatformMiniMax} {
		fmt.Printf("BRIDGE platform=%s enabled=%t\n", platform, svc.isCodexImageGenerationBridgeEnabled(context.Background(), &Account{Platform: platform, Type: AccountTypeAPIKey}, nil))
	}
	for _, kind := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{RequestID: "sync-" + kind, Model: "gpt-6.1-sol", UpstreamModel: "gpt-6.1-sol", Duration: time.Second, Usage: OpenAIUsage{InputTokens: 300000, OutputTokens: 2000}},
			APIKey: &APIKey{ID: 1, UserID: 2}, User: &User{ID: 2},
			Account: &Account{ID: 3, Platform: PlatformOpenAI, Type: kind, Extra: map[string]any{openAILongContextBillingEnabledKey: true}},
		})
		if err != nil { t.Fatal(err) }
		fmt.Printf("BILLING credential=%s total=%.6f long_context=%t\n", kind, logs.lastLog.TotalCost, logs.lastLog.LongContextBillingApplied)
	}
}
