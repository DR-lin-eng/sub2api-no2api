//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestPaymentDeepFollowupPostgres(t *testing.T) {
	f := newPaymentRecoveryFixture(t)
	ctx := t.Context()
	t.Run("negative code preserves existing debt", func(t *testing.T) {
		order := f.order(t, service.OrderStatusPending, payment.OrderTypeBalance, time.Now().Add(time.Hour))
		_, err := f.client.User.UpdateOneID(order.UserID).SetBalance(-100).Save(ctx)
		require.NoError(t, err)
		repo := NewUserRepository(f.client, integrationDB)
		adjuster, ok := repo.(service.RedeemUserAdjustmentRepository)
		require.True(t, ok)
		require.NoError(t, adjuster.ApplyRedeemBalanceAdjustment(ctx, order.UserID, -1))
		user, err := repo.GetByID(ctx, order.UserID)
		require.NoError(t, err)
		require.Equal(t, -100.0, user.Balance)
	})
	t.Run("gateway amount preserves three decimals", func(t *testing.T) {
		order := f.order(t, service.OrderStatusPending, payment.OrderTypeBalance, time.Now().Add(time.Hour))
		_, err := f.client.PaymentOrder.UpdateOneID(order.ID).SetPayAmount(12.345).Save(ctx)
		require.NoError(t, err)
		stored, err := f.client.PaymentOrder.Get(ctx, order.ID)
		require.NoError(t, err)
		require.Equal(t, 12.345, stored.PayAmount)
	})
	t.Run("dashboard range is bounded", func(t *testing.T) {
		stats, err := f.svc.GetDashboardStats(ctx, 2_000_000)
		require.NoError(t, err)
		require.Len(t, stats.DailySeries, service.MaxPaymentDashboardDays)
	})
	t.Run("pending refund protects original provider", func(t *testing.T) {
		order := f.order(t, service.OrderStatusRefundPending, payment.OrderTypeBalance, time.Now().Add(time.Hour))
		cfg := service.NewPaymentConfigService(f.client, NewSettingRepository(f.client), make([]byte, payment.AES256KeySize))
		require.Error(t, cfg.DeleteProviderInstance(ctx, f.instanceID))
		_, err := f.client.PaymentOrder.UpdateOneID(order.ID).SetStatus(service.OrderStatusRefunded).Save(ctx)
		require.NoError(t, err)
	})
	t.Run("concurrent subscription deductions serialize", func(t *testing.T) {
		order := f.order(t, service.OrderStatusPending, payment.OrderTypeSubscription, time.Now().Add(time.Hour))
		expires := time.Now().AddDate(0, 0, 90).Truncate(time.Second)
		sub, err := f.client.UserSubscription.Create().SetUserID(order.UserID).SetGroupID(*order.SubscriptionGroupID).
			SetStartsAt(time.Now()).SetExpiresAt(expires).SetStatus(service.SubscriptionStatusActive).Save(ctx)
		require.NoError(t, err)
		repo := NewUserSubscriptionRepository(f.client)
		svc := service.NewSubscriptionService(NewGroupRepository(f.client, integrationDB), repo, nil, f.client, nil)
		results := make(chan error, 2)
		var workers sync.WaitGroup
		for range 2 {
			workers.Go(func() { _, err := svc.ExtendSubscription(ctx, sub.ID, -30); results <- err })
		}
		workers.Wait()
		for range 2 {
			require.NoError(t, <-results)
		}
		stored, err := repo.GetByID(ctx, sub.ID)
		require.NoError(t, err)
		require.Equal(t, expires.AddDate(0, 0, -60), stored.ExpiresAt)
	})
	t.Run("revoked admin key cannot be restored by usage metadata", func(t *testing.T) {
		repo := NewSettingRepository(f.client)
		svc := service.NewSettingService(repo, nil)
		key, secret, err := svc.CreateAdminAPIKey(ctx, service.AdminAPIKeyCreateInput{Name: "usage fixture", Scopes: []string{service.AdminAPIKeyScopeRead}}, 0)
		require.NoError(t, err)
		_, err = svc.AuthenticateAdminAPIKey(ctx, secret)
		require.NoError(t, err)
		require.NoError(t, svc.RevokeAdminAPIKey(ctx, key.ID))
		// A stale authentication recorder only has a hash; it cannot replace
		// revoked status or scopes with its earlier JSON snapshot.
		concrete, ok := repo.(*settingRepository)
		require.True(t, ok)
		digest := sha256.Sum256([]byte(secret))
		require.NoError(t, concrete.RecordAdminAPIKeyUsage(ctx, key.ID, hex.EncodeToString(digest[:]), time.Now()))
		_, err = svc.AuthenticateAdminAPIKey(ctx, secret)
		require.Error(t, err)
	})
}

type paymentRefundCacheFailure struct{ service.BillingCache }

func (paymentRefundCacheFailure) InvalidateSubscriptionCache(context.Context, int64, int64) error {
	return errors.New("injected cache outage")
}

