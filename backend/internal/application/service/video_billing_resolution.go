package service

import (
	"fmt"
	"strings"
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

// ValidateMultimodalVideoResolution 拒绝无法映射到计费档位的视频分辨率。
// 空值表示客户端未指定分辨率，由上游默认值决定，保持放行。
func ValidateMultimodalVideoResolution(rawResolution string) error {
	rawResolution = strings.TrimSpace(rawResolution)
	if rawResolution == "" {
		return nil
	}
	if _, ok := ClassifyVideoBillingResolution(rawResolution); ok {
		return nil
	}
	return ErrVideoPriceNotConfigured.WithCause(fmt.Errorf("unsupported video resolution: %q", rawResolution))
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
