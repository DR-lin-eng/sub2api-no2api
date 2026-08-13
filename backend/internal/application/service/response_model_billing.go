package service

import (
	"context"
	"log/slog"
	"strings"
)

// responseModelBillingCostEpsilon absorbs insignificant floating-point noise
// when the same token usage is evaluated against two price cards.
const responseModelBillingCostEpsilon = 1e-12

// responseModelBillingDeclaration returns the trusted upstream declaration that
// may be considered for billing. Empty means the existing billing basis wins.
// Media and per-call charges stay on their dedicated price paths.
func responseModelBillingDeclaration(source, responseModel string, conflict, mediaBilled bool) string {
	if source != BillingModelSourceResponse || conflict || mediaBilled {
		return ""
	}
	return strings.TrimSpace(responseModel)
}

// responseModelBillingAdoptable preserves three billing invariants: an upstream
// declaration cannot increase cost, turn a billable request into a free one, or
// bypass an administrator's explicit channel price with a global price.
func responseModelBillingAdoptable(baseline, response *CostBreakdown, baselineChannelPriced, responseChannelPriced bool) bool {
	if baseline == nil || response == nil {
		return false
	}
	if response.TotalCost > baseline.TotalCost+responseModelBillingCostEpsilon {
		return false
	}
	if response.TotalCost <= 0 && baseline.TotalCost > 0 {
		return false
	}
	return !baselineChannelPriced || responseChannelPriced
}

func responseModelPricingIdentified(resolved *ResolvedPricing, billingService *BillingService, model string) bool {
	if isExplicitAdminPricing(resolved) {
		return true
	}
	return billingService != nil && billingService.HasIdentifiedTokenPricing(model)
}

func pricingResolvedFromChannel(resolved *ResolvedPricing) bool {
	return isExplicitAdminPricing(resolved)
}

func isExplicitAdminPricing(resolved *ResolvedPricing) bool {
	return resolved != nil && (resolved.Source == PricingSourceGroup || resolved.Source == PricingSourceChannel)
}

func (s *GatewayService) applyResponseModelBilling(
	ctx context.Context,
	input *recordUsageCoreInput,
	result *ForwardResult,
	apiKey *APIKey,
	account *Account,
	baselineModel string,
	baselinePricing *ResolvedPricing,
	baselineCost *CostBreakdown,
	multiplier float64,
	imageMultiplier float64,
	opts *recordUsageOpts,
) *CostBreakdown {
	responseModel := responseModelBillingDeclaration(
		input.BillingModelSource,
		result.UpstreamResponseModel,
		result.UpstreamResponseModelConflict,
		result.ImageCount > 0,
	)
	if responseModel == "" || strings.EqualFold(responseModel, strings.TrimSpace(baselineModel)) {
		return baselineCost
	}

	responsePricing := s.resolveBillingPricing(ctx, apiKey, responseModel)
	if !responseModelPricingIdentified(responsePricing, s.billingService, responseModel) {
		return baselineCost
	}
	responseCost := s.calculateRecordUsageCostWithPricing(
		ctx, result, apiKey, responseModel, multiplier, imageMultiplier, opts, responsePricing,
	)
	if !responseModelBillingAdoptable(
		baselineCost,
		responseCost,
		pricingResolvedFromChannel(baselinePricing),
		pricingResolvedFromChannel(responsePricing),
	) {
		return baselineCost
	}

	logResponseModelBillingApplied(
		"service.gateway", account, result.RequestID, baselineModel, responseModel, baselineCost, responseCost,
	)
	return responseCost
}

// logResponseModelBillingApplied leaves an audit trail only when the billed
// model actually changes, avoiding per-request noise for matching declarations.
func logResponseModelBillingApplied(component string, account *Account, requestID, baselineModel, responseModel string, baselineCost, responseCost *CostBreakdown) {
	baselineModel = strings.TrimSpace(baselineModel)
	responseModel = strings.TrimSpace(responseModel)
	if strings.EqualFold(baselineModel, responseModel) {
		return
	}

	attrs := []any{
		"component", component,
		"request_id", strings.TrimSpace(requestID),
		"baseline_model", baselineModel,
		"response_model", responseModel,
	}
	if baselineCost != nil && responseCost != nil {
		attrs = append(attrs, "baseline_cost", baselineCost.TotalCost, "billed_cost", responseCost.TotalCost)
	}
	if account != nil {
		attrs = append(attrs, "platform", account.Platform, "account_id", account.ID)
	}
	slog.Info("billing.response_model_applied", attrs...)
}
