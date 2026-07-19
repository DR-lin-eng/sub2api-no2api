package service

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrTurnstileVerificationFailed = infraerrors.BadRequest("TURNSTILE_VERIFICATION_FAILED", "turnstile verification failed")
	ErrTurnstileNotConfigured      = infraerrors.ServiceUnavailable("TURNSTILE_NOT_CONFIGURED", "turnstile not configured")
	ErrTurnstileInvalidSecretKey   = infraerrors.BadRequest("TURNSTILE_INVALID_SECRET_KEY", "invalid turnstile secret key")
	ErrRecaptchaVerificationFailed = infraerrors.BadRequest("RECAPTCHA_VERIFICATION_FAILED", "reCAPTCHA verification failed")
	ErrRecaptchaNotConfigured      = infraerrors.ServiceUnavailable("RECAPTCHA_NOT_CONFIGURED", "reCAPTCHA not configured")
	ErrRecaptchaInvalidSecretKey   = infraerrors.BadRequest("RECAPTCHA_INVALID_SECRET_KEY", "invalid reCAPTCHA secret key")
	ErrCapVerificationFailed       = infraerrors.BadRequest("CAP_VERIFICATION_FAILED", "Cap verification failed")
	ErrCapNotConfigured            = infraerrors.ServiceUnavailable("CAP_NOT_CONFIGURED", "Cap not configured")
	ErrCapInvalidSecretKey         = infraerrors.BadRequest("CAP_INVALID_SECRET_KEY", "invalid Cap site key or secret")
	ErrHumanVerificationConflict   = infraerrors.ServiceUnavailable("HUMAN_VERIFICATION_CONFLICT", "multiple human verification providers are enabled")
)

// TurnstileVerifier 验证 Turnstile token 的接口
type TurnstileVerifier interface {
	VerifyToken(ctx context.Context, secretKey, token, remoteIP string) (*TurnstileVerifyResponse, error)
}

// RecaptchaVerifier 验证 Google reCAPTCHA token 的接口。
type RecaptchaVerifier interface {
	VerifyToken(ctx context.Context, secretKey, token, remoteIP string) (*RecaptchaVerifyResponse, error)
}

// CapVerifier 验证 Cap token 的接口。
type CapVerifier interface {
	VerifyToken(ctx context.Context, apiEndpoint, secretKey, token string) (*CapVerifyResponse, error)
}

// TurnstileService Turnstile 验证服务
type TurnstileService struct {
	settingService    *SettingService
	verifier          TurnstileVerifier
	recaptchaVerifier RecaptchaVerifier
	capVerifier       CapVerifier
}

// TurnstileVerifyResponse Cloudflare Turnstile 验证响应
type TurnstileVerifyResponse struct {
	Success     bool     `json:"success"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
	ErrorCodes  []string `json:"error-codes"`
	Action      string   `json:"action"`
	CData       string   `json:"cdata"`
}

type RecaptchaVerifyResponse struct {
	Success     bool     `json:"success"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
	ErrorCodes  []string `json:"error-codes"`
}

type CapVerifyResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// NewTurnstileService 创建 Turnstile 服务实例
func NewTurnstileService(settingService *SettingService, verifier TurnstileVerifier) *TurnstileService {
	return &TurnstileService{
		settingService: settingService,
		verifier:       verifier,
	}
}

// NewHumanVerificationService 创建支持全部外部渠道的人机验证服务。
// 返回旧类型名以保持现有依赖注入和内部调用兼容。
func NewHumanVerificationService(settingService *SettingService, turnstileVerifier TurnstileVerifier, recaptchaVerifier RecaptchaVerifier, capVerifier CapVerifier) *TurnstileService {
	return &TurnstileService{
		settingService:    settingService,
		verifier:          turnstileVerifier,
		recaptchaVerifier: recaptchaVerifier,
		capVerifier:       capVerifier,
	}
}

// VerifyToken 验证 Turnstile token
func (s *TurnstileService) VerifyToken(ctx context.Context, token string, remoteIP string) error {
	if s == nil || s.settingService == nil {
		return nil
	}

	switch s.settingService.GetHumanVerificationProvider(ctx) {
	case HumanVerificationProviderNone, HumanVerificationProviderLocal:
		return nil
	case HumanVerificationProviderTurnstile:
		return s.verifyTurnstile(ctx, token, remoteIP)
	case HumanVerificationProviderRecaptcha:
		return s.verifyRecaptcha(ctx, token, remoteIP)
	case HumanVerificationProviderCap:
		return s.verifyCap(ctx, token)
	default:
		return ErrHumanVerificationConflict
	}
}

func (s *TurnstileService) verifyTurnstile(ctx context.Context, token string, remoteIP string) error {
	if s.verifier == nil {
		return ErrTurnstileNotConfigured
	}

	// 获取 Secret Key
	secretKey := s.settingService.GetTurnstileSecretKey(ctx)
	if secretKey == "" {
		logger.LegacyPrintf("service.turnstile", "%s", "[Turnstile] Secret key not configured")
		return ErrTurnstileNotConfigured
	}

	// 如果 token 为空，返回错误
	if token == "" {
		logger.LegacyPrintf("service.turnstile", "%s", "[Turnstile] Token is empty")
		return ErrTurnstileVerificationFailed
	}

	logger.LegacyPrintf("service.turnstile", "[Turnstile] Verifying token for IP: %s", remoteIP)
	result, err := s.verifier.VerifyToken(ctx, secretKey, token, remoteIP)
	if err != nil {
		logger.LegacyPrintf("service.turnstile", "[Turnstile] Request failed: %v", err)
		return fmt.Errorf("send request: %w", err)
	}

	if result == nil {
		logger.LegacyPrintf("service.turnstile", "%s", "[Turnstile] Verification failed: empty response")
		return ErrTurnstileVerificationFailed
	}
	if !result.Success {
		logger.LegacyPrintf("service.turnstile", "[Turnstile] Verification failed, error codes: %v", result.ErrorCodes)
		return ErrTurnstileVerificationFailed
	}

	logger.LegacyPrintf("service.turnstile", "%s", "[Turnstile] Verification successful")
	return nil
}

