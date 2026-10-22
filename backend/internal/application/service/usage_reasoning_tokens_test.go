package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/stretchr/testify/require"
)

func TestReasoningUsageParsing(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		want          *int
	}{
		{"responses", `{"usage":{"input_tokens":12,"output_tokens":30,"output_tokens_details":{"reasoning_tokens":24}}}`, reasoningCount(24)},
		{"chat", `{"usage":{"prompt_tokens":12,"completion_tokens":30,"completion_tokens_details":{"reasoning_tokens":24}}}`, reasoningCount(24)},
		{"zero", `{"usage":{"input_tokens":12,"output_tokens":30,"output_tokens_details":{"reasoning_tokens":0}}}`, reasoningCount(0)},
		{"missing", `{"usage":{"input_tokens":12,"output_tokens":30}}`, nil},
		{"null", `{"usage":{"input_tokens":12,"output_tokens":30,"output_tokens_details":{"reasoning_tokens":null}}}`, nil},
		{"negative", `{"usage":{"input_tokens":12,"output_tokens":30,"output_tokens_details":{"reasoning_tokens":-1}}}`, nil},
		{"fractional", `{"usage":{"input_tokens":12,"output_tokens":30,"output_tokens_details":{"reasoning_tokens":1.5}}}`, nil},
		{"string", `{"usage":{"input_tokens":12,"output_tokens":30,"output_tokens_details":{"reasoning_tokens":"24"}}}`, nil},
		{"overflow", `{"usage":{"input_tokens":12,"output_tokens":30,"output_tokens_details":{"reasoning_tokens":2147483648}}}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractOpenAIUsageFromJSONBytes([]byte(tc.payload))
			require.True(t, ok)
			require.Equal(t, tc.want, got.ReasoningTokens)
			require.Equal(t, 30, got.OutputTokens)
			require.Equal(t, tc.want, extractCCStreamUsage(tc.payload).ReasoningTokens)
			var envelope map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(tc.payload), &envelope))
			var typed apicompat.ResponsesUsage
			err := json.Unmarshal(envelope["usage"], &typed)
			if tc.name == "fractional" || tc.name == "string" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, copyOpenAIUsageFromResponsesUsage(&typed).ReasoningTokens)
			}
			stream := &OpenAIUsage{}
			(&OpenAIGatewayService{}).parseSSEUsageBytes([]byte(`{"type":"response.completed","response":`+tc.payload+`}`), stream)
			require.Equal(t, tc.want, stream.ReasoningTokens)
			require.Equal(t, 30, stream.OutputTokens)
		})
	}
}

func reasoningCount(count int) *int { return &count }

func TestReasoningUsageRecordedWithoutAdditionalCharge(t *testing.T) {
	for _, count := range []*int{nil, reasoningCount(0), reasoningCount(250)} {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		userRepo := &openAIRecordUsageUserRepoStub{}
		svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
		usage := OpenAIUsage{InputTokens: 1200, OutputTokens: 300, ReasoningTokens: count}
		err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{RequestID: "reasoning-record", Model: "gpt-5.1", Usage: usage},
			APIKey: &APIKey{ID: 2}, User: &User{ID: 1}, Account: &Account{ID: 3, Type: AccountTypeAPIKey},
		})
		require.NoError(t, err)
		require.NotNil(t, usageRepo.lastLog)
		require.Equal(t, count, usageRepo.lastLog.ReasoningTokens)
		require.Equal(t, 300, usageRepo.lastLog.OutputTokens)
		require.InDelta(t, expectedOpenAICost(t, svc, "gpt-5.1", usage, 1.1).ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
	}
}

func TestGeminiReasoningUsage(t *testing.T) {
	for _, tc := range []struct {
		payload string
		want    *int
	}{
		{`{"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"thoughtsTokenCount":7}}`, reasoningCount(7)},
		{`{"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"thoughtsTokenCount":0}}`, reasoningCount(0)},
		{`{"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`, nil},
	} {
		require.Equal(t, tc.want, extractGeminiUsage([]byte(tc.payload)).ReasoningTokens)
	}
}
