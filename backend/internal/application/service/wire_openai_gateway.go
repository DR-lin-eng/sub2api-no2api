package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
)

func ProvideOpenAIGatewayService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	lockCache LeaderLockCache,
	usageLogRepo UsageLogRepository,
	usageBillingRepo UsageBillingRepository,
	userRepo UserRepository,
	userSubRepo UserSubscriptionRepository,
	userGroupRateRepo UserGroupRateRepository,
	cache GatewayCache,
	rpmCache RPMCache,
	cfg *config.Config,
	schedulerSnapshot *SchedulerSnapshotService,
	concurrencyService *ConcurrencyService,
	billingService *BillingService,
	rateLimitService *RateLimitService,
	billingCacheService *BillingCacheService,
	httpUpstream HTTPUpstream,
	deferredService *DeferredService,
	openAITokenProvider *OpenAITokenProvider,
	grokTokenProvider *GrokTokenProvider,
	resolver *ModelPricingResolver,
	channelService *ChannelService,
	balanceNotifyService *BalanceNotifyService,
	settingService *SettingService,
	userPlatformQuotaRepo UserPlatformQuotaRepository,
	customModelCapabilities CustomModelCapabilityResolver,
	tlsFPProfileService *TLSFingerprintProfileService,
) *OpenAIGatewayService {
	svc := NewOpenAIGatewayService(
		accountRepo,
		usageLogRepo,
		usageBillingRepo,
		userRepo,
		userSubRepo,
		userGroupRateRepo,
		cache,
		cfg,
		schedulerSnapshot,
		concurrencyService,
		billingService,
		rateLimitService,
		billingCacheService,
		httpUpstream,
		deferredService,
		openAITokenProvider,
		grokTokenProvider,
		resolver,
		channelService,
		balanceNotifyService,
		settingService,
		userPlatformQuotaRepo,
	)
	svc.customModelCapabilities = customModelCapabilities
	svc.proxyRepo = proxyRepo
	svc.codexAutoProbeLock = lockCache
	svc.SetTLSFingerprintProfileService(tlsFPProfileService)
	if tlsFPProfileService != nil {
		tlsFPProfileService.SetCodexSimulationSettingService(settingService)
	}
	if counter, ok := rpmCache.(distillationCounter); ok {
		svc.SetDistillationCounter(counter)
	}
	svc.StartCodexTurnStateAutoProbe(context.Background())
	return svc
}
