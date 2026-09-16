package service

import "github.com/Wei-Shaw/sub2api/internal/platform/config"

func ProvideCRSSyncService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	oauthService *OAuthService,
	openaiOAuthService *OpenAIOAuthService,
	geminiOAuthService *GeminiOAuthService,
	cfg *config.Config,
	settingService *SettingService,
) *CRSSyncService {
	svc := NewCRSSyncService(accountRepo, proxyRepo, oauthService, openaiOAuthService, geminiOAuthService, cfg)
	svc.SetSettingService(settingService)
	return svc
}
