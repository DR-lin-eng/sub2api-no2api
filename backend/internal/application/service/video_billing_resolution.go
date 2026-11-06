package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

const (
	VideoBillingResolution480P  = "480p"
	VideoBillingResolution720P  = "720p"
	VideoBillingResolution1080P = "1080p"
)

// ClassifyVideoBillingResolution 严格解析视频计费分辨率。
// ok=false 表示该值无法映射到已配置的计费档位（例如 1440p、4k），
// 调用方必须拒绝请求或明确报错，不能静默按 480p 计费。
func ClassifyVideoBillingResolution(resolution string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "480", "480p", "sd":
		return VideoBillingResolution480P, true
	case "720", "720p", "hd":
		return VideoBillingResolution720P, true
	case "1080", "1080p", "full_hd", "full-hd", "fhd":
		return VideoBillingResolution1080P, true
	default:
		return "", false
	}
}

// MultimodalVideoRequestTierBilling reports whether the selected model uses
// per-request video pricing. Only that mode needs a resolution tier before
// the request can be submitted upstream.
func (s *OpenAIGatewayService) MultimodalVideoRequestTierBilling(ctx context.Context, apiKey *APIKey, model string) bool {
	if apiKey == nil || apiKey.Group == nil {
		return false
	}
	if pricing := apiKey.Group.modelPricingFor(model); pricing != nil && pricing.BillingMode != "" {
		return pricing.BillingMode == BillingModePerRequest
	}
	if s == nil || s.channelService == nil {
		return false
	}
	groupID := apiKey.Group.ID
	if apiKey.GroupID != nil && *apiKey.GroupID > 0 {
		groupID = *apiKey.GroupID
	}
	if groupID <= 0 {
		return false
	}
	pricing := s.channelService.GetChannelModelPricing(ctx, groupID, model)
	return pricing != nil && pricing.BillingMode == BillingModePerRequest
}

// ValidateMultimodalVideoResolution validates a video resolution only when
// the request is billed per request. Per-second video pricing does not use the
// resolution as a price key, so provider-supported aliases remain allowed.
func ValidateMultimodalVideoResolution(resolution string, tierBilling bool) error {
	if !tierBilling {
		return nil
	}
	return ValidateMultimodalVideoRequestTierResolution(resolution)
}

// ValidateMultimodalVideoRequestTierResolution rejects missing or unsupported
// resolution tiers. An unknown value must never fall back to 480p.
func ValidateMultimodalVideoRequestTierResolution(rawResolution string) error {
	rawResolution = strings.TrimSpace(rawResolution)
	if _, ok := ClassifyVideoBillingResolution(rawResolution); ok {
		return nil
	}
	if rawResolution == "" {
		return infraerrors.Newf(http.StatusServiceUnavailable, "VIDEO_PRICE_NOT_CONFIGURED",
			"video price is not configured: resolution is required for per-request video billing")
	}
	return infraerrors.Newf(http.StatusServiceUnavailable, "VIDEO_PRICE_NOT_CONFIGURED",
		"video price is not configured: unsupported resolution %q for per-request video billing", rawResolution)
}

// VideoPriceNotConfiguredError includes enough context for administrators to
// repair a model's price card without exposing an internal wrapped cause.
func (s *OpenAIGatewayService) VideoPriceNotConfiguredError(
	ctx context.Context,
	apiKey *APIKey,
	model string,
	resolution string,
	cause error,
) error {
	mode := ""
	var configuredTiers []string
	if s != nil {
		if resolved := s.resolveOpenAIChannelPricing(ctx, model, apiKey); resolved != nil {
			mode = string(resolved.Mode)
			for _, tier := range resolved.RequestTiers {
				if tier.PerRequestPrice != nil {
					configuredTiers = append(configuredTiers, tier.TierLabel)
				}
			}
		}
	}
	detail := fmt.Sprintf("model=%q resolution=%q mode=%q configured_tiers=%v",
		strings.TrimSpace(model), strings.TrimSpace(resolution), mode, configuredTiers)
	if cause != nil {
		detail = fmt.Sprintf("%s: %v", detail, cause)
	}
	return infraerrors.Newf(http.StatusServiceUnavailable, "VIDEO_PRICE_NOT_CONFIGURED",
		"video price is not configured: %s", detail)
}

// xAI 视频生成按秒计费，duration 请求参数允许 1-15 秒；未指定时上游默认生成 8 秒。
// 计费时长必须与上游实际消耗对齐，否则用户可通过拉长 duration 套利（提交时长由用户控制）。
const (
	VideoBillingMinDurationSeconds     = 1
	VideoBillingMaxDurationSeconds     = 15
	VideoBillingDefaultDurationSeconds = 8
)

// NormalizeVideoBillingDurationSecondsOrDefault 归一化计费用视频时长：
// 未指定（<=0）按上游默认 8 秒计，超出上游允许区间按边界收敛。
func NormalizeVideoBillingDurationSecondsOrDefault(durationSeconds int) int {
	if durationSeconds <= 0 {
		return VideoBillingDefaultDurationSeconds
	}
	if durationSeconds < VideoBillingMinDurationSeconds {
		return VideoBillingMinDurationSeconds
	}
	if durationSeconds > VideoBillingMaxDurationSeconds {
		return VideoBillingMaxDurationSeconds
	}
	return durationSeconds
}

func NormalizeVideoBillingResolutionOrDefault(resolution string) string {
	if tier, ok := ClassifyVideoBillingResolution(resolution); ok {
		return tier
	}
	return VideoBillingResolution480P
}

// VideoBillingResolutionTier returns the normalized tier for per-request
// video pricing, or an empty string when the request cannot be priced safely.
func VideoBillingResolutionTier(resolution string) string {
	tier, ok := ClassifyVideoBillingResolution(resolution)
	if !ok {
		return ""
	}
	return tier
}

// VideoResolutionTierMatches compares configured and requested resolution
// aliases without introducing a default tier.
func VideoResolutionTierMatches(storedLabel, resolution string) bool {
	want, ok := ClassifyVideoBillingResolution(resolution)
	if !ok {
		return false
	}
	got, ok := ClassifyVideoBillingResolution(storedLabel)
	return ok && got == want
}