func TestPaymentSubscriptionRefundCacheFailureRollsBackPostgres(t *testing.T) {
	f := newPaymentRecoveryFixture(t)
	ctx := t.Context()
	order := f.order(t, service.OrderStatusPending, payment.OrderTypeSubscription, time.Now().Add(time.Hour))
	require.Equal(t, 200, f.notify(order).Code)
	repo := NewUserSubscriptionRepository(f.client)
	sub, err := repo.GetByUserIDAndGroupID(ctx, order.UserID, *order.SubscriptionGroupID)
	require.NoError(t, err)
	_, err = f.client.UserSubscription.UpdateOneID(sub.ID).SetExpiresAt(time.Now().AddDate(0, 0, 5)).Save(ctx)
	require.NoError(t, err)
	cache := service.NewBillingCacheService(paymentRefundCacheFailure{}, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(cache.Stop)
	users := NewUserRepository(f.client, integrationDB)
	subscriptions := service.NewSubscriptionService(NewGroupRepository(f.client, integrationDB), repo, cache, f.client, nil)
	svc := service.NewPaymentService(f.client, payment.NewRegistry(), payment.NewDefaultLoadBalancer(f.client, make([]byte, payment.AES256KeySize)), nil, subscriptions, nil, users, nil, nil)
	plan, early, err := svc.PrepareRefund(ctx, order.ID, 25, "fixture cache failure", false, true)
	require.NoError(t, err)
	require.Nil(t, early)
	_, err = svc.ExecuteRefund(ctx, plan)
	require.Error(t, err)
	_, err = repo.GetByID(ctx, sub.ID)
	require.NoError(t, err, "the durable revoke must roll back when cache reporting fails")
	stored, err := f.client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, service.OrderStatusCompleted, stored.Status)
	require.Zero(t, f.refundCalls.Load())
}

func TestPaymentAffiliateCapConcurrentPostgres(t *testing.T) {
	f := newPaymentRecoveryFixture(t)
	ctx := t.Context()
	settings := NewSettingRepository(f.client)
	require.NoError(t, settings.SetMultiple(ctx, map[string]string{service.SettingKeyAffiliateEnabled: "true", service.SettingKeyAffiliateRebateRate: "10", service.SettingKeyAffiliateRebatePerInviteeCap: "5", service.SettingKeyAffiliateRebateFreezeHours: "0"}))
	affiliates := NewAffiliateRepository(f.client, integrationDB)
	svc := service.NewAffiliateService(affiliates, service.NewSettingService(settings, nil), nil, nil)
	order := f.order(t, service.OrderStatusPending, payment.OrderTypeBalance, time.Now().Add(time.Hour))
	inviter, err := f.client.User.Create().SetEmail(fmt.Sprintf("cap-%d@fixture.invalid", time.Now().UnixNano())).SetPasswordHash("fixture").Save(ctx)
	require.NoError(t, err)
	_, err = affiliates.BindInviter(ctx, order.UserID, inviter.ID)
	require.NoError(t, err)
	results := make(chan error, 8)
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() { _, err := svc.AccrueInviteRebate(ctx, order.UserID, 25); results <- err })
	}
	workers.Wait()
	for range 8 {
		require.NoError(t, <-results)
	}
	summary, err := affiliates.EnsureUserAffiliate(ctx, inviter.ID)
	require.NoError(t, err)
	require.Equal(t, 5.0, summary.AffQuota)
}

func TestPaymentRefundReclaimsActualInflatedCreditPostgres(t *testing.T) {
	f := newPaymentRecoveryFixture(t)
	f.inflate.Store(true)
	ctx := t.Context()
	order := f.order(t, service.OrderStatusPending, payment.OrderTypeBalance, time.Now().Add(time.Hour))
	require.Equal(t, 200, f.notify(order).Code)
	user, err := f.client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Equal(t, 27.5, user.Balance)
	plan, early, err := f.svc.PrepareRefund(ctx, order.ID, 25, "refund actual credit", false, true)
	require.NoError(t, err)
	require.Nil(t, early)
	require.Equal(t, 27.5, plan.BalanceToDeduct)
	require.Equal(t, 25.0, plan.GatewayAmount)
	result, err := f.svc.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.True(t, result.Success)
	user, err = f.client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Zero(t, user.Balance)
}

func TestPaymentRefundLostReplyKeepsHoldPostgres(t *testing.T) {
	f := newPaymentRecoveryFixture(t)
	ctx := t.Context()
	order := f.order(t, service.OrderStatusPending, payment.OrderTypeBalance, time.Now().Add(time.Hour))
	require.Equal(t, 200, f.notify(order).Code)
	f.loseRefundReply.Store(true)
	plan, early, err := f.svc.PrepareRefund(ctx, order.ID, 25, "lost refund reply", false, true)
	require.NoError(t, err)
	require.Nil(t, early)
	result, err := f.svc.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.False(t, result.Success)
	stored, err := f.client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, service.OrderStatusRefundPending, stored.Status)
	user, err := f.client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Zero(t, user.Balance, "uncertain external refund cannot restore spendable internal credit")
	require.EqualValues(t, 1, f.refundCalls.Load())
}
