package handler

import (
	"context"
	"strings"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
)

func firstHumanVerificationToken(captchaToken, turnstileToken string) string {
	if token := strings.TrimSpace(captchaToken); token != "" {
		return token
	}
	return strings.TrimSpace(turnstileToken)
}

func humanVerificationProof(captchaToken, turnstileToken, tencentTicket, tencentRandstr string) service.HumanVerificationProof {
	return service.HumanVerificationProof{
		Token:          firstHumanVerificationToken(captchaToken, turnstileToken),
		TencentTicket:  strings.TrimSpace(tencentTicket),
		TencentRandstr: strings.TrimSpace(tencentRandstr),
	}
}

// verifyActionCaptcha keeps partial dependency graphs compatible while failing
// closed when a popup captcha provider is selected without its verifier.
func verifyActionCaptcha(
	ctx context.Context,
	authService *service.AuthService,
	settingService *service.SettingService,
	proof service.HumanVerificationProof,
	remoteIP string,
) error {
	if authService != nil {
		return authService.VerifyActionCaptchaIfEnabled(ctx, proof, remoteIP)
	}
	if settingService == nil {
		return nil
	}

	config, err := settingService.GetHumanVerificationConfig(ctx)
	if err != nil {
		return service.ErrHumanVerificationUnavailable
	}
	switch config.Provider {
	case service.HumanVerificationProviderTencent:
		return service.ErrTencentCaptchaNotConfigured
	case service.HumanVerificationProviderAliyun:
		return service.ErrAliyunCaptchaNotConfigured
	case service.HumanVerificationProviderInvalid:
		return service.ErrHumanVerificationConflict
	default:
		return nil
	}
}
