package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/activitycenter"
)

type activityCenterRewardGranter struct {
	userRepo             UserRepository
	subscriptionService  *SubscriptionService
	authCacheInvalidator APIKeyAuthCacheInvalidator
	billingCacheService  *BillingCacheService
}

func ProvideActivityCenterService(
	repo activitycenter.Repository,
	userRepo UserRepository,
	subscriptionService *SubscriptionService,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
	billingCacheService *BillingCacheService,
	redeemService *RedeemService,
	settingService *SettingService,
) *activitycenter.Service {
	svc := activitycenter.NewService(repo)
	svc.SetRewardGranter(&activityCenterRewardGranter{
		userRepo:             userRepo,
		subscriptionService:  subscriptionService,
		authCacheInvalidator: authCacheInvalidator,
		billingCacheService:  billingCacheService,
	})
	if redeemService != nil {
		redeemService.SetInflationResolver(&activityCenterInflationResolver{Service: svc, settings: settingService})
	}
	return svc
}

func (g *activityCenterRewardGranter) Grant(ctx context.Context, userID int64, grant activitycenter.RewardGrant) error {
	var err error
	switch grant.PrizeType {
	case "card":
		if grant.Code == "" {
			err = fmt.Errorf("card prize has no available code")
		}
	case "balance":
		var amount float64
		amount, err = strconv.ParseFloat(grant.ValueAmount, 64)
		if err == nil && !math.IsNaN(amount) && !math.IsInf(amount, 0) && amount > 0 {
			err = g.userRepo.UpdateBalance(ctx, userID, amount)
		} else if err == nil {
			err = fmt.Errorf("balance reward must be positive")
		}
	case "concurrency":
		var amount int
		amount, err = strconv.Atoi(grant.ValueAmount)
		if err == nil && amount > 0 {
			err = g.userRepo.UpdateConcurrency(ctx, userID, amount)
		} else if err == nil {
			err = fmt.Errorf("concurrency reward must be positive")
		}
	case "subscription":
		var days int
		days, err = strconv.Atoi(grant.ValueAmount)
		if err == nil && days > 0 && grant.GroupID > 0 {
			_, _, err = g.subscriptionService.assignOrExtendSubscription(ctx, &AssignSubscriptionInput{
				UserID:       userID,
				GroupID:      grant.GroupID,
				ValidityDays: days,
				AssignedBy:   0,
				Notes:        "活动中心抽奖奖励",
			}, true)
		} else if err == nil {
			err = fmt.Errorf("subscription reward is incomplete")
		}
	case "none":
		return nil
	default:
		err = fmt.Errorf("unsupported activity reward type %q", grant.PrizeType)
	}
	if err != nil {
		return activitycenter.ErrCampaignRewardFailed.WithCause(err)
	}
	return nil
}

func (g *activityCenterRewardGranter) AfterCommit(ctx context.Context, userID int64, grant activitycenter.RewardGrant) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if g.authCacheInvalidator != nil && grant.PrizeType != "card" && grant.PrizeType != "none" {
		g.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if grant.PrizeType == "subscription" && g.subscriptionService != nil {
		g.subscriptionService.InvalidateSubCacheSync(userID, grant.GroupID)
	}
	if g.billingCacheService == nil {
		return
	}
	cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	switch grant.PrizeType {
	case "balance":
		_ = g.billingCacheService.InvalidateUserBalance(cacheCtx, userID)
	case "subscription":
		_ = g.billingCacheService.InvalidateSubscription(cacheCtx, userID, grant.GroupID)
		_ = g.billingCacheService.PublishSubscriptionCacheInvalidation(cacheCtx, subCacheKey(userID, grant.GroupID))
	}
}

// Preserve redemption behavior when activity center is disabled or not configured.
type activityCenterInflationResolver struct {
	*activitycenter.Service
	settings *SettingService
}

func (r *activityCenterInflationResolver) ResolveInflatedBalance(ctx context.Context, userID int64, amount float64) (float64, int64, string, string, float64, error) {
	if r.settings == nil || !r.settings.IsActivityCenterEnabled(ctx) {
		return amount, 0, "", "", 0, nil
	}
	return r.Service.ResolveInflatedBalance(ctx, userID, amount)
}
