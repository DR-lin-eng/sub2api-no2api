package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
)

// imageUnitPriceConfigured 报告图片定价是否为当前尺寸档位配置了单价。
// 只认该档位的显式层级价：multimodal 没有“默认价”，
// 图片档位也只能按尺寸命中，不能回退到默认价或其它档位。
func imageUnitPriceConfigured(resolved *ResolvedPricing, sizeTier string) bool {
	if resolved == nil {
		return false
	}
	for _, tier := range resolved.RequestTiers {
		if tier.TierLabel == sizeTier && tier.PerRequestPrice != nil {
			return true
		}
	}
	return false
}

// MultimodalImagePricingRequest 描述一次图片生成的前置价格校验输入。
type MultimodalImagePricingRequest struct {
	APIKey  *APIKey
	UserID  int64
	GroupID int64
	Model   string
	Size    string
	Count   int
}

// ValidateMultimodalImagePricing 在提交上游前校验图片单价可解析。
// 图片用量由异步任务记录，RecordUsage 的缺价错误只会写日志、无法让请求失败，
// 因此必须在转发前拦截，避免用户拿到未计费的图片。
func (s *OpenAIGatewayService) ValidateMultimodalImagePricing(ctx context.Context, req MultimodalImagePricingRequest) error {
	if s == nil || s.billingService == nil || req.APIKey == nil {
		return nil
	}
	// 简单模式只记录用量、不扣费，不因缺少价格拒绝生成。
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return nil
	}
	model := strings.TrimSpace(req.Model)
	// token 计费的图片模型由 token 用量结算，提交前给不出按次价，交由用量记录处理。
	// multimodal 是逐模型定价分组：只有显式配成 token 计费的条目才允许放行，
	// 否则解析不到条目时会退化成 LiteLLM token 价，等于给未配置价格的模型免费出图。
	if resolved := s.resolveOpenAIChannelPricing(ctx, model, req.APIKey); resolved != nil &&
		resolved.Mode == BillingModeToken &&
		(!IsMultimodal(PlatformFromAPIKey(req.APIKey)) ||
			resolved.Source == PricingSourceGroup ||
			resolved.Source == PricingSourceChannel) {
		return nil
	}
	count := req.Count
	if count <= 0 {
		count = 1
	}
	// 请求没有给出可分类尺寸（未传 size / auto / 非标准尺寸）时不能猜 2K：
	// 实际计费档位只能由上游输出尺寸确定，任意档位都可能成为本次请求的结算档位，
	// 因此必须所有档位都配价才允许提交。只配了部分档位时直接拒绝，
	// 而不是先出图、再在落账阶段因为实际档位缺价而漏账。
	if _, ok := ClassifyImageBillingTier(req.Size); !ok {
		if !s.multimodalImageAllTierPricesConfigured(ctx, model, req.APIKey) {
			return ErrImagePriceNotConfigured
		}
		return nil
	}
	multiplier := s.multimodalImageRateMultiplier(ctx, req.APIKey, req.UserID, req.GroupID)
	cost, err := s.calculateOpenAIImageCost(ctx, model, req.APIKey, &OpenAIForwardResult{
		ImageCount: count,
		ImageSize:  req.Size,
	}, multiplier)
	if err != nil {
		return ErrImagePriceNotConfigured.WithCause(err)
	}
	if cost == nil {
		return ErrImagePriceNotConfigured
	}
	return nil
}

// multimodalImageAllTierPricesConfigured 报告模型是否为所有图片档位都配置了单价。
// 请求未指定可分类尺寸时，上游可能返回任意档位的图片；只要有一个档位缺价，
// 该请求就可能以无法结算的档位交付，因此必须全部有价才允许提交。
func (s *OpenAIGatewayService) multimodalImageAllTierPricesConfigured(ctx context.Context, model string, apiKey *APIKey) bool {
	resolved := s.resolveOpenAIChannelPricing(ctx, model, apiKey)
	if resolved == nil {
		return false
	}
	for _, tier := range []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K} {
		if !imageUnitPriceConfigured(resolved, tier) {
			return false
		}
	}
	return true
}

// validateMultimodalImageSettlement 在把上游图片结果写回客户端之前，
// 用与 RecordUsage 一致的档位解析确认实际计费档位已配置单价。
// 请求未指定可分类尺寸时，档位只能在拿到上游响应后确定；
// 若该档位缺价，必须在这里失败，不能先交付图片再让落账阶段的缺价错误只写日志。
func (s *OpenAIGatewayService) validateMultimodalImageSettlement(
	ctx context.Context,
	model string,
	apiKey *APIKey,
	usage multimodalMediaUsageMetadata,
) error {
	if s == nil || s.billingService == nil || apiKey == nil || usage.ImageCount <= 0 {
		return nil
	}
	// 简单模式只记录用量、不扣费，不因缺少价格拒绝交付。
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return nil
	}
	model = strings.TrimSpace(model)
	// token 计费的图片模型由 token 用量结算，不存在按次图片价。
	if resolved := s.resolveOpenAIChannelPricing(ctx, model, apiKey); resolved != nil &&
		resolved.Mode == BillingModeToken &&
		(!IsMultimodal(PlatformFromAPIKey(apiKey)) ||
			resolved.Source == PricingSourceGroup ||
			resolved.Source == PricingSourceChannel) {
		return nil
	}
	probe := &OpenAIForwardResult{
		ImageCount:             usage.ImageCount,
		ImageSize:              usage.ImageSize,
		ImageInputSize:         usage.ImageInputSize,
		ImageOutputSizes:       usage.ImageOutputSizes,
		ImageBillingSizeLocked: usage.ImageBillingSizeLocked,
	}
	if !probe.ImageBillingSizeLocked {
		ApplyOpenAIImageBillingResolution(probe)
	}
	tier, ok := ClassifyImageBillingTier(probe.ImageSize)
	if !ok {
		return ErrImagePriceNotConfigured.WithCause(fmt.Errorf(
			"image size cannot be mapped to a billing tier: model=%q size=%q", model, probe.ImageSize))
	}
	resolved := s.resolveOpenAIChannelPricing(ctx, model, apiKey)
	if !imageUnitPriceConfigured(resolved, tier) {
		return ErrImagePriceNotConfigured.WithCause(fmt.Errorf(
			"image price is not configured: model=%q size=%q", model, tier))
	}
	return nil
}

// multimodalImageRateMultiplier 复刻 RecordUsage 的图片倍率口径：
// 分组默认倍率或用户专属倍率，图片倍率独立时改用图片倍率。
func (s *OpenAIGatewayService) multimodalImageRateMultiplier(
	ctx context.Context,
	apiKey *APIKey,
	userID int64,
	groupID int64,
) float64 {
	baseMultiplier := 1.0
	if s != nil && s.cfg != nil {
		baseMultiplier = s.cfg.Default.RateMultiplier
	}
	if apiKey != nil && apiKey.Group != nil && groupID > 0 {
		baseMultiplier = s.ResolveUserGroupRateMultiplier(ctx, userID, groupID, apiKey.Group.RateMultiplier)
	}
	return resolveImageRateMultiplier(apiKey, baseMultiplier)
}
