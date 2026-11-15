package service

import (
	dbent "github.com/DR-lin-eng/sub2api-no2api/ent"
	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
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
