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

// MultimodalVideoRequestTierBilling 报告该视频请求是否按次数（per_request）计费。
// 只有按次计费才依赖请求里的分辨率档位取价，需要提前校验档位可解析；
// 按秒计费的 video 模式（以及没有任何显式视频定价的模型）不依赖分辨率档位。
func (s *OpenAIGatewayService) MultimodalVideoRequestTierBilling(ctx context.Context, apiKey *APIKey, model string) bool {
	if apiKey == nil || apiKey.Group == nil {
		return false
	}
	if pricing := apiKey.Group.modelPricingFor(model); pricing != nil && pricing.BillingMode != "" {
		return pricing.BillingMode == BillingModePerRequest
	}
	if s == nil || s.channelService == nil || apiKey.GroupID == nil {
		return false
	}
	pricing := s.channelService.GetChannelModelPricing(ctx, *apiKey.GroupID, model)
	return pricing != nil && pricing.BillingMode == BillingModePerRequest
}

// ValidateMultimodalVideoResolution 按计费模式校验视频请求的分辨率档位。
// 只有按次数（per_request）计费时才需要用分辨率锁定价格档位；按秒计费的 video 模式价格
// 来自每秒单价，请求里的分辨率只是上游生成参数，不能因为 1440p/4k 这类上游允许、
// 但本地没有每秒价档位的取值直接拒绝请求。
func ValidateMultimodalVideoResolution(resolution string, tierBilling bool) error {
	if tierBilling {
		return ValidateMultimodalVideoRequestTierResolution(resolution)
	}
	return nil
}

// ValidateMultimodalVideoRequestTierResolution 拒绝无法映射到计费档位的视频分辨率。
// 按次计费的价格完全由分辨率档位决定，未指定分辨率同样无从取价，必须直接拒绝；
// 绝不把空值或未知值折成 480p 计费。
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

// VideoPriceNotConfiguredError 构造带诊断信息的缺价错误。
// 错误响应只序列化 Status.Message，WithCause 的 cause 不会返回给客户端，
// 因此排查所需的模型、请求分辨率、计费模式和已配置档位必须写进 Message。
func (s *OpenAIGatewayService) VideoPriceNotConfiguredError(
	ctx context.Context,
	apiKey *APIKey,
	model string,
	resolution string,
	cause error,
) error {
	mode := ""
	var configuredTiers []string
	if resolved := s.resolveOpenAIChannelPricing(ctx, model, apiKey); resolved != nil {
		mode = string(resolved.Mode)
		for _, tier := range resolved.RequestTiers {
			if tier.PerRequestPrice != nil {
				configuredTiers = append(configuredTiers, tier.TierLabel)
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

// VideoBillingResolutionTier 返回按次计费视频用于取价的档位：
// 档位可解析时用归一化档位，其余一律返回空串，让计价链路按缺档位报缺价，
// 而不是把 1440p/4k 或未指定分辨率静默折成 480p 的价格。
func VideoBillingResolutionTier(resolution string) string {
	tier, ok := ClassifyVideoBillingResolution(resolution)
	if !ok {
		return ""
	}
	return tier
}

// VideoResolutionTierMatches 判断定价档位标签是否指向同一个视频分辨率档位。
// 管理员可能把档位写成 720P / 720 / HD / FULL_HD，这些写法在取价时都必须命中，
// 否则一个大小写差异就会让整条视频计费链路报“未配置价格”。
// 两边都必须能解析成 480p/720p/1080p 之一才算匹配，不做任何默认档位兜底。
func VideoResolutionTierMatches(storedLabel, resolution string) bool {
	want, ok := ClassifyVideoBillingResolution(resolution)
	if !ok {
		return false
	}
	got, ok := ClassifyVideoBillingResolution(storedLabel)
	if !ok {
		return false
	}
	return got == want
}
