package service

import "context"

// validateRegistrationEmailPolicy 校验邮箱能否用于新建账号或绑定为账号邮箱：精确黑名单与域名白名单。
func (s *AuthService) validateRegistrationEmailPolicy(ctx context.Context, email string) error {
	if s.settingService == nil {
		return nil
	}
	if s.settingService.IsRegistrationEmailBlacklisted(ctx, email) {
		return ErrEmailBlocked
	}
	return s.validateRegistrationEmailSuffixPolicy(ctx, email)
}

// validateRegistrationEmailSuffixPolicy 只校验域名白名单。
func (s *AuthService) validateRegistrationEmailSuffixPolicy(ctx context.Context, email string) error {
	if s.settingService == nil {
		return nil
	}
	whitelist := s.settingService.GetRegistrationEmailSuffixWhitelist(ctx)
	if !IsRegistrationEmailSuffixAllowed(email, whitelist) {
		return buildEmailSuffixNotAllowedError(whitelist)
	}
	return nil
}
