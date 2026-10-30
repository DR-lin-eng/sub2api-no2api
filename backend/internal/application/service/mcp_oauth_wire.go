package service

import (
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
)

func ProvideMCPAwareAuthService(
	entClient *dbent.Client,
	userRepo UserRepository,
	redeemRepo RedeemCodeRepository,
	refreshTokenCache RefreshTokenCache,
	cfg *config.Config,
	settingService *SettingService,
	emailService *EmailService,
	turnstileService *TurnstileService,
	emailQueueService *EmailQueueService,
	promoService *PromoService,
	defaultSubAssigner DefaultSubscriptionAssigner,
	affiliateService *AffiliateService,
	userPlatformQuotaRepo UserPlatformQuotaRepository,
	mcp *MCPOAuthService,
) *AuthService {
	svc := NewAuthService(entClient, userRepo, redeemRepo, refreshTokenCache, cfg, settingService, emailService, turnstileService, emailQueueService, promoService, defaultSubAssigner, affiliateService, userPlatformQuotaRepo)
	svc.SetMCPGrantRevoker(mcp)
	return svc
}