func (s *TurnstileService) verifyRecaptcha(ctx context.Context, token string, remoteIP string) error {
	secretKey := s.settingService.GetRecaptchaSecretKey(ctx)
	if secretKey == "" || s.recaptchaVerifier == nil {
		return ErrRecaptchaNotConfigured
	}
	if strings.TrimSpace(token) == "" {
		return ErrRecaptchaVerificationFailed
	}
	result, err := s.recaptchaVerifier.VerifyToken(ctx, secretKey, token, remoteIP)
	if err != nil {
		return fmt.Errorf("verify reCAPTCHA token: %w", err)
	}
	if result == nil || !result.Success {
		return ErrRecaptchaVerificationFailed
	}
	return nil
}

func (s *TurnstileService) verifyCap(ctx context.Context, token string) error {
	apiEndpoint := s.settingService.GetCapAPIEndpoint(ctx)
	secretKey := s.settingService.GetCapSecretKey(ctx)
	if apiEndpoint == "" || secretKey == "" || s.capVerifier == nil {
		return ErrCapNotConfigured
	}
	if strings.TrimSpace(token) == "" {
		return ErrCapVerificationFailed
	}
	result, err := s.capVerifier.VerifyToken(ctx, apiEndpoint, secretKey, token)
	if err != nil {
		return fmt.Errorf("verify Cap token: %w", err)
	}
	if result == nil || !result.Success {
		return ErrCapVerificationFailed
	}
	return nil
}

// IsEnabled 检查 Turnstile 是否启用
func (s *TurnstileService) IsEnabled(ctx context.Context) bool {
	if s == nil || s.settingService == nil {
		return false
	}
	provider := s.settingService.GetHumanVerificationProvider(ctx)
	return provider != HumanVerificationProviderNone && provider != HumanVerificationProviderInvalid
}

// ValidateRecaptchaSecretKey 检查 Google 是否接受该 Secret Key。
func (s *TurnstileService) ValidateRecaptchaSecretKey(ctx context.Context, secretKey string) error {
	if s == nil || s.recaptchaVerifier == nil {
		return ErrRecaptchaNotConfigured
	}
	result, err := s.recaptchaVerifier.VerifyToken(ctx, secretKey, "test-validation", "")
	if err != nil {
		return fmt.Errorf("validate reCAPTCHA secret key: %w", err)
	}
	if result == nil {
		return ErrRecaptchaInvalidSecretKey
	}
	for _, code := range result.ErrorCodes {
		if code == "invalid-input-secret" {
			return ErrRecaptchaInvalidSecretKey
		}
	}
	return nil
}

// ValidateCapAPIEndpoint 规范并校验 Cap widget 使用的站点 endpoint。
func ValidateCapAPIEndpoint(raw string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("cap API Endpoint must be a valid HTTP(S) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("cap API Endpoint must not contain credentials, query parameters, or fragments")
	}
	if strings.Trim(strings.TrimSuffix(parsed.Path, "/"), "/") == "" {
		return fmt.Errorf("cap API Endpoint must include the site key path")
	}
	return nil
}

// ValidateCapConfiguration 验证 Cap endpoint 可达且 site secret 匹配。
func (s *TurnstileService) ValidateCapConfiguration(ctx context.Context, apiEndpoint, secretKey string) error {
	if err := ValidateCapAPIEndpoint(apiEndpoint); err != nil {
		return err
	}
	if s == nil || s.capVerifier == nil {
		return ErrCapNotConfigured
	}
	parsed, _ := url.Parse(apiEndpoint)
	testToken := path.Base(strings.TrimRight(parsed.Path, "/")) + ":test-validation:token"
	result, err := s.capVerifier.VerifyToken(ctx, apiEndpoint, secretKey, testToken)
	if err != nil {
		return fmt.Errorf("validate Cap configuration: %w", err)
	}
	if result == nil {
		return ErrCapInvalidSecretKey
	}
	if strings.Contains(strings.ToLower(result.Error), "site key or secret") {
		return ErrCapInvalidSecretKey
	}
	return nil
}

// ValidateSecretKey 验证 Turnstile Secret Key 是否有效
func (s *TurnstileService) ValidateSecretKey(ctx context.Context, secretKey string) error {
	if s == nil || s.verifier == nil {
		return ErrTurnstileNotConfigured
	}
	// 发送一个测试token的验证请求来检查secret_key是否有效
	result, err := s.verifier.VerifyToken(ctx, secretKey, "test-validation", "")
	if err != nil {
		return fmt.Errorf("validate secret key: %w", err)
	}

	if result == nil {
		return ErrTurnstileInvalidSecretKey
	}
	// 检查是否有 invalid-input-secret 错误
	for _, code := range result.ErrorCodes {
		if code == "invalid-input-secret" {
			return ErrTurnstileInvalidSecretKey
		}
	}

	// 其他错误（如 invalid-input-response）说明 secret key 是有效的
	return nil
}
