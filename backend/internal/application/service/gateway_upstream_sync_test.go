//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/shared/claude"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestUpstreamSyncAllowedToolsPreserved(t *testing.T) {
	body := map[string]any{"tool_choice": map[string]any{"type": "allowed_tools", "mode": "required", "tools": []any{map[string]any{"type": "function", "name": "lookup"}}}}
	require.False(t, normalizeCodexToolChoice(body))
	require.Equal(t, "allowed_tools", body["tool_choice"].(map[string]any)["type"])
}

func TestUpstreamSyncThinkingBindingBeta(t *testing.T) {
	body := []byte(`{"thinking":{"type":"adaptive","block_binding":"none"},"messages":[]}`)
	for _, header := range []string{"", claude.BetaThinkingBindingControls} {
		got, changed := sanitizeAnthropicBodyForBetaTokens(body, header)
		require.Equal(t, header == "", changed)
		require.Equal(t, header != "", gjson.GetBytes(got, "thinking.block_binding").Exists())
		require.Equal(t, "adaptive", gjson.GetBytes(got, "thinking.type").String())
	}
}

func TestUpstreamSyncGeminiProbeRole(t *testing.T) {
	body, err := providerAdapters[PlatformGemini].buildBody("gemini-3.7-flash", "hello")
	require.NoError(t, err)
	require.Equal(t, "user", gjson.GetBytes(body, "contents.0.role").String())
}

func TestUpstreamSyncGeminiPriceAliases(t *testing.T) {
	for _, base := range []string{"gemini-3.6-flash", "gemini-3.7-flash", "gemini-3.8-flash"} {
		for _, tier := range []string{"high", "low", "medium", "tiered"} {
			require.Equal(t, base, normalizeGeminiThinkingTierAlias(base+"-"+tier))
		}
	}
	require.Equal(t, "gemini-9-flash-high", normalizeGeminiThinkingTierAlias("gemini-9-flash-high"))
}

func TestUpstreamSyncGLMPricing(t *testing.T) {
	service := &BillingService{fallbackPrices: make(map[string]*ModelPricing)}
	service.initFallbackPricing()
	for _, model := range []string{"glm-5.3-flash", "zai/glm-5.3flash"} {
		price := service.getFallbackPricing(model)
		require.NotNil(t, price)
		require.Equal(t, 0.15e-6, price.InputPricePerToken)
		require.Equal(t, 0.5e-6, price.OutputPricePerToken)
	}
	require.Equal(t, 1.4e-6, service.getFallbackPricing("glm-5.3").InputPricePerToken)
}

type upstreamSyncRedeemLookup struct {
	RedeemCodeRepository
	err error
}

func (r upstreamSyncRedeemLookup) GetByCode(context.Context, string) (*RedeemCode, error) {
	return nil, r.err
}

func TestUpstreamSyncPaymentLookupFailureStopsFulfillment(t *testing.T) {
	lookupErr := errors.New("temporary database failure")
	service := &PaymentService{redeemService: &RedeemService{redeemRepo: upstreamSyncRedeemLookup{err: lookupErr}}}
	err := service.doBalance(context.Background(), &dbent.PaymentOrder{RechargeCode: "TEST"}, nil)
	require.ErrorIs(t, err, lookupErr)
}

func TestUpstreamSyncFableCreditsRemainModelScoped(t *testing.T) {
	for _, sharedWindow := range []bool{false, true} {
		repo := &anthropicWindowLimitRepo{}
		service := NewRateLimitService(repo, nil, nil, nil, nil)
		reset := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
		headers := http.Header{}
		headers.Set("anthropic-ratelimit-unified-reset", reset)
		if sharedWindow {
			headers.Set("anthropic-ratelimit-unified-5h-status", "rejected")
			headers.Set("anthropic-ratelimit-unified-5h-utilization", "1")
			headers.Set("anthropic-ratelimit-unified-5h-reset", reset)
		}
		service.HandleUpstreamError(context.Background(), &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuth},
			429, headers, []byte(`{"error":{"details":{"error_code":"credits_required","model":"claude-fable-5-1"}}}`))
		require.Equal(t, 1, repo.modelRateLimitCalls)
		require.Equal(t, anthropicFableRateLimitKey, repo.lastModelRateLimitScope)
		if sharedWindow {
			require.Equal(t, 1, repo.rateLimitCalls)
		} else {
			require.Zero(t, repo.rateLimitCalls)
		}
		require.Zero(t, repo.tempUnschedCalls)
	}
}
