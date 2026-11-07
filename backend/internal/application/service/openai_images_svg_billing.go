package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

// SVG authoring is a separate model invocation. Keep its usage separate from
// raster rendering until each model has been priced, including durable retries.
type OpenAIImagesSVGUsage struct {
	Model           string `json:"model"`
	InputTokens     int    `json:"input_tokens"`
	OutputTokens    int    `json:"output_tokens"`
	ReasoningTokens *int   `json:"reasoning_tokens,omitempty"`
}

func parseOpenAIImagesSVGUsage(value gjson.Result) *OpenAIImagesSVGUsage {
	model := value.Get("model")
	if !value.IsObject() || model.Type != gjson.String || strings.TrimSpace(model.String()) == "" {
		return nil
	}
	input, inputOK := boundedJSONNonNegativeInt(value.Get("input_tokens"))
	output, outputOK := boundedJSONNonNegativeInt(value.Get("output_tokens"))
	if !inputOK || !outputOK {
		return nil
	}
	usage := &OpenAIImagesSVGUsage{Model: strings.TrimSpace(model.String()), InputTokens: input, OutputTokens: output}
	if reasoning, ok := boundedJSONNonNegativeInt(value.Get("reasoning_tokens")); ok {
		reasoning = min(reasoning, output)
		usage.ReasoningTokens = &reasoning
	}
	return usage
}

func (s *OpenAIGatewayService) addOpenAIImagesSVGCost(ctx context.Context, result *OpenAIForwardResult, apiKey *APIKey, base *CostBreakdown, multiplier float64, longContextBillingEnabled bool) (*CostBreakdown, error) {
	if !result.ImageEndpoint || result.Usage.SVGUsage == nil || base == nil {
		return base, nil
	}
	// An explicit image/per-request tariff covers the complete generated asset.
	if base.BillingMode != "" && base.BillingMode != string(BillingModeToken) {
		return base, nil
	}
	author := result.Usage.SVGUsage
	cost, _, err := s.calculateOpenAIRecordUsageTokenCost(ctx, apiKey, author.Model, multiplier, nil,
		UsageTokens{InputTokens: author.InputTokens, OutputTokens: author.OutputTokens}, "", longContextBillingEnabled, false, nil)
	if err != nil {
		return nil, fmt.Errorf("calculate SVG author cost for %s: %w", author.Model, err)
	}
	combined := *base
	combined.InputCost += cost.InputCost
	combined.OutputCost += cost.OutputCost
	combined.CacheReadCost += cost.CacheReadCost
	combined.CacheCreationCost += cost.CacheCreationCost
	combined.TotalCost += cost.TotalCost
	combined.ActualCost += cost.ActualCost
	combined.LongContextBillingApplied = combined.LongContextBillingApplied || cost.LongContextBillingApplied
	return &combined, nil
}

func addOpenAIImagesSVGUsageToLog(log *UsageLog, result *OpenAIForwardResult) {
	if !result.ImageEndpoint || result.Usage.SVGUsage == nil {
		return
	}
	author := result.Usage.SVGUsage
	log.InputTokens += author.InputTokens
	log.OutputTokens += author.OutputTokens
	if author.ReasoningTokens != nil {
		reasoning := *author.ReasoningTokens
		if log.ReasoningTokens != nil {
			reasoning += *log.ReasoningTokens
		}
		log.ReasoningTokens = &reasoning
	}
}
