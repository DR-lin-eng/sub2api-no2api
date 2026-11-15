package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	infraerrors "github.com/DR-lin-eng/sub2api-no2api/internal/shared/errors"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/logger"
)

var (
	ErrAliyunCaptchaVerificationFailed = infraerrors.BadRequest("ALIYUN_CAPTCHA_VERIFICATION_FAILED", "aliyun captcha verification failed")
	ErrAliyunCaptchaNotConfigured      = infraerrors.ServiceUnavailable("ALIYUN_CAPTCHA_NOT_CONFIGURED", "aliyun captcha not configured")
	ErrAliyunCaptchaInvalidCredentials = infraerrors.BadRequest("ALIYUN_CAPTCHA_INVALID_CREDENTIALS", "invalid aliyun captcha credentials")
)

const (
	aliyunCaptchaEndpointCN       = "captcha.cn-shanghai.aliyuncs.com"
	aliyunCaptchaEndpointSGP      = "captcha.ap-southeast-1.aliyuncs.com"
	aliyunCredentialTestParameter = "sub2api-credential-validation"
	aliyunCaptchaPrefixMaxLength  = 63
)

// ValidateAliyunCaptchaPrefix limits the SDK prefix to one DNS label. Besides
// matching Alibaba Cloud's endpoint format, this keeps dynamically generated
// CSP origins from accepting separators or additional directives.
func ValidateAliyunCaptchaPrefix(value string) error {
	prefix := strings.TrimSpace(value)
	if prefix == "" || len(prefix) > aliyunCaptchaPrefixMaxLength {
		return fmt.Errorf("aliyun captcha prefix must be 1-%d characters", aliyunCaptchaPrefixMaxLength)
	}
	for i := 0; i < len(prefix); i++ {
		character := prefix[i]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') {
			continue
		}
		if character == '-' && i > 0 && i < len(prefix)-1 {
			continue
		}
		return fmt.Errorf("aliyun captcha prefix must be a valid DNS label")
	}
	return nil
}

type AliyunCaptchaCredentials struct {
	AccessKeyID     string
	AccessKeySecret string
	SceneID         string
	Endpoint        string
}

type AliyunCaptchaVerifyResult struct {
	VerifyResult bool
	VerifyCode   string
	RequestID    string
}

type AliyunCaptchaAPIError struct {
	Code      string
	Message   string
	RequestID string
}

func (e *AliyunCaptchaAPIError) Error() string {
	return fmt.Sprintf("aliyun captcha API error: %s", e.Code)
}

type AliyunCaptchaVerifier interface {
	VerifyCaptcha(context.Context, AliyunCaptchaCredentials, string) (*AliyunCaptchaVerifyResult, error)
}

func aliyunCaptchaEndpoint(region string) string {
	if normalizeAliyunCaptchaRegion(region) == AliyunCaptchaRegionSGP {
		return aliyunCaptchaEndpointSGP
	}
	return aliyunCaptchaEndpointCN
}

func parseAliyunCaptchaCredentials(config AliyunCaptchaConfig) (AliyunCaptchaCredentials, bool) {
	credentials := AliyunCaptchaCredentials{
		AccessKeyID:     strings.TrimSpace(config.AccessKeyID),
		AccessKeySecret: strings.TrimSpace(config.AccessKeySecret),
		SceneID:         strings.TrimSpace(config.SceneID),
		Endpoint:        aliyunCaptchaEndpoint(config.Region),
	}
	if credentials.AccessKeyID == "" || credentials.AccessKeySecret == "" || credentials.SceneID == "" {
		return AliyunCaptchaCredentials{}, false
	}
	return credentials, true
}

func (s *TurnstileService) verifyAliyun(ctx context.Context, config AliyunCaptchaConfig, token string) error {
	credentials, ok := parseAliyunCaptchaCredentials(config)
	if !ok || s.aliyunVerifier == nil {
		return ErrAliyunCaptchaNotConfigured
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrAliyunCaptchaVerificationFailed
	}
	result, err := s.aliyunVerifier.VerifyCaptcha(ctx, credentials, token)
	if err != nil {
		logger.LegacyPrintf("service.aliyun_captcha", "%s", "[AliyunCaptcha] verification request failed")
		return fmt.Errorf("%w: verifier request failed", ErrAliyunCaptchaVerificationFailed)
	}
	if result == nil || !result.VerifyResult {
		if result != nil {
			logger.LegacyPrintf("service.aliyun_captcha", "[AliyunCaptcha] rejected code=%s request_id=%s", result.VerifyCode, result.RequestID)
		}
		return ErrAliyunCaptchaVerificationFailed
	}
	return nil
}

func (s *TurnstileService) ValidateAliyunCaptchaConfiguration(ctx context.Context, config AliyunCaptchaConfig) error {
	credentials, ok := parseAliyunCaptchaCredentials(config)
	if !ok || s == nil || s.aliyunVerifier == nil {
		return ErrAliyunCaptchaNotConfigured
	}
	_, err := s.aliyunVerifier.VerifyCaptcha(ctx, credentials, aliyunCredentialTestParameter)
	if err == nil {
		return nil
	}
	var apiErr *AliyunCaptchaAPIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("validate aliyun captcha configuration: %w", err)
	}
	switch strings.TrimSpace(apiErr.Code) {
	case "InvalidAccessKeyId.NotFound", "InvalidAccessKeyId.Inactive", "SignatureDoesNotMatch",
		"Forbidden.AccessKeyDisabled", "IncompleteSignature", "InvalidSecurityToken.Expired":
		return ErrAliyunCaptchaInvalidCredentials
	default:
		return infraerrors.BadRequest("ALIYUN_CAPTCHA_CONFIGURATION_INVALID", "aliyun captcha configuration validation failed")
	}
}
